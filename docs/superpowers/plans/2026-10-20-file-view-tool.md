# 文件查看工具实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `askai` 增加可配置的 `view_file` 模型工具，使其能查看任意可读普通文件的元信息和 UTF-8 文本范围。

**Architecture:** `internal/config` 负责 `view_file.json` 的默认值、校验、原子保存和 `_meta.json` 注册；`internal/cli` 负责工具定义、权限确认、路径解析、范围读取、结果预览及与 `webSearch` 的组合。连续对话和文件确认共用同一个 `bufio.Reader`，确保确认输入不会吞掉后续问题。

**Tech Stack:** Go 1.22、`encoding/json`、`bufio`、`io`、`os`、`unicode/utf8`、现有 `internal/chat` 工具协议和 `internal/i18n`。

**Spec:** `docs/superpowers/specs/2026-10-20-file-view-tool-design.md`

## Global Constraints

- `view_file` 默认模式为 `ask`，默认单次内容上限为 `512000` 字节。
- 模式只允许 `ask`、`bypass`、`off`。
- `max_bytes` 只允许 `1` 到 `2097152`。
- 只读取普通文件，不修改文件，不设置目录白名单。
- 相对路径按 askai 进程当前工作目录解析。
- 单次最多返回 `max_bytes` 个内容字节。
- 只有有效 UTF-8 且不含 NUL 的读取范围才返回 `content`。
- `ask` 模式中 Enter、`y`、`Y` 批准；其他输入、EOF 或非交互输入拒绝。
- 确认提示写 `stderr`，最终回答正文仍只写 `stdout`。
- `off` 模式不发送 `view_file` 定义，也不追加文件工具提示。
- 所有用户可见文案同时提供简体中文和英文。
- 配置文件使用 `0600` 权限和现有原子写入函数。
- 不覆盖工作树中已有的截图和流式输出改动；未获用户要求不提交混合改动。

---

### Task 1: 文件查看配置模型与持久化

**Files:**
- Create: `internal/config/viewfile.go`
- Create: `internal/config/viewfile_test.go`

**Interfaces:**
- Consumes: `checkSecretFile`、`writeExclusive`、`writeReplace`、`i18n.Errorf`。
- Produces: `ViewFileMode`、`ViewFileConfig`、`DefaultViewFileConfig()`、`Validate()`、`LoadOrCreateViewFile(path) (ViewFileConfig, bool, error)`、`EnsureViewFile(path) error`、`SaveViewFile(path, config) error`、`DefaultViewFileMaxBytes`、`MaxViewFileMaxBytes`。

- [ ] **Step 1: 写默认文件、权限和加载测试**

```go
func TestLoadOrCreateViewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "view_file.json")
	got, created, err := LoadOrCreateViewFile(path)
	if err != nil {
		t.Fatalf("LoadOrCreateViewFile() error = %v", err)
	}
	if !created {
		t.Fatal("LoadOrCreateViewFile() did not report creation")
	}
	want := ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: 512000}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config -run TestLoadOrCreateViewFile -count=1`

Expected: FAIL，提示 `undefined: LoadOrCreateViewFile`。

- [ ] **Step 3: 实现配置类型和默认文件**

```go
const (
	DefaultViewFileMaxBytes int64 = 512000
	MaxViewFileMaxBytes     int64 = 2 * 1024 * 1024
)

type ViewFileMode string

const (
	ViewFileModeAsk    ViewFileMode = "ask"
	ViewFileModeBypass ViewFileMode = "bypass"
	ViewFileModeOff    ViewFileMode = "off"
)

type ViewFileConfig struct {
	Mode     ViewFileMode `json:"mode"`
	MaxBytes int64        `json:"max_bytes"`
}
```

实现 `LoadOrCreateViewFile` 时先调用 `checkSecretFile`，不存在时写入默认 JSON，存在时解析并调用 `Validate`。

- [ ] **Step 4: 写校验与保存测试**

```go
func TestViewFileConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config ViewFileConfig
		ok     bool
	}{
		{name: "ask", config: ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: 1}, ok: true},
		{name: "bypass", config: ViewFileConfig{Mode: ViewFileModeBypass, MaxBytes: MaxViewFileMaxBytes}, ok: true},
		{name: "off", config: ViewFileConfig{Mode: ViewFileModeOff, MaxBytes: 512000}, ok: true},
		{name: "bad mode", config: ViewFileConfig{Mode: "always", MaxBytes: 1}},
		{name: "zero", config: ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: 0}},
		{name: "too large", config: ViewFileConfig{Mode: ViewFileModeAsk, MaxBytes: MaxViewFileMaxBytes + 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err == nil) != tt.ok {
				t.Fatalf("Validate() error = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}
```

