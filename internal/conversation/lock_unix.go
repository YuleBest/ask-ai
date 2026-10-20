//go:build !windows

package conversation

import (
	"os"
	"path/filepath"

	"github.com/YuleBest/ask-ai/internal/i18n"
	"golang.org/x/sys/unix"
)

func withStoreLock(root string, fn func() error) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return i18n.Errorf("创建缓存目录失败: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return i18n.Errorf("打开缓存锁失败: %w", err)
	}
	defer file.Close()

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return i18n.Errorf("获取缓存锁失败: %w", err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return fn()
}
