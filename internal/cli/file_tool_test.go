package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

func TestFileToolDefinitionsUseMetadata(t *testing.T) {
	tools, err := fileToolDefinitionsFromMeta(config.DefaultMeta(), i18n.English)
	if err != nil {
		t.Fatalf("fileToolDefinitionsFromMeta() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	tool := tools[0]
	if tool.Type != "function" || tool.Function.Name != "view_file" {
		t.Fatalf("tool = %+v", tool)
	}
	if !strings.Contains(tool.Function.Description, "regular files") {
		t.Fatalf("description = %q, want English file description", tool.Function.Description)
	}

	var parameters map[string]any
	if err := json.Unmarshal(tool.Function.Parameters, &parameters); err != nil {
		t.Fatalf("decode parameters: %v", err)
	}
	required, ok := parameters["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "path" {
		t.Fatalf("required = %+v, want path", parameters["required"])
	}
}

func TestFileToolDefinitionsFollowPromptLanguage(t *testing.T) {
	tools, err := fileToolDefinitionsFromMeta(config.DefaultMeta(), i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("fileToolDefinitionsFromMeta() error = %v", err)
	}
	if got := tools[0].Function.Description; !strings.Contains(got, "普通文件") {
		t.Fatalf("description = %q, want localized file description", got)
	}
	if got := string(tools[0].Function.Parameters); !strings.Contains(got, "文件路径") {
		t.Fatalf("parameters = %q, want localized path description", got)
	}
}

func TestFileToolDefinitionsPreserveCustomMetadata(t *testing.T) {
	meta := config.DefaultMeta()
	tool := meta.Tools["viewFile"]
	tool.Definitions[0].Description = "custom file viewer"
	meta.Tools["viewFile"] = tool

	tools, err := fileToolDefinitionsFromMeta(meta, i18n.ChineseSimplified)
	if err != nil {
		t.Fatalf("fileToolDefinitionsFromMeta() error = %v", err)
	}
	if got, want := tools[0].Function.Description, "custom file viewer"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestAddFileToolInstructions(t *testing.T) {
	chinese := addFileToolInstructions("基础提示", i18n.ChineseSimplified)
	if !strings.HasPrefix(chinese, "基础提示\n\n") || !strings.Contains(chinese, "view_file") {
		t.Fatalf("Chinese instructions = %q", chinese)
	}
	english := addFileToolInstructions("Base prompt", i18n.English)
	if !strings.HasPrefix(english, "Base prompt\n\n") || !strings.Contains(english, "view_file") {
		t.Fatalf("English instructions = %q", english)
	}
	if got := addFileToolInstructions("  ", i18n.English); !strings.HasPrefix(got, "You can use view_file") {
		t.Fatalf("empty prompt instructions = %q", got)
	}
}

func TestFileToolExecutorReadsTextRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("abcdef"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	executor := fileToolExecutor{
		config:   config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 4},
		workDir:  dir,
		language: i18n.ChineseSimplified,
	}

	offset := int64(1)
	length := int64(3)
	result := decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, &offset, &length),
	))
	if result.Path != path || result.ResolvedPath != path {
		t.Fatalf("paths = %q / %q, want %q", result.Path, result.ResolvedPath, path)
	}
	if result.Size != 6 {
		t.Fatalf("size = %d, want 6", result.Size)
	}
	if result.Mode == "" {
		t.Fatal("mode is empty")
	}
	if _, err := time.Parse(time.RFC3339Nano, result.ModifiedAt); err != nil {
		t.Fatalf("modified_at = %q: %v", result.ModifiedAt, err)
	}
	if result.Offset != 1 || result.RequestedLength != 3 || result.ReturnedBytes != 3 {
		t.Fatalf("range = %+v", result)
	}
	if result.Content == nil || *result.Content != "bcd" {
		t.Fatalf("content = %v, want bcd", result.Content)
	}
	if !result.Truncated || result.NextOffset != 4 {
		t.Fatalf("truncation = %+v, want next offset 4", result)
	}
	if result.Binary || result.Notice == "" {
		t.Fatalf("binary/notice = %v / %q", result.Binary, result.Notice)
	}

	offset = 4
	length = 4
	result = decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, &offset, &length),
	))
	if result.Content == nil || *result.Content != "ef" {
		t.Fatalf("continued content = %v, want ef", result.Content)
	}
	if result.RequestedLength != 4 || result.ReturnedBytes != 2 || result.Truncated || result.NextOffset != 6 {
		t.Fatalf("continued range = %+v", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source file: %v", err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("source file was modified: %q", data)
	}
}

