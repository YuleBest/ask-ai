package cli

import (
	"bytes"
	"fmt"
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
	for _, want := range []string{"webSearch", "配置", "off,use", "搜索公开网络", "viewFile", "mode,limit", "查看本机普通文件"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output = %q, want %q", stdout.String(), want)
		}
	}
}

func TestToolViewFileMode(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "models.json")
	meta, _, err := config.LoadOrCreateMeta(config.MetaPath(modelPath))
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	viewPath, err := meta.PathFor("viewFile", dir)
	if err != nil {
		t.Fatalf("PathFor(viewFile) error = %v", err)
	}

	for _, mode := range []config.ViewFileMode{
		config.ViewFileModeBypass,
		config.ViewFileModeOff,
		config.ViewFileModeAsk,
	} {
		t.Run(string(mode), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runTool([]string{"viewFile", "mode", string(mode)}, &stdout, &stderr, func() (string, error) {
				return modelPath, nil
			})
			if code != 0 {
				t.Fatalf("tool mode code = %d, stderr = %q", code, stderr.String())
			}
			loaded, _, err := config.LoadOrCreateViewFile(viewPath)
			if err != nil {
				t.Fatalf("LoadOrCreateViewFile() error = %v", err)
			}
			if loaded.Mode != mode {
				t.Fatalf("mode = %q, want %q", loaded.Mode, mode)
			}
		})
	}

	var stdout, stderr bytes.Buffer
	if code := runTool([]string{"viewFile", "mode", "always"}, &stdout, &stderr, func() (string, error) {
		return modelPath, nil
	}); code == 0 {
		t.Fatalf("invalid mode code = 0, stderr = %q", stderr.String())
	}
	loaded, _, err := config.LoadOrCreateViewFile(viewPath)
	if err != nil {
		t.Fatalf("LoadOrCreateViewFile() after invalid mode error = %v", err)
	}
	if loaded.Mode != config.ViewFileModeAsk {
		t.Fatalf("mode after invalid update = %q, want ask", loaded.Mode)
	}
}

func TestToolViewFileLimit(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "models.json")
	meta, _, err := config.LoadOrCreateMeta(config.MetaPath(modelPath))
	if err != nil {
		t.Fatalf("LoadOrCreateMeta() error = %v", err)
	}
	viewPath, err := meta.PathFor("viewFile", dir)
	if err != nil {
		t.Fatalf("PathFor(viewFile) error = %v", err)
	}

	for _, limit := range []string{"1", "512000", "2097152"} {
		t.Run(limit, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runTool([]string{"viewFile", "limit", limit}, &stdout, &stderr, func() (string, error) {
				return modelPath, nil
			})
			if code != 0 {
				t.Fatalf("tool limit code = %d, stderr = %q", code, stderr.String())
			}
			loaded, _, err := config.LoadOrCreateViewFile(viewPath)
			if err != nil {
				t.Fatalf("LoadOrCreateViewFile() error = %v", err)
			}
			if got := fmt.Sprintf("%d", loaded.MaxBytes); got != limit {
				t.Fatalf("limit = %s, want %s", got, limit)
			}
		})
	}

	for _, limit := range []string{"0", "-1", "2097153", "1.5", ""} {
		t.Run("invalid "+limit, func(t *testing.T) {
			before, _, err := config.LoadOrCreateViewFile(viewPath)
			if err != nil {
				t.Fatalf("LoadOrCreateViewFile() error = %v", err)
			}
			var stdout, stderr bytes.Buffer
			if code := runTool([]string{"viewFile", "limit", limit}, &stdout, &stderr, func() (string, error) {
				return modelPath, nil
			}); code == 0 {
				t.Fatalf("invalid limit %q code = 0", limit)
			}
			after, _, err := config.LoadOrCreateViewFile(viewPath)
			if err != nil {
				t.Fatalf("LoadOrCreateViewFile() after invalid limit error = %v", err)
			}
			if after.MaxBytes != before.MaxBytes {
				t.Fatalf("limit changed from %d to %d", before.MaxBytes, after.MaxBytes)
			}
		})
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
