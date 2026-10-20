package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

func TestMain(m *testing.M) {
	os.Setenv("LANGUAGE", "zh_CN.UTF-8")
	os.Setenv("LC_ALL", "zh_CN.UTF-8")
	os.Setenv("LANG", "zh_CN.UTF-8")
	os.Exit(m.Run())
}

func TestHelpUsesEnvironmentLanguage(t *testing.T) {
	restoreLanguage(t)
	t.Setenv("LANGUAGE", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	dir := t.TempDir()

	var stdout, stderr strings.Builder
	code := runWithTerminalInput(context.Background(), []string{"help"}, nil, &stdout, &stderr, func() (string, error) {
		return filepath.Join(dir, "models.json"), nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("English help lacks Usage heading: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "用法:") {
		t.Fatalf("English help contains Chinese heading: %q", stdout.String())
	}
	for _, char := range stdout.String() {
		if unicode.Is(unicode.Han, char) {
			t.Fatalf("English help contains untranslated Chinese: %q", stdout.String())
		}
	}
}

func TestLanguageConfigOverridesEnvironmentIndependently(t *testing.T) {
	restoreLanguage(t)
	t.Setenv("LANGUAGE", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "language.json"), []byte(`{"output":"zh-CN","prompt":"en"}`), 0o600); err != nil {
		t.Fatalf("write language config: %v", err)
	}

	var stdout, stderr strings.Builder
	code := runWithTerminalInput(context.Background(), []string{"help"}, nil, &stdout, &stderr, func() (string, error) {
		return filepath.Join(dir, "models.json"), nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "用法:") {
		t.Fatalf("output language override was not applied: %q", stdout.String())
	}
	if got := i18n.PromptLanguage(); got != i18n.English {
		t.Fatalf("prompt language = %q, want English", got)
	}
}

func TestEarlyLanguageResolutionUsesMetadataPath(t *testing.T) {
	restoreLanguage(t)
	t.Setenv("LANGUAGE", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	dir := t.TempDir()
	customDir := filepath.Join(dir, "custom")
	if err := os.MkdirAll(customDir, 0o700); err != nil {
		t.Fatalf("create custom config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "language.json"), []byte(`{"output":"zh-CN"}`), 0o600); err != nil {
		t.Fatalf("write custom language config: %v", err)
	}
	metaData, err := json.Marshal(config.Meta{
		Version: 1,
		Configs: map[string]config.MetaEntry{
			"models":   {File: "models.json"},
			"language": {File: "custom/language.json"},
		},
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_meta.json"), metaData, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	var stdout, stderr strings.Builder
	code := runWithTerminalInput(context.Background(), []string{"help"}, nil, &stdout, &stderr, func() (string, error) {
		return filepath.Join(dir, "models.json"), nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "用法:") {
		t.Fatalf("help did not use custom language path: %q", stdout.String())
	}
}

func TestPromptLanguageSelectsEnglishPromptAndToolInstructions(t *testing.T) {
	restoreLanguage(t)
	t.Setenv("LANGUAGE", "zh_CN.UTF-8")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")

	var systemPrompt string
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
			systemPrompt = payload.Messages[0].Content
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	writeTestModelsConfig(t, modelsPath, server.URL)
	if err := os.WriteFile(filepath.Join(dir, "language.json"), []byte(`{"output":"zh-CN","prompt":"en"}`), 0o600); err != nil {
		t.Fatalf("write language config: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "web_search.json"),
		[]byte(`{"use":"exa","provider":[{"id":"exa","key":"test-key"}]}`),
		0o600,
	); err != nil {
		t.Fatalf("write web search config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"hello"}, &stdout, &stderr, func() (string, error) {
		return modelsPath, nil
	})
	if code != 0 {
		t.Fatalf("run code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"quick-question", "You can use web_search and web_fetch"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt lacks %q: %q", want, systemPrompt)
		}
	}
	if strings.Contains(systemPrompt, "随手问") {
		t.Fatalf("English prompt contains Chinese default: %q", systemPrompt)
	}
}

func TestErrorsUseOutputLanguage(t *testing.T) {
	restoreLanguage(t)
	t.Setenv("LANGUAGE", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	dir := t.TempDir()

	var stdout, stderr strings.Builder
	code := runWithTerminalInput(context.Background(), nil, nil, &stdout, &stderr, func() (string, error) {
		return filepath.Join(dir, "models.json"), nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	for _, want := range []string{"Missing question", "Usage:"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
	for _, char := range stderr.String() {
		if unicode.Is(unicode.Han, char) {
			t.Fatalf("English error output contains untranslated Chinese: %q", stderr.String())
		}
	}
}

func TestHelpFlagSurvivesMalformedLanguageConfig(t *testing.T) {
	restoreLanguage(t)
	t.Setenv("LANGUAGE", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "language.json"), []byte(`{broken`), 0o600); err != nil {
		t.Fatalf("write language config: %v", err)
	}

	var stdout, stderr strings.Builder
	code := runWithTerminalInput(context.Background(), []string{"--help"}, nil, &stdout, &stderr, func() (string, error) {
		return filepath.Join(dir, "models.json"), nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("--help did not open with malformed language config: %q", stdout.String())
	}
}

func TestStoredSentinelErrorsUseOutputLanguage(t *testing.T) {
	restoreLanguage(t)
	t.Setenv("LANGUAGE", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()

	var stdout, stderr strings.Builder
	code := runWithTerminalInput(
		context.Background(),
		[]string{"-f", "continue"},
		nil,
		&stdout,
		&stderr,
		func() (string, error) {
			return filepath.Join(dir, "models.json"), nil
		},
		func(io.Writer, io.Writer) (string, error) {
			return "", nil
		},
	)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if got, want := strings.TrimSpace(stderr.String()), "askai: There is no previous conversation to follow"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestEnglishCommandOutput(t *testing.T) {
	commands := []struct {
		name string
		args []string
	}{
		{name: "config list", args: []string{"config"}},
		{name: "model list", args: []string{"ml"}},
		{name: "task list", args: []string{"tl"}},
		{name: "tool list", args: []string{"tool", "ls"}},
		{name: "task help", args: []string{"help", "task"}},
		{name: "config help", args: []string{"help", "config"}},
		{name: "model help", args: []string{"help", "model"}},
		{name: "tool help", args: []string{"help", "tool"}},
	}

	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			restoreLanguage(t)
			t.Setenv("LANGUAGE", "en_US.UTF-8")
			t.Setenv("LC_ALL", "en_US.UTF-8")
			t.Setenv("LANG", "en_US.UTF-8")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			dir := t.TempDir()

			var stdout, stderr strings.Builder
			code := runWithTerminalInput(
				context.Background(),
				command.args,
				nil,
				&stdout,
				&stderr,
				func() (string, error) {
					return filepath.Join(dir, "models.json"), nil
				},
				func(io.Writer, io.Writer) (string, error) {
					return "", nil
				},
			)
			if code != 0 {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			for _, char := range stdout.String() + stderr.String() {
				if unicode.Is(unicode.Han, char) {
					t.Fatalf("output contains untranslated Chinese:\nstdout=%q\nstderr=%q", stdout.String(), stderr.String())
				}
			}
		})
	}
}

func restoreLanguage(t *testing.T) {
	t.Helper()
	previousOutput := i18n.OutputLanguage()
	previousPrompt := i18n.PromptLanguage()
	t.Cleanup(func() {
		i18n.SetOutputLanguage(previousOutput)
		i18n.SetPromptLanguage(previousPrompt)
	})
}
