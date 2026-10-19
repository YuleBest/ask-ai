package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const metaVersion = 1

type MetaEntry struct {
	File        string `json:"file"`
	Description string `json:"description"`
}

type Meta struct {
	Version int                  `json:"version"`
	Configs map[string]MetaEntry `json:"configs"`
	Tools   map[string]MetaTool  `json:"tools"`
}

type MetaTool struct {
	Description string               `json:"description"`
	Config      string               `json:"config"`
	Operations  []string             `json:"operations"`
	Definitions []MetaToolDefinition `json:"definitions,omitempty"`
}

type MetaToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func MetaPath(modelPath string) string {
	return filepath.Join(filepath.Dir(modelPath), "_meta.json")
}

func LoadOrCreateMeta(path string) (Meta, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		meta := defaultMeta()
		if err := writeDefaultMeta(path, meta); err != nil {
			return Meta{}, false, err
		}
		return meta, true, nil
	}
	if err != nil {
		return Meta{}, false, fmt.Errorf("读取配置元数据失败: %w", err)
	}

	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, false, fmt.Errorf("解析配置元数据失败: %w", err)
	}
	if meta.Version != metaVersion {
		return Meta{}, false, fmt.Errorf("不支持的配置元数据版本: %d", meta.Version)
	}
	if len(meta.Configs) == 0 {
		return Meta{}, false, errors.New("配置元数据没有配置项")
	}
	defaults := defaultMeta()
	changed := false
	if _, exists := meta.Configs["webSearch"]; !exists {
		meta.Configs["webSearch"] = defaults.Configs["webSearch"]
		changed = true
	}
	if meta.Tools == nil {
		meta.Tools = make(map[string]MetaTool)
	}
	for id, tool := range defaults.Tools {
		existing, exists := meta.Tools[id]
		if !exists {
			meta.Tools[id] = tool
			changed = true
			continue
		}
		if len(existing.Definitions) == 0 {
			existing.Definitions = tool.Definitions
			meta.Tools[id] = existing
			changed = true
		}
	}
	if changed {
		data, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return Meta{}, false, fmt.Errorf("生成配置元数据失败: %w", err)
		}
		data = append(data, '\n')
		if err := writeReplace(path, data); err != nil {
			return Meta{}, false, fmt.Errorf("升级配置元数据失败: %w", err)
		}
	}
	return meta, false, nil
}

func (m Meta) PathFor(key, configDir string) (string, error) {
	entry, ok := m.Configs[key]
	if !ok {
		return "", fmt.Errorf("未知配置键: %s", key)
	}
	clean := filepath.Clean(entry.File)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("配置键 %s 的文件路径无效", key)
	}
	return filepath.Join(configDir, clean), nil
}

func defaultMeta() Meta {
	return Meta{
		Version: metaVersion,
		Configs: map[string]MetaEntry{
			"meta": {
				File:        "_meta.json",
				Description: "配置键与配置文件的映射",
			},
			"models": {
				File:        "models.json",
				Description: "模型提供商与模型配置",
			},
			"systemPrompt": {
				File:        "system_prompt.txt",
				Description: "系统提示词",
			},
			"webSearch": {
				File:        "web_search.json",
				Description: "网络搜索提供商配置",
			},
		},
		Tools: map[string]MetaTool{
			"webSearch": {
				Description: "让模型搜索公开网络，并在需要时抓取网页正文",
				Config:      "webSearch",
				Operations:  []string{"off", "use"},
				Definitions: defaultWebToolDefinitions(),
			},
		},
	}
}

func DefaultMeta() Meta {
	return defaultMeta()
}

func defaultWebToolDefinitions() []MetaToolDefinition {
	return []MetaToolDefinition{
		{
			Name:        "web_search",
			Description: "Search the public web with Exa. Use this proactively for current events, latest versions, prices, releases, and other time-sensitive facts.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "Search query"},
					"num_results": {"type": "integer", "minimum": 1, "maximum": 10, "description": "Number of results, default 5"}
				},
				"required": ["query"],
				"additionalProperties": false
			}`),
		},
		{
			Name:        "web_fetch",
			Description: "Fetch readable page text from a public URL with Exa. Use this when search results are insufficient and the primary source needs to be inspected.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {"type": "string", "description": "HTTP or HTTPS URL to fetch"}
				},
				"required": ["url"],
				"additionalProperties": false
			}`),
		},
	}
}

func writeDefaultMeta(path string, meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("生成配置元数据失败: %w", err)
	}
	data = append(data, '\n')
	return writeExclusive(path, data)
}
