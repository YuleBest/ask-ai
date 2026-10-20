package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type Language string

const (
	ChineseSimplified Language = "zh-CN"
	English           Language = "en"
)

type Settings struct {
	Output Language
	Prompt Language
}

var outputLanguage atomic.Value
var promptLanguage atomic.Value

func init() {
	outputLanguage.Store(ChineseSimplified)
	promptLanguage.Store(ChineseSimplified)
}

func ParseLanguage(value string) (Language, error) {
	normalized := normalizeLocale(value)
	switch {
	case normalized == "zh", strings.HasPrefix(normalized, "zh-"):
		return ChineseSimplified, nil
	case normalized == "en", strings.HasPrefix(normalized, "en-"):
		return English, nil
	default:
		return "", Errorf("不支持的语言 %q；请使用 %q 或 %q", value, ChineseSimplified, English)
	}
}

func Detect(getenv func(string) string) Language {
	for _, key := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		locale := strings.TrimSpace(getenv(key))
		if locale == "" {
			continue
		}
		for _, candidate := range strings.Split(locale, ":") {
			if language, err := ParseLanguage(candidate); err == nil {
				return language
			}
			normalized := normalizeLocale(candidate)
			if normalized == "c" || normalized == "posix" {
				return English
			}
		}
	}
	return English
}

func Resolve(getenv func(string) string, outputOverride, promptOverride Language) Settings {
	inherited := Detect(getenv)
	settings := Settings{Output: outputOverride, Prompt: promptOverride}
	if settings.Output == "" {
		settings.Output = inherited
	}
	if settings.Prompt == "" {
		settings.Prompt = inherited
	}
	return settings
}

func normalizeLocale(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if index := strings.IndexAny(value, ".@"); index >= 0 {
		value = value[:index]
	}
	return strings.ReplaceAll(value, "_", "-")
}

func SetOutputLanguage(language Language) {
	if language == "" {
		language = English
	}
	outputLanguage.Store(language)
}

func OutputLanguage() Language {
	return outputLanguage.Load().(Language)
}

func SetPromptLanguage(language Language) {
	if language == "" {
		language = English
	}
	promptLanguage.Store(language)
}

func PromptLanguage() Language {
	return promptLanguage.Load().(Language)
}

func TFor(language Language, source string, args ...any) string {
	message := source
	if language == English {
		if translated, ok := english[source]; ok {
			message = translated
		}
	}
	if len(args) == 0 {
		return message
	}
	return fmt.Sprintf(message, args...)
}

func T(source string, args ...any) string {
	return TFor(OutputLanguage(), source, args...)
}

func New(source string) error {
	return fmt.Errorf("%s", T(source))
}

func NewFor(language Language, source string) error {
	return fmt.Errorf("%s", TFor(language, source))
}

func NewDynamic(source string) error {
	return localizedError{source: source}
}

func Errorf(format string, args ...any) error {
	return fmt.Errorf(T(format), args...)
}

func ErrorfFor(language Language, format string, args ...any) error {
	return fmt.Errorf(TFor(language, format), args...)
}

type localizedError struct {
	source string
}

func (e localizedError) Error() string {
	return T(e.source)
}
