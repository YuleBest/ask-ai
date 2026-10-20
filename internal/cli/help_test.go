package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpCommandAndSubcommandHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help command", args: []string{"help"}, want: "askai - 在命令行中询问 AI"},
		{name: "help task", args: []string{"help", "task"}, want: "askai task - 管理任务"},
		{name: "help config", args: []string{"help", "config"}, want: "askai config - 查看或编辑配置文件"},
		{name: "help model", args: []string{"help", "model"}, want: "askai model - 查看和管理模型"},
		{name: "help tool", args: []string{"help", "tool"}, want: "askai tool - 查看或配置模型可用工具"},
		{name: "help version", args: []string{"help", "version"}, want: "askai version - 显示当前版本号"},
		{name: "task help", args: []string{"task", "--help"}, want: "askai task - 管理任务"},
		{name: "config help", args: []string{"config", "--help"}, want: "askai config - 查看或编辑配置文件"},
		{name: "model help", args: []string{"model", "--help"}, want: "askai model - 查看和管理模型"},
		{name: "tool help", args: []string{"tool", "--help"}, want: "askai tool - 查看或配置模型可用工具"},
		{name: "version help", args: []string{"version", "--help"}, want: "askai version - 显示当前版本号"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithTerminal(context.Background(), tt.args, &stdout, &stderr, func() (string, error) {
				return filepath.Join(t.TempDir(), "models.json"), nil
			}, func(io.Writer, io.Writer) (string, error) {
				t.Fatal("terminal ID should not be requested for help")
				return "", nil
			})
			if code != 0 {
				t.Fatalf("code = %d, want 0", code)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tt.want)
			}
			for _, section := range []string{"用法:", "命令:", "示例:"} {
				if !strings.Contains(stdout.String(), section) {
					t.Fatalf("stdout lacks %q section: %q", section, stdout.String())
				}
			}
			if tt.args[0] == "help" && len(tt.args) == 1 {
				for _, section := range []string{"说明:", "快捷命令:", "选项:", "参数:", "文件:", "退出状态:"} {
					if !strings.Contains(stdout.String(), section) {
						t.Fatalf("top-level help lacks %q section: %q", section, stdout.String())
					}
				}
				for _, shortcut := range []string{"tl", "tv [hash]", "ts <hash>", "tr [hash...]"} {
					if !strings.Contains(stdout.String(), shortcut) {
						t.Fatalf("top-level help lacks task shortcut %q: %q", shortcut, stdout.String())
					}
				}
				if !strings.Contains(stdout.String(), "--show-think") {
					t.Fatalf("top-level help lacks --show-think: %q", stdout.String())
				}
				if !strings.Contains(stdout.String(), "-p, --picture <path>") {
					t.Fatalf("top-level help lacks picture option: %q", stdout.String())
				}
				if !strings.Contains(stdout.String(), "-s, --screenshot") {
					t.Fatalf("top-level help lacks screenshot option: %q", stdout.String())
				}
				if !strings.Contains(stdout.String(), "version") {
					t.Fatalf("top-level help lacks version command: %q", stdout.String())
				}
			}
			if strings.Contains(stdout.String(), "undefined") {
				t.Fatalf("stdout lacks standard help sections: %q", stdout.String())
			}
		})
	}
}

func TestDisplayWidth(t *testing.T) {
	for text, want := range map[string]int{
		"task":  4,
		"命令":    4,
		"(无参数)": 8,
		"<问题>":  6,
	} {
		if got := displayWidth(text); got != want {
			t.Fatalf("displayWidth(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestToolHelpIncludesViewFileCommands(t *testing.T) {
	var output bytes.Buffer
	printToolUsage(&output)

	for _, want := range []string{
		"askai tool viewFile mode <ask|bypass|off>",
		"askai tool viewFile limit <bytes>",
		"2097152",
		"~/.config/ask-ai/view_file.json",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("tool help = %q, want %q", output.String(), want)
		}
	}
}
