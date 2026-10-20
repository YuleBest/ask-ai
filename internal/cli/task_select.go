package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/YuleBest/ask-ai/internal/conversation"
	"github.com/YuleBest/ask-ai/internal/i18n"
	"github.com/YuleBest/ask-ai/internal/terminaltext"
	"golang.org/x/term"
)

var (
	errTaskSelectionCanceled    = i18n.New("已取消任务删除")
	errTaskSelectionInterrupted = i18n.New("任务选择已中断")
)

type taskSelectionKey int

const (
	taskSelectionUnknown taskSelectionKey = iota
	taskSelectionUp
	taskSelectionDown
	taskSelectionToggle
	taskSelectionConfirm
	taskSelectionCancel
	taskSelectionInterrupt
	taskSelectionYes
)

func selectTasksInTerminal(ctx context.Context, summaries []conversation.Summary, stdout io.Writer) ([]conversation.Summary, error) {
	stdin := os.Stdin
	if !term.IsTerminal(int(stdin.Fd())) || !isTerminal(stdout) {
		return nil, i18n.New("交互选择任务需要在终端中运行，请显式提供至少一个 hash")
	}

	oldState, err := term.MakeRaw(int(stdin.Fd()))
	if err != nil {
		return nil, i18n.Errorf("启用终端交互模式失败: %w", err)
	}
	defer term.Restore(int(stdin.Fd()), oldState)

	width, height, err := term.GetSize(int(stdin.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		width, height = 80, 24
	}

	if _, err := io.WriteString(stdout, "\x1b[?25l"); err != nil {
		return nil, err
	}
	defer io.WriteString(stdout, "\x1b[?25h")

	return runTaskSelectorContext(ctx, stdin, stdout, summaries, width, height)
}

func runTaskSelector(input io.Reader, output io.Writer, summaries []conversation.Summary, width, height int) ([]conversation.Summary, error) {
	return runTaskSelectorContext(context.Background(), input, output, summaries, width, height)
}

func runTaskSelectorContext(ctx context.Context, input io.Reader, output io.Writer, summaries []conversation.Summary, width, height int) ([]conversation.Summary, error) {
	if len(summaries) == 0 {
		return nil, nil
	}
	if width < 40 {
		return nil, i18n.New("终端宽度不足 40 列，无法显示任务选择器")
	}
	visibleCount := height - 3
	if visibleCount < 1 {
		visibleCount = 1
	}
	if visibleCount > len(summaries) {
		visibleCount = len(summaries)
	}

	reader := bufio.NewReader(input)
	selected := make([]bool, len(summaries))
	cursor := 0
	offset := 0
	view := taskSelectorView{writer: output}
	style := newOutputStyle(output)

	draw := func(footer string) error {
		lines := []string{style.bold(i18n.T("选择要删除的任务"))}
		end := offset + visibleCount
		if end > len(summaries) {
			end = len(summaries)
		}
		for index := offset; index < end; index++ {
			lines = append(lines, taskSelectorLine(summaries[index], selected[index], index == cursor, width, style))
		}
		lines = append(lines, style.dim(taskSelectorHelp(width)), style.dim(footer))
		return view.render(lines)
	}

	footer := taskSelectorFooter(0)
	for {
		if err := draw(footer); err != nil {
			return nil, err
		}
		key, err := readTaskSelectionKeyContext(ctx, reader)
		if err != nil {
			view.clear()
			return nil, err
		}

		switch key {
		case taskSelectionUp:
			if cursor > 0 {
				cursor--
			}
		case taskSelectionDown:
			if cursor < len(summaries)-1 {
				cursor++
			}
		case taskSelectionToggle:
			selected[cursor] = !selected[cursor]
			footer = taskSelectorFooter(selectedTaskCount(selected))
		case taskSelectionConfirm:
			count := selectedTaskCount(selected)
			if count == 0 {
				footer = i18n.T("至少选择一个任务；空格选择，q 取消")
				continue
			}
			if err := draw(i18n.T("确认删除 %d 个任务？y 确认，其他取消", count)); err != nil {
				return nil, err
			}
			key, err := readTaskSelectionKeyContext(ctx, reader)
			if err != nil {
				view.clear()
				return nil, err
			}
			switch key {
			case taskSelectionYes:
				result := make([]conversation.Summary, 0, count)
				for index, summary := range summaries {
					if selected[index] {
						result = append(result, summary)
					}
				}
				view.clear()
				return result, nil
			case taskSelectionInterrupt:
				view.clear()
				return nil, errTaskSelectionInterrupted
			default:
				view.clear()
				return nil, errTaskSelectionCanceled
			}
		case taskSelectionCancel:
			view.clear()
			return nil, errTaskSelectionCanceled
		case taskSelectionInterrupt:
			view.clear()
			return nil, errTaskSelectionInterrupted
		}

		if cursor < offset {
			offset = cursor
		}
		if cursor >= offset+visibleCount {
			offset = cursor - visibleCount + 1
		}
	}
}

func readTaskSelectionKeyContext(ctx context.Context, reader *bufio.Reader) (taskSelectionKey, error) {
	type result struct {
		key taskSelectionKey
		err error
	}
	resultChannel := make(chan result, 1)
	go func() {
		key, err := readTaskSelectionKeyRaw(reader)
		resultChannel <- result{key: key, err: err}
	}()
	select {
	case <-ctx.Done():
		return taskSelectionUnknown, ctx.Err()
	case result := <-resultChannel:
		return result.key, result.err
	}
}

func readTaskSelectionKeyRaw(reader *bufio.Reader) (taskSelectionKey, error) {
	char, err := reader.ReadByte()
	if err != nil {
		return taskSelectionUnknown, err
	}

	switch char {
	case ' ', '\t':
		return taskSelectionToggle, nil
	case '\r', '\n':
		return taskSelectionConfirm, nil
	case 'q', 'Q', 4:
		return taskSelectionCancel, nil
	case 3:
		return taskSelectionInterrupt, nil
	case 'y', 'Y':
		return taskSelectionYes, nil
	case 'k', 'K':
		return taskSelectionUp, nil
	case 'j', 'J':
		return taskSelectionDown, nil
	case 27:
		second, err := reader.ReadByte()
		if err != nil {
			return taskSelectionUnknown, err
		}
		if second != '[' {
			return taskSelectionUnknown, nil
		}
		third, err := reader.ReadByte()
		if err != nil {
			return taskSelectionUnknown, err
		}
		switch third {
		case 'A':
			return taskSelectionUp, nil
		case 'B':
			return taskSelectionDown, nil
		}
	}
	return taskSelectionUnknown, nil
}

func taskSelectorFooter(selected int) string {
	return i18n.T("已选 %d 个任务", selected)
}

func taskSelectorHelp(width int) string {
	if width >= 60 {
		return i18n.T("空格选择  ↑/↓ 或 j/k 移动  Enter 确认  q 取消")
	}
	if width >= 45 {
		return i18n.T("空格选择  ↑↓/jk 移动  Enter 确认  q 取消")
	}
	return i18n.T("Space选择 ↑↓/jk移动 Enter确认 q取消")
}

func taskSelectorLine(summary conversation.Summary, selected, cursor bool, width int, style outputStyle) string {
	pointer := " "
	if cursor {
		pointer = style.cyan(">")
	}

	check := style.dim("[ ]")
	if selected {
		check = style.green("[x]")
	}

	shortID := summary.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	current := ""
	if summary.Current {
		current = style.yellow("*")
	}
	hashStyle := style.cyan
	if summary.Current {
		hashStyle = style.boldCyan
	}

	prefixWidth := 20
	if summary.Current {
		prefixWidth++
	}
	messageLimit := width - prefixWidth
	if messageLimit < 8 {
		messageLimit = 8
	}
	message := taskMessageForDisplayWidth(summary.FirstMessage, messageLimit)
	if selected || summary.Current {
		message = style.bold(message)
	}

	return fmt.Sprintf("%s %s %s%s  %s", pointer, check, hashStyle(shortID), current, message)
}

func taskMessageForDisplayWidth(content string, limit int) string {
	message := strings.Join(strings.Fields(terminaltext.Clean(content)), " ")
	if message == "" {
		return i18n.T("（无首条用户消息）")
	}
	if limit < 4 {
		limit = 4
	}
	if displayWidth(message) <= limit {
		return message
	}

	var truncated strings.Builder
	width := 0
	for _, char := range message {
		charWidth := displayWidth(string(char))
		if width+charWidth > limit-3 {
			break
		}
		truncated.WriteRune(char)
		width += charWidth
	}
	return truncated.String() + "..."
}

func selectedTaskCount(selected []bool) int {
	count := 0
	for _, value := range selected {
		if value {
			count++
		}
	}
	return count
}

type taskSelectorView struct {
	writer   io.Writer
	rendered int
}

func (v *taskSelectorView) render(lines []string) error {
	var output strings.Builder
	if v.rendered > 0 {
		fmt.Fprintf(&output, "\x1b[%dA", v.rendered)
	}
	for _, line := range lines {
		output.WriteString("\r\x1b[2K")
		output.WriteString(line)
		output.WriteByte('\n')
	}
	v.rendered = len(lines)
	_, err := io.WriteString(v.writer, output.String())
	return err
}

func (v *taskSelectorView) clear() {
	if v.rendered == 0 {
		return
	}
	var output strings.Builder
	fmt.Fprintf(&output, "\x1b[%dA", v.rendered)
	for range v.rendered {
		output.WriteString("\r\x1b[2K\n")
	}
	io.WriteString(v.writer, output.String())
	v.rendered = 0
}
