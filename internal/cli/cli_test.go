package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/conversation"
)

func TestThinkingElapsedText(t *testing.T) {
	if got := thinkingElapsedText(5); got != "已思考 5 秒..." {
		t.Fatalf("thinkingElapsedText(5) = %q", got)
	}
	if got := thinkingDurationText(2500 * time.Millisecond); got != "已思考 3 秒" {
		t.Fatalf("thinkingDurationText(2.5s) = %q", got)
	}
	if got := thinkingDurationText(0); got != "已思考 1 秒" {
		t.Fatalf("thinkingDurationText(0) = %q", got)
	}
	if got := thinkingDividerText(time.Second); got != "──────── 已思考 1 秒 ────────" {
		t.Fatalf("thinkingDividerText(1s) = %q", got)
	}
	if got := thinkingStatusText(thinkingFrames[0], 5); got != "⠋ 已思考 5 秒..." {
		t.Fatalf("thinking status = %q", got)
	}
}

func TestThinkingIndicatorAddsBlankLineBeforeOutput(t *testing.T) {
	var output bytes.Buffer
	stopped := make(chan struct{})
	close(stopped)
	indicator := &thinkingIndicator{
		writer:  &output,
		visible: true,
		stop:    make(chan struct{}),
		stopped: stopped,
	}

	indicator.clearBeforeOutput()

	if got := output.String(); got != "\r\033[2K\n" {
		t.Fatalf("output = %q, want cleared line and blank line", got)
	}
}

func TestResponseStatsText(t *testing.T) {
	got := responseStatsText("local/model", chat.Usage{
		PromptTokens:     20,
		CompletionTokens: 10,
		TotalTokens:      30,
	}, 2*time.Second)
	want := "local/model | ↑ 20 tok | ↓ 10 tok | 5 tok/s"
	if got != want {
		t.Fatalf("responseStatsText() = %q, want %q", got, want)
	}
}

