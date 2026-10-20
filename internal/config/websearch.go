package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

type WebSearchConfig struct {
	Use       ToolUse             `json:"use"`
	Providers []WebSearchProvider `json:"provider"`
}

type ToolUse string

type WebSearchProvider struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

func LoadOrCreateWebSearch(path string) (WebSearchConfig, bool, error) {
	if err := checkSecretFile(path); err != nil {
		return WebSearchConfig{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		config := defaultWebSearchConfig()
		if err := writeDefaultWebSearch(path, config); err != nil {
			return WebSearchConfig{}, false, err
		}
		return config, true, nil
	}
	if err != nil {
		return WebSearchConfig{}, false, i18n.Errorf("读取网络搜索配置失败: %w", err)
	}

	var config WebSearchConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return WebSearchConfig{}, false, i18n.Errorf("解析网络搜索配置失败: %w", err)
	}
	config.Use = ToolUse(strings.TrimSpace(string(config.Use)))
	for index := range config.Providers {
		config.Providers[index].ID = strings.TrimSpace(config.Providers[index].ID)
		config.Providers[index].Key = strings.TrimSpace(config.Providers[index].Key)
	}
	return config, false, nil
}

func EnsureWebSearchFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return checkSecretFile(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return i18n.Errorf("检查网络搜索配置失败: %w", err)
	}
	return writeDefaultWebSearch(path, defaultWebSearchConfig())
}

func (c WebSearchConfig) ResolveProvider() (WebSearchProvider, error) {
	if c.Use == "" {
		return WebSearchProvider{}, i18n.New("网络搜索未配置 use")
	}
	if c.Use != "exa" {
		return WebSearchProvider{}, i18n.Errorf("不支持的网络搜索 provider: %s", c.Use)
	}
	for _, provider := range c.Providers {
		if provider.ID != string(c.Use) {
			continue
		}
		if provider.Key == "" {
			return WebSearchProvider{}, i18n.Errorf("网络搜索 provider %s 未配置 key", provider.ID)
		}
		return provider, nil
	}
	return WebSearchProvider{}, i18n.Errorf("网络搜索 provider 不存在: %s", c.Use)
}

func (c WebSearchConfig) IsConfigured() bool {
	if c.Use == "" {
		return false
	}
	for _, provider := range c.Providers {
		if provider.ID == string(c.Use) && provider.Key != "" {
			return true
		}
	}
	return false
}

func defaultWebSearchConfig() WebSearchConfig {
	return WebSearchConfig{
		Use: ToolUse("exa"),
		Providers: []WebSearchProvider{
			{ID: "exa", Key: ""},
		},
	}
}

func SaveWebSearch(path string, config WebSearchConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return i18n.Errorf("生成网络搜索配置失败: %w", err)
	}
	data = append(data, '\n')
	return writeReplace(path, data)
}

func writeDefaultWebSearch(path string, config WebSearchConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return i18n.Errorf("生成网络搜索配置失败: %w", err)
	}
	data = append(data, '\n')
	return writeExclusive(path, data)
}

func (u *ToolUse) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("false")) {
		*u = ""
		return nil
	}
	if bytes.Equal(data, []byte("true")) {
		return i18n.New("工具 use 必须是 provider 字符串或 false")
	}
	var provider string
	if err := json.Unmarshal(data, &provider); err != nil {
		return i18n.New("工具 use 必须是 provider 字符串或 false")
	}
	*u = ToolUse(strings.TrimSpace(provider))
	return nil
}

func (u ToolUse) MarshalJSON() ([]byte, error) {
	if u == "" {
		return []byte("false"), nil
	}
	return json.Marshal(string(u))
}
