# 查看文件工具设计

## 目标

为模型新增 `view_file` 工具，使其可以查看当前用户有权读取的任意普通文件，并根据文件大小选择全文或字节范围。

工具默认使用 `ask` 模式。每次调用前必须由用户确认。用户可以配置为 `bypass` 或 `off`。

## 配置

新增配置键 `viewFile`，文件为 `~/.config/ask-ai/view_file.json`。

```json
{
  "mode": "ask",
  "max_bytes": 512000
}
```

- `mode` 支持 `ask`、`bypass`、`off`，默认 `ask`。
- `max_bytes` 默认 `512000`，允许范围为 `1` 到 `2097152`。
- 已有配置在加载 `_meta.json` 时自动补充配置键和工具定义。
- 配置文件使用 `0600` 权限和原子替换。

命令行配置：

```sh
askai tool viewFile mode ask
askai tool viewFile mode bypass
askai tool viewFile mode off
askai tool viewFile limit 512000
```

`limit` 只接受十进制字节数。超过 2 MiB 或不是正数时拒绝保存。

## 工具协议

工具函数名为 `view_file`。

参数：

- `path`，必填，文件路径。相对路径按 askai 当前工作目录解析。
- `offset`，可选，起始字节位置，默认 `0`，最小 `0`。
- `length`，可选，请求读取的字节数。省略时使用当前 `max_bytes`。

读取规则：

- 允许访问进程有权限读取的任意路径。
- 只处理普通文件。目录、设备、管道和其他特殊文件返回结构化错误。
- 不设置工作目录或主目录白名单。
- 单次工具结果最多包含 `max_bytes` 个内容字节。
- 请求的 `length` 超过配置值时截断到配置值。
- 文件剩余内容超过单次限制时返回 `truncated: true`、`returned_bytes`、`next_offset` 和提示文本。
- 文件小于限制或范围覆盖文件末尾时返回实际可读内容。
- `offset` 超过文件大小时返回空内容和提示文本。
- 读取只读文件内容，不修改文件。

文本和二进制：

- 返回的字节必须是有效 UTF-8，且不能包含 NUL 字节，才作为文本返回。
- 检测到非法 UTF-8 或 NUL 时，返回文件元信息和 `binary: true`，不返回正文。
- 二进制检测范围是本次实际读取的字节范围。
- 后续另做二进制查看工具，本工具不返回 Base64 或十六进制内容。

成功结果字段：

- `path`
- `resolved_path`
- `size`
- `mode`
- `modified_at`
- `offset`
- `requested_length`
- `returned_bytes`
- `truncated`
- `next_offset`
- `binary`
- `content`，仅文本结果包含
- `notice`，存在截断、空范围或二进制情况时包含

错误结果使用：

- `error`
- `error_code`

## 权限模式

### ask

- 每次 `view_file` 调用都要求确认。
- 提示写入 `stderr`，显示绝对路径、起始偏移和读取长度。
- 空输入、`y` 或 `Y` 批准。
- 其他输入、EOF 或非交互终端拒绝。
- 拒绝时向模型返回 `permission_denied`，问答继续。

### bypass

- 不显示确认提示。
- 每次调用默认批准。

### off

- 不向模型发送 `view_file` 定义或文件工具提示。
- 模型无法调用该工具。

确认输入与连续对话的后续问题必须复用同一个 buffered reader，避免读取抢占或丢失输入。

## 错误处理

以下情况返回工具错误，不终止当前问答：

- JSON 参数无效
- 路径为空
- `offset` 或 `length` 无效
- 文件不存在
- 路径不是普通文件
- 权限不足
- 读取失败
- 用户拒绝

错误和提示由 i18n 同时提供简体中文和英文。

## 实现边界

- `internal/config`：配置结构、默认值、校验、加载、保存和旧配置迁移。
- `internal/cli/file_tool.go`：工具定义、确认、路径解析、范围读取和结果序列化。
- `internal/cli/tool.go`：`mode` 和 `limit` 命令。
- `internal/cli/cli.go`：合并网页与文件工具，复用输入 reader，并只在工具启用时发送定义和提示。
- `internal/i18n`、帮助和用户文档：同步配置和行为。

文件访问逻辑不进入 `internal/chat`，OpenAI 工具协议细节不进入文件读取实现。

## 测试

- 默认配置、配置校验、保存和旧 `_meta.json` 迁移。
- `ask` 模式 Enter、`y`、`Y`、拒绝、EOF 和非交互终端。
- `bypass` 不提示且直接执行。
- `off` 不发送工具定义。
- 全文、指定范围、跨范围续读、超过上限截断和越界偏移。
- UTF-8 文本、空文件、非法 UTF-8、NUL、目录、缺失文件和权限错误。
- `mode` 和 `limit` 命令，以及 1 字节和 2 MiB 边界。
- 连续对话中确认输入与后续问题不丢失。
- `go test ./...`、`go vet ./...` 和 `go test -race ./...`。