func TestRun(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var requestMessages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestMessages = payload.Messages

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"内部思考\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "models.json")
	writeTestModelsConfig(t, path, server.URL)
	systemPromptPath := filepath.Join(filepath.Dir(path), "system_prompt.json")
	if err := os.WriteFile(systemPromptPath, []byte(`{"zh-CN":"自定义系统提示词","en":"Custom system prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"测试", "问题"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "你好\n" {
		t.Fatalf("stdout = %q, want %q", got, "你好\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if len(requestMessages) != 2 {
		t.Fatalf("request messages = %+v, want system and user messages", requestMessages)
	}
	if requestMessages[0].Role != "system" || requestMessages[0].Content != "自定义系统提示词" {
		t.Fatalf("system message = %+v", requestMessages[0])
	}
	if requestMessages[1].Role != "user" || requestMessages[1].Content != "测试 问题" {
		t.Fatalf("user message = %+v", requestMessages[1])
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	session, err := store.LoadLast()
	if err != nil {
		t.Fatalf("load saved session: %v", err)
	}
	wantHistory := []conversation.Message{
		{Role: "user", Content: "测试 问题"},
		{Role: "assistant", Content: "你好"},
	}
	if len(session.Messages) != len(wantHistory) {
		t.Fatalf("saved messages = %+v, want %+v", session.Messages, wantHistory)
	}
	for i := range wantHistory {
		if session.Messages[i] != wantHistory[i] {
			t.Fatalf("saved message[%d] = %+v, want %+v", i, session.Messages[i], wantHistory[i])
		}
	}
	rawHistory, err := os.ReadFile(filepath.Join(store.Root, "sessions", session.ID+".json"))
	if err != nil {
		t.Fatalf("read saved history: %v", err)
	}
	if bytes.Contains(rawHistory, []byte("内部思考")) {
		t.Fatalf("saved history contains reasoning: %s", rawHistory)
	}
}

func TestRunPictureAttachment(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var userContent json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		for _, message := range payload.Messages {
			if message.Role == "user" {
				userContent = message.Content
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"这是 GitHub\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, server.URL)
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}
	imagePath := filepath.Join(dir, "github.png")
	imageData := writeTestPNG(t, imagePath)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"这张图是什么", "-p", imagePath}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "这是 GitHub\n" {
		t.Fatalf("stdout = %q", got)
	}

	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(userContent, &parts); err != nil {
		t.Fatalf("decode user content: %v\ncontent=%s", err, userContent)
	}
	if len(parts) != 2 || parts[0].Type != "text" || parts[0].Text != "这张图是什么" ||
		parts[1].Type != "image_url" {
		t.Fatalf("parts = %+v", parts)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(parts[1].ImageURL.URL, prefix) {
		t.Fatalf("image URL = %q", parts[1].ImageURL.URL)
	}
	gotImage, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(parts[1].ImageURL.URL, prefix))
	if err != nil {
		t.Fatalf("decode image data: %v", err)
	}
	if !bytes.Equal(gotImage, imageData) {
		t.Fatalf("uploaded image data = %v, want %v", gotImage, imageData)
	}
}

func TestRunPrintsResponseStats(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"答案\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2,\"total_tokens\":9}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "models.json")
	writeTestModelsConfig(t, path, server.URL)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"测试"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "答案\n" {
		t.Fatalf("stdout = %q, want only answer", got)
	}
	stats := strings.TrimSuffix(strings.TrimPrefix(stderr.String(), "\n"), "\n")
	if !strings.HasPrefix(stats, "local/test-model | ↑ 7 tok | ↓ 2 tok | ") ||
		!strings.HasSuffix(stats, " tok/s") {
		t.Fatalf("stderr = %q, want response statistics", stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "\n") {
		t.Fatalf("stderr = %q, want blank line before response statistics", stderr.String())
	}
}

func TestAskExecutesToolsAndContinues(t *testing.T) {
	var requests []struct {
		Messages []chat.Message `json:"messages"`
		Tools    []chat.Tool    `json:"tools"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []chat.Message `json:"messages"`
			Tools    []chat.Tool    `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests = append(requests, struct {
			Messages []chat.Message `json:"messages"`
			Tools    []chat.Tool    `json:"tools"`
		}{Messages: payload.Messages, Tools: payload.Tools})

		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"web_search\",\"arguments\":\"{\\\"query\\\":\\\"Go latest\\\"}\"}}]}}]}\n\n")
			io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":3,\"total_tokens\":13}}\n\n")
		} else {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"最终答案\"}}]}\n\n")
			io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":5,\"total_tokens\":25}}\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var executed []chat.ToolCall
	var stdout, stderr bytes.Buffer
	answer, err := ask(
		context.Background(),
		chat.Client{APIURL: server.URL, APIKey: "test"},
		chat.Request{Messages: []chat.Message{{Role: "user", Content: "Go 最新版本？"}}},
		&stdout,
		&stderr,
		false,
		"local/test-model",
		webToolDefinitions(),
		func(_ context.Context, call chat.ToolCall) string {
			executed = append(executed, call)
			return `{"results":[{"title":"Go","url":"https://go.dev"}]}`
		},
	)
	if err != nil {
		t.Fatalf("ask() error = %v", err)
	}
	if answer != "最终答案" || stdout.String() != "最终答案\n" {
		t.Fatalf("answer = %q, stdout = %q", answer, stdout.String())
	}
	if len(executed) != 1 || executed[0].Function.Name != "web_search" {
		t.Fatalf("executed tools = %+v", executed)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if len(requests[0].Tools) != 2 {
		t.Fatalf("first request tools = %+v", requests[0].Tools)
	}
	secondMessages := requests[1].Messages
	if len(secondMessages) != 3 ||
		secondMessages[1].Role != "assistant" ||
		len(secondMessages[1].ToolCalls) != 1 ||
		secondMessages[2].Role != "tool" ||
		secondMessages[2].ToolCallID != "call_1" {
		t.Fatalf("second request messages = %+v", secondMessages)
	}
	if !strings.Contains(stderr.String(), "↳ 搜索: Go latest") {
		t.Fatalf("stderr = %q, want tool status", stderr.String())
	}
	if !strings.Contains(stderr.String(), "local/test-model | ↑ 30 tok | ↓ 8 tok |") {
		t.Fatalf("stderr = %q, want aggregated usage", stderr.String())
	}
}

func TestAskSeparatesContentAcrossToolRounds(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/event-stream")
		if requestCount == 1 {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"检查\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"web_search\",\"arguments\":\"{\\\"query\\\":\\\"x\\\"}\"}}]}}]}\n\n")
		} else {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"}}]}\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	answer, err := ask(
		context.Background(),
		chat.Client{APIURL: server.URL, APIKey: "test"},
		chat.Request{Messages: []chat.Message{{Role: "user", Content: "测试"}}},
		&stdout,
		&stderr,
		false,
		"local/test-model",
		webToolDefinitions(),
		func(context.Context, chat.ToolCall) string { return "{}" },
	)
	if err != nil {
		t.Fatalf("ask() error = %v", err)
	}
	if answer != "检查\n完成" || stdout.String() != "检查\n完成\n" {
		t.Fatalf("answer = %q, stdout = %q", answer, stdout.String())
	}
}

func TestAskSuppressesPartialUsage(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/event-stream")
		if requestCount == 1 {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"web_search\",\"arguments\":\"{\\\"query\\\":\\\"x\\\"}\"}}]}}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":3,\"total_tokens\":13}}\n\n")
		} else {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"}}]}\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	_, err := ask(
		context.Background(),
		chat.Client{APIURL: server.URL, APIKey: "test"},
		chat.Request{Messages: []chat.Message{{Role: "user", Content: "测试"}}},
		&stdout,
		&stderr,
		false,
		"local/test-model",
		webToolDefinitions(),
		func(context.Context, chat.ToolCall) string { return "{}" },
	)
	if err != nil {
		t.Fatalf("ask() error = %v", err)
	}
	if strings.Contains(stderr.String(), "tok/s") {
		t.Fatalf("stderr contains incomplete usage stats: %q", stderr.String())
	}
}

func TestAskStopsBeforeExecutingToolOnLastRound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"web_search\",\"arguments\":\"{\\\"query\\\":\\\"x\\\"}\"}}]}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	executions := 0
	var stdout, stderr bytes.Buffer
	_, err := ask(
		context.Background(),
		chat.Client{APIURL: server.URL, APIKey: "test"},
		chat.Request{Messages: []chat.Message{{Role: "user", Content: "测试"}}},
		&stdout,
		&stderr,
		false,
		"local/test-model",
		webToolDefinitions(),
		func(context.Context, chat.ToolCall) string {
			executions++
			return "{}"
		},
	)
	if err == nil {
		t.Fatal("ask() error = nil, want tool round limit")
	}
	if executions != 7 {
		t.Fatalf("tool executions = %d, want 7 before rejecting the eighth round", executions)
	}
}

func TestRunWebSearchToolToggle(t *testing.T) {
	tests := []struct {
		name      string
		use       string
		wantTools int
	}{
		{name: "disabled", use: "false", wantTools: 0},
		{name: "enabled", use: `"exa"`, wantTools: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())

			var requestTools []chat.Tool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					Tools []chat.Tool `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				requestTools = payload.Tools
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"答案\"}}]}\n\n")
				io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer server.Close()

			dir := t.TempDir()
			modelPath := filepath.Join(dir, "models.json")
			writeTestModelsConfig(t, modelPath, server.URL)
			if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
				t.Fatalf("write system prompt: %v", err)
			}
			meta, _, err := config.LoadOrCreateMeta(config.MetaPath(modelPath))
			if err != nil {
				t.Fatalf("LoadOrCreateMeta() error = %v", err)
			}
			webPath, err := meta.PathFor("webSearch", dir)
			if err != nil {
				t.Fatalf("PathFor(webSearch) error = %v", err)
			}
			webConfig := `{"use":` + tt.use + `,"provider":[{"id":"exa","key":"test-key"}]}`
			if err := os.WriteFile(webPath, []byte(webConfig), 0o600); err != nil {
				t.Fatalf("write web search config: %v", err)
			}

			var stdout, stderr bytes.Buffer
			code := run(context.Background(), []string{"测试"}, &stdout, &stderr, func() (string, error) {
				return modelPath, nil
			})
			if code != 0 {
				t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
			}
			if len(requestTools) != tt.wantTools {
				t.Fatalf("tools = %+v, want %d", requestTools, tt.wantTools)
			}
		})
	}
}

