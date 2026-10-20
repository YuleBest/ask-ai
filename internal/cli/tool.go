package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

func runTool(args []string, stdout, stderr io.Writer, configPath configPathFunc) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printToolUsage(stdout)
		return 0
	}
	if len(args) == 0 {
		printToolUsage(stderr)
		return 2
	}

	defaultModelsPath, err := configPath()
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	configDir := filepath.Dir(defaultModelsPath)
	metaPath := config.MetaPath(defaultModelsPath)
	meta, _, err := config.LoadOrCreateMeta(metaPath)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, metaPath)
		return 1
	}

	if args[0] == "ls" || args[0] == "list" {
		if len(args) != 1 {
			printToolUsage(stderr)
			return 2
		}
		listTools(meta, stdout)
		return 0
	}

	toolID := args[0]
	tool, exists := meta.Tools[toolID]
	if !exists {
		printErrorf(stderr, "未知工具: %s", toolID)
		return 1
	}
	if len(args) < 2 {
		printErrorf(stderr, "工具 %s 缺少操作", toolID)
		return 2
	}
	operation := args[1]
	if !containsString(tool.Operations, operation) {
		printErrorf(stderr, "工具 %s 不支持操作: %s", toolID, operation)
		return 2
	}

	switch tool.Config {
	case "webSearch":
		return runWebSearchTool(args, toolID, operation, meta, configDir, stdout, stderr)
	default:
		printErrorf(stderr, "工具 %s 的配置类型不受支持: %s", toolID, tool.Config)
		return 1
	}
}

func runWebSearchTool(args []string, toolID, operation string, meta config.Meta, configDir string, stdout, stderr io.Writer) int {
	path, err := meta.PathFor("webSearch", configDir)
	if err != nil {
		printErrorf(stderr, "%v", err)
		return 1
	}
	webSearch, _, err := config.LoadOrCreateWebSearch(path)
	if err != nil {
		printErrorf(stderr, "%v: %s", err, path)
		return 1
	}

	switch operation {
	case "off":
		if len(args) != 2 {
			printToolUsage(stderr)
			return 2
		}
		webSearch.Use = ""
		if err := config.SaveWebSearch(path, webSearch); err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s %s\n", newOutputStyle(stdout).yellow(i18n.T("已关闭工具:")), toolID)
		return 0
	case "use":
		if len(args) != 3 {
			printToolUsage(stderr)
			return 2
		}
		providerID := strings.TrimSpace(args[2])
		if providerID != "exa" {
			printErrorf(stderr, "工具 %s 当前仅支持 provider: exa", toolID)
			return 1
		}
		found := false
		keyConfigured := false
		for _, provider := range webSearch.Providers {
			if provider.ID == providerID {
				found = true
				keyConfigured = provider.Key != ""
				break
			}
		}
		if !found {
			printErrorf(stderr, "网络搜索配置中不存在 provider: %s", providerID)
			return 1
		}
		if !keyConfigured {
			printErrorf(stderr, "网络搜索 provider %s 未配置 key", providerID)
			return 1
		}
		webSearch.Use = config.ToolUse(providerID)
		if err := config.SaveWebSearch(path, webSearch); err != nil {
			printErrorf(stderr, "%v", err)
			return 1
		}
		fmt.Fprintf(
			stdout,
			"%s %s/%s\n",
			newOutputStyle(stdout).green(i18n.T("已切换工具提供商:")),
			newOutputStyle(stdout).cyan(toolID),
			newOutputStyle(stdout).cyan(providerID),
		)
		return 0
	default:
		printErrorf(stderr, "工具 %s 不支持操作: %s", toolID, operation)
		return 2
	}
}

func listTools(meta config.Meta, stdout io.Writer) {
	style := newOutputStyle(stdout)
	ids := make([]string, 0, len(meta.Tools))
	for id := range meta.Tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Fprintf(
		stdout,
		"%s  %s  %s  %s\n",
		style.dim(fmt.Sprintf("%-16s", i18n.T("工具"))),
		style.dim(fmt.Sprintf("%-16s", i18n.T("配置"))),
		style.dim(fmt.Sprintf("%-20s", i18n.T("操作"))),
		style.dim(i18n.T("用途")),
	)
	for _, id := range ids {
		tool := meta.Tools[id]
		fmt.Fprintf(
			stdout,
			"%s  %s  %s  %s\n",
			style.cyan(fmt.Sprintf("%-16s", id)),
			fmt.Sprintf("%-16s", tool.Config),
			fmt.Sprintf("%-20s", strings.Join(tool.Operations, ",")),
			toolDescription(id, tool.Description),
		)
	}
}

func toolDescription(id, fallback string) string {
	defaults := config.DefaultMeta()
	defaultTool, exists := defaults.Tools[id]
	if !exists || fallback != defaultTool.Description {
		return fallback
	}
	return i18n.T(fallback)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
