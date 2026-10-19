package terminaltext

import "testing"

func TestCleanRemovesTerminalControlSequences(t *testing.T) {
	input := "前\x1b[2J中\x1b]52;c;YWJj\x07后\r\n\t结束\x00"
	want := "前中后\n    结束"
	if got := Clean(input); got != want {
		t.Fatalf("Clean() = %q, want %q", got, want)
	}
}

func TestStreamHandlesSplitEscapeSequences(t *testing.T) {
	stream := NewStream()
	var output string
	for _, part := range []string{"前\x1b[", "2J中\x1b]52;c;", "YWJj\x1b", "\\后\n"} {
		output += stream.WriteString(part)
	}
	output += stream.Close()
	if output != "前中后\n" {
		t.Fatalf("stream output = %q, want %q", output, "前中后\n")
	}
}

func TestStreamDropsUnfinishedEscapeSequence(t *testing.T) {
	stream := NewStream()
	if got := stream.WriteString("safe\x1b[31"); got != "safe" {
		t.Fatalf("WriteString() = %q, want safe prefix", got)
	}
	if got := stream.Close(); got != "" {
		t.Fatalf("Close() = %q, want unfinished escape discarded", got)
	}
}
