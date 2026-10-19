package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarkdownRendererInlineStylesAcrossChunks(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, true)

	for _, chunk := range []string{"**粗", "体** 和 *斜", "体*，仍显示 `a*b`"} {
		if _, err := renderer.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	want := ansiReset + "\x1b[1m粗体" + ansiReset +
		" 和 " + ansiReset + "\x1b[3m斜体" + ansiReset +
		"，仍显示 " + ansiReset + "\x1b[36ma*b" + ansiReset
	if got := output.String(); got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestMarkdownRendererBlocks(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, true)
	input := "# 标题\n- **重点**\n> 引用\n```go\nfmt.Println(\"*原样*\")\n```\n结尾\n"

	if _, err := renderer.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	got := output.String()
	for _, hidden := range []string{"- **", "> 引用"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("rendered output still contains %q: %q", hidden, got)
		}
	}
	for _, visible := range []string{"# 标题", "• ", "重点", "│ 引用", "```go", `fmt.Println("*原样*")`, "结尾"} {
		if !strings.Contains(got, visible) {
			t.Fatalf("rendered output does not contain %q: %q", visible, got)
		}
	}
	if count := strings.Count(got, "```"); count != 2 {
		t.Fatalf("rendered output contains %d fence markers, want 2: %q", count, got)
	}
}

func TestMarkdownRendererDisabledPreservesSource(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, false)
	input := "**粗体**\n```\ncode\n```\n"

	if _, err := renderer.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := output.String(); got != input {
		t.Fatalf("output = %q, want unchanged source %q", got, input)
	}
}

func TestMarkdownRendererLeavesSpacedAsterisksLiteral(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, true)

	if _, err := renderer.Write([]byte("2 * 3 * 4\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := output.String(); got != "2 * 3 * 4\n" {
		t.Fatalf("output = %q, want literal multiplication", got)
	}
}

func TestMarkdownRendererClosesUnfinishedCodeStyle(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, true)

	if _, err := renderer.Write([]byte("```\ncode")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	want := ansiReset + "\x1b[36m```" + ansiReset + "\n" +
		ansiReset + "\x1b[36m  code" + ansiReset
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMarkdownRendererEscapesPunctuationAcrossChunks(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, true)

	for _, chunk := range []string{"\\", "*不是斜体", "\\*", "\n"} {
		if _, err := renderer.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := output.String(); got != "*不是斜体*\n" {
		t.Fatalf("output = %q, want literal escaped asterisks", got)
	}
}

func TestMarkdownRendererEscapesLineMarkers(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, true)
	input := "\\# 不是标题\n\\`不是代码\\`\n\\~\\~不是删除线\\~\\~\n"

	if _, err := renderer.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	want := "# 不是标题\n`不是代码`\n~~不是删除线~~\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMarkdownRendererRemovesTerminalControls(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			var output bytes.Buffer
			renderer := NewMarkdown(&output, enabled)
			for _, chunk := range []string{"前\x1b[", "2J中\x1b]52;c;", "YWJj\a后\r\n"} {
				if _, err := renderer.Write([]byte(chunk)); err != nil {
					t.Fatalf("Write() error = %v", err)
				}
			}
			if err := renderer.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got, want := output.String(), "前中后\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestMarkdownRendererRequiresMatchingFenceLength(t *testing.T) {
	var output bytes.Buffer
	renderer := NewMarkdown(&output, true)
	input := "````markdown\n```go\ninside\n```\n````\nafter\n"
	if _, err := renderer.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := strings.Count(output.String(), "````"); got != 2 {
		t.Fatalf("output has %d four-backtick fences, want opening and closing: %q", got, output.String())
	}
	if strings.Contains(output.String(), "\x1b[36m  after") {
		t.Fatalf("after was rendered as code after premature fence close: %q", output.String())
	}
}