- [ ] **Step 5: 实现校验和原子保存**

`Validate` 检查模式和字节范围。`SaveViewFile` 先校验，再使用 `json.MarshalIndent`、追加换行并调用 `writeReplace`。`EnsureViewFile` 对已存在文件只做 `checkSecretFile`，不存在则写入默认值。

- [ ] **Step 6: 运行配置测试**

Run: `go test ./internal/config -run ViewFile -count=1`

Expected: PASS。

### Task 2: 元数据注册和默认配置创建

**Files:**
- Modify: `internal/config/meta.go`
- Modify: `internal/config/meta_test.go`
- Modify: `internal/cli/config.go`
- Modify: `internal/cli/config_test.go`

**Interfaces:**
- Consumes: `ViewFileConfig` 和 `EnsureViewFile`。
- Produces: `_meta.json` 中的 `viewFile` 配置键、`viewFile` 工具条目和 `view_file` 默认定义；`askai config` 自动创建 `view_file.json`。

- [ ] **Step 1: 扩展默认元数据测试**

在 `TestLoadOrCreateMeta` 的配置键列表加入 `viewFile`，并断言：

```go
tool, ok := meta.Tools["viewFile"]
if !ok || tool.Config != "viewFile" {
	t.Fatalf("viewFile tool = %+v, exists = %v", tool, ok)
}
if len(tool.Definitions) != 1 || tool.Definitions[0].Name != "view_file" {
	t.Fatalf("view_file definitions = %+v", tool.Definitions)
}
```

- [ ] **Step 2: 写旧元数据升级测试**

