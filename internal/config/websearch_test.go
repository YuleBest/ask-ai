package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOrCreateWebSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_search.json")

	config, created, err := LoadOrCreateWebSearch(path)
	if err != nil {
		t.Fatalf("LoadOrCreateWebSearch() error = %v", err)
	}
	if !created || config.Use != "exa" || len(config.Providers) != 1 {
		t.Fatalf("config = %+v, created = %v", config, created)
	}
	if config.IsConfigured() {
		t.Fatal("blank key reported as configured")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode = %o, want 600", mode)
	}
}

func TestWebSearchResolveProvider(t *testing.T) {
	config := WebSearchConfig{
		Use: "exa",
		Providers: []WebSearchProvider{
			{ID: "example", Key: "other"},
			{ID: "exa", Key: "test-key"},
		},
	}
	if !config.IsConfigured() {
		t.Fatal("IsConfigured() = false")
	}
	provider, err := config.ResolveProvider()
	if err != nil {
		t.Fatalf("ResolveProvider() error = %v", err)
	}
	if provider.ID != "exa" || provider.Key != "test-key" {
		t.Fatalf("provider = %+v", provider)
	}

	config.Use = "unknown"
	if _, err := config.ResolveProvider(); err == nil {
		t.Fatal("ResolveProvider() accepted unknown provider")
	}
}

func TestWebSearchUseFalseRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_search.json")
	data := []byte(`{"use":false,"provider":[{"id":"exa","key":"test-key"}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, _, err := LoadOrCreateWebSearch(path)
	if err != nil {
		t.Fatalf("LoadOrCreateWebSearch() error = %v", err)
	}
	if config.IsConfigured() {
		t.Fatal("use=false reported as configured")
	}
	if err := SaveWebSearch(path, config); err != nil {
		t.Fatalf("SaveWebSearch() error = %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(saved, &raw); err != nil {
		t.Fatalf("decode saved config: %v", err)
	}
	if raw["use"] != false {
		t.Fatalf("use = %#v, want false", raw["use"])
	}

	config.Use = "exa"
	if err := SaveWebSearch(path, config); err != nil {
		t.Fatalf("SaveWebSearch(exa) error = %v", err)
	}
	saved, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(saved), `"use": "exa"`) {
		t.Fatalf("saved config = %s", saved)
	}
}

func TestLoadOrCreateWebSearchRejectsInsecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file modes")
	}
	path := filepath.Join(t.TempDir(), "web_search.json")
	data := []byte(`{"use":"exa","provider":[{"id":"exa","key":"secret"}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, _, err := LoadOrCreateWebSearch(path); err == nil {
		t.Fatal("LoadOrCreateWebSearch() accepted a world-readable secret file")
	}
}
