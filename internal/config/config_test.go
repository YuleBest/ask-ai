package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

func TestLoadOrCreateSystemPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ask-ai", "system_prompt.json")

	prompt, created, err := LoadOrCreateSystemPrompt(path, i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("LoadOrCreateSystemPrompt() error = %v", err)
	}
	if !created {
		t.Fatal("LoadOrCreateSystemPrompt() did not report a created prompt")
	}
	if !strings.Contains(prompt, "随手问") {
		t.Fatalf("default prompt = %q, want a casual assistant prompt", prompt)
	}
	for _, variable := range []string{"{{os}}", "{{arch}}", "{{date}}", "{{time}}", "{{timezone}}"} {
		if !strings.Contains(prompt, variable) {
			t.Fatalf("default prompt lacks %s: %q", variable, prompt)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat prompt: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("prompt mode = %o, want 600", got)
	}

	prompt, created, err = LoadOrCreateSystemPrompt(path, i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("second LoadOrCreateSystemPrompt() error = %v", err)
	}
	if created {
		t.Fatal("LoadOrCreateSystemPrompt() reported a second creation")
	}
	if !strings.Contains(prompt, "随手问") {
		t.Fatalf("saved prompt = %q, want default content", prompt)
	}
}

func TestLoadOrCreateSystemPromptIncludesEnglishPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ask-ai", "system_prompt.json")

	prompt, created, err := LoadOrCreateSystemPrompt(path, i18n.English)
	if err != nil {
		t.Fatalf("LoadOrCreateSystemPrompt() error = %v", err)
	}
	if !created {
		t.Fatal("LoadOrCreateSystemPrompt() did not report creation")
	}
	for _, want := range []string{"quick-question", "{{os}}", "{{timezone}}"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("English prompt lacks %q: %q", want, prompt)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prompt JSON: %v", err)
	}
	for _, want := range []string{`"zh-CN"`, `"en"`, "随手问", "quick-question"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("prompt JSON lacks %q: %s", want, data)
		}
	}
}

func TestRenderSystemPromptVariables(t *testing.T) {
	t.Setenv("TZ", "Test/Zone")
	now := time.Date(2026, 10, 20, 13, 14, 15, 0, time.FixedZone("Test/Zone", 8*60*60))
	prompt := "os={{os}} system={{system}} arch={{arch}} architecture={{architecture}} " +
		"date={{date}} time={{time}} timezone={{timezone}} datetime={{datetime}} unknown={{unknown}}"

	got := RenderSystemPrompt(prompt, now)
	want := "os=" + runtime.GOOS + " system=" + runtime.GOOS +
		" arch=" + runtime.GOARCH + " architecture=" + runtime.GOARCH +
		" date=2026-10-20 time=13:14:15 timezone=Test/Zone (UTC+08:00)" +
		" datetime=2026-10-20 13:14:15 unknown={{unknown}}"
	if got != want {
		t.Fatalf("RenderSystemPrompt() = %q, want %q", got, want)
	}
}

func TestLoadOrCreateSystemPromptUpgradesLegacyDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system_prompt.json")
	legacyPath := filepath.Join(dir, "system_prompt.txt")
	if err := os.WriteFile(legacyPath, []byte(legacyDefaultSystemPrompt+"\n"), 0o600); err != nil {
		t.Fatalf("write legacy prompt: %v", err)
	}

	prompt, created, err := LoadOrCreateSystemPrompt(path, i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("LoadOrCreateSystemPrompt() error = %v", err)
	}
	if created {
		t.Fatal("legacy default prompt reported as newly created")
	}
	if !strings.Contains(prompt, "{{os}}") || !strings.Contains(prompt, "{{timezone}}") {
		t.Fatalf("upgraded prompt = %q, want environment variables", prompt)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read upgraded prompt: %v", err)
	}
	if !strings.Contains(string(data), "{{arch}}") {
		t.Fatalf("saved prompt = %q, want upgraded content", data)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy prompt still exists, stat error = %v", err)
	}
}

func TestLoadSystemPromptAllowsEmptyOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system_prompt.json")
	if err := os.WriteFile(path, []byte(`{"zh-CN":"","en":"English prompt"}`), 0o600); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	prompt, created, err := LoadOrCreateSystemPrompt(path, i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("LoadOrCreateSystemPrompt() error = %v", err)
	}
	if created {
		t.Fatal("LoadOrCreateSystemPrompt() created an existing prompt")
	}
	if prompt != "" {
		t.Fatalf("prompt = %q, want empty", prompt)
	}
}

func TestLoadOrCreateSystemPromptMigratesCustomPromptToJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system_prompt.json")
	legacyPath := filepath.Join(dir, "system_prompt.txt")
	if err := os.WriteFile(legacyPath, []byte("自定义提示词\n"), 0o600); err != nil {
		t.Fatalf("write legacy prompt: %v", err)
	}

	prompt, created, err := LoadOrCreateSystemPrompt(path, i18n.English)
	if err != nil {
		t.Fatalf("LoadOrCreateSystemPrompt() error = %v", err)
	}
	if created {
		t.Fatal("migrated prompt reported as newly created")
	}
	if got, want := prompt, "自定义提示词"; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prompt JSON: %v", err)
	}
	var prompts map[string]string
	if err := json.Unmarshal(data, &prompts); err != nil {
		t.Fatalf("parse prompt JSON: %v", err)
	}
	for _, language := range []string{"zh-CN", "en"} {
		if got, want := prompts[language], "自定义提示词\n"; got != want {
			t.Fatalf("prompt[%s] = %q, want %q", language, got, want)
		}
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy prompt still exists, stat error = %v", err)
	}
}

func TestLoadSystemPromptFallsBackToBuiltInForMissingLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system_prompt.json")
	if err := os.WriteFile(path, []byte(`{"zh-CN":"自定义"}`), 0o600); err != nil {
		t.Fatalf("write prompt JSON: %v", err)
	}

	prompt, _, err := LoadOrCreateSystemPrompt(path, i18n.English)
	if err != nil {
		t.Fatalf("LoadOrCreateSystemPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "quick-question") {
		t.Fatalf("fallback prompt = %q, want built-in English prompt", prompt)
	}
}

func TestLoadOrCreateSystemPromptConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ask-ai", "system_prompt.json")
	const workers = 8
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, err := LoadOrCreateSystemPrompt(path, i18n.ChineseSimplified)
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("LoadOrCreateSystemPrompt() error = %v", err)
		}
	}
}
