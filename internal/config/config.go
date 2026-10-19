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
)

const assistantInstructions = `你是一个命令行里的“随手问”助手，目标是快速、准确地给出用户真正需要的信息。

- 先直接回答，不寒暄，不复述问题。
- 默认简洁。简单问题给结论，复杂问题再按需分点解释。
- 优先给出可执行方案；需要代码时使用带语言标记的代码块，只解释必要部分。
- 如果用户前提有误，先指出错误，再给出正确结论。
- 信息不足但仍可回答时，明确关键假设；无法可靠回答时，说明缺什么，不要猜测。
- 不确定的事实、命令和结果要明确说明，不要编造。
- 使用用户提问的语言回答。
- Markdown 只用于提升可读性，避免不必要的标题、列表和总结。`

const defaultSystemPrompt = `当前运行环境：
- 系统：{{os}}
- 架构：{{arch}}
- 日期：{{date}}
- 时间：{{time}}
- 时区：{{timezone}}

` + assistantInstructions

const legacyDefaultSystemPrompt = assistantInstructions

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户配置目录: %w", err)
	}
	return filepath.Join(dir, "ask-ai", "models.json"), nil
}

func SystemPromptPath(modelsPath string) string {
	return filepath.Join(filepath.Dir(modelsPath), "system_prompt.txt")
}

func LoadOrCreateSystemPrompt(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := writeDefaultSystemPrompt(path); err != nil {
			return "", false, err
		}
		return defaultSystemPrompt, true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("读取系统提示词失败: %w", err)
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == legacyDefaultSystemPrompt {
		if err := writeReplace(path, []byte(defaultSystemPrompt+"\n")); err != nil {
			return "", false, fmt.Errorf("升级默认系统提示词失败: %w", err)
		}
		return defaultSystemPrompt, false, nil
	}
	return prompt, false, nil
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

func EnsureSystemPromptFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查系统提示词失败: %w", err)
	}
	return writeDefaultSystemPrompt(path)
}

func writeDefaultSystemPrompt(path string) error {
	return writeExclusive(path, []byte(defaultSystemPrompt+"\n"))
}

func writeExclusive(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("写入文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("同步文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("关闭文件失败: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("同步配置目录失败: %w", err)
	}
	return nil
}

func writeReplace(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("设置文件权限失败: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := fileutil.Replace(tempPath, path); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("同步配置目录失败: %w", err)
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
