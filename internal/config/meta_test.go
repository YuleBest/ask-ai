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
	for _, key := range []string{"meta", "models", "systemPrompt", "language", "webSearch", "viewFile"} {
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
	viewTool, ok := meta.Tools["viewFile"]
	if !ok {
		t.Fatal("default meta is missing viewFile tool")
	}
	if viewTool.Config != "viewFile" || len(viewTool.Definitions) != 1 || viewTool.Definitions[0].Name != "view_file" {
		t.Fatalf("viewFile tool = %+v", viewTool)
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
	if promptPath != filepath.Join(dir, "system_prompt.json") {
		t.Fatalf("systemPrompt path = %q", promptPath)
	}
	languagePath, err := meta.PathFor("language", dir)
	if err != nil {
		t.Fatalf("PathFor(language) error = %v", err)
	}
	if languagePath != filepath.Join(dir, "language.json") {
		t.Fatalf("language path = %q", languagePath)
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
			"systemPrompt": {File: "system_prompt.json", Description: "prompt"},
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

func TestLoadMetaAddsViewFileConfigAndTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_meta.json")
	meta := Meta{
		Version: metaVersion,
		Configs: map[string]MetaEntry{
			"meta":         {File: "_meta.json", Description: "meta"},
			"models":       {File: "models.json", Description: "models"},
			"systemPrompt": {File: "system_prompt.json", Description: "prompt"},
		},
		Tools: map[string]MetaTool{},
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
	entry, exists := loaded.Configs["viewFile"]
	if !exists || entry.File != "view_file.json" {
		t.Fatalf("viewFile entry = %+v, exists = %v", entry, exists)
	}
	tool, exists := loaded.Tools["viewFile"]
	if !exists || tool.Config != "viewFile" {
		t.Fatalf("viewFile tool = %+v, exists = %v", tool, exists)
	}
	if len(tool.Definitions) != 1 || tool.Definitions[0].Name != "view_file" {
		t.Fatalf("view_file definitions = %+v", tool.Definitions)
	}
}

func TestLoadMetaMigratesLegacyPromptPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_meta.json")
	meta := Meta{
		Version: metaVersion,
		Configs: map[string]MetaEntry{
			"meta":         {File: "_meta.json", Description: "meta"},
			"models":       {File: "models.json", Description: "models"},
			"systemPrompt": {File: "custom/prompt.txt", Description: "prompt"},
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
	promptPath, err := loaded.PathFor("systemPrompt", dir)
	if err != nil {
		t.Fatalf("PathFor(systemPrompt) error = %v", err)
	}
	if got, want := promptPath, filepath.Join(dir, "custom", "prompt.json"); got != want {
		t.Fatalf("prompt path = %q, want %q", got, want)
	}
	if _, err := loaded.PathFor("language", dir); err != nil {
		t.Fatalf("language key was not added: %v", err)
	}
}