构造不包含 `viewFile` 的合法旧 `_meta.json`，调用 `LoadOrCreateMeta` 后断言配置路径为 `view_file.json` 且工具定义被补齐。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/config -run 'TestLoadOrCreateMeta|TestLoadMetaAddsFileTool' -count=1`

Expected: FAIL，缺少 `viewFile` 配置和工具。

- [ ] **Step 4: 注册元数据**

在 `defaultMeta().Configs` 增加：

```go
"viewFile": {
	File:        "view_file.json",
	Description: "文件查看工具配置",
},
```

在 `defaultMeta().Tools` 增加：

```go
"viewFile": {
	Description: "让模型查看本机普通文件的元信息和 UTF-8 文本内容",
	Config:      "viewFile",
	Operations:  []string{"mode", "limit"},
	Definitions: defaultViewFileToolDefinitions(),
}
```

`view_file` 参数对象必须包含必填 `path`、可选非负 `offset`、可选正整数 `length`，并设置 `additionalProperties: false`。

- [ ] **Step 5: 在配置命令中创建 view_file.json**

在 `ensureDefaultConfigFiles` 中查 `meta.PathFor("viewFile", configDir)` 并调用 `config.EnsureViewFile`；在 `resolveConfigPath` 的创建分支加入 `"viewFile"`。

- [ ] **Step 6: 更新配置命令测试**

在 `TestConfigListCreatesFiles` 中加入键 `viewFile` 和文件 `view_file.json`。

Run: `go test ./internal/config ./internal/cli -run 'TestLoadOrCreateMeta|TestLoadMeta|TestConfigListCreatesFiles' -count=1`

Expected: PASS。

### Task 3: `view_file` 工具定义和本地化提示

**Files:**
- Create: `internal/cli/file_tool.go`
- Create: `internal/cli/file_tool_test.go`
- Modify: `internal/i18n/catalog.go`

**Interfaces:**
- Consumes: `config.Meta` 中的 `viewFile` 定义、`config.DefaultMeta()`、`i18n.PromptLanguage()`。
- Produces: `fileToolDefinitionsFromMeta(meta config.Meta, language i18n.Language) ([]chat.Tool, error)`、`addFileToolInstructions(prompt string, language i18n.Language) string`、`fileToolExecutor`。

- [ ] **Step 1: 写定义和提示词测试**

```go
func TestFileToolDefinitionsUseMetadata(t *testing.T) {
	tools, err := fileToolDefinitionsFromMeta(config.DefaultMeta(), i18n.English)
	if err != nil {
		t.Fatalf("fileToolDefinitionsFromMeta() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Function.Name != "view_file" {
		t.Fatalf("tools = %+v", tools)
	}
	if !strings.Contains(tools[0].Function.Description, "regular files") {
		t.Fatalf("description = %q", tools[0].Function.Description)
	}
}
```

另写中文定义、自定义描述保留和 `addFileToolInstructions` 中英文测试。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cli -run 'TestFileToolDefinitions|TestAddFileToolInstructions' -count=1`

Expected: FAIL，文件和函数不存在。

- [ ] **Step 3: 实现定义提取和默认定义本地化**

从 `_meta.json` 读取 `viewFile.Definitions`，沿用网页工具的 JSON 参数校验方式。只有定义仍与默认值完全一致时才替换中文描述和参数说明；用户自定义描述必须保留。

中文提示词：

```text
你可以使用 view_file 查看本机普通文件的元信息和 UTF-8 文本内容。
- 用户提到本地文件、路径、配置、源码或日志时，相关情况下主动查看。
- 根据任务选择 offset 和 length，不要无目的地读取大文件。
- 结果被截断时按 next_offset 继续；binary: true 时不要声称已经看到正文。
```

英文提示词表达同样约束。`addFileToolInstructions` 先 `TrimSpace`，空提示词直接返回工具提示，否则用两个换行追加。

- [ ] **Step 4: 运行定义测试**

Run: `go test ./internal/cli -run 'TestFileToolDefinitions|TestAddFileToolInstructions' -count=1`

Expected: PASS。

### Task 4: 文件元信息、范围读取和二进制检测

**Files:**
- Modify: `internal/cli/file_tool.go`
- Modify: `internal/cli/file_tool_test.go`
- Modify: `internal/i18n/catalog.go`

**Interfaces:**
- Consumes: `ViewFileConfig`、`chat.ToolCall`、`readQuestionLine`、`isInteractiveInput`。
- Produces: `fileToolExecutor.Execute(context.Context, chat.ToolCall) string`。

执行器字段固定为：

```go
type fileToolExecutor struct {
	config      config.ViewFileConfig
	workDir     string
	reader      *bufio.Reader
	stderr      io.Writer
	interactive bool
	language    i18n.Language
}
```

- [ ] **Step 1: 写文本范围与截断测试**

使用 `bypass` 模式读取临时文件，断言：

```go
file := filepath.Join(t.TempDir(), "sample.txt")
os.WriteFile(file, []byte("abcdef"), 0o600)
executor := fileToolExecutor{
	config: config.ViewFileConfig{Mode: config.ViewFileModeBypass, MaxBytes: 4},
}
raw := executor.Execute(context.Background(), chat.ToolCall{
	Function: chat.ToolCallFunction{
		Name:      "view_file",
		Arguments: `{"path":"` + file + `","offset":1,"length":3}`,
	},
})
```

解析 JSON 后断言 `content == "bcd"`、`requested_length == 3`、`returned_bytes == 3`、`truncated == true`、`next_offset == 4`。

再用 `offset: 4` 读取，断言 `"ef"` 且 `truncated == false`。

- [ ] **Step 2: 写范围上限、空文件、越界和相对路径测试**

覆盖：

- `length` 大于 `MaxBytes` 时结果中的 `requested_length` 截断到 `MaxBytes`。
- 空文件返回 `content: ""`、`returned_bytes: 0`、`binary: false` 和 `notice`。
- `offset == size` 与 `offset > size` 返回空内容及 `notice`。
- `workDir` 注入临时目录时，相对路径的 `resolved_path` 是该目录下的绝对路径。
- 不改写原文件内容。

- [ ] **Step 3: 写二进制和特殊文件测试**

表驱动覆盖：

```go
{name: "invalid utf8", data: []byte{0xff, 0xfe}},
{name: "nul byte", data: []byte("a\x00b")},
```

断言 `binary: true`、`returned_bytes` 正确、结果对象没有 `content` 字段。目录返回 `not_regular_file`，不存在路径返回 `file_not_found`。在非 Windows 且非 root 环境写权限错误测试，权限不足返回 `permission_error`。

- [ ] **Step 4: 运行测试确认失败**

Run: `go test ./internal/cli -run 'TestFileToolExecutor' -count=1`

Expected: FAIL，`Execute` 尚未实现。

- [ ] **Step 5: 实现参数解析和路径解析**

参数结构使用指针区分省略值：

```go
type fileToolArguments struct {
	Path   string `json:"path"`
	Offset *int64 `json:"offset,omitempty"`
	Length *int64 `json:"length,omitempty"`
}
```

offset 缺省为 0，负值返回 `invalid_offset`；length 省略时使用配置上限，显式值必须大于 0，大于配置上限时封顶。

相对路径先与 `workDir` 连接，再调用 `filepath.Abs` 和 `filepath.Clean`。`workDir` 为空时使用 `os.Getwd`。

- [ ] **Step 6: 实现普通文件检查和范围读取**

先 `os.Stat`，再要求 `info.Mode().IsRegular()`。打开文件后使用 `io.NewSectionReader` 和 `io.LimitReader` 读取最多有效 `requested_length` 字节。错误统一返回：

```go
toolErrorJSON(i18n.TFor(e.language, "文件不存在: %s", resolved), "file_not_found")
```

文本判定使用 `utf8.Valid(data) && !bytes.ContainsRune(data, 0)`。文本结果必须包含 `content` 字段，包括空字符串；二进制结果的 JSON 不包含 `content` 键。

- [ ] **Step 7: 实现成功结果**

结果至少包含规格列出的字段。`mode` 使用 `info.Mode().String()`，`modified_at` 使用 UTC RFC3339Nano。截断、越界、空范围和二进制情况设置本地化 `notice`。

- [ ] **Step 8: 运行范围读取测试**

Run: `go test ./internal/cli -run 'TestFileToolExecutor' -count=1`

Expected: PASS。

### Task 5: `ask`、`bypass`、`off` 权限模式

**Files:**
- Modify: `internal/cli/file_tool.go`
- Modify: `internal/cli/file_tool_test.go`
- Modify: `internal/i18n/catalog.go`

**Interfaces:**
- Consumes: `isInteractiveInput`、`readQuestionLine` 和共享 `bufio.Reader`。
- Produces: 权限拒绝错误码 `permission_denied`，以及不读取输入、不打印提示的 bypass 路径。

- [ ] **Step 1: 写确认输入表驱动测试**

```go
answers := []string{"\n", "y\n", "Y\n"}
```

每个答案都必须读出文件正文。`"n\n"`、`""`、任意其他文本和非交互状态都必须返回 `permission_denied`，且不能读取文件正文。

- [ ] **Step 2: 写提示和 bypass 测试**

`ask` 模式断言 `stderr` 包含绝对路径、offset 和有效读取长度。`bypass` 模式传空 `strings.Reader`，断言不输出提示且成功读取。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/cli -run 'TestFileToolPermission' -count=1`

Expected: FAIL，尚未执行确认。

- [ ] **Step 4: 实现权限检查**

参数和路径校验完成后执行：

- `off` 返回 `error_code: "permission_denied"`，作为防御性行为。
- `bypass` 直接继续。
- `ask` 在 `interactive == false` 时不读输入并拒绝。
- 交互 ask 先向 `stderr` 写提示，再调用 `readQuestionLine`；EOF、context canceled 之外的读取错误也拒绝，只有 trim 后为 `""`、`"y"` 或 `"Y"` 才批准。

提示和错误文案全部使用 `i18n`，路径经过 `terminaltext.Clean`。

- [ ] **Step 5: 运行权限测试**

Run: `go test ./internal/cli -run 'TestFileToolPermission' -count=1`

Expected: PASS。

### Task 6: `askai tool viewFile mode|limit`

**Files:**
- Modify: `internal/cli/tool.go`
- Modify: `internal/cli/tool_test.go`
- Modify: `internal/cli/help.go`
- Modify: `internal/cli/help_test.go`
- Modify: `internal/i18n/catalog.go`

**Interfaces:**
- Consumes: `meta.PathFor("viewFile", configDir)`、`config.LoadOrCreateViewFile`、`config.SaveViewFile`。
- Produces: `runViewFileTool(args, toolID, operation, meta, configDir, stdout, stderr) int` 和 `parseViewFileLimit(value) (int64, error)`。

- [ ] **Step 1: 写工具列表和模式命令测试**

`askai tool ls` 输出必须包含：

```text
viewFile
viewFile
mode,limit
```

依次执行 `mode ask`、`mode bypass`、`mode off`，每次重新加载文件并断言模式已保存。无效模式返回非零并保留原配置。

- [ ] **Step 2: 写 limit 边界测试**

成功值：`1`、`512000`、`2097152`。

失败值：`0`、`-1`、`2097153`、`1.5`、空字符串。

失败时配置文件中的 `max_bytes` 不变。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/cli -run 'TestToolViewFile|TestToolList' -count=1`

Expected: FAIL，未知配置类型或操作。

- [ ] **Step 4: 实现分派和命令**

在 `runTool` 的 `switch tool.Config` 中加入 `"viewFile"`。`mode` 和 `limit` 都要求恰好三个参数。十进制解析只接受由 `0` 到 `9` 组成的字符串，再用 `strconv.ParseInt` 防止溢出。

成功输出使用颜色和 i18n，分别显示新的模式和字节上限。

- [ ] **Step 5: 更新 tool 帮助**

`printToolUsage` 增加：

```text
askai tool viewFile mode <ask|bypass|off>
askai tool viewFile limit <bytes>
```

说明默认值、允许范围、文件路径和权限行为。更新 `help_test.go`，确保 `askai tool --help` 包含 `viewFile` 和 `2097152`。

- [ ] **Step 6: 运行 CLI 工具测试**

Run: `go test ./internal/cli -run 'TestTool|TestHelp' -count=1`

Expected: PASS。

### Task 7: 集成模型请求与共享输入 reader

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`
- Modify: `internal/cli/web_tools.go`

**Interfaces:**

- Consumes: `fileToolDefinitionsFromMeta`、`fileToolExecutor`、`addFileToolInstructions`、`webToolDefinitionsFromMeta`、`webToolExecutor`。
- Produces: 组合后的 `[]chat.Tool` 和单个 `executeTool func(context.Context, chat.ToolCall) string`；`runBoundQuestions` 改为接收 `*bufio.Reader`。

- [ ] **Step 1: 写 `off` 不发送定义的测试**

在测试配置中将 `view_file.json` 写成 `{"mode":"off","max_bytes":512000}`，关闭网页搜索并启动模型请求。断言请求 JSON 的 `tools` 为空，system prompt 不含 `view_file`。

- [ ] **Step 2: 写 ask/bypass 发送定义的测试**

分别使用 `ask` 和 `bypass`，断言请求包含一个名为 `view_file` 的工具，system prompt 包含文件工具说明。

- [ ] **Step 3: 写共享 reader 回归测试**

绑定连续对话时输入：

```text
y
第二个问题
/exit
```

第一轮模型请求 `view_file`，用户输入 `y` 批准；该轮回答结束后，绑定循环必须从同一个 `bufio.Reader` 读到“第二个问题”。服务端请求记录必须证明第二个问题没有被确认逻辑吞掉。

- [ ] **Step 4: 运行测试确认失败**

Run: `go test ./internal/cli -run 'TestRunFileTool|TestBoundFileConfirmationSharesReader' -count=1`

Expected: FAIL，文件工具尚未进入请求。

- [ ] **Step 5: 加载配置并组合工具**

在正常问答路径中：

1. 通过 `_meta.json` 找到 `viewFile` 路径。
2. 调用 `LoadOrCreateViewFile`。
3. `mode != off` 时加入定义和提示词。
4. 创建执行器并复用后续绑定循环的 reader。
5. 按工具名分派 `web_search`、`web_fetch`、`view_file`；未知工具返回 `unknown_tool`。

组合后的工具数量和顺序不影响既有网页工具测试。

- [ ] **Step 6: 重构连续对话输入**

在 `runWithTerminalInput` 早期创建：

```go
stdinReader := bufio.NewReader(stdin)
```

文件执行器和 `runBoundQuestions` 使用同一指针。将：

```go
func runBoundQuestions(ctx context.Context, stdin io.Reader, ...)
```

改为：

```go
func runBoundQuestions(ctx context.Context, reader *bufio.Reader, ...)
```

不要创建第二个 reader。

- [ ] **Step 7: 更新配置创建提示**

首次创建模型配置时，`printCreatedConfig` 也列出 `view_file.json`。保持现有网页搜索配置提示行为不变。

- [ ] **Step 8: 运行 CLI 集成测试**

Run: `go test ./internal/cli -run 'TestRunFileTool|TestBoundFileConfirmationSharesReader|TestRunWebSearchToolToggle|TestBind' -count=1`

Expected: PASS。

### Task 8: 终端预览、错误映射和文档

**Files:**
- Modify: `internal/cli/web_tools.go`
- Modify: `internal/cli/web_tools_test.go`
- Modify: `internal/cli/help.go`
- Modify: `internal/cli/help_test.go`
- Modify: `internal/i18n/catalog.go`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `PROJECT.md`
- Modify: `DEVELOPMENT.md`

**Interfaces:**
- Consumes: 文件工具状态和 JSON 结果。
- Produces: 不泄露正文的 `stderr` 预览、稳定的错误说明、用户配置文档。

- [ ] **Step 1: 写状态与预览测试**

`toolStatusText` 对 `view_file` 输出“查看文件: ...”，控制字符被清理。成功预览只显示路径、文件大小、返回范围和二进制/截断状态，不打印文件正文。权限拒绝、不存在、非普通文件、读取失败都有稳定的本地化错误行。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cli -run 'TestToolStatusText|TestPrintFileTool' -count=1`

Expected: FAIL，尚无 `view_file` 分支。

- [ ] **Step 3: 实现预览和错误映射**

在 `toolStatusText`、`printToolResultPreview`、`toolErrorOutputText` 中加入 `view_file` 分支。所有路径和提示经过 `terminaltext.Clean`；正文不进入预览。

- [ ] **Step 4: 更新用户文档**

README 增加简短说明：

```sh
askai tool viewFile mode ask
askai tool viewFile mode bypass
askai tool viewFile mode off
askai tool viewFile limit 512000
```

说明 `ask`、`bypass`、`off`、`1` 到 `2097152` 字节范围和 `~/.config/ask-ai/view_file.json`。README 中文保持 2000 字以内。

`PROJECT.md` 把当前工具更新为 `webSearch` 和 `viewFile`。`DEVELOPMENT.md` 的本地配置、运行结构、工具修改和测试策略加入文件工具。

- [ ] **Step 5: 更新总帮助**

`printUsage` 的文件列表加入 `view_file.json`，说明模型可在配置允许时读取本机文本文件。不要扩大成完整的目录浏览或二进制说明。

- [ ] **Step 6: 运行文档相关测试和字数检查**

Run: `go test ./internal/cli ./internal/i18n -count=1`

Run: `python3 - <<'PY'\nfrom pathlib import Path\nprint(len(Path('README.md').read_text()))\nPY`

Expected: 测试通过，README 字符数小于 2000。

### Task 9: 完整验证与人工冒烟

**Files:**
- Verify: 所有修改文件

**Interfaces:**
- Consumes: 已完成实现。
- Produces: 可复现的测试、静态检查、竞态检查和真实 CLI 冒烟结论。

- [ ] **Step 1: 格式化**

Run: `gofmt -w internal/config/viewfile.go internal/config/viewfile_test.go internal/config/meta.go internal/config/meta_test.go internal/cli/file_tool.go internal/cli/file_tool_test.go internal/cli/tool.go internal/cli/tool_test.go internal/cli/cli.go internal/cli/cli_test.go internal/cli/web_tools.go internal/cli/web_tools_test.go internal/cli/config.go internal/cli/config_test.go internal/cli/help.go internal/cli/help_test.go internal/i18n/catalog.go`

Run: `git diff --check`

Expected: 无格式错误和空白错误。

- [ ] **Step 2: 运行完整测试**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 3: 运行静态检查**

Run: `go vet ./...`

Run: `gopls check internal/config/viewfile.go internal/config/meta.go internal/cli/file_tool.go internal/cli/tool.go internal/cli/cli.go internal/cli/web_tools.go internal/cli/config.go internal/cli/help.go`

Expected: 两条命令通过；若 `gopls` 未安装，记录未运行原因，不伪造结果。

- [ ] **Step 4: 运行竞态测试**

Run: `go test -race ./...`

Expected: PASS。

- [ ] **Step 5: 构建可执行文件**

Run: `go build -trimpath -o /tmp/askai ./cmd/askai`

Run: `/tmp/askai tool --help`

Expected: 构建成功，帮助包含 `viewFile mode` 和 `viewFile limit`。

- [ ] **Step 6: 临时目录人工冒烟**

使用隔离的 `XDG_CONFIG_HOME` 和 `XDG_CACHE_HOME`，依次验证：

```sh
askai config list
askai tool viewFile mode bypass
askai tool viewFile limit 64
cat ~/.config/ask-ai/view_file.json
```

再通过 `httptest` 集成测试验证真实模型工具请求；不调用外部模型，不读取真实用户文件作为测试夹具。

- [ ] **Step 7: 检查改动范围**

Run: `git status --short`

Run: `git diff --stat`

Expected: 只包含文件查看功能、必要的帮助文档，以及实现前已经存在的截图和流式输出改动；没有密钥、真实用户内容或无关格式化。
