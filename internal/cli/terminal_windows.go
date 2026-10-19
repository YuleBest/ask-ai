//go:build windows

package cli

import (
	"fmt"
	"os"
)

func platformTerminalID(terminal *os.File) (string, error) {
	return terminalKey(fmt.Sprintf("windows:%s", terminal.Name()), os.Getppid()), nil
}
