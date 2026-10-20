package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YuleBest/ask-ai/internal/config"
)

func TestToolList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")

	var stdout, stderr bytes.Buffer
	if code := runTool([]string{"ls"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	}); code != 0 {
		t.Fatalf("tool ls code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"webSearch", "配置", "off,use", "搜索公开网络"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output = %q, want %q", stdout.String(), want)
		}
	}
}

func TestToolWebSearchOffAndUse(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "models.json")
	meta, _, err := config.LoadOrCreateMeta(config.MetaPath(modelPath))
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	webPath, err := meta.PathFor("webSearch", dir)
	if err != nil {
		t.Fatalf("PathFor(webSearch) error = %v", err)
	}
	if err := os.WriteFile(webPath, []byte(`{"use":"exa","provider":[{"id":"exa","key":"test-key"}]}`), 0o600); err != nil {
		t.Fatalf("write web search config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runTool([]string{"webSearch", "off"}, &stdout, &stderr, func() (string, error) {
		return modelPath, nil
	}); code != 0 {
		t.Fatalf("tool off code = %d, stderr = %q", code, stderr.String())
	}
	data, err := os.ReadFile(webPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `"use": false`) {
		t.Fatalf("config after off = %s", data)
	}

	stdout.Reset()
	stderr.Reset()
	if code := runTool([]string{"webSearch", "use", "exa"}, &stdout, &stderr, func() (string, error) {
		return modelPath, nil
	}); code != 0 {
		t.Fatalf("tool use code = %d, stderr = %q", code, stderr.String())
	}
	loaded, _, err := config.LoadOrCreateWebSearch(webPath)
	if err != nil {
		t.Fatalf("LoadOrCreateWebSearch() error = %v", err)
	}
	if loaded.Use != "exa" || !loaded.IsConfigured() {
		t.Fatalf("loaded config = %+v", loaded)
	}
}

func TestToolRejectsUnknownToolAndProvider(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "models.json")
	meta, _, err := config.LoadOrCreateMeta(config.MetaPath(modelPath))
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	webPath, err := meta.PathFor("webSearch", dir)
	if err != nil {
		t.Fatalf("PathFor(webSearch) error = %v", err)
	}
	if err := os.WriteFile(webPath, []byte(`{"use":"exa","provider":[{"id":"exa","key":"test-key"}]}`), 0o600); err != nil {
		t.Fatalf("write web search config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runTool([]string{"unknown", "off"}, &stdout, &stderr, func() (string, error) {
		return modelPath, nil
	}); code != 1 || !strings.Contains(stderr.String(), "未知工具") {
		t.Fatalf("unknown tool code = %d, stderr = %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runTool([]string{"webSearch", "use", "other"}, &stdout, &stderr, func() (string, error) {
		return modelPath, nil
	}); code != 1 || !strings.Contains(stderr.String(), "仅支持 provider: exa") {
		t.Fatalf("unknown provider code = %d, stderr = %q", code, stderr.String())
	}
}

func TestToolRejectsEnablingUnconfiguredProvider(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "models.json")
	meta, _, err := config.LoadOrCreateMeta(config.MetaPath(modelPath))
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	webPath, err := meta.PathFor("webSearch", dir)
	if err != nil {
		t.Fatalf("PathFor(webSearch) error = %v", err)
	}
	if err := os.WriteFile(webPath, []byte(`{"use":false,"provider":[{"id":"exa","key":""}]}`), 0o600); err != nil {
		t.Fatalf("write web search config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runTool([]string{"webSearch", "use", "exa"}, &stdout, &stderr, func() (string, error) {
		return modelPath, nil
	}); code == 0 {
		t.Fatalf("tool use code = 0, stderr = %q, want incomplete configuration error", stderr.String())
	}
	loaded, _, err := config.LoadOrCreateWebSearch(webPath)
	if err != nil {
		t.Fatalf("LoadOrCreateWebSearch() error = %v", err)
	}
	if loaded.IsConfigured() {
		t.Fatal("tool was enabled with an empty Exa key")
	}
}

func TestToolDescriptionPreservesCustomMetadata(t *testing.T) {
	if got, want := toolDescription("webSearch", "custom search"), "custom search"; got != want {
		t.Fatalf("toolDescription() = %q, want %q", got, want)
	}
}
