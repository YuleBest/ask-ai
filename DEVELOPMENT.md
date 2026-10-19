# ask-ai 开发指南

## 环境

- Go 1.22 或更高版本。
- 可选：`gopls`，用于语言服务检查。
- 本地真实模型和 Exa 测试需要可用的用户配置，不得把凭据写入仓库。

安装依赖：

```sh
go mod download
```

## 常用命令

```sh
go fmt ./...
go test ./...
go test -race ./...
go vet ./...
gopls check ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build ./...
go build -trimpath -o ~/.local/bin/askai ./cmd/askai
~/.local/bin/askai version
```

`gopls check ./...` 在某些版本中不接受 `./...` 作为文件参数。此时传入修改过的 `.go` 文件。

CI 定义在 `.github/workflows/ci.yml`，会运行测试、竞态检查、`vet` 和 Windows/Darwin 交叉构建。

发布由 `.github/workflows/release.yml` 和 `.goreleaser.yaml` 完成。推送 `v*` 标签后，工作流先运行测试，再构建 Linux、macOS 和 Windows 的 amd64/arm64 产物，附带 README、LICENSE、ZIP/TAR.GZ 压缩包和 `checksums.txt`，最后创建 GitHub Release。含预发布后缀的标签会自动标记为 Pre-Release。

发布前确认版本号、文档和标签一致：

```sh
go run ./cmd/askai version
git tag -a v0.1.0-alpha.1 -m "askai v0.1.0-alpha.1"
git push origin main v0.1.0-alpha.1
```

可在本地验证 GoReleaser 配置和完整快照：

```sh
goreleaser check
goreleaser release --snapshot --clean
```

版本号定义在 `internal/cli/version.go`。修改版本时同步更新 `README.md` 和 `PROJECT.md`，并运行 `go run ./cmd/askai version` 验证输出。

## 本地配置

默认路径：

```text
~/.config/ask-ai/models.json
~/.config/ask-ai/system_prompt.txt
~/.config/ask-ai/web_search.json
~/.config/ask-ai/_meta.json
~/.cache/ask-ai/sessions/
~/.cache/ask-ai/last.json
~/.cache/ask-ai/terminals/
```

测试必须通过临时目录、`t.TempDir()`、`XDG_CACHE_HOME` 或测试配置路径隔离，不得修改真实用户数据。

## 运行结构

1. `cmd/askai/main.go` 创建可取消的上下文并调用 `cli.Run`。
2. `internal/cli` 解析参数、加载配置、选择会话和模型。
3. `internal/config` 负责 `_meta.json`、模型、提示词和搜索配置。
4. `internal/chat` 发送 OpenAI 兼容请求并解析 SSE、usage 和 tool calls。
5. `internal/cli` 执行工具、追加 tool message，再请求模型，最多 8 轮。
6. `internal/render` 将最终正文增量渲染到 `stdout`。
7. `internal/conversation` 保存 user 和 assistant 正文。

任何内部错误和诊断都写 `stderr`。最终正文只能写 `stdout`。

## 包职责

```text
cmd/askai/              进程入口
internal/cli/           命令、交互、提示词组合和输出
internal/chat/          Chat Completions、SSE、usage、工具协议
internal/config/        配置、元数据、搜索配置和模板变量
internal/conversation/  会话持久化、当前任务和绑定
internal/fileutil/      跨平台原子替换
internal/render/        Markdown 渲染
internal/terminaltext/  终端控制字符合理器
internal/websearch/     Exa API
```

新增功能优先放入对应领域包。只有 CLI 编排逻辑放在 `internal/cli`。

## 新增命令

1. 在 `internal/cli/cli.go` 增加命令分发。
2. 在独立文件中实现命令，避免继续膨胀入口文件。
3. 在 `internal/cli/help.go` 增加标准帮助，内容包含用法、命令、参数、说明和示例。
4. 支持 `help <topic>` 和 `<command> --help`。
5. 补充成功路径、参数错误和文件错误测试。
6. 更新 README 的用户命令表。

## 修改模型协议

- 请求和响应字段放在 `internal/chat/client.go`。
- SSE 必须按行解析并处理 `[DONE]`、错误对象、usage 和工具调用分片。
- 工具调用按 `index` 合并，参数是分片 JSON 字符串，结束后才能解析。
- 修改协议时更新 `internal/chat/client_test.go`，使用 `httptest` 模拟完整 SSE。

## 修改工具

- 工具定义放在 `_meta.json` 的 `tools` 字段。
- 配置键必须能在默认元数据中解析。
- 当前网络搜索工具为 `webSearch`，provider 仅支持 `exa`。
- 关闭工具时向 `web_search.json` 写入 `"use": false`。
- 工具关闭后，模型请求的 `tools` 必须为空。
- Exa 客户端实现位于 `internal/websearch`，不得在 CLI 中直接拼 HTTP 请求。
- 搜索和抓取结果既要回传模型，也要在 `stderr` 输出经过截断和控制字符清洗的预览。

## 修改任务存储

- 会话 ID 为 32 位小写十六进制。
- 支持 6 到 32 位前缀解析，歧义时拒绝操作。
- `last.json` 只记录当前任务。
- 会话读写使用 `.lock` 文件锁，更新时合并并发消息并递增 revision。
- 会话文件内容 ID 必须与文件名一致。
- 删除当前任务时清除当前指针。
- 会话只允许 `user` 和 `assistant` 正文，不保存推理、图片或工具原文。
- 所有缓存测试必须使用临时 `XDG_CACHE_HOME`。

## 修改系统提示词

- 默认提示词模板中包含环境变量。
- 支持变量：`{{os}}`、`{{arch}}`、`{{date}}`、`{{time}}`、`{{timezone}}`、`{{datetime}}`。
- 兼容别名：`{{system}}`、`{{architecture}}`。
- 变量在每次请求前动态渲染。
- 只升级与旧内置默认值完全一致的提示词，不覆盖用户自定义内容。

## 测试策略

- 纯逻辑：表驱动测试。
- HTTP/SSE：`httptest.Server`。
- 配置和缓存：`t.TempDir()`。
- TTY 行为：可注入的 reader/writer，必要时使用伪终端人工验证。
- 并发和流式改动：运行 race 测试。
- 真实模型和 Exa 调用仅用于人工端到端验证。
- 平台相关代码必须通过 Windows 和 Darwin 交叉构建。

提交前至少确认：

1. `gofmt` 没有产生新改动。
2. `go test ./...` 通过。
3. `go vet ./...` 通过。
4. 涉及并发或流式逻辑时 `go test -race ./...` 通过。
5. `--help` 与实际参数一致。
6. git diff 中没有密钥、用户缓存或无关格式化。
