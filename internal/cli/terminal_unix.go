//go:build !windows

package cli

import (
	"fmt"
	"os"
	"strconv"
	"syscall"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

func platformTerminalID(terminal *os.File) (string, error) {
	terminalPath, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", terminal.Fd()))
	if err != nil {
		info, statErr := terminal.Stat()
		if statErr != nil {
			return "", i18n.Errorf("无法确定当前终端: %w", err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return "", i18n.Errorf("无法确定当前终端: %w", err)
		}
		terminalPath = "device:" + strconv.FormatUint(uint64(stat.Rdev), 10)
	}
	sessionID, _, errno := syscall.Syscall(syscall.SYS_GETSID, 0, 0, 0)
	if errno != 0 {
		return "", i18n.Errorf("无法确定终端会话: %w", errno)
	}
	return terminalKey(terminalPath, int(sessionID)), nil
}
