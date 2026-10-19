package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientStream(t *testing.T) {
	var requestBody requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"内部思考\"}}]}\n\n")
		flusher.Flush()
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
		flusher.Flush()
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"，世界\"}}]}\n\n")
		flusher.Flush()
		io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":33,\"completion_tokens\":11,\"total_tokens\":44}}\n\n")
		flusher.Flush()
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := Client{
		APIURL: server.URL + "/v1",
		APIKey: "test-key",
	}
	var content, reasoning strings.Builder
	result, err := client.Stream(context.Background(), Request{
		Model:           "test-model",
		ReasoningEffort: "high",
		Messages:        []Message{{Role: "user", Content: "测试问题"}},
	}, func(delta Delta) error {
		content.WriteString(delta.Content)
		reasoning.WriteString(delta.Reasoning)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if got := content.String(); got != "你好，世界" {
		t.Fatalf("content = %q, want %q", got, "你好，世界")
	}
	if got := reasoning.String(); got != "内部思考" {
		t.Fatalf("reasoning = %q, want %q", got, "内部思考")
	}
	if !result.HasUsage {
		t.Fatal("hasUsage = false, want true")
	}
	if result.Usage.PromptTokens != 33 || result.Usage.CompletionTokens != 11 || result.Usage.TotalTokens != 44 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	if requestBody.Model != "test-model" {
		t.Errorf("model = %q, want test-model", requestBody.Model)
	}
	if requestBody.ReasoningEffort != "high" {
		t.Errorf("reasoning_effort = %q, want high", requestBody.ReasoningEffort)
	}
	if !requestBody.Stream {
		t.Error("stream = false, want true")
	}
	if requestBody.StreamOptions == nil || !requestBody.StreamOptions.IncludeUsage {
		t.Errorf("stream_options = %+v, want include_usage=true", requestBody.StreamOptions)
	}
	if len(requestBody.Messages) != 1 || requestBody.Messages[0].Content != "测试问题" {
		t.Errorf("messages = %+v, want one test question", requestBody.Messages)
	}
}

func TestClientStreamAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad model"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	client := Client{
		APIURL: server.URL,
		APIKey: "test-key",
	}
	_, err := client.Stream(context.Background(), Request{}, func(Delta) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "400 Bad Request") {
		t.Fatalf("Stream() error = %v, want HTTP 400 error", err)
	}
}

func TestClientStreamToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"web_search\",\"arguments\":\"{\\\"query\\\":\"}}]}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"Go release\\\"}\"}}]}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"tool_calls\",\"index\":0}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := Client{APIURL: server.URL, APIKey: "test-key"}
	toolEvent := false
	result, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "搜索 Go 版本"}},
	}, func(delta Delta) error {
		toolEvent = toolEvent || delta.ToolCall
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if !toolEvent {
		t.Fatal("tool call delta was not reported")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
	call := result.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "web_search" ||
		call.Function.Arguments != `{"query":"Go release"}` {
		t.Fatalf("tool call = %+v", call)
	}
}

func TestClientStreamRejectsLengthFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"截断\"},\"finish_reason\":\"length\"}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := Client{APIURL: server.URL, APIKey: "test-key"}
	_, err := client.Stream(context.Background(), Request{}, func(Delta) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "截断") {
		t.Fatalf("Stream() error = %v, want length finish error", err)
	}
}

func TestClientStreamRejectsUnterminatedStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"没有结束\"}}]}\n\n")
	}))
	defer server.Close()

	client := Client{APIURL: server.URL, APIKey: "test-key"}
	_, err := client.Stream(context.Background(), Request{}, func(Delta) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "正常结束") {
		t.Fatalf("Stream() error = %v, want unterminated stream error", err)
	}
}

func TestClientStreamAcceptsFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	client := Client{APIURL: server.URL, APIKey: "test-key"}
	_, err := client.Stream(context.Background(), Request{}, func(Delta) error { return nil })
	if err != nil {
		t.Fatalf("Stream() error = %v, want finish_reason without [DONE] accepted", err)
	}
}

func TestClientStreamHandlesMultilineDataEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"多行\"}}],\n")
		io.WriteString(w, "data: \"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := Client{APIURL: server.URL, APIKey: "test-key"}
	result, err := client.Stream(context.Background(), Request{}, func(Delta) error { return nil })
	if err != nil {
		t.Fatalf("Stream() error = %v, want multiline event accepted", err)
	}
	if !result.HasUsage || result.Usage.TotalTokens != 2 {
		t.Fatalf("usage = %+v, hasUsage = %v", result.Usage, result.HasUsage)
	}
}

func TestClientStreamRejectsMalformedToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{}\"}}]}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := Client{APIURL: server.URL, APIKey: "test-key"}
	_, err := client.Stream(context.Background(), Request{}, func(Delta) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "工具调用") {
		t.Fatalf("Stream() error = %v, want malformed tool call error", err)
	}
}

func TestNewUserMessageWithImages(t *testing.T) {
	message := NewUserMessage("描述图片", []ImageAttachment{
		{MIMEType: "image/png", Data: []byte{1, 2, 3}},
	})
	if message.Role != "user" {
		t.Fatalf("role = %q, want user", message.Role)
	}
	parts, ok := message.Content.([]contentPart)
	if !ok || len(parts) != 2 {
		t.Fatalf("content = %#v, want text and image parts", message.Content)
	}
	if parts[0].Type != "text" || parts[0].Text != "描述图片" {
		t.Fatalf("text part = %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil {
		t.Fatalf("image part = %+v", parts[1])
	}
	if parts[1].ImageURL.URL != "data:image/png;base64,AQID" {
		t.Fatalf("image URL = %q", parts[1].ImageURL.URL)
	}

	textOnly := NewUserMessage("纯文本", nil)
	if got, ok := textOnly.Content.(string); !ok || got != "纯文本" {
		t.Fatalf("text-only content = %#v", textOnly.Content)
	}
}
