package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateMeta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_meta.json")

	meta, created, err := LoadOrCreateMeta(path)
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	if !created {
		t.Fatal("LoadOrCreateMeta() did not report creation")
	}
	for _, key := range []string{"meta", "models", "systemPrompt", "webSearch"} {
		if _, ok := meta.Configs[key]; !ok {
			t.Fatalf("default meta is missing key %q", key)
		}
	}
	if _, ok := meta.Tools["webSearch"]; !ok {
		t.Fatal("default meta is missing webSearch tool")
	}
	if got := len(meta.Tools["webSearch"].Definitions); got != 2 {
		t.Fatalf("webSearch definitions = %d, want 2", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat meta: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("meta mode = %o, want 600", mode)
	}

	promptPath, err := meta.PathFor("systemPrompt", dir)
	if err != nil {
		t.Fatalf("PathFor() error = %v", err)
	}
	if promptPath != filepath.Join(dir, "system_prompt.txt") {
		t.Fatalf("systemPrompt path = %q", promptPath)
	}
}

func TestMetaPathRejectsTraversal(t *testing.T) {
	meta := Meta{
		Version: metaVersion,
		Configs: map[string]MetaEntry{
			"unsafe": {File: "../outside", Description: "unsafe"},
		},
	}
	if _, err := meta.PathFor("unsafe", t.TempDir()); err == nil {
		t.Fatal("PathFor() accepted path traversal")
	}
}

func TestLoadMetaCustomPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_meta.json")
	meta := Meta{
		Version: metaVersion,
		Configs: map[string]MetaEntry{
			"models": {File: "models/default.json", Description: "models"},
		},
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	loaded, created, err := LoadOrCreateMeta(path)
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	if created {
		t.Fatal("LoadOrCreateMeta() created an existing file")
	}
	modelPath, err := loaded.PathFor("models", dir)
	if err != nil {
		t.Fatalf("PathFor() error = %v", err)
	}
	if modelPath != filepath.Join(dir, "models/default.json") {
		t.Fatalf("model path = %q", modelPath)
	}
	if _, err := loaded.PathFor("webSearch", dir); err != nil {
		t.Fatalf("webSearch key was not added during metadata upgrade: %v", err)
	}
	if _, exists := loaded.Tools["webSearch"]; !exists {
		t.Fatal("webSearch tool was not added during metadata upgrade")
	}
	if len(loaded.Tools["webSearch"].Definitions) != 2 {
		t.Fatal("webSearch tool definitions were not added during metadata upgrade")
	}
}

func TestLoadMetaAddsWebSearchKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_meta.json")
	meta := Meta{
		Version: metaVersion,
		Configs: map[string]MetaEntry{
			"meta":         {File: "_meta.json", Description: "meta"},
			"models":       {File: "models.json", Description: "models"},
			"systemPrompt": {File: "system_prompt.txt", Description: "prompt"},
		},
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	loaded, _, err := LoadOrCreateMeta(path)
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	entry, exists := loaded.Configs["webSearch"]
	if !exists || entry.File != "web_search.json" {
		t.Fatalf("webSearch entry = %+v, exists = %v", entry, exists)
	}
	tool, exists := loaded.Tools["webSearch"]
	if !exists || tool.Config != "webSearch" {
		t.Fatalf("webSearch tool = %+v, exists = %v", tool, exists)
	}
}
