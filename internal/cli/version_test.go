package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		t.Run(args[0], func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithTerminalInput(context.Background(), args, nil, &stdout, &stderr, func() (string, error) {
				t.Fatal("config path should not be requested for version")
				return "", nil
			}, func(io.Writer, io.Writer) (string, error) {
				t.Fatal("terminal ID should not be requested for version")
				return "", nil
			})
			if code != 0 {
				t.Fatalf("code = %d, want 0", code)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if got, want := stdout.String(), Version+"\n"; got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestVersionCommandRejectsArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWithTerminalInput(context.Background(), []string{"version", "extra"}, nil, &stdout, &stderr, func() (string, error) {
		t.Fatal("config path should not be requested for version")
		return "", nil
	}, func(io.Writer, io.Writer) (string, error) {
		t.Fatal("terminal ID should not be requested for version")
		return "", nil
	})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "askai version - 显示当前版本号") {
		t.Fatalf("stderr = %q, want version usage", stderr.String())
	}
}

func TestVersionFlagAfterQuestion(t *testing.T) {
	for _, flag := range []string{"-v", "--version"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithTerminalInput(context.Background(), []string{"问题", flag}, nil, &stdout, &stderr, func() (string, error) {
				t.Fatal("config path should not be requested for version")
				return "", nil
			}, func(io.Writer, io.Writer) (string, error) {
				t.Fatal("terminal ID should not be requested for version")
				return "", nil
			})
			if code != 0 {
				t.Fatalf("code = %d, want 0, stderr = %q", code, stderr.String())
			}
			if got, want := stdout.String(), Version+"\n"; got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
		})
	}
}
