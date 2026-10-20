package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

func TestLoadOrCreateLanguageInheritsEnvironmentByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ask-ai", "language.json")

	language, created, err := LoadOrCreateLanguage(path)
	if err != nil {
		t.Fatalf("LoadOrCreateLanguage() error = %v", err)
	}
	if !created {
		t.Fatal("LoadOrCreateLanguage() did not report creation")
	}
	if language.Output != "" || language.Prompt != "" {
		t.Fatalf("default language overrides = %#v, want inherited values", language)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read language file: %v", err)
	}
	if got, want := string(data), "{}\n"; got != want {
		t.Fatalf("language file = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat language file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("language file mode = %o, want 600", got)
	}
}

func TestLanguageConfigResolvesOverridesIndependently(t *testing.T) {
	language := LanguageConfig{Output: i18n.English}
	settings := language.Resolve(func(key string) string {
		if key == "LANG" {
			return "zh_CN.UTF-8"
		}
		return ""
	})
	if settings.Output != i18n.English {
		t.Fatalf("output = %q, want English", settings.Output)
	}
	if settings.Prompt != i18n.ChineseSimplified {
		t.Fatalf("prompt = %q, want inherited Chinese", settings.Prompt)
	}
}

func TestLoadLanguageRejectsUnsupportedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "language.json")
	if err := os.WriteFile(path, []byte(`{"output":"fr","prompt":"en"}`), 0o600); err != nil {
		t.Fatalf("write language file: %v", err)
	}

	if _, _, err := LoadOrCreateLanguage(path); err == nil {
		t.Fatal("LoadOrCreateLanguage() accepted unsupported language")
	}
}

func TestLoadLanguageTreatsWhitespaceAsInherited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "language.json")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatalf("write language file: %v", err)
	}

	language, exists, err := LoadLanguage(path)
	if err != nil {
		t.Fatalf("LoadLanguage() error = %v", err)
	}
	if !exists {
		t.Fatal("LoadLanguage() reported a missing existing file")
	}
	if language.Output != "" || language.Prompt != "" {
		t.Fatalf("language overrides = %#v, want inherited values", language)
	}
}

func TestLoadOrCreateLanguageConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ask-ai", "language.json")
	const workers = 8
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, err := LoadOrCreateLanguage(path)
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("LoadOrCreateLanguage() error = %v", err)
		}
	}
}
