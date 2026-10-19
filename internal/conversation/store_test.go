package conversation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "ask-ai")}
	session, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	session.Messages = []Message{
		{Role: "user", Content: "上一个问题"},
		{Role: "assistant", Content: "上一个回答"},
	}

	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if got.ID != session.ID {
		t.Fatalf("session ID = %q, want %q", got.ID, session.ID)
	}
	for i := range session.Messages {
		if got.Messages[i] != session.Messages[i] {
			t.Fatalf("message[%d] = %+v, want %+v", i, got.Messages[i], session.Messages[i])
		}
	}

	sessionPath := filepath.Join(store.Root, "sessions", session.ID+".json")
	lastPath := filepath.Join(store.Root, "last.json")
	for _, path := range []string{sessionPath, lastPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("%s mode = %o, want 600", path, mode)
		}
	}

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if strings.Contains(string(data), "reasoning") {
		t.Fatalf("session contains reasoning data: %s", data)
	}
}

func TestNewSessionIDsAreUnique(t *testing.T) {
	first, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	second, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("session IDs are equal: %q", first.ID)
	}
	if len(first.ID) != sessionIDLength {
		t.Fatalf("session ID length = %d, want %d", len(first.ID), sessionIDLength)
	}
}

func TestStoreMissing(t *testing.T) {
	_, err := (Store{Root: t.TempDir()}).LoadLast()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadLast() error = %v, want ErrNotFound", err)
	}
}

func TestStoreUpdateMergesConcurrentMessages(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "ask-ai")}
	session, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	session.Messages = []Message{
		{Role: "user", Content: "初始问题"},
		{Role: "assistant", Content: "初始回答"},
	}
	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	first, err := store.Load(session.ID)
	if err != nil {
		t.Fatalf("Load(first) error = %v", err)
	}
	second, err := store.Load(session.ID)
	if err != nil {
		t.Fatalf("Load(second) error = %v", err)
	}
	first.Messages = append(first.Messages,
		Message{Role: "user", Content: "进程一问题"},
		Message{Role: "assistant", Content: "进程一回答"},
	)
	second.Messages = append(second.Messages,
		Message{Role: "user", Content: "进程二问题"},
		Message{Role: "assistant", Content: "进程二回答"},
	)

	if err := store.Update(first); err != nil {
		t.Fatalf("Update(first) error = %v", err)
	}
	if err := store.Update(second); err != nil {
		t.Fatalf("Update(second) error = %v", err)
	}

	saved, err := store.Load(session.ID)
	if err != nil {
		t.Fatalf("Load(saved) error = %v", err)
	}
	if len(saved.Messages) != 6 {
		t.Fatalf("saved messages = %+v, want 6 messages", saved.Messages)
	}
	if saved.Messages[2].Content != "进程一问题" || saved.Messages[4].Content != "进程二问题" {
		t.Fatalf("saved messages = %+v, want both processes preserved", saved.Messages)
	}
}

func TestStoreRejectsSessionIDMismatch(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "ask-ai")}
	session, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	session.Messages = []Message{
		{Role: "user", Content: "问题"},
		{Role: "assistant", Content: "回答"},
	}
	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	mismatched := session
	mismatched.ID = strings.Repeat("f", sessionIDLength)
	data, err := json.Marshal(mismatched)
	if err != nil {
		t.Fatalf("marshal mismatch: %v", err)
	}
	if err := os.WriteFile(store.sessionPath(session.ID), data, 0o600); err != nil {
		t.Fatalf("write mismatch: %v", err)
	}

	if _, err := store.Load(session.ID); err == nil || !strings.Contains(err.Error(), "ID") {
		t.Fatalf("Load() error = %v, want session ID mismatch", err)
	}
}
