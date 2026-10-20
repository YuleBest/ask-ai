package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
	"github.com/YuleBest/ask-ai/internal/terminaltext"
)

const chineseFileToolInstructions = `你可以使用 view_file 查看本机普通文件的元信息和 UTF-8 文本内容。
- 用户提到本地文件、路径、配置、源码或日志时，相关情况下主动查看。
- 根据任务选择 offset 和 length，不要无目的地读取大文件。
- 结果被截断时按 next_offset 继续；binary: true 时不要声称已经看到正文。`

const englishFileToolInstructions = `You can use view_file to inspect metadata and UTF-8 text from regular files on this machine.
- Proactively view relevant local files, paths, configuration, source code, or logs when the user refers to them.
- Choose offset and length for the task instead of reading large files without a clear reason.
- When the result is truncated, continue from next_offset. When binary is true, do not claim to have seen the content.`

type fileToolExecutor struct {
	config      config.ViewFileConfig
	workDir     string
	reader      *bufio.Reader
	stderr      io.Writer
	interactive bool
	language    i18n.Language
}

type fileToolArguments struct {
	Path   string `json:"path"`
	Offset *int64 `json:"offset,omitempty"`
	Length *int64 `json:"length,omitempty"`
}

type fileToolSuccess struct {
	Path            string  `json:"path"`
	ResolvedPath    string  `json:"resolved_path"`
	Size            int64   `json:"size"`
	Mode            string  `json:"mode"`
	ModifiedAt      string  `json:"modified_at"`
	Offset          int64   `json:"offset"`
	RequestedLength int64   `json:"requested_length"`
	ReturnedBytes   int     `json:"returned_bytes"`
	Truncated       bool    `json:"truncated"`
	NextOffset      int64   `json:"next_offset"`
	Binary          bool    `json:"binary"`
	Content         *string `json:"content,omitempty"`
	Notice          string  `json:"notice,omitempty"`
}

func fileToolDefinitionsFromMeta(meta config.Meta, language i18n.Language) ([]chat.Tool, error) {
	tool, ok := meta.Tools["viewFile"]
	if !ok {
		return nil, i18n.New("配置元数据缺少 viewFile 工具")
	}
	if len(tool.Definitions) == 0 {
		return nil, i18n.New("配置元数据缺少 viewFile 工具定义")
	}

	tools := make([]chat.Tool, 0, len(tool.Definitions))
	for _, definition := range tool.Definitions {
		definition = localizeDefaultFileToolDefinition(definition, language)
		if definition.Name == "" {
			return nil, i18n.New("工具定义缺少 name")
		}
		parameters := definition.Parameters
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, chat.Tool{
			Type: "function",
			Function: chat.ToolFunction{
				Name:        definition.Name,
				Description: definition.Description,
				Parameters:  parameters,
			},
		})
	}
	return tools, nil
}

