package terminaltext

import (
	"strings"
	"unicode"
)

type state uint8

const (
	stateText state = iota
	stateEscape
	stateCSI
	stateOSC
	stateOSCEscape
	stateStringControl
	stateStringControlEscape
	stateEscapeIntermediate
)

type Stream struct {
	state state
}

func NewStream() *Stream {
	return &Stream{}
}

func Clean(text string) string {
	stream := NewStream()
	return stream.WriteString(text) + stream.Close()
}

func (s *Stream) WriteString(text string) string {
	var output strings.Builder
	for _, char := range text {
		switch s.state {
		case stateText:
			switch {
			case char == '\x1b':
				s.state = stateEscape
			case char == '\u009b':
				s.state = stateCSI
			case char == '\u009d':
				s.state = stateOSC
			case char == '\u0090' || char == '\u0098' || char == '\u009e' || char == '\u009f':
				s.state = stateStringControl
			case char == '\n':
				output.WriteRune(char)
			case char == '\t':
				output.WriteString("    ")
			case unicode.IsControl(char):
				continue
			default:
				output.WriteRune(char)
			}
		case stateEscape:
			switch char {
			case '[':
				s.state = stateCSI
			case ']':
				s.state = stateOSC
			case 'P', 'X', '^', '_':
				s.state = stateStringControl
			case '(', ')', '*', '+', '-', '.', '/':
				s.state = stateEscapeIntermediate
			default:
				s.state = stateText
			}
		case stateCSI:
			if char >= 0x40 && char <= 0x7e {
				s.state = stateText
			}
		case stateOSC:
			switch char {
			case '\a':
				s.state = stateText
			case '\x1b':
				s.state = stateOSCEscape
			}
		case stateOSCEscape:
			if char == '\\' {
				s.state = stateText
			} else if char != '\x1b' {
				s.state = stateOSC
			}
		case stateStringControl:
			switch char {
			case '\a':
				s.state = stateText
			case '\x1b':
				s.state = stateStringControlEscape
			}
		case stateStringControlEscape:
			if char == '\\' {
				s.state = stateText
			} else if char != '\x1b' {
				s.state = stateStringControl
			}
		case stateEscapeIntermediate:
			if char < 0x20 || char > 0x2f {
				s.state = stateText
			}
		}
	}
	return output.String()
}

func (s *Stream) Close() string {
	s.state = stateText
	return ""
}
