package render

import (
	"io"
	"strings"
	"unicode"

	"github.com/YuleBest/ask-ai/internal/terminaltext"
)

const ansiReset = "\x1b[0m"

type Markdown struct {
	writer  io.Writer
	enabled bool

	lineStart bool
	lineChars []rune

	inFence     bool
	fenceChar   rune
	fenceLength int
	fenceStart  bool
	codeLine    []rune

	pendingChar   rune
	pendingCount  int
	escapePending bool

	heading       bool
	bold          bool
	italic        bool
	inlineCode    bool
	strikethrough bool

	appliedStyle string
	sanitizer    *terminaltext.Stream
	closed       bool
}

func NewMarkdown(writer io.Writer, enabled bool) *Markdown {
	return &Markdown{
		writer:    writer,
		enabled:   enabled,
		lineStart: true,
		sanitizer: terminaltext.NewStream(),
	}
}

func (r *Markdown) Write(data []byte) (int, error) {
	text := r.sanitizer.WriteString(string(data))
	if !r.enabled {
		if _, err := io.WriteString(r.writer, text); err != nil {
			return 0, err
		}
		return len(data), nil
	}

	for offset, char := range text {
		if err := r.writeRune(char); err != nil {
			return offset, err
		}
	}
	return len(data), nil
}

func (r *Markdown) writeRune(char rune) error {
	if r.inFence {
		return r.writeFenceRune(char)
	}
	if char == '\n' {
		return r.endLine()
	}
	if r.lineStart {
		r.lineChars = append(r.lineChars, char)
		_, err := r.resolveLinePrefix()
		return err
	}
	return r.writeInlineRune(char)
}

