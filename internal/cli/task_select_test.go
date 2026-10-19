package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/YuleBest/ask-ai/internal/conversation"
)

func TestRunTaskSelectorMultipleSelection(t *testing.T) {
	summaries := []conversation.Summary{
		{ID: "111111" + strings.Repeat("1", 26), FirstMessage: "任务一", UpdatedAt: time.Now()},
		{ID: "222222" + strings.Repeat("2", 26), FirstMessage: "任务二", UpdatedAt: time.Now()},
		{ID: "333333" + strings.Repeat("3", 26), FirstMessage: "任务三", UpdatedAt: time.Now()},
	}
	input := bytes.NewBufferString(" \x1b[B \ry")
	var output bytes.Buffer

	selected, err := runTaskSelector(input, &output, summaries, 80, 10)
	if err != nil {
		t.Fatalf("runTaskSelector() error = %v", err)
	}
	if len(selected) != 2 || selected[0].ID != summaries[0].ID || selected[1].ID != summaries[1].ID {
		t.Fatalf("selected = %+v, want first two tasks", selected)
	}
	if !strings.Contains(output.String(), "确认删除 2 个任务") {
		t.Fatalf("output = %q, want confirmation prompt", output.String())
	}
	if strings.Count(output.String(), "[x]") < 2 {
		t.Fatalf("output = %q, want selected task markers", output.String())
	}
}

func TestRunTaskSelectorRequiresSelection(t *testing.T) {
	summaries := []conversation.Summary{
		{ID: "111111" + strings.Repeat("1", 26), FirstMessage: "任务一"},
	}
	input := bytes.NewBufferString("\rq")
	var output bytes.Buffer

	_, err := runTaskSelector(input, &output, summaries, 80, 10)
	if !errors.Is(err, errTaskSelectionCanceled) {
		t.Fatalf("runTaskSelector() error = %v, want cancellation", err)
	}
	if !strings.Contains(output.String(), "至少选择一个任务") {
		t.Fatalf("output = %q, want empty selection hint", output.String())
	}
}

func TestRunTaskSelectorCancel(t *testing.T) {
	summaries := []conversation.Summary{
		{ID: "111111" + strings.Repeat("1", 26), FirstMessage: "任务一"},
	}
	var output bytes.Buffer

	_, err := runTaskSelector(bytes.NewBufferString("q"), &output, summaries, 80, 10)
	if !errors.Is(err, errTaskSelectionCanceled) {
		t.Fatalf("runTaskSelector() error = %v, want cancellation", err)
	}
}

func TestTaskMessageForDisplayWidth(t *testing.T) {
	got := taskMessageForDisplayWidth("中文消息 abcdef", 12)
	if displayWidth(got) > 12 {
		t.Fatalf("display width = %d, want at most 12: %q", displayWidth(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("message = %q, want ellipsis", got)
	}
}

func TestRunTaskSelectorRejectsNarrowTerminal(t *testing.T) {
	summaries := []conversation.Summary{
		{ID: "111111" + strings.Repeat("1", 26), FirstMessage: "任务一"},
	}
	if _, err := runTaskSelector(bytes.NewBufferString("q"), &bytes.Buffer{}, summaries, 20, 10); err == nil ||
		!strings.Contains(err.Error(), "40") {
		t.Fatalf("runTaskSelector() error = %v, want narrow terminal error", err)
	}
}

func TestRunTaskSelectorContextCancel(t *testing.T) {
	summaries := []conversation.Summary{
		{ID: "111111" + strings.Repeat("1", 26), FirstMessage: "任务一"},
	}
	input := &blockingTaskReader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runTaskSelectorContext(ctx, input, &bytes.Buffer{}, summaries, 80, 10)
		done <- err
	}()

	select {
	case <-input.started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("selector did not start reading input")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runTaskSelectorContext() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("selector did not stop after cancellation")
	}
	close(input.release)
}

type blockingTaskReader struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingTaskReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.release
	return 0, io.EOF
}
