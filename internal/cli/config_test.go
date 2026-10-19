package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigListCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	var stdout, stderr bytes.Buffer

	code := runConfig(context.Background(), nil, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("config code = %d, stderr = %q", code, stderr.String())
	}
	for _, key := range []string{"meta", "models", "systemPrompt", "webSearch"} {
		if !strings.Contains(stdout.String(), key) {
			t.Fatalf("config output = %q, want key %q", stdout.String(), key)
		}
	}
	for _, file := range []string{"_meta.json", "models.json", "system_prompt.txt", "web_search.json"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatalf("expected config file %s: %v", file, err)
		}
	}
}

func TestConfigViewUsesBatcat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "batcat"), `#!/bin/sh
printf 'BATCAT:%s\n' "$*"
`)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	code := runConfig(context.Background(), []string{"view", "systemPrompt"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("config view code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "BATCAT:-pP ") {
		t.Fatalf("config view output = %q, want batcat call", stdout.String())
	}
}

func TestConfigViewFallsBackToCat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	binDir := t.TempDir()
	if err := os.Symlink("/bin/cat", filepath.Join(binDir, "cat")); err != nil {
		t.Fatalf("create cat symlink: %v", err)
	}
	t.Setenv("PATH", binDir)

	var stdout, stderr bytes.Buffer
	code := runConfig(context.Background(), []string{"view", "systemPrompt"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("config view code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "随手问") {
		t.Fatalf("config view output = %q, want system prompt content", stdout.String())
	}
}

func TestConfigEditUsesNano(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "nano"), `#!/bin/sh
printf 'NANO:%s\n' "$1"
`)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	code := runConfig(context.Background(), []string{"edit", "models"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("config edit code = %d, stderr = %q", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "NANO:"+path {
		t.Fatalf("config edit output = %q", got)
	}
}

func TestConfigViewRedactsSecretsByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, path, "http://127.0.0.1:1")

	var stdout, stderr bytes.Buffer
	code := runConfig(context.Background(), []string{"view", "models"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("config view code = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "test-key") {
		t.Fatalf("config view leaked API key: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"key": "***"`) {
		t.Fatalf("config view did not redact key: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runConfig(context.Background(), []string{"view", "models", "--show-secrets"}, &stdout, &stderr, func() (string, error) {
		return path, nil
	})
	if code != 0 {
		t.Fatalf("config view --show-secrets code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "test-key") {
		t.Fatalf("config view --show-secrets omitted key: %q", stdout.String())
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}
