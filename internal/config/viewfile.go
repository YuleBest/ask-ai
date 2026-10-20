package config

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

const (
	DefaultViewFileMaxBytes int64 = 512000
	MaxViewFileMaxBytes     int64 = 2 * 1024 * 1024
)

type ViewFileMode string

const (
	ViewFileModeAsk    ViewFileMode = "ask"
	ViewFileModeBypass ViewFileMode = "bypass"
	ViewFileModeOff    ViewFileMode = "off"
)

type ViewFileConfig struct {
	Mode     ViewFileMode `json:"mode"`
	MaxBytes int64        `json:"max_bytes"`
}

func DefaultViewFileConfig() ViewFileConfig {
	return ViewFileConfig{
		Mode:     ViewFileModeAsk,
		MaxBytes: DefaultViewFileMaxBytes,
	}
}

func (c ViewFileConfig) Validate() error {
	switch c.Mode {
	case ViewFileModeAsk, ViewFileModeBypass, ViewFileModeOff:
	default:
		return i18n.Errorf("文件查看模式无效: %s", c.Mode)
	}
	if c.MaxBytes < 1 || c.MaxBytes > MaxViewFileMaxBytes {
		return i18n.Errorf(
			"文件查看上限必须在 1 到 %d 字节之间: %d",
			MaxViewFileMaxBytes,
			c.MaxBytes,
		)
	}
	return nil
}

func LoadOrCreateViewFile(path string) (ViewFileConfig, bool, error) {
	if err := checkSecretFile(path); err != nil {
		return ViewFileConfig{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		config := DefaultViewFileConfig()
		if err := writeDefaultViewFile(path, config); err != nil {
			return ViewFileConfig{}, false, err
		}
		return config, true, nil
	}
	if err != nil {
		return ViewFileConfig{}, false, i18n.Errorf("读取文件查看配置失败: %w", err)
	}

	var config ViewFileConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ViewFileConfig{}, false, i18n.Errorf("解析文件查看配置失败: %w", err)
	}
	if err := config.Validate(); err != nil {
		return ViewFileConfig{}, false, err
	}
	return config, false, nil
}

func EnsureViewFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return checkSecretFile(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return i18n.Errorf("检查文件查看配置失败: %w", err)
	}
	return writeDefaultViewFile(path, DefaultViewFileConfig())
}

func SaveViewFile(path string, config ViewFileConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return i18n.Errorf("生成文件查看配置失败: %w", err)
	}
	data = append(data, '\n')
	return writeReplace(path, data)
}

func writeDefaultViewFile(path string, config ViewFileConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return i18n.Errorf("生成文件查看配置失败: %w", err)
	}
	data = append(data, '\n')
	return writeExclusive(path, data)
}