func localizeDefaultFileToolDefinition(definition config.MetaToolDefinition, language i18n.Language) config.MetaToolDefinition {
	if language != i18n.ChineseSimplified {
		return definition
	}
	defaults := config.DefaultMeta().Tools["viewFile"].Definitions
	for _, defaultDefinition := range defaults {
		if definition.Name != defaultDefinition.Name ||
			definition.Description != defaultDefinition.Description ||
			!sameJSON(definition.Parameters, defaultDefinition.Parameters) {
			continue
		}
		definition.Description = "查看当前用户可读的普通文件元信息和 UTF-8 文本内容。相对路径按当前工作目录解析。"
		definition.Parameters = json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "文件路径；相对路径按当前工作目录解析"},
				"offset": {"type": "integer", "minimum": 0, "description": "起始字节位置，默认 0"},
				"length": {"type": "integer", "minimum": 1, "description": "读取字节数；受每次调用上限限制"}
			},
			"required": ["path"],
			"additionalProperties": false
		}`)
		return definition
	}
	return definition
}

func addFileToolInstructions(prompt string, language i18n.Language) string {
	prompt = strings.TrimSpace(prompt)
	instructions := chineseFileToolInstructions
	if language == i18n.English {
		instructions = englishFileToolInstructions
	}
	if prompt == "" {
		return instructions
	}
	return prompt + "\n\n" + instructions
}

func (e fileToolExecutor) Execute(ctx context.Context, call chat.ToolCall) string {
	var arguments fileToolArguments
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		return fileToolFailureJSON(
			e.language,
			i18n.TFor(e.language, "解析文件查看参数失败: %s", err),
			"parse_view_file_arguments",
		)
	}
	arguments.Path = strings.TrimSpace(arguments.Path)
	if arguments.Path == "" {
		return fileToolFailureJSON(e.language, i18n.TFor(e.language, "文件路径不能为空"), "path_required")
	}

	offset := int64(0)
	if arguments.Offset != nil {
		offset = *arguments.Offset
	}
	if offset < 0 {
		return fileToolFailureJSON(e.language, i18n.TFor(e.language, "offset 不能小于 0"), "invalid_offset")
	}

	requestedLength := e.config.MaxBytes
	if arguments.Length != nil {
		requestedLength = *arguments.Length
		if requestedLength <= 0 {
			return fileToolFailureJSON(
				e.language,
				i18n.TFor(e.language, "length 必须大于 0"),
				"invalid_length",
			)
		}
		if requestedLength > e.config.MaxBytes {
			requestedLength = e.config.MaxBytes
		}
	}

	resolvedPath, err := e.resolvePath(arguments.Path)
	if err != nil {
		return fileToolFailureJSON(
			e.language,
			i18n.TFor(e.language, "解析文件路径失败: %s", err),
			"resolve_path_failed",
		)
	}
	if !e.confirm(ctx, resolvedPath, offset, requestedLength) {
		return fileToolFailureJSON(
			e.language,
			i18n.TFor(e.language, "用户未允许查看该文件。"),
			"permission_denied",
		)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return fileToolFailureJSON(
				e.language,
				i18n.TFor(e.language, "文件不存在: %s", resolvedPath),
				"file_not_found",
			)
		case errors.Is(err, os.ErrPermission):
			return fileToolFailureJSON(
				e.language,
				i18n.TFor(e.language, "没有权限读取文件: %s", resolvedPath),
				"permission_error",
			)
		default:
			return fileToolFailureJSON(
				e.language,
				i18n.TFor(e.language, "读取文件信息失败: %s", err),
				"stat_failed",
			)
		}
	}
	if !info.Mode().IsRegular() {
		return fileToolFailureJSON(
			e.language,
			i18n.TFor(e.language, "路径不是普通文件: %s", resolvedPath),
			"not_regular_file",
		)
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		code := "read_failed"
		if errors.Is(err, os.ErrPermission) {
			code = "permission_error"
		}
		return fileToolFailureJSON(
			e.language,
			i18n.TFor(e.language, "打开文件失败: %s", err),
			code,
		)
	}
	defer file.Close()

	section := io.NewSectionReader(file, offset, requestedLength)
	data, err := io.ReadAll(io.LimitReader(section, requestedLength))
	if err != nil {
		return fileToolFailureJSON(
			e.language,
			i18n.TFor(e.language, "读取文件失败: %s", err),
			"read_failed",
		)
	}

	result := fileToolSuccess{
		Path:            arguments.Path,
		ResolvedPath:    resolvedPath,
		Size:            info.Size(),
		Mode:            info.Mode().String(),
		ModifiedAt:      info.ModTime().UTC().Format(time.RFC3339Nano),
		Offset:          offset,
		RequestedLength: requestedLength,
		ReturnedBytes:   len(data),
		NextOffset:      offset + int64(len(data)),
	}
	result.Truncated = offset < info.Size() && info.Size()-offset > requestedLength
	result.Binary = !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0

	var notices []string
	if result.Truncated {
		notices = append(notices, i18n.TFor(
			e.language,
			"文件仍有未读取内容，可继续使用 offset %d。",
			result.NextOffset,
		))
	}
	if result.Binary {
		notices = append(notices, i18n.TFor(e.language, "检测到二进制内容，未返回正文。"))
	} else if len(data) == 0 {
		notices = append(notices, i18n.TFor(e.language, "读取范围为空。"))
	}
	result.Notice = strings.Join(notices, " ")
	if !result.Binary {
		content := string(data)
		result.Content = &content
	}
	return marshalToolResult(result, e.language)
}

func (e fileToolExecutor) confirm(ctx context.Context, path string, offset, length int64) bool {
	switch e.config.Mode {
	case config.ViewFileModeBypass:
		return true
	case config.ViewFileModeAsk:
		if !e.interactive || e.reader == nil || e.stderr == nil {
			return false
		}
	default:
		return false
	}

	prompt := i18n.TFor(
		e.language,
		"允许查看文件 %s（offset %d，length %d）？[Enter/y 批准] ",
		terminaltext.Clean(path),
		offset,
		length,
	)
	if _, err := fmt.Fprint(e.stderr, newOutputStyle(e.stderr).yellow(prompt)); err != nil {
		return false
	}
	answer, err := readQuestionLine(ctx, e.reader)
	if err != nil {
		return false
	}
	switch strings.TrimSpace(answer) {
	case "", "y", "Y":
		return true
	default:
		return false
	}
}

func (e fileToolExecutor) resolvePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		workDir := e.workDir
		if workDir == "" {
			current, err := os.Getwd()
			if err != nil {
				return "", err
			}
			workDir = current
		}
		path = filepath.Join(workDir, path)
	}
	return filepath.Abs(filepath.Clean(path))
}

func fileToolFailureJSON(_ i18n.Language, message, code string) string {
	return toolErrorJSON(message, code)
}
