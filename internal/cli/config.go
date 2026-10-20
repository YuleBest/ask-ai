package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

func runConfig(ctx context.Context, args []string, stdout, stderr io.Writer, configPath configPathFunc) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printConfigUsage(stdout)
		return 0
	}

	modelsPath, err := configPath()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	configDir := filepath.Dir(modelsPath)
	metaPath := config.MetaPath(modelsPath)
	meta, _, err := config.LoadOrCreateMeta(metaPath)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, metaPath)
		return 1
	}
	if err := ensureDefaultConfigFiles(meta, configDir); err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}

	if len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		if len(args) > 1 {
			printConfigUsage(stderr)
			return 2
		}
		return listConfig(meta, configDir, stdout, stderr)
	}

	switch args[0] {
	case "edit":
		if len(args) != 2 {
			printConfigUsage(stderr)
			return 2
		}
		return editConfig(ctx, meta, configDir, args[1], stdout, stderr)
	case "view":
		key, showSecrets, err := parseConfigViewArgs(args[1:])
		if err != nil {
			printConfigUsage(stderr)
			return 2
		}
		return viewConfig(ctx, meta, configDir, key, showSecrets, stdout, stderr)
	default:
		printErrorf(stderr, "未知配置命令: %s", args[0])
		printConfigUsage(stderr)
		return 2
	}
}

func ensureDefaultConfigFiles(meta config.Meta, configDir string) error {
	modelsPath, err := meta.PathFor("models", configDir)
	if err != nil {
		return err
	}
	promptPath, err := meta.PathFor("systemPrompt", configDir)
	if err != nil {
		return err
	}
	languagePath, err := meta.PathFor("language", configDir)
	if err != nil {
		return err
	}
	webSearchPath, err := meta.PathFor("webSearch", configDir)
	if err != nil {
		return err
	}
	viewFilePath, err := meta.PathFor("viewFile", configDir)
	if err != nil {
		return err
	}
	if err := config.EnsureModelsFile(modelsPath); err != nil {
		return err
	}
	if err := config.EnsureSystemPromptFile(promptPath); err != nil {
		return err
	}
	if err := config.EnsureLanguageFile(languagePath); err != nil {
		return err
	}
	if err := config.EnsureWebSearchFile(webSearchPath); err != nil {
		return err
	}
	if err := config.EnsureViewFile(viewFilePath); err != nil {
		return err
	}
	return nil
}

func listConfig(meta config.Meta, configDir string, stdout, stderr io.Writer) int {
	keys := make([]string, 0, len(meta.Configs))
	for key := range meta.Configs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	style := newOutputStyle(stdout)
	fmt.Fprintf(
		stdout,
		"%s  %s  %s\n",
		style.dim(fmt.Sprintf("%-16s", i18n.T("键"))),
		style.dim(fmt.Sprintf("%-24s", i18n.T("文件"))),
		style.dim(i18n.T("说明")),
	)
	for _, key := range keys {
		entry := meta.Configs[key]
		fmt.Fprintf(stdout, "%s  %s  %s\n", style.cyan(fmt.Sprintf("%-16s", key)), fmt.Sprintf("%-24s", entry.File), configDescription(key, entry.Description))
	}
	return 0
}

func editConfig(ctx context.Context, meta config.Meta, configDir, key string, stdout, stderr io.Writer) int {
	path, err := resolveConfigPath(meta, configDir, key, true)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	if err := runExternalCommand(ctx, "nano", []string{path}, stdout, stderr); err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	return 0
}

func parseConfigViewArgs(args []string) (string, bool, error) {
	if len(args) == 0 {
		return "", false, i18n.New("缺少配置键")
	}
	key := args[0]
	showSecrets := false
	for _, arg := range args[1:] {
		if arg != "--show-secrets" || showSecrets {
			return "", false, i18n.Errorf("未知参数: %s", arg)
		}
		showSecrets = true
	}
	return key, showSecrets, nil
}

func viewConfig(ctx context.Context, meta config.Meta, configDir, key string, showSecrets bool, stdout, stderr io.Writer) int {
	path, err := resolveConfigPath(meta, configDir, key, false)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	if isSensitiveConfigKey(key) && !showSecrets {
		if err := viewRedactedJSON(path, stdout); err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		return 0
	}
	if err := viewFile(ctx, path, stdout, stderr); err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	return 0
}

func isSensitiveConfigKey(key string) bool {
	return key == "models" || key == "webSearch"
}

func viewRedactedJSON(path string, stdout io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return i18n.Errorf("读取配置文件失败: %w", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return i18n.Errorf("解析配置文件失败: %w", err)
	}
	redactJSON(value)
	data, err = json.MarshalIndent(value, "", "  ")
	if err != nil {
		return i18n.Errorf("生成脱敏配置失败: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(data))
	return err
}

func redactJSON(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if normalized == "key" || normalized == "apikey" || normalized == "token" {
				typed[key] = "***"
				continue
			}
			redactJSON(child)
		}
	case []any:
		for _, child := range typed {
			redactJSON(child)
		}
	}
}

func resolveConfigPath(meta config.Meta, configDir, key string, create bool) (string, error) {
	path, err := meta.PathFor(key, configDir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", i18n.Errorf("检查配置文件失败: %w", err)
	}
	if !create {
		return "", i18n.Errorf("配置文件不存在: %s", path)
	}

	switch key {
	case "models":
		err = config.EnsureModelsFile(path)
	case "systemPrompt":
		err = config.EnsureSystemPromptFile(path)
	case "language":
		err = config.EnsureLanguageFile(path)
	case "webSearch":
		err = config.EnsureWebSearchFile(path)
	case "viewFile":
		err = config.EnsureViewFile(path)
	default:
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			file, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if openErr != nil {
				err = openErr
			} else if closeErr := file.Close(); closeErr != nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return "", i18n.Errorf("创建配置文件失败: %w", err)
	}
	return path, nil
}

func viewFile(ctx context.Context, path string, stdout, stderr io.Writer) error {
	batcat, err := exec.LookPath("batcat")
	if err == nil {
		return runExternalCommand(ctx, batcat, []string{"-pP", path}, stdout, stderr)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		return i18n.Errorf("查找 batcat 失败: %w", err)
	}

	cat, err := exec.LookPath("cat")
	if err != nil {
		return i18n.Errorf("找不到 batcat 或 cat: %w", err)
	}
	return runExternalCommand(ctx, cat, []string{path}, stdout, stderr)
}

func runExternalCommand(ctx context.Context, command string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return i18n.Errorf("%s 执行失败: %w", filepath.Base(command), err)
	}
	return nil
}

func configDescription(key, fallback string) string {
	defaults := config.DefaultMeta()
	defaultEntry, exists := defaults.Configs[key]
	if !exists || fallback != defaultEntry.Description {
		return fallback
	}
	return i18n.T(fallback)
}
