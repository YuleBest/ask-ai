# askai

Go 命令行 AI 助手，通过 OpenAI 兼容接口回答问题。支持追问、图片、Exa 搜索及任务管理。

## 安装

预编译版本见 [GitHub Releases](https://github.com/YuleBest/ask-ai/releases)。

Go 1.22+ 源码安装：

```sh
go build -trimpath -o ~/.local/bin/askai ./cmd/askai
```

版本 `askai version`、`--version` 或 `-v`。

## 使用

```sh
askai "解释 Go 的 context"
askai "再举一个例子" -f
askai "这张图是什么" -p ./logo.png
askai "分析这个报错" --show-think
askai -b "进入连续对话"
```

`-f [hash]` 追问任务。`-b` 进入连续对话，`/exit` 或 `Ctrl+C` 退出并解绑。`-p` 可重复附图片，支持 PNG、JPEG、GIF、WebP，最多 8 张。

## 任务与模型

任务哈希可使用 6-32 位前缀。

```sh
askai task list          # tl
askai task view [hash]   # tv，省略时看上一次
askai task switch <hash> # ts
askai task remove [hash...] # tr，省略时交互多选
```

```sh
askai ml
askai mu local/cn:glm-5.3-flash
askai md local/cn:deepseek-v4.1-flash
```

## 配置

配置位于 `~/.config/ask-ai`：

- `models.json`：模型提供商和模型。
- `system_prompt.txt`：系统提示词。
- `web_search.json`：网络搜索配置。
- `_meta.json`：配置和工具定义。

```sh
askai config                 # 列出配置
askai config edit models     # 使用 nano 编辑
askai config view webSearch
```

`config view` 默认隐藏 `key`，加 `--show-secrets` 显示明文。

系统提示词支持 `{{os}}`、`{{arch}}`、`{{date}}`、`{{time}}`、`{{timezone}}`、`{{datetime}}` 变量。

Exa 配置：

```json
{"use":"exa","provider":[{"id":"exa","key":"your-exa-key"}]}
```

启用后模型可主动调用 `web_search` 和 `web_fetch`。工具管理：

```sh
askai tool ls
askai tool webSearch off
askai tool webSearch use exa
```

关闭后不发送工具定义。

## 输出与数据

最终正文写入 `stdout`。思考、工具状态、统计和交互提示写入 `stderr`。非交互终端不输出颜色，外部文本清理控制字符。

会话存储在 `~/.cache/ask-ai`。历史只保存正文，不保存推理、图片或工具原文。
