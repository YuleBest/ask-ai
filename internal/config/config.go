package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/YuleBest/ask-ai/internal/fileutil"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", i18n.Errorf("无法确定用户配置目录: %w", err)
	}
	return filepath.Join(dir, "ask-ai", "models.json"), nil
}

func RenderSystemPrompt(prompt string, now time.Time) string {
	if prompt == "" {
		return ""
	}
	timezone := systemTimezoneName(now)
	return strings.NewReplacer(
		"{{os}}", runtime.GOOS,
		"{{system}}", runtime.GOOS,
		"{{arch}}", runtime.GOARCH,
		"{{architecture}}", runtime.GOARCH,
		"{{date}}", now.Format("2006-01-02"),
		"{{time}}", now.Format("15:04:05"),
		"{{timezone}}", timezone,
		"{{datetime}}", now.Format("2006-01-02 15:04:05"),
	).Replace(prompt)
}

func systemTimezoneName(now time.Time) string {
	name := strings.TrimSpace(os.Getenv("TZ"))
	if name == "" {
		location := now.Location().String()
		if location != "" && location != "Local" {
			name = location
		}
	}
	if name == "" {
		if data, err := os.ReadFile("/etc/timezone"); err == nil {
			name = strings.TrimSpace(string(data))
		}
	}
	if name == "" {
		if resolved, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
			const zoneInfoMarker = "/zoneinfo/"
			if index := strings.Index(resolved, zoneInfoMarker); index >= 0 {
				name = resolved[index+len(zoneInfoMarker):]
			}
		}
	}
	if name == "" {
		name = now.Format("MST")
	}
	return fmt.Sprintf("%s (UTC%s)", name, now.Format("-07:00"))
}

func writeExclusive(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return i18n.Errorf("创建配置目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, ".config-create-*.tmp")
	if err != nil {
		return i18n.Errorf("创建临时文件失败: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return i18n.Errorf("设置文件权限失败: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return i18n.Errorf("写入临时文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return i18n.Errorf("同步临时文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return i18n.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		return i18n.Errorf("创建文件失败: %w", err)
	}
	if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return i18n.Errorf("删除临时文件失败: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return i18n.Errorf("同步配置目录失败: %w", err)
	}
	return nil
}

func writeReplace(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return i18n.Errorf("创建配置目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return i18n.Errorf("创建临时文件失败: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return i18n.Errorf("设置文件权限失败: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return i18n.Errorf("写入临时文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return i18n.Errorf("同步临时文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return i18n.Errorf("关闭临时文件失败: %w", err)
	}
	if err := fileutil.Replace(tempPath, path); err != nil {
		return i18n.Errorf("替换配置文件失败: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return i18n.Errorf("同步配置目录失败: %w", err)
	}
	return nil
}

func syncDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	err = file.Sync()
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	return err
}
