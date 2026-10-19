//go:build windows

package conversation

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func withStoreLock(root string, fn func() error) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("打开缓存锁失败: %w", err)
	}
	defer file.Close()

	var overlapped windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("获取缓存锁失败: %w", err)
	}
	defer windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	return fn()
}
