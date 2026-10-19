package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

func checkSecretFile(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查配置文件失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("配置文件不是普通文件: %s", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("配置文件权限过宽: %s，请执行 chmod 600", path)
	}
	return nil
}
