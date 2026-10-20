package cli

import (
	"os"
	"path/filepath"

	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

func configureLanguage(configPath configPathFunc) error {
	setLanguage(i18n.Resolve(os.Getenv, "", ""))

	modelsPath, err := configPath()
	if err != nil {
		return err
	}
	configDir := filepath.Dir(modelsPath)
	languagePath := config.LanguagePath(modelsPath)
	meta, exists, err := config.ReadExistingMeta(config.MetaPath(modelsPath))
	if err != nil {
		return err
	}
	if exists {
		if _, ok := meta.Configs["language"]; ok {
			languagePath, err = meta.PathFor("language", configDir)
			if err != nil {
				return err
			}
		}
	}
	language, _, err := config.LoadLanguage(languagePath)
	if err != nil {
		return err
	}
	setLanguage(language.Resolve(os.Getenv))
	return nil
}

func setLanguage(settings i18n.Settings) {
	i18n.SetOutputLanguage(settings.Output)
	i18n.SetPromptLanguage(settings.Prompt)
}

func languageConfigOperation(args []string) bool {
	if len(args) < 3 || args[0] != "config" {
		return false
	}
	if args[1] != "edit" && args[1] != "view" {
		return false
	}
	return filepath.Base(args[2]) == "language"
}
