package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/YuleBest/ask-ai/internal/terminaltext"
)

const (
	ansiReset     = "\x1b[0m"
	ansiBold      = "\x1b[1m"
	ansiDim       = "\x1b[2m"
	ansiRed       = "\x1b[31m"
	ansiGreen     = "\x1b[32m"
	ansiYellow    = "\x1b[33m"
	ansiCyan      = "\x1b[36m"
	ansiGray      = "\x1b[90m"
	ansiBoldCyan  = "\x1b[1;36m"
	ansiBoldGreen = "\x1b[1;32m"
)

type outputStyle struct {
	enabled bool
}

func newOutputStyle(w io.Writer) outputStyle {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return outputStyle{}
	}
	if os.Getenv("TERM") == "dumb" {
		return outputStyle{}
	}
	return outputStyle{enabled: isTerminal(w)}
}

func (s outputStyle) paint(code, text string) string {
	text = terminaltext.Clean(text)
	if !s.enabled || text == "" {
		return text
	}
	return code + text + ansiReset
}

func (s outputStyle) bold(text string) string {
	return s.paint(ansiBold, text)
}

func (s outputStyle) dim(text string) string {
	return s.paint(ansiDim, text)
}

func (s outputStyle) red(text string) string {
	return s.paint(ansiRed, text)
}

func (s outputStyle) green(text string) string {
	return s.paint(ansiGreen, text)
}

func (s outputStyle) yellow(text string) string {
	return s.paint(ansiYellow, text)
}

func (s outputStyle) cyan(text string) string {
	return s.paint(ansiCyan, text)
}

func (s outputStyle) gray(text string) string {
	return s.paint(ansiGray, text)
}

func (s outputStyle) boldCyan(text string) string {
	return s.paint(ansiBoldCyan, text)
}

func (s outputStyle) boldGreen(text string) string {
	return s.paint(ansiBoldGreen, text)
}

func printErrorf(w io.Writer, format string, args ...any) {
	style := newOutputStyle(w)
	fmt.Fprintf(w, "%s %s\n", style.red("askai:"), fmt.Sprintf(format, args...))
}
