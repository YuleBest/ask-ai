package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

type LanguageConfig struct {
	Output i18n.Language `json:"output,omitempty"`
	Prompt i18n.Language `json:"prompt,omitempty"`
}

func LanguagePath(modelsPath string) string {
	return filepath.Join(filepath.Dir(modelsPath), "language.json")
}

func LoadOrCreateLanguage(path string) (LanguageConfig, bool, error) {
	language, exists, err := LoadLanguage(path)
	if err != nil || exists {
		return language, false, err
	}
	if err := writeDefaultLanguage(path); err != nil {
		if errors.Is(err, os.ErrExist) {
			language, exists, readErr := LoadLanguage(path)
			if readErr == nil && exists {
				return language, false, nil
			}
			if readErr != nil {
				return LanguageConfig{}, false, readErr
			}
		}
		return LanguageConfig{}, false, err
	}
	return LanguageConfig{}, true, nil
}

func LoadLanguage(path string) (LanguageConfig, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return LanguageConfig{}, false, nil
	}
	if err != nil {
		return LanguageConfig{}, false, i18n.Errorf("读取语言配置失败: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}")
	}

	var raw struct {
		Output string `json:"output"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return LanguageConfig{}, false, i18n.Errorf("解析语言配置失败: %w", err)
	}
	language, err := parseLanguageOverrides(raw.Output, raw.Prompt)
	if err != nil {
		return LanguageConfig{}, false, err
	}
	return language, true, nil
}

func parseLanguageOverrides(output, prompt string) (LanguageConfig, error) {
	var language LanguageConfig
	if output != "" {
		parsed, err := i18n.ParseLanguage(output)
		if err != nil {
			return LanguageConfig{}, i18n.Errorf("language.output: %w", err)
		}
		language.Output = parsed
	}
	if prompt != "" {
		parsed, err := i18n.ParseLanguage(prompt)
		if err != nil {
			return LanguageConfig{}, i18n.Errorf("language.prompt: %w", err)
		}
		language.Prompt = parsed
	}
	return language, nil
}

func (c LanguageConfig) Resolve(getenv func(string) string) i18n.Settings {
	return i18n.Resolve(getenv, c.Output, c.Prompt)
}

func EnsureLanguageFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return i18n.Errorf("检查语言配置失败: %w", err)
	}
	if err := writeDefaultLanguage(path); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

func writeDefaultLanguage(path string) error {
	return writeExclusive(path, []byte("{}\n"))
}
