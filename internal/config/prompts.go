package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/YuleBest/ask-ai/internal/i18n"
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

const englishAssistantInstructions = `You are a quick-question command-line assistant. Your goal is to give users the information they actually need, quickly and accurately.

- Answer directly. Do not add pleasantries or repeat the question.
- Be concise by default. Give a conclusion for simple questions and add structured detail only when needed.
- Prefer actionable answers. When code is needed, use fenced code blocks with language markers and explain only the necessary parts.
- If the user starts from a false premise, correct it before giving the right conclusion.
- If information is incomplete but an answer is possible, state the key assumptions. If no reliable answer is possible, say what is missing instead of guessing.
- Clearly identify uncertain facts, commands, and results. Never invent them.
- Respond in the language used by the user.
- Use Markdown only to improve readability. Avoid unnecessary headings, lists, and summaries.`

const defaultSystemPrompt = `当前运行环境：
- 系统：{{os}}
- 架构：{{arch}}
- 日期：{{date}}
- 时间：{{time}}
- 时区：{{timezone}}

` + assistantInstructions

const englishDefaultSystemPrompt = `Current environment:
- System: {{os}}
- Architecture: {{arch}}
- Date: {{date}}
- Time: {{time}}
- Timezone: {{timezone}}

` + englishAssistantInstructions

const legacyDefaultSystemPrompt = assistantInstructions

func SystemPromptPath(modelsPath string) string {
	return filepath.Join(filepath.Dir(modelsPath), "system_prompt.json")
}

func LoadOrCreateSystemPrompt(path string, language i18n.Language) (string, bool, error) {
	prompts, created, err := loadOrCreateSystemPrompts(path)
	if err != nil {
		return "", false, err
	}
	prompt, exists := prompts[language]
	if !exists {
		prompt = defaultPrompt(language)
	}
	return strings.TrimSpace(prompt), created, nil
}

func EnsureSystemPromptFile(path string) error {
	_, _, err := loadOrCreateSystemPrompts(path)
	return err
}

func loadOrCreateSystemPrompts(path string) (map[i18n.Language]string, bool, error) {
	prompts, exists, err := loadSystemPrompts(path)
	if err != nil {
		return nil, false, err
	}
	if exists {
		return prompts, false, nil
	}

	legacyPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".txt"
	legacyData, legacyErr := os.ReadFile(legacyPath)
	created := false
	switch {
	case legacyErr == nil:
		prompts = migrateLegacyPrompts(string(legacyData))
		err = writeSystemPrompts(path, prompts)
	case errors.Is(legacyErr, os.ErrNotExist):
		created = true
		prompts = defaultSystemPrompts()
		err = writeSystemPrompts(path, prompts)
	default:
		return nil, false, i18n.Errorf("读取旧系统提示词失败: %w", legacyErr)
	}
	if errors.Is(err, os.ErrExist) {
		var readErr error
		prompts, exists, readErr = loadSystemPrompts(path)
		if readErr != nil {
			return nil, false, readErr
		}
		if !exists {
			return nil, false, err
		}
		created = false
		err = removeLegacyPrompt(legacyPath)
	} else if err == nil {
		err = removeLegacyPrompt(legacyPath)
	} else {
		return nil, false, err
	}
	return prompts, created, err
}

func loadSystemPrompts(path string) (map[i18n.Language]string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, i18n.Errorf("读取系统提示词失败: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		data = []byte("{}")
	}

	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false, i18n.Errorf("解析系统提示词失败: %w", err)
	}
	prompts := make(map[i18n.Language]string, len(raw))
	for key, prompt := range raw {
		language, err := i18n.ParseLanguage(key)
		if err != nil {
			return nil, false, i18n.Errorf("system_prompt.json: %w", err)
		}
		prompts[language] = prompt
	}
	return prompts, true, nil
}

func removeLegacyPrompt(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return i18n.Errorf("清理旧系统提示词失败: %w", err)
	}
	return nil
}

func migrateLegacyPrompts(prompt string) map[i18n.Language]string {
	trimmed := strings.TrimSpace(prompt)
	switch {
	case trimmed == legacyDefaultSystemPrompt,
		trimmed == strings.TrimSpace(defaultSystemPrompt):
		return defaultSystemPrompts()
	case trimmed == "":
		return map[i18n.Language]string{
			i18n.ChineseSimplified: "",
			i18n.English:           "",
		}
	default:
		return map[i18n.Language]string{
			i18n.ChineseSimplified: prompt,
			i18n.English:           prompt,
		}
	}
}

func defaultSystemPrompts() map[i18n.Language]string {
	return map[i18n.Language]string{
		i18n.ChineseSimplified: defaultSystemPrompt,
		i18n.English:           englishDefaultSystemPrompt,
	}
}

func defaultPrompt(language i18n.Language) string {
	if language == i18n.English {
		return englishDefaultSystemPrompt
	}
	return defaultSystemPrompt
}

func writeSystemPrompts(path string, prompts map[i18n.Language]string) error {
	data, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		return i18n.Errorf("生成系统提示词失败: %w", err)
	}
	data = append(data, '\n')
	return writeExclusive(path, data)
}
