package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YuleBest/ask-ai/internal/conversation"
)

func TestTaskListSwitchRemoveAliases(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("1", 26)
	idB := "abcdef" + strings.Repeat("2", 26)
	idC := "123456" + strings.Repeat("3", 26)
	sessionA := taskTestSession(idA, "任务 A 的第一条消息")
	sessionA.Provider = "local"
	sessionA.Model = "model-a"
	sessionB := taskTestSession(idB, "任务 B 的第一条消息")
	sessionB.Provider = "local"
	sessionB.Model = "model-b"
	sessionC := taskTestSession(idC, "任务 C 的第一条消息\n带换行")
	sessionC.Provider = "local"
	sessionC.Model = "model-c"
	for _, session := range []conversation.Session{sessionA, sessionB, sessionC} {
		if err := store.Save(session); err != nil {
			t.Fatalf("Save(%s) error = %v", session.ID, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runTask([]string{"ls"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task ls code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), idC) || !strings.Contains(stdout.String(), "任务 C 的第一条消息 带换行") {
		t.Fatalf("task list output = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "* "+idC) {
		t.Fatalf("task list does not mark current task: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b") {
		t.Fatalf("non-terminal task list contains ANSI: %q", stdout.String())
	}
	var taskALine string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.Contains(line, idA) {
			taskALine = line
			break
		}
	}
	if taskALine == "" || !strings.HasSuffix(strings.TrimSpace(taskALine), "model-a") {
		t.Fatalf("task list line = %q, want model ID as rightmost field", taskALine)
	}

	stdout.Reset()
	stderr.Reset()
	if code := runTask([]string{"st", "abcdef1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task st code = %d, stderr = %q", code, stderr.String())
	}
	current, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if current.ID != idA {
		t.Fatalf("current task = %q, want %q", current.ID, idA)
	}

	stdout.Reset()
	stderr.Reset()
	if code := runTask([]string{"rm", "123456"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task rm code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(store.Root, "sessions", idC+".json")); !os.IsNotExist(err) {
		t.Fatalf("removed task file still exists: %v", err)
	}
	if _, err := store.LoadLast(); err != nil {
		t.Fatalf("LoadLast() after removing non-current task error = %v", err)
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("remaining tasks = %d, want 2", len(summaries))
	}
}

func TestTaskShortcutCommands(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("7", 26)
	idB := "abcdef" + strings.Repeat("8", 26)
	for _, session := range []conversation.Session{
		taskTestSession(idA, "任务 A"),
		taskTestSession(idB, "任务 B"),
	} {
		if err := store.Save(session); err != nil {
			t.Fatalf("Save(%s) error = %v", session.ID, err)
		}
	}

	runShortcut := func(t *testing.T, args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runWithTerminal(context.Background(), args, &stdout, &stderr, func() (string, error) {
			t.Fatal("config path should not be requested for task shortcuts")
			return "", nil
		}, func(io.Writer, io.Writer) (string, error) {
			t.Fatal("terminal ID should not be requested for task shortcuts")
			return "", nil
		})
		if code != 0 {
			t.Fatalf("runWithTerminal(%v) code = %d, stderr = %q", args, code, stderr.String())
		}
		return stdout.String()
	}

	listOutput := runShortcut(t, "tl")
	if !strings.Contains(listOutput, idA) || !strings.Contains(listOutput, idB) {
		t.Fatalf("tl output = %q", listOutput)
	}

	viewOutput := runShortcut(t, "tv", idA[:7])
	if !strings.Contains(viewOutput, "任务 A") || !strings.Contains(viewOutput, "[assistant]") {
		t.Fatalf("tv output = %q", viewOutput)
	}

	runShortcut(t, "ts", idA[:7])
	current, err := store.LoadLast()
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if current.ID != idA {
		t.Fatalf("current task = %q, want %q", current.ID, idA)
	}

	viewOutput = runShortcut(t, "tv")
	if !strings.Contains(viewOutput, idA) || !strings.Contains(viewOutput, "任务 A") {
		t.Fatalf("tv output = %q, want current task A", viewOutput)
	}

	runShortcut(t, "tr", idB[:7])
	if _, err := os.Stat(filepath.Join(store.Root, "sessions", idB+".json")); !os.IsNotExist(err) {
		t.Fatalf("tr did not remove task B: %v", err)
	}
}

func TestTaskRemoveCurrentClearsCurrent(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	id := "abcdef" + strings.Repeat("4", 26)
	if err := store.Save(taskTestSession(id, "当前任务")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runTask([]string{"remove", "abcdef"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task remove code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "当前任务已清除") {
		t.Fatalf("output = %q, want current cleared message", stdout.String())
	}
	if _, err := store.LoadLast(); err == nil {
		t.Fatal("LoadLast() error = nil after removing current task")
	}
}

func TestTaskRemoveMultipleHashes(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("a", 26)
	idB := "abcdef" + strings.Repeat("b", 26)
	idC := "123456" + strings.Repeat("c", 26)
	for _, session := range []conversation.Session{
		taskTestSession(idA, "任务 A"),
		taskTestSession(idB, "任务 B"),
		taskTestSession(idC, "任务 C"),
	} {
		if err := store.Save(session); err != nil {
			t.Fatalf("Save(%s) error = %v", session.ID, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runTask([]string{"remove", idA[:7], idB[:7]}, &stdout, &stderr); code != 0 {
		t.Fatalf("task remove multiple code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "已删除 2 个任务") ||
		!strings.Contains(stdout.String(), idA) ||
		!strings.Contains(stdout.String(), idB) {
		t.Fatalf("output = %q, want both removed task IDs", stdout.String())
	}
	for _, id := range []string{idA, idB} {
		if _, err := os.Stat(filepath.Join(store.Root, "sessions", id+".json")); !os.IsNotExist(err) {
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
}

func TestTaskInteractiveRemoveUsesSelection(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("d", 26)
	idB := "abcdef" + strings.Repeat("e", 26)
	idC := "abcdef" + strings.Repeat("f", 26)
	for _, session := range []conversation.Session{
		taskTestSession(idA, "任务 A"),
		taskTestSession(idB, "任务 B"),
		taskTestSession(idC, "任务 C"),
	} {
		if err := store.Save(session); err != nil {
			t.Fatalf("Save(%s) error = %v", session.ID, err)
		}
	}

	selector := func(summaries []conversation.Summary, _ io.Writer) ([]conversation.Summary, error) {
		if len(summaries) != 3 {
			t.Fatalf("selector received %d summaries, want 3", len(summaries))
		}
		return []conversation.Summary{summaries[0], summaries[2]}, nil
	}

	var stdout, stderr bytes.Buffer
	if code := removeTasksWithSelector(store, nil, &stdout, &stderr, selector); code != 0 {
		t.Fatalf("interactive remove code = %d, stderr = %q", code, stderr.String())
	}
	for _, id := range []string{idA, idC} {
		if _, err := os.Stat(filepath.Join(store.Root, "sessions", id+".json")); !os.IsNotExist(err) {
			t.Fatalf("task %s still exists: %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(store.Root, "sessions", idB+".json")); err != nil {
		t.Fatalf("unselected task B was deleted: %v", err)
	}
	if !strings.Contains(stdout.String(), "当前任务已清除") {
		t.Fatalf("output = %q, want current task cleared message", stdout.String())
	}
}

func TestTaskInteractiveRemoveCancelKeepsTasks(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	id := "abcdef" + strings.Repeat("9", 26)
	if err := store.Save(taskTestSession(id, "保留任务")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	selector := func([]conversation.Summary, io.Writer) ([]conversation.Summary, error) {
		return nil, errTaskSelectionCanceled
	}

	var stdout, stderr bytes.Buffer
	if code := removeTasksWithSelector(store, nil, &stdout, &stderr, selector); code != 0 {
		t.Fatalf("interactive remove code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := store.Load(id); err != nil {
		t.Fatalf("task was deleted after cancel: %v", err)
	}
	if !strings.Contains(stdout.String(), "已取消") {
		t.Fatalf("output = %q, want cancellation message", stdout.String())
	}
}

func TestTaskView(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	id := "abcdef" + strings.Repeat("9", 26)
	session := taskTestSession(id, "第一轮问题")
	session.Messages = append(session.Messages,
		conversation.Message{Role: "user", Content: "第二轮问题\x1b[2J"},
		conversation.Message{Role: "assistant", Content: "第二轮回答"},
	)
	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := runTask([]string{"view", "abcdef"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task view code = %d, stderr = %q", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{id, "[user]\n第一轮问题", "[assistant]\n回答", "第二轮问题", "第二轮回答"} {
		if !strings.Contains(got, want) {
			t.Fatalf("task view output = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "\x1b") {
		t.Fatalf("non-terminal task view contains ANSI: %q", got)
	}
}

func TestTaskViewDefaultsToLast(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	idA := "abcdef" + strings.Repeat("7", 26)
	idB := "abcdef" + strings.Repeat("8", 26)
	for _, session := range []conversation.Session{
		taskTestSession(idA, "上上个任务"),
		taskTestSession(idB, "上一次任务"),
	} {
		if err := store.Save(session); err != nil {
			t.Fatalf("Save(%s) error = %v", session.ID, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runTask([]string{"view"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task view code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), idB) || !strings.Contains(stdout.String(), "上一次任务") {
		t.Fatalf("output = %q, want current task B", stdout.String())
	}
	if strings.Contains(stdout.String(), idA) {
		t.Fatalf("output = %q, should not show previous task A", stdout.String())
	}
}

func TestTaskResolverRequiresLongerAmbiguousHash(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	store, err := conversation.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	for _, id := range []string{
		"abcdef" + strings.Repeat("5", 26),
		"abcdef" + strings.Repeat("6", 26),
	} {
		if err := store.Save(taskTestSession(id, "任务 "+id[:8])); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runTask([]string{"switch", "abcdef"}, &stdout, &stderr); code != 1 {
		t.Fatalf("task switch code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "请提供更长的哈希") {
		t.Fatalf("stderr = %q, want ambiguity error", stderr.String())
	}
}

func TestTaskMessageTruncatesAndCollapsesWhitespace(t *testing.T) {
	got := taskMessage("第一行\n\n第二行 " + strings.Repeat("长", 100))
	if strings.Contains(got, "\n") || !strings.HasSuffix(got, "...") {
		t.Fatalf("taskMessage() = %q", got)
	}
	if len([]rune(got)) != taskSummaryLimit {
		t.Fatalf("message length = %d, want %d", len([]rune(got)), taskSummaryLimit)
	}
}

func taskTestSession(id, firstMessage string) conversation.Session {
	now := time.Now().UTC()
	return conversation.Session{
		Version:   1,
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []conversation.Message{
			{Role: "user", Content: firstMessage},
			{Role: "assistant", Content: "回答"},
		},
	}
}
