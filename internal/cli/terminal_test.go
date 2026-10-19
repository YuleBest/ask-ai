package cli

import (
	"os"
	"testing"
)

func TestTerminalKey(t *testing.T) {
	first := terminalKey("/dev/pts/3", 100)
	if again := terminalKey("/dev/pts/3", 100); again != first {
		t.Fatalf("terminalKey() is not deterministic: %q != %q", again, first)
	}
	if otherTTY := terminalKey("/dev/pts/4", 100); otherTTY == first {
		t.Fatal("different TTYs produced the same terminal key")
	}
	if otherSession := terminalKey("/dev/pts/3", 101); otherSession == first {
		t.Fatal("different POSIX sessions produced the same terminal key")
	}
	if len(first) != 32 {
		t.Fatalf("terminal key length = %d, want 32", len(first))
	}
}

func TestIsTerminalRejectsDevNull(t *testing.T) {
	file, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatalf("isTerminal(%s) = true, want false", os.DevNull)
	}
}
