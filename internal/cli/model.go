package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/conversation"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

func runModel(ctx context.Context, args []string, stdout, stderr io.Writer, configPath configPathFunc, terminalID terminalIDProvider) int {
	if len(args) == 0 {
		printModelUsage(stderr)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printModelUsage(stdout)
		return 0
	}
	if args[0] == "list" {
		if len(args) != 1 {
			printModelUsage(stderr)
			return 2
		}
		models, _, err := loadModelsConfig(configPath)
		if err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		return listModels(models, stdout)
	}
	if len(args) != 2 {
		printModelUsage(stderr)
		return 2
	}

	providerID, modelID, err := parseModelRef(args[1])
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	models, modelsPath, err := loadModelsConfig(configPath)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	if _, _, err := models.Resolve(providerID, modelID); err != nil {
		printErrorf(stderr, "%v: %s", err, modelsPath)
		return 1
	}

	switch args[0] {
	case "use":
		return useModel(providerID, modelID, stdout, stderr, terminalID)
	case "default":
		if err := models.SetDefault(providerID, modelID); err != nil {
			printErrorf(stderr, "%v: %s", err, modelsPath)
			return 1
		}
		if err := config.SaveModels(modelsPath, models); err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		style := newOutputStyle(stdout)
		fmt.Fprintf(stdout, "%s %s/%s\n", style.green(i18n.T("已设置默认模型:")), style.cyan(providerID), style.cyan(modelID))
		return 0
	default:
		printErrorf(stderr, "未知模型命令: %s", args[0])
		printModelUsage(stderr)
		return 2
	}
}

func listModels(models config.Models, stdout io.Writer) int {
	style := newOutputStyle(stdout)
	for providerIndex, provider := range models.Providers {
		if providerIndex > 0 {
			fmt.Fprintln(stdout)
		}
		name := provider.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(stdout, "%s  %s\n", style.boldCyan(provider.ID), style.bold(name))
		fmt.Fprintf(stdout, "  %s %s\n", style.dim("URL:"), provider.URL)
		for _, model := range provider.Models {
			effort := model.ReasoningEffort
			if effort == "" {
				effort = "-"
			}
			marker := ""
			if model.Default {
				marker = "  " + style.green(i18n.T("[默认]"))
			}
			fmt.Fprintf(stdout, "  %s  %s%s\n", style.cyan(model.ID), style.dim(i18n.T("推理强度=%s", effort)), marker)
		}
	}
	return 0
}

func useModel(providerID, modelID string, stdout, stderr io.Writer, terminalID terminalIDProvider) int {
	store, err := conversation.DefaultStore()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}

	terminalSessionID, err := terminalID(stdout, stderr)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}

	var session conversation.Session
	if terminalSessionID != "" {
		bindingStore := conversation.NewBindingStore(store.Root, terminalSessionID)
		boundID, bindErr := bindingStore.Load()
		switch {
		case bindErr == nil:
			session, err = store.Load(boundID)
		case errors.Is(bindErr, conversation.ErrNotBound):
			session, err = store.LoadLast()
		default:
			err = bindErr
		}
	} else {
		session, err = store.LoadLast()
	}
	if err != nil {
		printErrorf(stderr, "没有可切换模型的任务: %v", err)
		return 1
	}

	session.Provider = providerID
	session.Model = modelID
	if err := store.Update(session); err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	style := newOutputStyle(stdout)
	fmt.Fprintf(stdout, "%s %s/%s  %s\n", style.green(i18n.T("已临时切换模型:")), style.cyan(providerID), style.cyan(modelID), style.dim(session.ID))
	return 0
}

func loadModelsConfig(configPath configPathFunc) (config.Models, string, error) {
	defaultModelsPath, err := configPath()
	if err != nil {
		return config.Models{}, "", err
	}
	configDir := filepath.Dir(defaultModelsPath)
	metaPath := config.MetaPath(defaultModelsPath)
	meta, _, err := config.LoadOrCreateMeta(metaPath)
	if err != nil {
		return config.Models{}, "", fmt.Errorf("%v: %s", err, metaPath)
	}
	modelsPath, err := meta.PathFor("models", configDir)
	if err != nil {
		return config.Models{}, "", fmt.Errorf("%v: %s", err, metaPath)
	}
	models, _, err := config.LoadOrCreateModels(modelsPath)
	if err != nil {
		return config.Models{}, "", err
	}
	return models, modelsPath, nil
}

func parseModelRef(ref string) (string, string, error) {
	providerID, modelID, ok := strings.Cut(ref, "/")
	if !ok || providerID == "" || modelID == "" {
		return "", "", i18n.New("模型格式必须为 <provider_id>/<model_id>")
	}
	return providerID, modelID, nil
}
