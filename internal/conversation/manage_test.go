package conversation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListResolveSwitchAndRemove(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "ask-ai")}
	idA := "abcdef" + strings.Repeat("1", 26)
	idB := "abcdef" + strings.Repeat("2", 26)
	idC := "123456" + strings.Repeat("3", 26)

	for _, session := range []Session{
		testSession(idA, "任务 A", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		testSession(idB, "任务 B", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
		testSession(idC, "任务 C", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)),
	} {
		if err := store.Save(session); err != nil {
			t.Fatalf("Save(%s) error = %v", session.ID, err)
		}
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("List() returned %d tasks, want 3", len(summaries))
	}
	if summaries[0].ID != idC || !summaries[0].Current {
		t.Fatalf("first summary = %+v, want current task C", summaries[0])
	}

	if _, err := store.Resolve("abcdef"); err == nil || !strings.Contains(err.Error(), "更长") {
		t.Fatalf("Resolve(ambiguous) error = %v, want longer hash error", err)
	}
	if id, err := store.Resolve("abcdef1"); err != nil || id != idA {
		t.Fatalf("Resolve(unique) = (%q, %v), want %q", id, err, idA)
	}
	if _, err := store.Resolve("abc"); err == nil {
		t.Fatal("Resolve(short) error = nil")
	}
	if _, err := store.Resolve("xyz123"); err == nil {
		t.Fatal("Resolve(non-hex) error = nil")
	}

	switched, err := store.Switch("123456")
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if switched.ID != idC || !switched.Current {
		t.Fatalf("Switch() = %+v, want task C", switched)
	}
	current, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if current.ID != idC {
		t.Fatalf("current task = %q, want %q", current.ID, idC)
	}

	removed, err := store.Remove("123456")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if removed.ID != idC || !removed.Current {
		t.Fatalf("Remove() = %+v, want current task C", removed)
	}
	if _, err := store.LoadLast(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadLast() after removing current error = %v, want ErrNotFound", err)
	}

	summaries, err = store.List()
	if err != nil {
		t.Fatalf("List() after remove error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("List() after remove returned %d tasks, want 2", len(summaries))
	}
}

func TestRemoveManyValidatesBeforeDeleting(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "ask-ai")}
	idA := "abcdef" + strings.Repeat("1", 26)
	idB := "abcdef" + strings.Repeat("2", 26)
	idC := "123456" + strings.Repeat("3", 26)

	for _, session := range []Session{
		testSession(idA, "任务 A", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		testSession(idB, "任务 B", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
		testSession(idC, "任务 C", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)),
	} {
		if err := store.Save(session); err != nil {
			t.Fatalf("Save(%s) error = %v", session.ID, err)
		}
	}

	if _, err := store.RemoveMany([]string{"123456", "ffffff"}); err == nil {
		t.Fatal("RemoveMany() error = nil for invalid batch")
	}
	for _, id := range []string{idA, idB, idC} {
		if _, err := os.Stat(store.sessionPath(id)); err != nil {
			t.Fatalf("task %s was deleted before validation completed: %v", id, err)
		}
	}

	removed, err := store.RemoveMany([]string{"abcdef1", "abcdef2"})
	if err != nil {
		t.Fatalf("RemoveMany() error = %v", err)
	}
	if len(removed) != 2 || removed[0].ID != idA || removed[1].ID != idB {
		t.Fatalf("RemoveMany() = %+v, want tasks A and B", removed)
	}
	for _, id := range []string{idA, idB} {
		if _, err := os.Stat(store.sessionPath(id)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("task %s still exists: %v", id, err)
		}
	}
	current, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if current.ID != idC {
		t.Fatalf("current task = %q, want %q", current.ID, idC)
	}

	removed, err = store.RemoveMany([]string{"123456", "123456"})
	if err != nil {
		t.Fatalf("RemoveMany(duplicates) error = %v", err)
	}
	if len(removed) != 1 || removed[0].ID != idC || !removed[0].Current {
		t.Fatalf("RemoveMany(duplicates) = %+v, want current task C once", removed)
	}
	if _, err := store.LoadLast(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadLast() after removing current error = %v, want ErrNotFound", err)
	}
}

func testSession(id, firstMessage string, updatedAt time.Time) Session {
	return Session{
		Version:   sessionVersion,
		ID:        id,
		CreatedAt: updatedAt.Add(-time.Minute),
		UpdatedAt: updatedAt,
		Messages: []Message{
			{Role: "user", Content: firstMessage},
			{Role: "assistant", Content: "回答"},
		},
	}
}
