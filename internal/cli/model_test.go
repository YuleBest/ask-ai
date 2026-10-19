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

	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/conversation"
)

func TestModelUsePreservesHistory(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, modelsPath, "http://127.0.0.1:7863/v1")
	appendTestModel(t, modelsPath, "model-b", "max", false)

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	session := taskTestSession("abcdef"+strings.Repeat("1", 26), "第一问")
	session.Messages = []conversation.Message{
		{Role: "user", Content: "第一问"},
		{Role: "assistant", Content: "第一答"},
	}
	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runWithTerminal(context.Background(), []string{"mu", "local/model-b"}, &stdout, &stderr, func() (string, error) {
		return modelsPath, nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 0 {
		t.Fatalf("mu code = %d, stderr = %q", code, stderr.String())
	}

	updated, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if updated.Provider != "local" || updated.Model != "model-b" {
		t.Fatalf("session model = %s/%s, want local/model-b", updated.Provider, updated.Model)
	}
	if len(updated.Messages) != 2 || updated.Messages[0].Content != "第一问" || updated.Messages[1].Content != "第一答" {
		t.Fatalf("session messages changed: %+v", updated.Messages)
	}
}

func TestModelDefaultShortcut(t *testing.T) {
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, modelsPath, "http://127.0.0.1:7863/v1")
	appendTestModel(t, modelsPath, "model-b", "max", false)

	var stdout, stderr bytes.Buffer
	code := runWithTerminal(context.Background(), []string{"md", "local/model-b"}, &stdout, &stderr, func() (string, error) {
		return modelsPath, nil
	}, func(io.Writer, io.Writer) (string, error) {
		return "", nil
	})
	if code != 0 {
		t.Fatalf("md code = %d, stderr = %q", code, stderr.String())
	}

	models, _, err := config.LoadOrCreateModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadOrCreateModels() error = %v", err)
	}
	_, model, err := models.ResolveDefault()
	if err != nil {
		t.Fatalf("ResolveDefault() error = %v", err)
	}
	if model.ID != "model-b" || !model.Default {
		t.Fatalf("default model = %+v, want model-b", model)
	}
	for _, candidate := range models.Providers[0].Models {
		if candidate.ID != "model-b" && candidate.Default {
			t.Fatalf("unexpected default model: %+v", candidate)
		}
	}
}

func TestModelListAndShortcut(t *testing.T) {
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, modelsPath, "http://127.0.0.1:7863/v1")
	appendTestModel(t, modelsPath, "model-b", "max", false)

	runList := func(args []string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runWithTerminal(context.Background(), args, &stdout, &stderr, func() (string, error) {
			return modelsPath, nil
		}, func(io.Writer, io.Writer) (string, error) {
			return "", nil
		})
		if code != 0 {
			t.Fatalf("%v code = %d, stderr = %q", args, code, stderr.String())
		}
		return stdout.String()
	}

	long := runList([]string{"model", "list"})
	short := runList([]string{"ml"})
	if long != short {
		t.Fatalf("model list and ml differ:\nlong=%q\nshort=%q", long, short)
	}
	for _, want := range []string{
		"local",
		"Local",
		"http://127.0.0.1:7863/v1",
		"test-model",
		"reasoning=high",
		"model-b",
		"reasoning=max",
		"[default]",
	} {
		if !strings.Contains(long, want) {
			t.Fatalf("model list output = %q, want %q", long, want)
		}
	}
	if strings.Contains(long, "test-key") {
		t.Fatalf("model list leaked API key: %q", long)
	}
	if strings.Contains(long, "\x1b") {
		t.Fatalf("non-terminal model list contains ANSI: %q", long)
	}
}

func TestRunUsesSessionSelectedModel(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var authorization string
	var requestModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestModel = payload.Model

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	writeTestModelsConfig(t, modelsPath, server.URL)
	appendTestModel(t, modelsPath, "model-b", "max", false)
	if err := os.WriteFile(filepath.Join(dir, "system_prompt.txt"), []byte("系统提示\n"), 0o600); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	session := conversation.Session{
		Version:  1,
		ID:       "abcdef" + "12345678901234567890123456",
		Provider: "local",
		Model:    "model-b",
		Messages: []conversation.Message{
			{Role: "user", Content: "上一问"},
			{Role: "assistant", Content: "上一答"},
		},
	}
	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-f", "继续"}, &stdout, &stderr, func() (string, error) {
		return modelsPath, nil
	})
	if code != 0 {
		t.Fatalf("run code = %d, stderr = %q", code, stderr.String())
	}
	if requestModel != "model-b" {
		t.Fatalf("request model = %q, want model-b", requestModel)
	}
	if authorization != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want provider key", authorization)
	}
}

func appendTestModel(t *testing.T, path, id, effort string, isDefault bool) {
	t.Helper()
	models, _, err := config.LoadOrCreateModels(path)
	if err != nil {
		t.Fatalf("LoadOrCreateModels() error = %v", err)
	}
	models.Providers[0].Models = append(models.Providers[0].Models, config.Model{
		ID:              id,
		ReasoningEffort: effort,
		Default:         isDefault,
	})
	if err := config.SaveModels(path, models); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}
}