func TestRunShowThink(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"先\\u001b[2J分析\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"问题\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"答案\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "models.json")
	writeTestModelsConfig(t, path, server.URL)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"测试", "--show-think"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "答案\n" {
		t.Fatalf("stdout = %q, want only answer", got)
	}
	gotStderr := stderr.String()
	if !strings.HasPrefix(gotStderr, "先分析问题\n──────── 已思考 ") ||
		!strings.HasSuffix(gotStderr, " 秒 ────────\n\n") {
		t.Fatalf("stderr = %q, want reasoning and duration divider", gotStderr)
	}
	if strings.ContainsRune(gotStderr, '\x1b') {
		t.Fatalf("stderr contains terminal escape: %q", gotStderr)
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	session, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	rawHistory, err := os.ReadFile(filepath.Join(store.Root, "sessions", session.ID+".json"))
	if err != nil {
		t.Fatalf("read saved history: %v", err)
	}
	if bytes.Contains(rawHistory, []byte("先分析")) {
		t.Fatalf("saved history contains reasoning: %s", rawHistory)
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--help"}, &stdout, &stderr, func() (string, error) {
		return filepath.Join(t.TempDir(), "models.json"), nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "命令:") || !strings.Contains(got, "选项:") || !strings.Contains(got, "示例:") {
		t.Fatalf("stdout = %q, want standard help sections", got)
	}
}

func TestRunCreatesConfigAndSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ask-ai", "models.json")
	var stdout, stderr bytes.Buffer

	code := run(context.Background(), []string{"测试"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	for _, createdPath := range []string{
		path,
		filepath.Join(dir, "ask-ai", "language.json"),
		filepath.Join(dir, "ask-ai", "system_prompt.json"),
		filepath.Join(dir, "ask-ai", "_meta.json"),
	} {
		if _, err := os.Stat(createdPath); err != nil {
			t.Fatalf("expected generated file %s: %v", createdPath, err)
		}
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		follow     bool
		followHash string
		bind       bool
		bindHash   string
		showThink  bool
		pictures   []string
		question   string
		wantErr    bool
	}{
		{name: "new conversation", args: []string{"普通", "问题"}, question: "普通 问题"},
		{name: "show think", args: []string{"--show-think", "普通", "问题"}, showThink: true, question: "普通 问题"},
		{name: "show think after question", args: []string{"普通", "问题", "--show-think"}, showThink: true, question: "普通 问题"},
		{name: "picture before question", args: []string{"-p", "shot.png", "普通", "问题"}, pictures: []string{"shot.png"}, question: "普通 问题"},
		{name: "picture after question", args: []string{"普通", "问题", "--picture", "shot.png"}, pictures: []string{"shot.png"}, question: "普通 问题"},
		{name: "multiple pictures", args: []string{"-p", "a.png", "--picture", "b.jpg", "问题"}, pictures: []string{"a.png", "b.jpg"}, question: "问题"},
		{name: "follow after question", args: []string{"继续", "问", "-f"}, follow: true, question: "继续 问"},
		{name: "bind hash after question", args: []string{"继续", "-b", "abcdef"}, bind: true, bindHash: "abcdef", question: "继续"},
		{name: "short follow", args: []string{"-f", "继续"}, follow: true, question: "继续"},
		{name: "follow hash", args: []string{"-f", "abcdef", "继续"}, follow: true, followHash: "abcdef", question: "继续"},
		{name: "long follow", args: []string{"--follow", "继续", "问"}, follow: true, question: "继续 问"},
		{name: "long follow hash", args: []string{"--follow", "123456", "继续"}, follow: true, followHash: "123456", question: "继续"},
		{name: "bind hash", args: []string{"-b", "abcdef", "继续"}, bind: true, bindHash: "abcdef", question: "继续"},
		{name: "bind current", args: []string{"--bind"}, bind: true},
		{name: "bind question escaped", args: []string{"-b", "--", "abcdef"}, bind: true, question: "abcdef"},
		{name: "option terminator", args: []string{"--", "-1", "是什么"}, question: "-1 是什么"},
		{name: "option terminator after question", args: []string{"问题", "--", "--show-think"}, question: "问题 --show-think"},
		{name: "follow question escaped", args: []string{"-f", "--", "abcdef"}, follow: true, question: "abcdef"},
		{name: "unknown option", args: []string{"-x", "问题"}, wantErr: true},
		{name: "picture without path", args: []string{"问题", "-p"}, wantErr: true},
		{name: "missing question", args: []string{"-f"}, follow: true, wantErr: true},
		{name: "missing question after hash", args: []string{"-f", "abcdef"}, follow: true, followHash: "abcdef", wantErr: true},
		{name: "follow bind conflict", args: []string{"-f", "-b", "问题"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseArgs() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs() error = %v", err)
			}
			if opts.follow != tt.follow || opts.followHash != tt.followHash ||
				opts.bind != tt.bind || opts.bindHash != tt.bindHash ||
				opts.showThink != tt.showThink || !slices.Equal(opts.pictures, tt.pictures) ||
				opts.question != tt.question {
				t.Fatalf(
					"parseArgs() = (%v, %q, %v, %q, %v, %v, %q), want (%v, %q, %v, %q, %v, %v, %q)",
					opts.follow, opts.followHash, opts.bind, opts.bindHash, opts.showThink, opts.pictures, opts.question,
					tt.follow, tt.followHash, tt.bind, tt.bindHash, tt.showThink, tt.pictures, tt.question,
				)
			}
		})
	}
}

func TestRunFollow(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var requestMessages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestMessages = payload.Messages

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"当前思考\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"当前回答\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, server.URL)
	systemPromptPath := filepath.Join(dir, "system_prompt.json")
	if err := os.WriteFile(systemPromptPath, []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	previous, err := conversation.NewSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	previous.Messages = []conversation.Message{
		{Role: "user", Content: "上一问"},
		{Role: "assistant", Content: "上一答"},
	}
	if err := store.Save(previous); err != nil {
		t.Fatalf("save previous history: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--follow", "当前问题"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "当前回答\n" {
		t.Fatalf("stdout = %q, want %q", got, "当前回答\n")
	}
	if len(requestMessages) != 4 {
		t.Fatalf("request messages = %+v, want system plus three conversation messages", requestMessages)
	}
	wantMessages := []struct {
		Role    string
		Content string
	}{
		{Role: "system", Content: "系统提示"},
		{Role: "user", Content: "上一问"},
		{Role: "assistant", Content: "上一答"},
		{Role: "user", Content: "当前问题"},
	}
	for i := range wantMessages {
		if requestMessages[i].Role != wantMessages[i].Role || requestMessages[i].Content != wantMessages[i].Content {
			t.Fatalf("request message[%d] = %+v, want %+v", i, requestMessages[i], wantMessages[i])
		}
	}

	saved, err := store.LoadLast()
	if err != nil {
		t.Fatalf("load saved session: %v", err)
	}
	if saved.ID != previous.ID {
		t.Fatalf("saved session ID = %q, want %q", saved.ID, previous.ID)
	}
	if len(saved.Messages) != 4 || saved.Messages[3].Role != "assistant" || saved.Messages[3].Content != "当前回答" {
		t.Fatalf("saved messages = %+v", saved.Messages)
	}
	rawHistory, err := os.ReadFile(filepath.Join(store.Root, "sessions", saved.ID+".json"))
	if err != nil {
		t.Fatalf("read saved history: %v", err)
	}
	if bytes.Contains(rawHistory, []byte("当前思考")) {
		t.Fatalf("saved history contains reasoning: %s", rawHistory)
	}
}

func TestRunFollowSpecifiedSession(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var requestMessages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestMessages = payload.Messages

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"指定思考\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"指定回答\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, server.URL)
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("7", 26)
	idB := "abcdef" + strings.Repeat("8", 26)
	sessionA := taskTestSession(idA, "任务 A 问题")
	sessionA.Messages[1].Content = "任务 A 回答"
	sessionB := taskTestSession(idB, "任务 B 问题")
	sessionB.Messages[1].Content = "任务 B 回答"
	if err := store.Save(sessionA); err != nil {
		t.Fatalf("save session A: %v", err)
	}
	if err := store.Save(sessionB); err != nil {
		t.Fatalf("save session B: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-f", idA[:7], "指定追问"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "指定回答\n" {
		t.Fatalf("stdout = %q, want %q", got, "指定回答\n")
	}

	wantMessages := []struct {
		Role    string
		Content string
	}{
		{Role: "system", Content: "系统提示"},
		{Role: "user", Content: "任务 A 问题"},
		{Role: "assistant", Content: "任务 A 回答"},
		{Role: "user", Content: "指定追问"},
	}
	if len(requestMessages) != len(wantMessages) {
		t.Fatalf("request messages = %+v", requestMessages)
	}
	for i := range wantMessages {
		if requestMessages[i].Role != wantMessages[i].Role || requestMessages[i].Content != wantMessages[i].Content {
			t.Fatalf("request message[%d] = %+v, want %+v", i, requestMessages[i], wantMessages[i])
		}
	}

	current, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if current.ID != idB {
		t.Fatalf("current task = %q, want unchanged %q", current.ID, idB)
	}
	updated, err := store.Load(idA[:7])
	if err != nil {
		t.Fatalf("load updated session A: %v", err)
	}
	if len(updated.Messages) != 4 || updated.Messages[3].Content != "指定回答" {
		t.Fatalf("updated session A messages = %+v", updated.Messages)
	}
}

func TestRunFollowWithoutHistory(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, "http://127.0.0.1:1")
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-f", "继续"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("没有可追问的上一次对话")) {
		t.Fatalf("stderr = %q, want missing history error", stderr.String())
	}
}

func TestRunUsesMetaConfigPaths(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var systemMessage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload.Messages) > 0 {
			systemMessage = payload.Messages[0].Content
		}

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	defaultModelsPath := filepath.Join(dir, "models.json")
	customDir := filepath.Join(dir, "custom")
	if err := os.MkdirAll(customDir, 0o700); err != nil {
		t.Fatalf("mkdir custom config: %v", err)
	}
	meta := map[string]any{
		"version": 1,
		"configs": map[string]any{
			"meta": map[string]string{
				"file":        "_meta.json",
				"description": "meta",
			},
			"models": map[string]string{
				"file":        "custom/models.json",
				"description": "models",
			},
			"systemPrompt": map[string]string{
				"file":        "custom/prompt.txt",
				"description": "prompt",
			},
		},
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_meta.json"), metaData, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	writeTestModelsConfig(t, filepath.Join(customDir, "models.json"), server.URL)
	if err := os.WriteFile(filepath.Join(customDir, "prompt.txt"), []byte("元数据 {{os}}/{{arch}}\n"), 0o600); err != nil {
		t.Fatalf("write custom prompt: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"测试"}, &stdout, &stderr, func() (string, error) {
		return defaultModelsPath, nil
	})
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if want := "元数据 " + runtime.GOOS + "/" + runtime.GOARCH; systemMessage != want {
		t.Fatalf("system message = %q, want custom prompt", systemMessage)
	}
}

func TestRunBindExitUnbinds(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("a", 26)
	idB := "abcdef" + strings.Repeat("b", 26)
	if err := store.Save(taskTestSession(idA, "任务 A")); err != nil {
		t.Fatalf("save session A: %v", err)
	}
	if err := store.Save(taskTestSession(idB, "任务 B")); err != nil {
		t.Fatalf("save session B: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, "http://127.0.0.1:1")
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	terminalID := strings.Repeat("1", 32)
	provider := testTerminalProvider(terminalID)
	var stdout, stderr bytes.Buffer
	code := runWithTerminalInput(context.Background(), []string{"--bind"}, strings.NewReader("/exit\n"), &stdout, &stderr, func() (string, error) {
		return path, nil
	}, provider)
	if code != 0 {
		t.Fatalf("bind current code = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "已绑定任务:") {
		t.Fatalf("stdout contains binding status: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "已绑定任务:") || !strings.Contains(stderr.String(), idB) {
		t.Fatalf("stderr = %q, want bound current task", stderr.String())
	}
	if !strings.Contains(stderr.String(), "/exit") {
		t.Fatalf("stderr = %q, want interactive hint", stderr.String())
	}

	bindingStore := conversation.NewBindingStore(store.Root, terminalID)
	if _, err := bindingStore.Load(); err == nil {
		t.Fatal("binding still exists after /exit")
	}

	stdout.Reset()
	stderr.Reset()
	code = runWithTerminalInput(context.Background(), []string{"-b", idA[:7]}, strings.NewReader("/exit\n"), &stdout, &stderr, func() (string, error) {
		return path, nil
	}, provider)
	if code != 0 {
		t.Fatalf("bind hash code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no binding status", stdout.String())
	}
	if !strings.Contains(stderr.String(), "已绑定任务:") || !strings.Contains(stderr.String(), idA) {
		t.Fatalf("stderr = %q, want bound task A", stderr.String())
	}
	if _, err := bindingStore.Load(); err == nil {
		t.Fatal("binding still exists after /exit")
	}
}

func TestRunBindStartsConversationLoop(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var requests [][]chat.Message
	answerIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []chat.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests = append(requests, payload.Messages)
		answerIndex++
		answer := "回答一"
		if answerIndex == 2 {
			answer = "回答二"
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", answer)
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, server.URL)
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	sessionID := "abcdef" + strings.Repeat("e", 26)
	session := taskTestSession(sessionID, "初始问题")
	session.Messages[1].Content = "初始回答"
	if err := store.Save(session); err != nil {
		t.Fatalf("save session: %v", err)
	}

	terminalID := strings.Repeat("3", 32)
	var stdout, stderr bytes.Buffer
	code := runWithTerminalInput(
		context.Background(),
		[]string{"--bind", sessionID[:7], "起始追问"},
		strings.NewReader("后续问题\n/exit\n"),
		&stdout,
		&stderr,
		func() (string, error) { return path, nil },
		testTerminalProvider(terminalID),
	)
	if code != 0 {
		t.Fatalf("bind loop code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "回答一") || !strings.Contains(stdout.String(), "回答二") {
		t.Fatalf("stdout = %q, want both answers", stdout.String())
	}
	if got := strings.Count(stderr.String(), strings.Repeat("─", 48)); got != 2 {
		t.Fatalf("divider count = %d, want 2: %q", got, stderr.String())
	}
	if got := strings.Count(stderr.String(), "问题> "); got != 2 {
		t.Fatalf("prompt count = %d, want 2: %q", got, stderr.String())
	}

	bindingStore := conversation.NewBindingStore(store.Root, terminalID)
	if _, err := bindingStore.Load(); err == nil {
		t.Fatal("binding still exists after /exit")
	}

	updated, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("load updated session: %v", err)
	}
	if len(updated.Messages) != 6 ||
		updated.Messages[2].Content != "起始追问" ||
		updated.Messages[3].Content != "回答一" ||
		updated.Messages[4].Content != "后续问题" ||
		updated.Messages[5].Content != "回答二" {
		t.Fatalf("saved messages = %+v", updated.Messages)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if len(requests[1]) != 6 ||
		requests[1][3].Content != "起始追问" ||
		requests[1][4].Content != "回答一" ||
		requests[1][5].Content != "后续问题" {
		t.Fatalf("second request messages = %+v", requests[1])
	}
}

func TestRunBindUsesPictureOnlyForFirstQuestion(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var lastUserContents []json.RawMessage
	answerIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		var lastUserContent json.RawMessage
		for _, message := range payload.Messages {
			if message.Role == "user" {
				lastUserContent = message.Content
			}
		}
		lastUserContents = append(lastUserContents, lastUserContent)

		answerIndex++
		answer := "回答一"
		if answerIndex == 2 {
			answer = "回答二"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", answer)
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, server.URL)
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}
	imagePath := filepath.Join(dir, "logo.png")
	writeTestPNG(t, imagePath)

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	sessionID := "abcdef" + strings.Repeat("9", 26)
	if err := store.Save(taskTestSession(sessionID, "初始问题")); err != nil {
		t.Fatalf("save session: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runWithTerminalInput(
		context.Background(),
		[]string{"--bind", sessionID[:7], "-p", imagePath},
		strings.NewReader("第一问\n第二问\n/exit\n"),
		&stdout,
		&stderr,
		func() (string, error) { return path, nil },
		testTerminalProvider(strings.Repeat("5", 32)),
	)
	if code != 0 {
		t.Fatalf("bind picture code = %d, stderr = %q", code, stderr.String())
	}
	if len(lastUserContents) != 2 {
		t.Fatalf("request count = %d, want 2", len(lastUserContents))
	}
	if len(lastUserContents[0]) == 0 || lastUserContents[0][0] != '[' {
		t.Fatalf("first question content = %s, want multimodal array", lastUserContents[0])
	}
	var secondQuestion string
	if err := json.Unmarshal(lastUserContents[1], &secondQuestion); err != nil {
		t.Fatalf("second question content = %s: %v", lastUserContents[1], err)
	}
	if secondQuestion != "第二问" {
		t.Fatalf("second question = %q, want text only", secondQuestion)
	}
}

func TestRunBindUnbindsOnContextCancel(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, "http://127.0.0.1:1")
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	sessionID := "abcdef" + strings.Repeat("f", 26)
	if err := store.Save(taskTestSession(sessionID, "任务")); err != nil {
		t.Fatalf("save session: %v", err)
	}

	input := &signaledReader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	terminalID := strings.Repeat("4", 32)
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runWithTerminalInput(
			ctx,
			[]string{"--bind", sessionID[:7]},
			input,
			&stdout,
			&stderr,
			func() (string, error) { return path, nil },
			testTerminalProvider(terminalID),
		)
	}()

	select {
	case <-input.started:
	case <-time.After(2 * time.Second):
		t.Fatal("bind loop did not start reading input")
	}
	cancel()

	var code int
	select {
	case code = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bind loop did not stop after context cancellation")
	}
	close(input.release)
	if code != 130 {
		t.Fatalf("bind loop code = %d, want 130", code)
	}

	bindingStore := conversation.NewBindingStore(store.Root, terminalID)
	if _, err := bindingStore.Load(); err == nil {
		t.Fatal("binding still exists after context cancellation")
	}
}

func TestBoundTerminalFollowsWithoutFlag(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var requestMessages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestMessages = payload.Messages

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"终端回答\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, server.URL)
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.json"), []byte(`{"zh-CN":"系统提示","en":"System prompt"}`), 0o600); err != nil {
		t.Fatalf("write system prompt: %v", err)
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("c", 26)
	idB := "abcdef" + strings.Repeat("d", 26)
	sessionA := taskTestSession(idA, "绑定任务问题")
	sessionA.Messages[1].Content = "绑定任务回答"
	if err := store.Save(sessionA); err != nil {
		t.Fatalf("save session A: %v", err)
	}
	if err := store.Save(taskTestSession(idB, "当前任务问题")); err != nil {
		t.Fatalf("save session B: %v", err)
	}

	terminalID := strings.Repeat("2", 32)
	bindingStore := conversation.NewBindingStore(store.Root, terminalID)
	if err := bindingStore.Save(idA); err != nil {
		t.Fatalf("save binding: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runWithTerminal(context.Background(), []string{"终端追问"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	}, testTerminalProvider(terminalID))
	if code != 0 {
		t.Fatalf("bound run code = %d, stderr = %q", code, stderr.String())
	}
	wantMessages := []struct {
		Role    string
		Content string
	}{
		{Role: "system", Content: "系统提示"},
		{Role: "user", Content: "绑定任务问题"},
		{Role: "assistant", Content: "绑定任务回答"},
		{Role: "user", Content: "终端追问"},
	}
	if len(requestMessages) != len(wantMessages) {
		t.Fatalf("request messages = %+v", requestMessages)
	}
	for i := range wantMessages {
		if requestMessages[i].Role != wantMessages[i].Role || requestMessages[i].Content != wantMessages[i].Content {
			t.Fatalf("request message[%d] = %+v, want %+v", i, requestMessages[i], wantMessages[i])
		}
	}
	current, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if current.ID != idB {
		t.Fatalf("current task = %q, want unchanged %q", current.ID, idB)
	}

	requestMessages = nil
	stdout.Reset()
	stderr.Reset()
	otherTerminal := strings.Repeat("3", 32)
	code = runWithTerminal(context.Background(), []string{"另一个终端"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	}, testTerminalProvider(otherTerminal))
	if code != 0 {
		t.Fatalf("other terminal run code = %d, stderr = %q", code, stderr.String())
	}
	if len(requestMessages) != 2 || requestMessages[1].Content != "另一个终端" {
		t.Fatalf("other terminal request messages = %+v, want new conversation", requestMessages)
	}
}

func TestBindRequiresInteractiveTerminal(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := runWithTerminal(context.Background(), []string{"-b"}, &stdout, &stderr, func() (string, error) {
		return filepath.Join(t.TempDir(), "models.json"), nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 1 {
		t.Fatalf("runWithTerminal() code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "绑定仅在交互终端中生效") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func testTerminalProvider(terminalID string) terminalIDProvider {
	return func(io.Writer, io.Writer) (string, error) {
		return terminalID, nil
	}
}

type signaledReader struct {
	started chan struct{}
	release chan struct{}
}

func (r *signaledReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.release
	return 0, io.EOF
}

func writeTestModelsConfig(t *testing.T, path, apiURL string) {
	t.Helper()
	models := map[string]any{
		"provider": []any{
			map[string]any{
				"id":   "local",
				"name": "Local",
				"url":  apiURL,
				"key":  "test-key",
				"models": []any{
					map[string]any{
						"id":               "test-model",
						"reasoning_effort": "high",
						"default":          true,
					},
				},
			},
		},
	}
	data, err := json.Marshal(models)
	if err != nil {
		t.Fatalf("marshal models config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write models config: %v", err)
	}
}

func writeTestPNG(t *testing.T, path string) []byte {
	t.Helper()
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if err := png.Encode(&data, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	return data.Bytes()
}
