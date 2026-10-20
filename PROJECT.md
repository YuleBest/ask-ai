# ask-ai 项目说明

## 基本信息

- 项目名：`ask-ai`
- 可执行文件：`askai`
- Go 模块：`github.com/YuleBest/ask-ai`
- 语言与版本：Go 1.22+
- 当前版本：`v0.1.0-alpha.1`
- 包管理：Go modules
- 主要依赖：`golang.org/x/term`、`golang.org/x/sys`

## 项目目标

`askai` 是一个面向终端的轻量 AI 助手。它使用 OpenAI 兼容的 Chat Completions 接口，提供流式正文、任务追问、连续对话、图片输入、Exa 网络搜索、会话管理、简单 Markdown 渲染和中英文界面。

项目优先保证以下体验：

- 启动和调用简单，适合随手提问。
- 正文、诊断、思考和工具信息使用清晰的标准输出与错误输出边界。
- 配置保存在标准 XDG 目录，不依赖项目目录。
- 模型、工具和终端展示逻辑分层，后续可以独立扩展。

## 功能边界

当前支持：

- OpenAI 兼容接口和 SSE 流式响应。
- 模型工具调用，当前工具为 `webSearch`，当前搜索 provider 为 Exa。
- 多 provider、多模型配置。
- 任务列表、查看、切换、批量删除和终端绑定。
- 图片作为 data URL 随当前问题发送。
- 程序输出与提示词语言独立配置，支持简体中文和英文。
- 系统提示词变量、多语言自定义提示词和旧文本提示词迁移。

当前不支持：

- 图形界面或常驻服务。
- Anthropic、Gemini 等非 OpenAI 兼容协议。
- Exa 之外的网络搜索 provider。
- 在会话历史中保存思考过程、图片二进制或完整工具原文。

## 行为约定

- `stdout` 只输出最终回答正文。
- 思考、工具调用状态、搜索摘要、统计和交互提示写入 `stderr`。
- 非交互终端不输出 ANSI 颜色。
- 推理内容和图片数据不写入会话历史。
- 会话文本保存在 `~/.cache/ask-ai`。
- 配置保存在 `~/.config/ask-ai`，权限按敏感配置处理。
- 语言默认继承 `LANGUAGE`、`LC_ALL`、`LC_MESSAGES`、`LANG`，`language.json` 可分别覆盖输出和提示词语言。
- 系统提示词保存在 `system_prompt.json`，按 `zh-CN` 和 `en` 映射。
- 会话写入使用跨进程锁和消息合并，避免并发追问丢记录。
- 模型、工具、图片和流式响应都有资源上限，错误响应不会被当作完整回答。
- 不信任的模型、网页和工具文本在进入终端前清理控制序列。
- API key 不得提交到仓库或打印到终端。

## 代码结构

```text
cmd/askai/              程序入口
internal/cli/           命令分发、交互和输出编排
internal/chat/          OpenAI 兼容客户端、SSE 和工具协议
internal/config/        配置、元数据和系统提示词
internal/conversation/  任务、当前指针和终端绑定
internal/fileutil/      跨平台原子替换
internal/i18n/         语言检测、消息目录和运行时语言
internal/render/        Markdown 终端渲染
internal/terminaltext/  终端控制字符合理器
internal/websearch/     Exa 网络搜索与抓取
```

接口协议细节放在 `internal/chat`，提供商实现放在对应领域包，命令层只负责组合。
