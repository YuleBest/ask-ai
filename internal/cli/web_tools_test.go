package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/websearch"
)

func TestPrintSearchResultPreview(t *testing.T) {
	var output bytes.Buffer
	call := chat.ToolCall{Function: chat.ToolCallFunction{Name: "web_search"}}
	result := `{"results":[
		{"title":"First","url":"https://one.example","highlights":["first highlight"]},
		{"title":"Second","url":"https://two.example"},
		{"title":"Third","url":"https://three.example"},
		{"title":"Fourth","url":"https://four.example"}
	]}`

	printToolResultPreview(&output, call, result)
	got := output.String()
	for _, want := range []string{"1. First", "https://one.example", "first highlight", "3. Third"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "Fourth") {
		t.Fatalf("output = %q, want only first three results", got)
	}
}

func TestPrintFetchResultPreview(t *testing.T) {
	var output bytes.Buffer
	call := chat.ToolCall{Function: chat.ToolCallFunction{Name: "web_fetch"}}
	result := `{"title":"Page title","url":"https://example.com","text":"first line\n\nsecond line"}`

	printToolResultPreview(&output, call, result)
	got := output.String()
	if !strings.Contains(got, "标题: Page title") || !strings.Contains(got, "正文: first line second line") {
		t.Fatalf("output = %q", got)
	}
}

func TestPrintToolErrorPreview(t *testing.T) {
	var output bytes.Buffer
	call := chat.ToolCall{Function: chat.ToolCallFunction{Name: "web_search"}}

	printToolResultPreview(&output, call, `{"error":"search failed"}`)
	if got := output.String(); !strings.Contains(got, "错误: search failed") {
		t.Fatalf("output = %q", got)
	}
}

func TestPreviewTextRespectsDisplayWidth(t *testing.T) {
	got := previewText("中文内容 and more", 12)
	if displayWidth(got) > 12 || !strings.HasSuffix(got, "...") {
		t.Fatalf("previewText() = %q, width = %d", got, displayWidth(got))
	}
	if got := previewText("safe\x1b[31mred", 80); strings.ContainsRune(got, '\x1b') {
		t.Fatalf("previewText() kept terminal control characters: %q", got)
	}
}

func TestToolStatusTextRemovesTerminalControls(t *testing.T) {
	call := chat.ToolCall{Function: chat.ToolCallFunction{
		Name:      "web_search",
		Arguments: `{"query":"x\u001b[2Jy"}`,
	}}
	got := toolStatusText(call)
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("toolStatusText() kept terminal escape: %q", got)
	}
}

func TestWebToolDefinitionsUseMetadata(t *testing.T) {
	tools := webToolDefinitions()
	if len(tools) != 2 || tools[0].Function.Name != "web_search" || tools[1].Function.Name != "web_fetch" {
		t.Fatalf("tools = %+v, want metadata-backed web search and fetch", tools)
	}
}

func TestWebToolExecutorSearchAndFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			io.WriteString(w, `{"results":[{"title":"Go","url":"https://go.dev"}]}`)
		case "/contents":
			io.WriteString(w, `{"results":[{"title":"Go docs","url":"https://go.dev/doc","text":"docs"}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	executor := webToolExecutor{client: websearch.ExaClient{
		APIKey:     "test",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}}
	searchResult := executor.Execute(context.Background(), chat.ToolCall{
		Function: chat.ToolCallFunction{Name: "web_search", Arguments: `{"query":"Go"}`},
	})
	if !strings.Contains(searchResult, `"https://go.dev"`) {
		t.Fatalf("search result = %q", searchResult)
	}
	fetchResult := executor.Execute(context.Background(), chat.ToolCall{
		Function: chat.ToolCallFunction{Name: "web_fetch", Arguments: `{"url":"https://go.dev/doc"}`},
	})
	if !strings.Contains(fetchResult, `"docs"`) {
		t.Fatalf("fetch result = %q", fetchResult)
	}
}

func TestWebToolExecutorRejectsMalformedArguments(t *testing.T) {
	executor := webToolExecutor{}
	result := executor.Execute(context.Background(), chat.ToolCall{
		Function: chat.ToolCallFunction{Name: "web_search", Arguments: `not-json`},
	})
	if !strings.Contains(result, "解析搜索参数失败") {
		t.Fatalf("result = %q, want malformed argument error", result)
	}
}
