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
	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
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

func TestToolErrorPreviewUsesOutputLanguage(t *testing.T) {
	previous := i18n.OutputLanguage()
	t.Cleanup(func() { i18n.SetOutputLanguage(previous) })
	i18n.SetOutputLanguage(i18n.English)

	var output bytes.Buffer
	call := chat.ToolCall{Function: chat.ToolCallFunction{Name: "web_search"}}
	printToolResultPreview(&output, call, `{"error":"搜索失败","error_code":"search_failed"}`)
	if got, want := strings.TrimSpace(output.String()), "Error: Search failed"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
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

func TestViewFileToolStatusText(t *testing.T) {
	call := chat.ToolCall{Function: chat.ToolCallFunction{
		Name:      "view_file",
		Arguments: `{"path":"/tmp/\u001b[31msecret.txt","offset":5,"length":10}`,
	}}

	got := toolStatusText(call)
	if !strings.Contains(got, "查看文件: /tmp/") || !strings.Contains(got, "secret.txt") {
		t.Fatalf("toolStatusText() = %q", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "offset") {
		t.Fatalf("toolStatusText() leaked controls or range details: %q", got)
	}
}

func TestPrintViewFileResultPreviewOmitsContent(t *testing.T) {
	var output bytes.Buffer
	call := chat.ToolCall{Function: chat.ToolCallFunction{Name: "view_file"}}
	result := `{"path":"notes.txt","resolved_path":"/home/user/notes.txt","size":20,"offset":5,"returned_bytes":5,"truncated":true,"next_offset":10,"binary":false,"content":"secret content","notice":"truncated"}`

	printToolResultPreview(&output, call, result)
	got := output.String()
	for _, want := range []string{"/home/user/notes.txt", "5 / 20", "offset 5", "next offset 10"} {
		if !strings.Contains(got, want) {
			t.Fatalf("preview = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "secret content") {
		t.Fatalf("preview leaked file content: %q", got)
	}
}

func TestPrintViewFileBinaryPreview(t *testing.T) {
	var output bytes.Buffer
	call := chat.ToolCall{Function: chat.ToolCallFunction{Name: "view_file"}}
	result := `{"path":"app.bin","resolved_path":"/home/user/app.bin","size":10,"offset":0,"returned_bytes":10,"truncated":false,"next_offset":10,"binary":true}`

	printToolResultPreview(&output, call, result)
	if !strings.Contains(output.String(), "二进制") {
		t.Fatalf("preview = %q, want binary status", output.String())
	}
}

func TestWebToolDefinitionsUseMetadata(t *testing.T) {
	tools := webToolDefinitions()
	if len(tools) != 2 || tools[0].Function.Name != "web_search" || tools[1].Function.Name != "web_fetch" {
		t.Fatalf("tools = %+v, want metadata-backed web search and fetch", tools)
	}
}

func TestWebToolDefinitionsFollowPromptLanguage(t *testing.T) {
	meta := config.DefaultMeta()
	chineseTools, err := webToolDefinitionsFromMeta(meta, i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("Chinese webToolDefinitionsFromMeta() error = %v", err)
	}
	if got := chineseTools[0].Function.Description; !strings.Contains(got, "搜索公开网络") {
		t.Fatalf("Chinese tool description = %q", got)
	}

	englishTools, err := webToolDefinitionsFromMeta(meta, i18n.English)
	if err != nil {
		t.Fatalf("English webToolDefinitionsFromMeta() error = %v", err)
	}
	if got := englishTools[0].Function.Description; !strings.Contains(got, "Search the public web") {
		t.Fatalf("English tool description = %q", got)
	}
}

func TestWebToolDefinitionsPreserveCustomDescription(t *testing.T) {
	meta := config.DefaultMeta()
	tool := meta.Tools["webSearch"]
	tool.Definitions[0].Description = "custom search description"
	meta.Tools["webSearch"] = tool

	tools, err := webToolDefinitionsFromMeta(meta, i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("webToolDefinitionsFromMeta() error = %v", err)
	}
	if got, want := tools[0].Function.Description, "custom search description"; got != want {
		t.Fatalf("custom description = %q, want %q", got, want)
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
