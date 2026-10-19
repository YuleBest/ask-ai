package conversation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBindingStoreRoundTrip(t *testing.T) {
	store := NewBindingStore(t.TempDir(), strings.Repeat("a", 32))
	sessionID := "abcdef" + strings.Repeat("1", 26)

	if _, err := store.Load(); !errors.Is(err, ErrNotBound) {
		t.Fatalf("Load() error = %v, want ErrNotBound", err)
	}
	if err := store.Save(sessionID); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != sessionID {
		t.Fatalf("Load() = %q, want %q", got, sessionID)
	}

	path := filepath.Join(store.Root, terminalBindingStore, store.TerminalID+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat binding: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("binding mode = %o, want 600", mode)
	}

	if err := store.Remove(); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNotBound) {
		t.Fatalf("Load() after Remove() error = %v, want ErrNotBound", err)
	}
}

func TestBindingStoreRemovesStaleBinding(t *testing.T) {
	store := NewBindingStore(t.TempDir(), strings.Repeat("b", 32))
	sessionID := "abcdef" + strings.Repeat("2", 26)
	if err := store.Save(sessionID); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	binding := terminalBinding{
		Version:   bindingVersion,
		SessionID: sessionID,
		ProcessID: -1,
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("marshal stale binding: %v", err)
	}
	if err := os.WriteFile(store.path(), data, 0o600); err != nil {
		t.Fatalf("write stale binding: %v", err)
	}

	if _, err := store.Load(); !errors.Is(err, ErrNotBound) {
		t.Fatalf("Load() error = %v, want stale binding removed", err)
	}
	if _, err := os.Stat(store.path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale binding still exists: %v", err)
	}
}