func TestFileToolExecutorCapsLengthAndHandlesEmptyRanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("abcdef"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	executor := fileToolExecutor{
		config:   config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 4},
		workDir:  dir,
		language: i18n.English,
	}

	length := int64(999)
	result := decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, nil, &length),
	))
	if result.RequestedLength != 4 || result.ReturnedBytes != 4 || !result.Truncated || result.NextOffset != 4 {
		t.Fatalf("capped result = %+v", result)
	}

	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	result = decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, empty, nil, nil),
	))
	if result.Content == nil || *result.Content != "" || result.ReturnedBytes != 0 {
		t.Fatalf("empty result = %+v", result)
	}
	if result.Binary || result.Notice == "" {
		t.Fatalf("empty binary/notice = %v / %q", result.Binary, result.Notice)
	}

	offset := int64(6)
	result = decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, &offset, nil),
	))
	if result.Content == nil || *result.Content != "" || result.Notice == "" {
		t.Fatalf("EOF result = %+v", result)
	}

	offset = 10
	result = decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, &offset, nil),
	))
	if result.Content == nil || *result.Content != "" || result.Notice == "" {
		t.Fatalf("past-EOF result = %+v", result)
	}
}

func TestFileToolExecutorResolvesRelativePaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "relative.txt"), []byte("relative"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	executor := fileToolExecutor{
		config:   config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 32},
		workDir:  dir,
		language: i18n.English,
	}

	result := decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, "relative.txt", nil, nil),
	))
	want := filepath.Join(dir, "relative.txt")
	if result.ResolvedPath != want {
		t.Fatalf("resolved path = %q, want %q", result.ResolvedPath, want)
	}
	if result.Content == nil || *result.Content != "relative" {
		t.Fatalf("content = %v, want relative", result.Content)
	}
}

func TestFileToolExecutorReturnsMetadataForBinaryRanges(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "invalid UTF-8", data: []byte{0xff, 0xfe}},
		{name: "NUL byte", data: []byte("a\x00b")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "binary.dat")
			if err := os.WriteFile(path, tt.data, 0o600); err != nil {
				t.Fatalf("write binary file: %v", err)
			}
			executor := fileToolExecutor{
				config:   config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 32},
				workDir:  dir,
				language: i18n.English,
			}

			result := decodeFileToolResult(t, executor.Execute(
				context.Background(),
				fileToolCall(t, path, nil, nil),
			))
			if !result.Binary || result.Content != nil {
				t.Fatalf("binary result = %+v", result)
			}
			if result.ReturnedBytes != len(tt.data) || result.Notice == "" {
				t.Fatalf("binary bytes/notice = %d / %q", result.ReturnedBytes, result.Notice)
			}
		})
	}
}

func TestFileToolExecutorRejectsInvalidArgumentsAndPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("text"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	executor := fileToolExecutor{
		config:   config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 32},
		workDir:  dir,
		language: i18n.English,
	}

	tests := []struct {
		name string
		call chat.ToolCall
		code string
	}{
		{
			name: "malformed JSON",
			call: chat.ToolCall{Function: chat.ToolCallFunction{Name: "view_file", Arguments: "{"}},
			code: "parse_view_file_arguments",
		},
		{
			name: "empty path",
			call: fileToolCall(t, "  ", nil, nil),
			code: "path_required",
		},
		{
			name: "negative offset",
			call: fileToolCall(t, file, int64Ptr(-1), nil),
			code: "invalid_offset",
		},
		{
			name: "zero length",
			call: fileToolCall(t, file, nil, int64Ptr(0)),
			code: "invalid_length",
		},
		{
			name: "missing file",
			call: fileToolCall(t, filepath.Join(dir, "missing.txt"), nil, nil),
			code: "file_not_found",
		},
		{
			name: "directory",
			call: fileToolCall(t, dir, nil, nil),
			code: "not_regular_file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := decodeFileToolError(t, executor.Execute(context.Background(), tt.call))
			if failure.Code != tt.code || failure.Error == "" {
				t.Fatalf("error = %+v, want code %q", failure, tt.code)
			}
		})
	}
}

func TestFileToolExecutorReturnsPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission test requires a non-root Unix user")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "private.txt")
	if err := os.WriteFile(path, []byte("private"), 0o000); err != nil {
		t.Fatalf("write private file: %v", err)
	}
	executor := fileToolExecutor{
		config:   config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 32},
		workDir:  dir,
		language: i18n.English,
	}

	data, err := os.ReadFile(path)
	if err == nil {
		t.Skipf("current environment can read mode 000 files: %q", data)
	}
	failure := decodeFileToolError(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, nil, nil),
	))
	if failure.Code != "permission_error" {
		t.Fatalf("error = %+v, want permission_error", failure)
	}
}

