# askai

[简体中文](README.md)

A command-line AI assistant written in Go. It works with OpenAI-compatible APIs and supports streaming answers, follow-ups, images, screenshots, task management, Exa web search, Markdown rendering, and Chinese or English output.

## Install

Download a prebuilt binary from [GitHub Releases](https://github.com/YuleBest/ask-ai/releases).

Build from source with Go 1.22+:

```sh
go build -trimpath -o ~/.local/bin/askai ./cmd/askai
```

Check the version with `askai version`, `--version`, or `-v`.

## Usage

```sh
askai "Explain Go context"
askai "Give another example" -f
askai "What is in this image?" -p ./logo.png
askai "Explain what is on this screen" -s
askai "Analyze this error" --show-think
askai -b "Start continuous chat"
```

Use `-f [hash]` to follow a task. `-b` binds the current terminal for continuous questions; enter `/exit` or press `Ctrl+C` to quit and unbind. Repeat `-p` to attach PNG, JPEG, GIF, or WebP images. `-s` captures the primary display and can be combined with `-p`; it requires an available desktop session and screen-capture permission.

## Tasks and Models

```sh
askai tl
askai tv [hash]
askai ts <hash>
askai tr [hash...]
askai ml
askai mu local/cn:glm-5.3-flash
askai md local/cn:deepseek-v4.1-flash
```

Task hashes accept prefixes from 6 to 32 characters.

## Language

askai reads `LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, and `LANG` in that order. Simplified Chinese is `zh-CN`; English is `en`.

`language.json` can override output and prompt languages independently:

```json
{
  "output": "en",
  "prompt": "zh-CN"
}
```

Omitted fields inherit the environment:

```sh
askai config edit language
```

`system_prompt.json` maps each language to its prompt and includes `"zh-CN"` and `"en"` by default. Prompts support `{{os}}`, `{{arch}}`, `{{date}}`, `{{time}}`, `{{timezone}}`, and `{{datetime}}`.

## Configuration

Configuration lives in `~/.config/ask-ai`:

- `models.json`: providers and models.
- `language.json`: output and prompt languages.
- `system_prompt.json`: multilingual system prompts.
- `web_search.json`: web search configuration.
- `view_file.json`: file viewing tool configuration.
- `_meta.json`: configuration and tool definitions.

```sh
askai config
askai config edit models
askai config view webSearch
askai tool ls
askai tool webSearch use exa
askai tool viewFile mode ask
askai tool viewFile limit 512000
```

`config view` redacts `key` values by default. Add `--show-secrets` to show plaintext.

`viewFile` lets the model read metadata and UTF-8 text from regular local files. `ask` prompts before every read, `bypass` reads without prompting, and `off` disables the tool. `limit` sets the maximum content bytes per call, from 1 to 2097152.

## Output and Data

Answers go to `stdout`. Status, reasoning, search previews, token statistics, and interactive prompts go to `stderr`. Non-interactive terminals do not receive ANSI colors, and external text is sanitized for terminal control sequences.

Conversations are stored in `~/.cache/ask-ai`. Only user and assistant text is persisted.

## License

This project is released under the MIT License. See [LICENSE](LICENSE).
