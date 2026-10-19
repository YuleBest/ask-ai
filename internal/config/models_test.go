package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadOrCreateModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ask-ai", "models.json")

	models, created, err := LoadOrCreateModels(path)
	if err != nil {
		t.Fatalf("LoadOrCreateModels() error = %v", err)
	}
	if !created {
		t.Fatal("LoadOrCreateModels() did not report creation")
	}
	if len(models.Providers) == 0 {
		t.Fatal("default models has no providers")
	}
	if len(models.Providers) == 0 || len(models.Providers[0].Models) == 0 {
		t.Fatal("default models has no model entries")
	}
	model := models.Providers[0].Models[0]
	if models.Providers[0].ID != "local" || model.ID != "cn:deepseek-v4.1-flash" || !model.Default {
		t.Fatalf("default model = (%s, %+v)", models.Providers[0].ID, model)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat models: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("models mode = %o, want 600", mode)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read models: %v", err)
	}
	var saved Models
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("unmarshal models: %v", err)
	}
	if len(saved.Providers) != 2 {
		t.Fatalf("saved providers = %d, want 2", len(saved.Providers))
	}
}

func TestResolveDefaultModel(t *testing.T) {
	models := Models{
		Providers: []Provider{
			{
				ID:   "local",
				Name: "Local",
				URL:  "http://127.0.0.1:7863/v1",
				Key:  "test-key",
				Models: []Model{
					{ID: "model-a", ReasoningEffort: "high"},
					{ID: "model-b", ReasoningEffort: "max", Default: true},
				},
			},
		},
	}

	provider, model, err := models.ResolveDefault()
	if err != nil {
		t.Fatalf("ResolveDefault() error = %v", err)
	}
	if provider.ID != "local" || model.ID != "model-b" || model.ReasoningEffort != "max" {
		t.Fatalf("ResolveDefault() = (%+v, %+v)", provider, model)
	}
}

func TestResolveDefaultFallsBackToFirstModel(t *testing.T) {
	models := Models{
		Providers: []Provider{
			{
				ID:     "local",
				Name:   "Local",
				URL:    "http://127.0.0.1:7863/v1",
				Key:    "test-key",
				Models: []Model{{ID: "model-a"}},
			},
		},
	}

	_, model, err := models.ResolveDefault()
	if err != nil {
		t.Fatalf("ResolveDefault() error = %v", err)
	}
	if model.ID != "model-a" {
		t.Fatalf("active model = %q, want model-a", model.ID)
	}
}

func TestResolveRejectsMissingModel(t *testing.T) {
	models := Models{
		Providers: []Provider{
			{
				ID:     "local",
				Name:   "Local",
				URL:    "http://127.0.0.1:7863/v1",
				Key:    "test-key",
				Models: []Model{{ID: "model-a"}},
			},
		},
	}
	if _, _, err := models.Resolve("local", "missing"); err == nil {
		t.Fatal("Resolve() accepted a missing model")
	}
}

func TestInactiveProviderDoesNotNeedCredentials(t *testing.T) {
	models := Models{
		Providers: []Provider{
			{
				ID:     "local",
				Name:   "Local",
				URL:    "http://127.0.0.1:7863/v1",
				Key:    "test-key",
				Models: []Model{{ID: "model-a", Default: true}},
			},
			{
				ID:     "placeholder",
				Models: []Model{{ID: "future-model"}},
			},
		},
	}
	if err := ValidateModels(models); err != nil {
		t.Fatalf("ValidateModels() error = %v", err)
	}
}

func TestSetDefault(t *testing.T) {
	models := Models{
		Providers: []Provider{
			{
				ID:   "local",
				Name: "Local",
				URL:  "http://127.0.0.1:7863/v1",
				Key:  "test-key",
				Models: []Model{
					{ID: "model-a", Default: true},
					{ID: "model-b"},
				},
			},
		},
	}
	if err := models.SetDefault("local", "model-b"); err != nil {
		t.Fatalf("SetDefault() error = %v", err)
	}
	_, model, err := models.ResolveDefault()
	if err != nil {
		t.Fatalf("ResolveDefault() error = %v", err)
	}
	if model.ID != "model-b" || !model.Default {
		t.Fatalf("default model = %+v, want model-b", model)
	}
}

func TestLoadOrCreateModelsRejectsInsecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file modes")
	}
	path := filepath.Join(t.TempDir(), "models.json")
	data := []byte(`{"provider":[{"id":"local","name":"Local","url":"http://127.0.0.1:1/v1","key":"secret","models":[{"id":"m"}]}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write models: %v", err)
	}
	if _, _, err := LoadOrCreateModels(path); err == nil {
		t.Fatal("LoadOrCreateModels() accepted a world-readable secret file")
	}
}

func TestValidateModelsRejectsProviderIDWithSlash(t *testing.T) {
	models := Models{Providers: []Provider{{
		ID:     "bad/id",
		Name:   "Bad",
		URL:    "http://127.0.0.1:1/v1",
		Key:    "secret",
		Models: []Model{{ID: "model"}},
	}}}
	if err := ValidateModels(models); err == nil {
		t.Fatal("ValidateModels() accepted a provider ID containing slash")
	}
}