func TestFileToolPermissionAskApprovesExpectedInputs(t *testing.T) {
	for _, answer := range []string{"\n", "y\n", "Y\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file.txt")
			if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}
			var stderr bytes.Buffer
			executor := fileToolExecutor{
				config:      config.ViewFileConfig{Mode: config.ViewFileModeAsk, MaxBytes: 32},
				workDir:     dir,
				reader:      bufio.NewReader(strings.NewReader(answer)),
				stderr:      &stderr,
				interactive: true,
				language:    i18n.English,
			}

			result := decodeFileToolResult(t, executor.Execute(
				context.Background(),
				fileToolCall(t, path, nil, nil),
			))
			if result.Content == nil || *result.Content != "content" {
				t.Fatalf("content = %v, want content", result.Content)
			}
			if !strings.Contains(stderr.String(), path) {
				t.Fatalf("stderr = %q, want absolute path", stderr.String())
			}
		})
	}
}

func TestFileToolPermissionAskRefusesOtherInputs(t *testing.T) {
	tests := []struct {
		name        string
		answer      string
		interactive bool
	}{
		{name: "n", answer: "n\n", interactive: true},
		{name: "other text", answer: "yes\n", interactive: true},
		{name: "EOF", answer: "", interactive: true},
		{name: "noninteractive", answer: "y\n", interactive: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file.txt")
			if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}
			var stderr bytes.Buffer
			executor := fileToolExecutor{
				config:      config.ViewFileConfig{Mode: config.ViewFileModeAsk, MaxBytes: 32},
				workDir:     dir,
				reader:      bufio.NewReader(strings.NewReader(tt.answer)),
				stderr:      &stderr,
				interactive: tt.interactive,
				language:    i18n.English,
			}

			failure := decodeFileToolError(t, executor.Execute(
				context.Background(),
				fileToolCall(t, path, nil, nil),
			))
			if failure.Code != "permission_denied" {
				t.Fatalf("error = %+v, want permission_denied", failure)
			}
			if !tt.interactive && stderr.Len() != 0 {
				t.Fatalf("noninteractive stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestFileToolPermissionPromptIncludesRequestedRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	var stderr bytes.Buffer
	executor := fileToolExecutor{
		config:      config.ViewFileConfig{Mode: config.ViewFileModeAsk, MaxBytes: 32},
		workDir:     dir,
		reader:      bufio.NewReader(strings.NewReader("y\n")),
		stderr:      &stderr,
		interactive: true,
		language:    i18n.English,
	}
	offset := int64(2)
	length := int64(3)
	decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, &offset, &length),
	))

	prompt := stderr.String()
	for _, want := range []string{path, "offset 2", "length 3"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want %q", prompt, want)
		}
	}
}

func TestFileToolPermissionBypassDoesNotPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	var stderr bytes.Buffer
	executor := fileToolExecutor{
		config:      config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 32},
		workDir:     dir,
		reader:      bufio.NewReader(strings.NewReader("")),
		stderr:      &stderr,
		interactive: true,
		language:    i18n.English,
	}

	decodeFileToolResult(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, nil, nil),
	))
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no prompt", stderr.String())
	}
}

func TestFileToolPermissionOffDeniesExecution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	var stderr bytes.Buffer
	executor := fileToolExecutor{
		config:      config.ViewFileConfig{Mode: config.ViewFileModeOff, MaxBytes: 32},
		workDir:     dir,
		reader:      bufio.NewReader(strings.NewReader("y\n")),
		stderr:      &stderr,
		interactive: true,
		language:    i18n.English,
	}

	failure := decodeFileToolError(t, executor.Execute(
		context.Background(),
		fileToolCall(t, path, nil, nil),
	))
	if failure.Code != "permission_denied" {
		t.Fatalf("error = %+v, want permission_denied", failure)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no prompt", stderr.String())
	}
}

type fileToolResult struct {
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
	Content         *string `json:"content"`
	Notice          string  `json:"notice"`
}

type fileToolError struct {
	Error string `json:"error"`
	Code  string `json:"error_code"`
}

func fileToolCall(t *testing.T, path string, offset, length *int64) chat.ToolCall {
	t.Helper()
	arguments := map[string]any{"path": path}
	if offset != nil {
		arguments["offset"] = *offset
	}
	if length != nil {
		arguments["length"] = *length
	}
	data, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("marshal tool arguments: %v", err)
	}
	return chat.ToolCall{
		Function: chat.ToolCallFunction{
			Name:      "view_file",
			Arguments: string(data),
		},
	}
}

func decodeFileToolResult(t *testing.T, raw string) fileToolResult {
	t.Helper()
	if !json.Valid([]byte(raw)) {
		t.Fatalf("tool result is not JSON: %q", raw)
	}
	var result fileToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if result.Path == "" {
		var failure fileToolError
		_ = json.Unmarshal([]byte(raw), &failure)
		t.Fatalf("tool returned error: %+v, raw = %s", failure, raw)
	}
	return result
}

func decodeFileToolError(t *testing.T, raw string) fileToolError {
	t.Helper()
	var failure fileToolError
	if err := json.Unmarshal([]byte(raw), &failure); err != nil {
		t.Fatalf("decode tool error: %v, raw = %s", err, raw)
	}
	return failure
}

func int64Ptr(value int64) *int64 {
	return &value
}
