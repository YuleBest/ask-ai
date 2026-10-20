package config

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

const defaultReasoningEffort = "high"

type Models struct {
	Providers []Provider `json:"provider"`
}

type Provider struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	URL    string  `json:"url"`
	Key    string  `json:"key"`
	Models []Model `json:"models"`
}

type Model struct {
	ID              string `json:"id"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Default         bool   `json:"default,omitempty"`
}

func LoadOrCreateModels(path string) (Models, bool, error) {
	if err := checkSecretFile(path); err != nil {
		return Models{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		models := defaultModels()
		if err := writeDefaultModels(path, models); err != nil {
			return Models{}, false, err
		}
		return models, true, nil
	}
	if err != nil {
		return Models{}, false, i18n.Errorf("读取模型配置失败: %w", err)
	}

	var models Models
	if err := json.Unmarshal(data, &models); err != nil {
		return Models{}, false, i18n.Errorf("解析模型配置失败: %w", err)
	}
	for providerIndex := range models.Providers {
		provider := &models.Providers[providerIndex]
		provider.ID = strings.TrimSpace(provider.ID)
		provider.Name = strings.TrimSpace(provider.Name)
		provider.URL = strings.TrimSpace(provider.URL)
		provider.Key = strings.TrimSpace(provider.Key)
		for modelIndex := range provider.Models {
			provider.Models[modelIndex].ID = strings.TrimSpace(provider.Models[modelIndex].ID)
			provider.Models[modelIndex].ReasoningEffort = strings.TrimSpace(provider.Models[modelIndex].ReasoningEffort)
		}
	}
	return models, false, nil
}

func ValidateModels(models Models) error {
	_, _, err := models.ResolveDefault()
	return err
}

func (m Models) Resolve(providerID, modelID string) (Provider, Model, error) {
	if err := m.validateStructure(); err != nil {
		return Provider{}, Model{}, err
	}
	for _, provider := range m.Providers {
		if provider.ID != providerID {
			continue
		}
		for _, model := range provider.Models {
			if model.ID == modelID {
				if err := validateActiveProvider(provider); err != nil {
					return Provider{}, Model{}, err
				}
				return provider, model, nil
			}
		}
		return Provider{}, Model{}, i18n.Errorf("provider %s 中不存在模型: %s", provider.ID, modelID)
	}
	return Provider{}, Model{}, i18n.Errorf("provider 不存在: %s", providerID)
}

func (m Models) ResolveDefault() (Provider, Model, error) {
	if err := m.validateStructure(); err != nil {
		return Provider{}, Model{}, err
	}

	var selected *Model
	selectedProvider := ""
	for _, provider := range m.Providers {
		for _, model := range provider.Models {
			if !model.Default {
				continue
			}
			if selected != nil {
				return Provider{}, Model{}, i18n.New("只能有一个模型设置 default: true")
			}
			modelCopy := model
			selected = &modelCopy
			selectedProvider = provider.ID
		}
	}
	if selected == nil {
		selectedProvider = m.Providers[0].ID
		first := m.Providers[0].Models[0]
		selected = &first
	}
	return m.Resolve(selectedProvider, selected.ID)
}

func (m *Models) SetDefault(providerID, modelID string) error {
	if _, _, err := m.Resolve(providerID, modelID); err != nil {
		return err
	}
	for providerIndex := range m.Providers {
		for modelIndex := range m.Providers[providerIndex].Models {
			model := &m.Providers[providerIndex].Models[modelIndex]
			model.Default = m.Providers[providerIndex].ID == providerID && model.ID == modelID
		}
	}
	return nil
}

func (m Models) validateStructure() error {
	if len(m.Providers) == 0 {
		return i18n.New("provider 不能为空")
	}

	providerIDs := make(map[string]bool, len(m.Providers))
	for _, provider := range m.Providers {
		if provider.ID == "" {
			return i18n.New("provider.id 不能为空")
		}
		if strings.Contains(provider.ID, "/") {
			return i18n.Errorf("provider.id 不能包含斜杠: %s", provider.ID)
		}
		if providerIDs[provider.ID] {
			return i18n.Errorf("provider.id 重复: %s", provider.ID)
		}
		providerIDs[provider.ID] = true
		if len(provider.Models) == 0 {
			return i18n.Errorf("provider %s 没有模型", provider.ID)
		}
		modelIDs := make(map[string]bool, len(provider.Models))
		for _, model := range provider.Models {
			if model.ID == "" {
				return i18n.Errorf("provider %s 包含空模型 ID", provider.ID)
			}
			if modelIDs[model.ID] {
				return i18n.Errorf("provider %s 的模型 ID 重复: %s", provider.ID, model.ID)
			}
			modelIDs[model.ID] = true
		}
	}
	return nil
}

func validateActiveProvider(provider Provider) error {
	if provider.Name == "" {
		return i18n.Errorf("provider %s 的 name 不能为空", provider.ID)
	}
	if provider.URL == "" {
		return i18n.Errorf("provider %s 的 url 不能为空", provider.ID)
	}
	if provider.Key == "" {
		return i18n.Errorf("provider %s 的 key 不能为空", provider.ID)
	}
	return nil
}

func EnsureModelsFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return checkSecretFile(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return i18n.Errorf("检查模型配置失败: %w", err)
	}
	return writeDefaultModels(path, defaultModels())
}

func defaultModels() Models {
	return Models{
		Providers: []Provider{
			{
				ID:   "local",
				Name: "Local LLMs",
				URL:  "http://127.0.0.1:7863/v1",
				Key:  "",
				Models: []Model{
					{ID: "cn:deepseek-v4.1-flash", ReasoningEffort: defaultReasoningEffort, Default: true},
					{ID: "cn:glm-5.3-flash", ReasoningEffort: defaultReasoningEffort},
				},
			},
			{
				ID:   "example",
				Name: "Example Provider",
				URL:  "https://api.example.com/v1",
				Key:  "",
				Models: []Model{
					{ID: "llm-1.0", ReasoningEffort: "max"},
				},
			},
		},
	}
}

func writeDefaultModels(path string, models Models) error {
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return i18n.Errorf("生成模型配置失败: %w", err)
	}
	data = append(data, '\n')
	return writeExclusive(path, data)
}

func SaveModels(path string, models Models) error {
	if err := ValidateModels(models); err != nil {
		return err
	}
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return i18n.Errorf("生成模型配置失败: %w", err)
	}
	data = append(data, '\n')
	return writeReplace(path, data)
}
