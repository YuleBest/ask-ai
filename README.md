# askai

[English](README.en.md)

Go 命令行 AI 助手，兼容 OpenAI 接口。支持流式回答、追问、图片、任务、Exa 搜索和中英文界面。

## 安装

预编译版本见 [GitHub Releases](https://github.com/YuleBest/ask-ai/releases)。

Go 1.22+：

```sh
go build -trimpath -o ~/.local/bin/askai ./cmd/askai
```

## 使用

```sh
askai "解释 Go 的 context"
askai "再举一个例子" -f
askai "这张图是什么" -p ./logo.png
askai "分析这个报错" --show-think
askai -b "进入连续对话"
```

`-f [hash]` 追问任务。`-b` 绑定当前终端进行连续对话，`/exit` 或 `Ctrl+C` 退出并解绑。`-p` 可重复，支持 PNG、JPEG、GIF、WebP。

## 任务与模型

```sh
askai tl
askai tv [hash]
askai ts <hash>
askai tr [hash...]
askai ml
askai mu local/cn:glm-5.3-flash
askai md local/cn:deepseek-v4.1-flash
```

任务哈希支持 6 到 32 位前缀。

## 语言

程序默认读取 `LANGUAGE`、`LC_ALL`、`LC_MESSAGES`、`LANG`，支持简体中文 `zh-CN` 和英文 `en`。

`language.json` 可分别覆盖输出与提示词语言：

```json
{
  "output": "zh-CN",
  "prompt": "en"
}
```

省略字段时继承环境变量：

```sh
askai config edit language
```

`system_prompt.json` 使用语言映射，默认包含 `"zh-CN"` 和 `"en"`。提示词支持 `{{os}}`、`{{arch}}`、`{{date}}`、`{{time}}`、`{{timezone}}`、`{{datetime}}`。

## 配置

配置位于 `~/.config/ask-ai`：

- `models.json`：提供商和模型。
- `language.json`：输出与提示词语言。
- `system_prompt.json`：多语言系统提示词。
- `web_search.json`：网络搜索配置。
- `_meta.json`：配置和工具定义。

```sh
askai config
askai config edit models
askai config view webSearch
askai tool ls
askai tool webSearch use exa
```

`config view` 默认隐藏 `key`，加 `--show-secrets` 显示明文。

## 输出与数据

正文写入 `stdout`，状态、思考、搜索、统计和提示写入 `stderr`。非交互终端不输出颜色，外部文本会清理控制字符。

会话保存在 `~/.cache/ask-ai`，只保存用户和助手正文。

## 许可

MIT License，详见 [LICENSE](LICENSE)。
