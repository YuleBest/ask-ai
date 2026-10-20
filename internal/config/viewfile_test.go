package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrCreateViewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view_file.json")

	got, created, err := LoadOrCreateViewFile(path)
	if err != nil {
		t.Fatalf("LoadOrCreateViewFile() error = %v", err)
	}
	if !created {
		t.Fatal("LoadOrCreateViewFile() did not report creation")
	}
	want := ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: DefaultViewFileMaxBytes}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("config mode = %o, want 600", mode)
	}

	loaded, created, err := LoadOrCreateViewFile(path)
	if err != nil {
		t.Fatalf("LoadOrCreateViewFile() second error = %v", err)
	}
	if created {
		t.Fatal("LoadOrCreateViewFile() recreated an existing file")
	}
	if loaded != want {
		t.Fatalf("loaded config = %+v, want %+v", loaded, want)
	}
}

func TestViewFileConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config ViewFileConfig
		ok     bool
	}{
		{name: "ask", config: ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: 1}, ok: true},
		{name: "bypass", config: ViewFileConfig{Mode: ViewFileModeBypass, MaxBytes: DefaultViewFileMaxBytes}, ok: true},
		{name: "off", config: ViewFileConfig{Mode: ViewFileModeOff, MaxBytes: MaxViewFileMaxBytes}, ok: true},
		{name: "bad mode", config: ViewFileConfig{Mode: "always", MaxBytes: 1}},
		{name: "empty mode", config: ViewFileConfig{MaxBytes: 1}},
		{name: "zero bytes", config: ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: 0}},
		{name: "negative bytes", config: ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: -1}},
		{name: "too many bytes", config: ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: MaxViewFileMaxBytes + 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err == nil) != tt.ok {
				t.Fatalf("Validate() error = %v, want ok = %v", err, tt.ok)
			}
		})
	}
}

func TestSaveViewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view_file.json")
	want := ViewFileConfig{Mode: ViewFileModeBypass, MaxBytes: 2097152}

	if err := SaveViewFile(path, want); err != nil {
		t.Fatalf("SaveViewFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("config mode = %o, want 600", mode)
	}

	got, created, err := LoadOrCreateViewFile(path)
	if err != nil {
		t.Fatalf("LoadOrCreateViewFile() error = %v", err)
	}
	if created {
		t.Fatal("LoadOrCreateViewFile() recreated a saved file")
	}
	if got != want {
		t.Fatalf("loaded config = %+v, want %+v", got, want)
	}
}

func TestSaveViewFileRejectsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view_file.json")
	want := ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: DefaultViewFileMaxBytes}
	if err := SaveViewFile(path, want); err != nil {
		t.Fatalf("SaveViewFile() error = %v", err)
	}

	invalid := ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: MaxViewFileMaxBytes + 1}
	if err := SaveViewFile(path, invalid); err == nil {
		t.Fatal("SaveViewFile() accepted invalid config")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `"max_bytes": 512000`) {
		t.Fatalf("config was overwritten with invalid values: %s", data)
	}
}
