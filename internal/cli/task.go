package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/YuleBest/ask-ai/internal/conversation"
	"github.com/YuleBest/ask-ai/internal/terminaltext"
)

const taskSummaryLimit = 80

type taskSelectorFunc func([]conversation.Summary, io.Writer) ([]conversation.Summary, error)

func runTask(args []string, stdout, stderr io.Writer) int {
	return runTaskContext(context.Background(), args, stdout, stderr)
}

func runTaskContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printTaskUsage(stderr)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printTaskUsage(stdout)
		return 0
	}

	store, err := conversation.DefaultStore()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}

	switch args[0] {
	case "list", "ls":
		if len(args) != 1 {
			printTaskUsage(stderr)
			return 2
		}
		return listTasks(store, stdout, stderr)
	case "remove", "rm":
		return removeTasksContext(ctx, store, args[1:], stdout, stderr)
	case "switch", "st":
		if len(args) != 2 {
			printTaskUsage(stderr)
			return 2
		}
		return switchTask(store, args[1], stdout, stderr)
	case "view":
		if len(args) > 2 {
			printTaskUsage(stderr)
			return 2
		}
		hash := ""
		if len(args) == 2 {
			hash = args[1]
		}
		return viewTask(store, hash, stdout, stderr)
	default:
		printErrorf(stderr, "未知任务命令: %s", args[0])
		printTaskUsage(stderr)
		return 2
	}
}

func listTasks(store conversation.Store, stdout, stderr io.Writer) int {
	summaries, err := store.List()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	style := newOutputStyle(stdout)
	if len(summaries) == 0 {
		fmt.Fprintln(stdout, style.dim("暂无任务"))
		return 0
	}

	fmt.Fprintf(
		stdout,
		"  %s  %s  %s\n",
		style.dim(fmt.Sprintf("%-32s", "HASH")),
		style.dim("首条消息"),
		style.dim("模型 ID"),
	)
	for _, summary := range summaries {
		marker := " "
		hash := style.cyan(summary.ID)
		message := taskMessage(summary.FirstMessage)
		modelID := summary.Model
		if modelID == "" {
			modelID = "-"
		}
		if summary.Current {
			marker = style.green("*")
			hash = style.boldCyan(summary.ID)
			message = style.bold(message)
		}
		fmt.Fprintf(stdout, "%s %s  %s  %s\n", marker, hash, message, style.dim(modelID))
	}
	return 0
}

func removeTasksContext(ctx context.Context, store conversation.Store, hashes []string, stdout, stderr io.Writer) int {
	return removeTasksWithSelectorContext(ctx, store, hashes, stdout, stderr, func(summaries []conversation.Summary, stdout io.Writer) ([]conversation.Summary, error) {
		return selectTasksInTerminal(ctx, summaries, stdout)
	})
}

func removeTasksWithSelector(store conversation.Store, hashes []string, stdout, stderr io.Writer, selectTasks taskSelectorFunc) int {
	return removeTasksWithSelectorContext(context.Background(), store, hashes, stdout, stderr, selectTasks)
}

func removeTasksWithSelectorContext(ctx context.Context, store conversation.Store, hashes []string, stdout, stderr io.Writer, selectTasks taskSelectorFunc) int {
	if len(hashes) > 0 {
		return removeTaskHashes(store, hashes, stdout, stderr)
	}

	summaries, err := store.List()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	if len(summaries) == 0 {
		fmt.Fprintln(stdout, newOutputStyle(stdout).dim("暂无任务"))
		return 0
	}

	selected, err := selectTasks(summaries, stdout)
	switch {
	case errors.Is(err, errTaskSelectionCanceled):
		fmt.Fprintln(stdout, newOutputStyle(stdout).dim("已取消，未删除任务"))
		return 0
	case errors.Is(err, errTaskSelectionInterrupted):
		fmt.Fprintln(stderr, newOutputStyle(stderr).dim("已取消，未删除任务"))
		return 130
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(stderr, newOutputStyle(stderr).dim("已取消，未删除任务"))
		return 130
	case err != nil:
		printErrorf(stderr, "%v", err)
		return 1
	}

	hashes = make([]string, 0, len(selected))
	for _, summary := range selected {
		hashes = append(hashes, summary.ID)
	}
	return removeTaskHashes(store, hashes, stdout, stderr)
}

func removeTaskHashes(store conversation.Store, hashes []string, stdout, stderr io.Writer) int {
	summaries, err := store.RemoveMany(hashes)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	printRemovedTasks(summaries, stdout)
	return 0
}

func printRemovedTasks(summaries []conversation.Summary, stdout io.Writer) {
	style := newOutputStyle(stdout)
	if len(summaries) == 0 {
		return
	}
	if len(summaries) == 1 {
		fmt.Fprintf(stdout, "%s %s\n", style.green("已删除任务:"), style.cyan(summaries[0].ID))
	} else {
		fmt.Fprintf(stdout, "%s %d 个任务:\n", style.green("已删除"), len(summaries))
		for _, summary := range summaries {
			fmt.Fprintf(stdout, "  %s  %s\n", style.cyan(summary.ID), style.dim(taskMessage(summary.FirstMessage)))
		}
	}
	for _, summary := range summaries {
		if summary.Current {
			fmt.Fprintln(stdout, style.yellow("当前任务已清除"))
			break
		}
	}
}

func switchTask(store conversation.Store, hash string, stdout, stderr io.Writer) int {
	summary, err := store.Switch(hash)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	style := newOutputStyle(stdout)
	fmt.Fprintf(stdout, "%s %s  %s\n", style.green("已切换任务:"), style.cyan(summary.ID), taskMessage(summary.FirstMessage))
	return 0
}

func viewTask(store conversation.Store, hash string, stdout, stderr io.Writer) int {
	var session conversation.Session
	var err error
	if hash == "" {
		session, err = store.LoadLast()
		if errors.Is(err, conversation.ErrNotFound) {
			printErrorf(stderr, "没有可查看的上一次任务")
			return 1
		}
	} else {
		session, err = store.Load(hash)
	}
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	style := newOutputStyle(stdout)
	fmt.Fprintf(stdout, "%s %s\n", style.dim("任务:"), style.boldCyan(session.ID))
	for _, message := range session.Messages {
		label := "[" + message.Role + "]"
		switch message.Role {
		case "user":
			label = style.boldCyan(label)
		case "assistant":
			label = style.boldGreen(label)
		default:
			label = style.bold(label)
		}
		fmt.Fprintf(stdout, "\n%s\n%s\n", label, terminaltext.Clean(message.Content))
	}
	return 0
}

func taskMessage(content string) string {
	return taskMessageWithLimit(content, taskSummaryLimit)
}

func taskMessageWithLimit(content string, limit int) string {
	message := strings.Join(strings.Fields(terminaltext.Clean(content)), " ")
	if message == "" {
		return "（无首条用户消息）"
	}
	if limit < 4 {
		limit = 4
	}
	runes := []rune(message)
	if len(runes) <= limit {
		return message
	}
	return string(runes[:limit-3]) + "..."
}