func (r *Markdown) resolveLinePrefix() (bool, error) {
	prefix := r.lineChars
	if len(prefix) == 0 {
		return false, nil
	}

	first := prefix[0]
	if first == '`' || first == '~' {
		if allRunes(prefix, first) {
			if len(prefix) < 3 {
				return false, nil
			}
			r.inFence = true
			r.fenceChar = first
			r.fenceLength = len(prefix)
			r.fenceStart = true
			r.codeLine = append([]rune(nil), prefix...)
			r.lineStart = false
			r.lineChars = nil
			return true, nil
		}
		return r.releaseLinePrefix()
	}

	if first == '#' {
		hashes := leadingRuneCount(prefix, '#')
		if hashes == len(prefix) && hashes < 6 {
			return false, nil
		}
		if hashes >= 1 && hashes <= 6 && len(prefix) == hashes+1 && prefix[hashes] == ' ' {
			r.heading = true
			r.lineStart = false
			r.lineChars = nil
			return true, r.writeText(string(prefix))
		}
		return r.releaseLinePrefix()
	}

	if first == '>' {
		if len(prefix) == 1 {
			return false, nil
		}
		r.lineStart = false
		r.lineChars = nil
		if err := r.writeText("│ "); err != nil {
			return false, err
		}
		if prefix[1] != ' ' {
			if err := r.writeInlineRune(prefix[1]); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	if first == '-' || first == '*' || first == '+' {
		if len(prefix) == 1 {
			return false, nil
		}
		if prefix[1] == ' ' {
			r.lineStart = false
			r.lineChars = nil
			return true, r.writeText("• ")
		}
		return r.releaseLinePrefix()
	}

	return r.releaseLinePrefix()
}

func (r *Markdown) releaseLinePrefix() (bool, error) {
	prefix := r.lineChars
	r.lineChars = nil
	r.lineStart = false
	for _, char := range prefix {
		if err := r.writeInlineRune(char); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *Markdown) writeInlineRune(char rune) error {
	if r.inlineCode {
		if r.pendingCount > 0 {
			if err := r.flushPendingLiteral(); err != nil {
				return err
			}
		}
		if char == '`' {
			r.inlineCode = false
			return r.syncStyle()
		}
		return r.writeText(string(char))
	}

	if r.escapePending {
		r.escapePending = false
		if isEscapable(char) {
			return r.writeText(string(char))
		}
		return r.writeText("\\" + string(char))
	}

	if char == '\\' {
		if r.pendingCount > 0 {
			if err := r.flushPending(true); err != nil {
				return err
			}
		}
		if r.inlineCode {
			return r.writeText("\\")
		}
		r.escapePending = true
		return nil
	}

	if char == '*' || char == '`' || char == '~' {
		if r.pendingCount > 0 && r.pendingChar != char {
			if err := r.flushPending(true); err != nil {
				return err
			}
		}
		if r.pendingCount == 0 {
			r.pendingChar = char
		}
		r.pendingCount++
		return nil
	}

	if unicode.IsSpace(char) && !r.pendingClosesStyle(r.pendingChar, r.pendingCount) {
		if err := r.flushPendingLiteral(); err != nil {
			return err
		}
	} else if err := r.flushPending(true); err != nil {
		return err
	}
	return r.writeText(string(char))
}

func (r *Markdown) flushPendingLiteral() error {
	if r.pendingCount == 0 {
		return nil
	}
	text := strings.Repeat(string(r.pendingChar), r.pendingCount)
	r.pendingChar = 0
	r.pendingCount = 0
	return r.writeText(text)
}

func (r *Markdown) flushPending(apply bool) error {
	if r.pendingCount == 0 {
		return nil
	}

	char := r.pendingChar
	count := r.pendingCount
	r.pendingChar = 0
	r.pendingCount = 0

	if !apply {
		if !r.pendingClosesStyle(char, count) {
			return r.writeText(strings.Repeat(string(char), count))
		}
	}

	switch char {
	case '*':
		switch count {
		case 1:
			r.italic = !r.italic
		case 2:
			r.bold = !r.bold
		case 3:
			r.bold = !r.bold
			r.italic = !r.italic
		default:
			return r.writeText(strings.Repeat("*", count))
		}
	case '~':
		if count != 2 {
			return r.writeText(strings.Repeat("~", count))
		}
		r.strikethrough = !r.strikethrough
	case '`':
		if count != 1 {
			return r.writeText(strings.Repeat("`", count))
		}
		r.inlineCode = !r.inlineCode
	default:
		return r.writeText(strings.Repeat(string(char), count))
	}
	return r.syncStyle()
}

func (r *Markdown) pendingClosesStyle(char rune, count int) bool {
	switch char {
	case '*':
		switch count {
		case 1:
			return r.italic
		case 2:
			return r.bold
		case 3:
			return r.bold || r.italic
		}
	case '~':
		return count == 2 && r.strikethrough
	case '`':
		return count == 1 && r.inlineCode
	}
	return false
}

func (r *Markdown) endLine() error {
	if r.lineStart && len(r.lineChars) > 0 {
		if _, err := r.releaseLinePrefix(); err != nil {
			return err
		}
	}
	if err := r.flushPending(false); err != nil {
		return err
	}
	if r.escapePending {
		r.escapePending = false
		if err := r.writeText("\\"); err != nil {
			return err
		}
	}

	r.heading = false
	r.bold = false
	r.italic = false
	r.inlineCode = false
	r.strikethrough = false
	if err := r.syncStyle(); err != nil {
		return err
	}
	if _, err := io.WriteString(r.writer, "\n"); err != nil {
		return err
	}
	r.lineStart = true
	return nil
}

func (r *Markdown) writeFenceRune(char rune) error {
	if r.fenceStart {
		if char != '\n' {
			r.codeLine = append(r.codeLine, char)
			if char == r.fenceChar {
				r.fenceLength++
			}
			return nil
		}
		line := string(r.codeLine)
		r.codeLine = nil
		r.fenceStart = false
		if err := r.writeFenceLine(line, true); err != nil {
			return err
		}
		return nil
	}
	if char == '\n' {
		line := string(r.codeLine)
		r.codeLine = nil
		if r.isClosingFence(line) {
			if err := r.writeFenceLine(line, true); err != nil {
				return err
			}
			r.inFence = false
			r.fenceLength = 0
			r.lineStart = true
			return nil
		}
		return r.writeCodeLine(line, true)
	}
	r.codeLine = append(r.codeLine, char)
	return nil
}

func (r *Markdown) writeFenceLine(line string, newline bool) error {
	if line != "" {
		if _, err := io.WriteString(r.writer, ansiReset+"\x1b[36m"+line+ansiReset); err != nil {
			return err
		}
	}
	if newline {
		if _, err := io.WriteString(r.writer, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func (r *Markdown) isClosingFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len([]rune(trimmed)) < 3 {
		return false
	}
	if len([]rune(trimmed)) < r.fenceLength {
		return false
	}
	for _, char := range trimmed {
		if char != r.fenceChar {
			return false
		}
	}
	return true
}

func (r *Markdown) writeCodeLine(line string, newline bool) error {
	if line != "" {
		if _, err := io.WriteString(r.writer, ansiReset+"\x1b[36m  "+line+ansiReset); err != nil {
			return err
		}
	}
	if newline {
		if _, err := io.WriteString(r.writer, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func (r *Markdown) writeText(text string) error {
	if text == "" {
		return nil
	}
	if err := r.syncStyle(); err != nil {
		return err
	}
	_, err := io.WriteString(r.writer, text)
	return err
}

func (r *Markdown) syncStyle() error {
	var style strings.Builder
	if r.heading || r.bold {
		style.WriteString("\x1b[1m")
	}
	if r.italic {
		style.WriteString("\x1b[3m")
	}
	if r.inlineCode {
		style.WriteString("\x1b[36m")
	}
	if r.strikethrough {
		style.WriteString("\x1b[9m")
	}

	next := style.String()
	if next != "" {
		next = ansiReset + next
	}
	if next == r.appliedStyle {
		return nil
	}
	if next == "" {
		next = ansiReset
	}
	if _, err := io.WriteString(r.writer, next); err != nil {
		return err
	}
	if style.Len() == 0 {
		r.appliedStyle = ""
	} else {
		r.appliedStyle = ansiReset + style.String()
	}
	return nil
}

func (r *Markdown) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	r.sanitizer.Close()

	if !r.enabled {
		return nil
	}
	if r.inFence {
		line := string(r.codeLine)
		r.codeLine = nil
		if r.fenceStart {
			if err := r.writeFenceLine(line, false); err != nil {
				return err
			}
		} else if r.isClosingFence(line) {
			if err := r.writeFenceLine(line, false); err != nil {
				return err
			}
		} else {
			if err := r.writeCodeLine(line, false); err != nil {
				return err
			}
		}
		r.inFence = false
		r.fenceLength = 0
	} else {
		if r.lineStart && len(r.lineChars) > 0 {
			if _, err := r.releaseLinePrefix(); err != nil {
				return err
			}
		}
		if err := r.flushPending(false); err != nil {
			return err
		}
		if r.escapePending {
			r.escapePending = false
			if err := r.writeText("\\"); err != nil {
				return err
			}
		}
	}

	if r.appliedStyle != "" {
		if _, err := io.WriteString(r.writer, ansiReset); err != nil {
			return err
		}
		r.appliedStyle = ""
	}
	return nil
}

func allRunes(chars []rune, target rune) bool {
	for _, char := range chars {
		if char != target {
			return false
		}
	}
	return len(chars) > 0
}

func leadingRuneCount(chars []rune, target rune) int {
	count := 0
	for _, char := range chars {
		if char != target {
			break
		}
		count++
	}
	return count
}

func isEscapable(char rune) bool {
	return strings.ContainsRune(`!"#$%&'()*+,-./:;<=>?@[\]^_`+"`"+`{|}~`, char)
}
