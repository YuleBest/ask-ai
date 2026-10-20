package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	if len(args) != 1 {
		printErrorf(stderr, "help 最多接受一个命令名")
		return 2
	}
	switch args[0] {
	case "task":
		printTaskUsage(stdout)
	case "config":
		printConfigUsage(stdout)
	case "model":
		printModelUsage(stdout)
	case "tool":
		printToolUsage(stdout)
	case "version":
		printVersionUsage(stdout)
	default:
		printErrorf(stderr, "未知帮助主题: %s", args[0])
		return 2
	}
	return 0
}

func printUsage(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.bold(i18n.T("askai - 在命令行中询问 AI")))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("用法:")))
	fmt.Fprintln(w, "  askai "+i18n.T("[选项]")+" "+i18n.T("<问题>"))
	fmt.Fprintln(w, "  askai "+i18n.T("[选项]")+" -f [hash] "+i18n.T("<问题>"))
	fmt.Fprintln(w, "  askai "+i18n.T("[选项]")+" -b [hash] "+i18n.T("[问题]"))
	fmt.Fprintln(w, "  askai "+i18n.T("<命令>")+" "+i18n.T("[参数]"))
	fmt.Fprintln(w, "  askai help "+i18n.T("[命令]"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("说明:")))
	fmt.Fprintln(w, "  "+i18n.T("通过 OpenAI 兼容接口发送问题，并流式输出回答正文。"))
	fmt.Fprintln(w, "  "+i18n.T("普通调用会创建新任务；-f 追问已有任务；-b 绑定当前终端并进入连续对话。"))
	fmt.Fprintln(w, "  "+i18n.T("连续对话输入 /exit 或按 Ctrl+C 退出，退出时自动解除终端绑定。"))
	fmt.Fprintln(w, "  "+i18n.T("配置 Exa 后，模型可以主动调用网络搜索和网页抓取工具。"))
	fmt.Fprintln(w, "  "+i18n.T("选项可以写在问题前后；需要把选项文本作为问题时，请先写 --。"))
	fmt.Fprintln(w, "  "+i18n.T("模型思考内容默认不显示且不会保存，交互终端会显示思考计时。"))
	fmt.Fprintln(w, "  "+i18n.T("回答结束后会在 stderr 显示模型、输入输出 token 和生成速度。"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("命令:")))
	printHelpRow(w, style, "task", "管理任务（会话）", 22)
	printHelpRow(w, style, "config", "列出、查看或编辑配置文件", 22)
	printHelpRow(w, style, "model", "列出、临时切换或设置默认模型", 22)
	printHelpRow(w, style, "tool", "查看或配置模型可用工具", 22)
	printHelpRow(w, style, "version", "显示当前版本号", 22)
	printHelpRow(w, style, "help [命令]", "显示总帮助或指定命令的帮助", 22)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("快捷命令:")))
	printHelpRow(w, style, "tl", "等价于 askai task list", 22)
	printHelpRow(w, style, "tv [hash]", "等价于 askai task view", 22)
	printHelpRow(w, style, "ts <hash>", "等价于 askai task switch", 22)
	printHelpRow(w, style, "tr [hash...]", "等价于 askai task remove", 22)
	printHelpRow(w, style, "ml", "等价于 askai model list", 22)
	printHelpRow(w, style, "mu <provider/model>", "等价于 askai model use", 22)
	printHelpRow(w, style, "md <provider/model>", "等价于 askai model default", 22)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("选项:")))
	printHelpRow(w, style, "-f, --follow [hash]", "追问当前任务或哈希指定的任务", 30)
	printHelpRow(w, style, "-b, --bind [hash]", "绑定任务并进入连续对话，退出时解绑", 30)
	printHelpRow(w, style, "-p, --picture <path>", "附带图片，可重复使用以添加多张", 30)
	printHelpRow(w, style, "--show-think", "在 stderr 中以灰色显示思考过程和时长", 30)
	printHelpRow(w, style, "-v, --version", "显示当前版本号", 30)
	printHelpRow(w, style, "-h, --help", "显示总帮助", 30)
	printHelpRow(w, style, "--", "结束选项解析，后续内容作为问题", 30)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("参数:")))
	printHelpRow(w, outputStyle{}, "<问题>", "要发送给模型的问题，可包含空格", 30)
	printHelpRow(w, outputStyle{}, "<hash>", "任务哈希前缀，支持 6 位到完整 32 位", 30)
	printHelpRow(w, outputStyle{}, "<path>", "PNG、JPEG、GIF 或 WebP 图片路径", 30)
	printHelpRow(w, outputStyle{}, "<provider/model>", "提供商 ID 和模型 ID，例如 local/model", 30)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("文件:")))
	fmt.Fprintln(w, "  ~/.config/ask-ai/models.json         "+i18n.T("模型提供商与模型配置"))
	fmt.Fprintln(w, "  ~/.config/ask-ai/language.json       "+i18n.T("程序输出与提示词语言"))
	fmt.Fprintln(w, "  ~/.config/ask-ai/system_prompt.json  "+i18n.T("多语言系统提示词"))
	fmt.Fprintln(w, "  ~/.config/ask-ai/_meta.json          "+i18n.T("配置键与文件映射"))
	fmt.Fprintln(w, "  ~/.config/ask-ai/web_search.json     "+i18n.T("网络搜索配置"))
	fmt.Fprintln(w, "  ~/.cache/ask-ai/sessions/            "+i18n.T("任务历史"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("退出状态:")))
	fmt.Fprintln(w, "  0    "+i18n.T("成功"))
	fmt.Fprintln(w, "  1    "+i18n.T("运行时或配置错误"))
	fmt.Fprintln(w, "  2    "+i18n.T("命令行用法错误"))
	fmt.Fprintln(w, "  130  "+i18n.T("用户中断"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("示例:")))
	fmt.Fprintln(w, `  askai "`+i18n.T("解释 Go 的 context")+`"`)
	fmt.Fprintln(w, `  askai -f "`+i18n.T("再举一个实际例子")+`"`)
	fmt.Fprintln(w, `  askai -f 0054b4 "`+i18n.T("继续这个任务")+`"`)
	fmt.Fprintln(w, `  askai -b 0054b4`)
	fmt.Fprintln(w, `  askai "`+i18n.T("这张图是什么")+`" -p ./logo.png`)
	fmt.Fprintln(w, "  askai task list")
	fmt.Fprintln(w, "  askai config view systemPrompt")
	fmt.Fprintln(w, "  askai tool ls")
	fmt.Fprintln(w, "  askai model use local/cn:glm-5.3-flash")
	fmt.Fprintln(w, "  askai version")
}

func printVersionUsage(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.bold(i18n.T("askai version - 显示当前版本号")))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("用法:")))
	fmt.Fprintln(w, "  askai version")
	fmt.Fprintln(w, "  askai --version")
	fmt.Fprintln(w, "  askai -v")
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("命令:")))
	printHelpRow(w, style, "version", "显示当前版本号", 20)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("选项:")))
	printHelpRow(w, style, "-h, --help", "显示本帮助", 20)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("示例:")))
	fmt.Fprintln(w, "  askai version")
}

func printToolUsage(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.bold(i18n.T("askai tool - 查看或配置模型可用工具")))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("用法:")))
	fmt.Fprintln(w, "  askai tool ls")
	fmt.Fprintln(w, "  askai tool webSearch off")
	fmt.Fprintln(w, "  askai tool webSearch use <provider>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("命令:")))
	printHelpRow(w, style, "ls, list", "列出工具、配置键、操作和用途", 24)
	printHelpRow(w, style, "webSearch off", "关闭网络搜索工具", 24)
	printHelpRow(w, style, "webSearch use <provider>", "切换网络搜索提供商", 24)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("说明:")))
	fmt.Fprintln(w, "  "+i18n.T("工具定义来自 ~/.config/ask-ai/_meta.json 的 tools 字段。"))
	fmt.Fprintln(w, "  "+i18n.T("当前仅支持 webSearch 工具和 exa provider。"))
	fmt.Fprintln(w, "  "+i18n.T("关闭后会向 web_search.json 写入 \"use\": false。"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("示例:")))
	fmt.Fprintln(w, "  askai tool ls")
	fmt.Fprintln(w, "  askai tool webSearch off")
	fmt.Fprintln(w, `  askai tool webSearch use "exa"`)
}

func printTaskUsage(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.bold(i18n.T("askai task - 管理任务（会话）")))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("用法:")))
	fmt.Fprintln(w, "  askai task list")
	fmt.Fprintln(w, "  askai task remove [hash...]")
	fmt.Fprintln(w, "  askai task switch <hash>")
	fmt.Fprintln(w, "  askai task view [hash]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("命令:")))
	printHelpRow(w, style, "list, ls, tl", "列出任务、哈希和首条用户消息", 24)
	printHelpRow(w, style, "remove, rm, tr [hash...]", "删除一个或多个任务", 32)
	printHelpRow(w, style, "switch, st, ts <hash>", "把指定任务切换为默认任务", 24)
	printHelpRow(w, style, "view, tv [hash]", "查看指定任务或上一次任务", 24)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("说明:")))
	fmt.Fprintln(w, "  "+i18n.T("当前任务在列表中用 * 标记。"))
	fmt.Fprintln(w, "  "+i18n.T("删除当前任务会同时清除默认任务指针。"))
	fmt.Fprintln(w, "  "+i18n.T("不带哈希时进入交互选择；空格多选，Enter 后按 y 确认删除。"))
	fmt.Fprintln(w, "  "+i18n.T("view 不带哈希时查看上一次任务。"))
	fmt.Fprintln(w, "  "+i18n.T("哈希支持 6 位到完整 32 位前缀；存在冲突时会要求补长。"))
	fmt.Fprintln(w, "  "+i18n.T("新任务在首次成功回答后写入缓存目录。"))
	fmt.Fprintln(w, "  "+i18n.T("任务只保存 user 和 assistant 正文，不保存模型思考内容。"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("参数:")))
	printHelpRow(w, outputStyle{}, "<hash...>", "一个或多个任务哈希，支持 6 位到完整 32 位", 24)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("文件:")))
	fmt.Fprintln(w, "  ~/.cache/ask-ai/sessions/<hash>.json   "+i18n.T("任务历史"))
	fmt.Fprintln(w, "  ~/.cache/ask-ai/last.json              "+i18n.T("当前任务指针"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("示例:")))
	fmt.Fprintln(w, "  askai task list")
	fmt.Fprintln(w, "  askai task switch 0054b4")
	fmt.Fprintln(w, "  askai task view e9b089")
	fmt.Fprintln(w, "  askai task view")
	fmt.Fprintln(w, "  askai task remove 0054b4")
	fmt.Fprintln(w, "  askai task remove 0054b4 e9b089")
	fmt.Fprintln(w, "  askai task remove")
	fmt.Fprintln(w, "  askai tl")
	fmt.Fprintln(w, "  askai tv e9b089")
}

func printConfigUsage(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.bold(i18n.T("askai config - 查看或编辑配置文件")))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("用法:")))
	fmt.Fprintln(w, "  askai config [list|ls]")
	fmt.Fprintln(w, "  askai config edit <key>")
	fmt.Fprintln(w, "  askai config view <key> [--show-secrets]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("命令:")))
	printHelpRow(w, style, "list, ls", "列出配置键、文件和说明", 20)
	printHelpRow(w, style, "edit <key>", "使用 nano 打开对应配置文件", 20)
	printHelpRow(w, style, "view <key> [--show-secrets]", "查看配置；密钥默认脱敏", 20)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("参数:")))
	printHelpRow(w, outputStyle{}, "<key>", "配置键，例如 models 或 systemPrompt", 20)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("说明:")))
	fmt.Fprintln(w, "  "+i18n.T("配置键和文件路径由 ~/.config/ask-ai/_meta.json 定义。"))
	fmt.Fprintln(w, "  "+i18n.T("默认包含 meta、models、language、systemPrompt 和 webSearch 配置键。"))
	fmt.Fprintln(w, "  "+i18n.T("language 可分别设置 output 和 prompt；省略时继承环境 locale。"))
	fmt.Fprintln(w, "  "+i18n.T("systemPrompt 是语言映射，支持 {{os}}、{{arch}}、{{date}}、{{time}}、{{timezone}} 和 {{datetime}} 变量。"))
	fmt.Fprintln(w, "  "+i18n.T("edit 命令要求 nano 已安装；view 不会修改配置文件。"))
	fmt.Fprintln(w, "  "+i18n.T("models 和 webSearch 的 key 默认显示为 ***，需要明文时加 --show-secrets。"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("文件:")))
	fmt.Fprintln(w, "  ~/.config/ask-ai/_meta.json   "+i18n.T("配置键与文件映射"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("示例:")))
	fmt.Fprintln(w, "  askai config")
	fmt.Fprintln(w, "  askai config edit models")
	fmt.Fprintln(w, "  askai config edit language")
	fmt.Fprintln(w, "  askai config view systemPrompt")
	fmt.Fprintln(w, "  askai config view models --show-secrets")
	fmt.Fprintln(w, "  askai config edit webSearch")
}

func printModelUsage(w io.Writer) {
	style := newOutputStyle(w)
	fmt.Fprintln(w, style.bold(i18n.T("askai model - 查看和管理模型")))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("用法:")))
	fmt.Fprintln(w, "  askai model list")
	fmt.Fprintln(w, "  askai model use <provider_id>/<model_id>")
	fmt.Fprintln(w, "  askai model default <provider_id>/<model_id>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("命令:")))
	printHelpRow(w, style, "list", "列出提供商和模型信息", 24)
	printHelpRow(w, style, "use <provider/model>", "为当前任务临时切换模型，保留历史", 24)
	printHelpRow(w, style, "default <provider/model>", "设置新会话使用的默认模型", 24)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("快捷命令:")))
	printHelpRow(w, style, "ml", "等价于 askai model list", 24)
	printHelpRow(w, style, "mu <provider/model>", "等价于 askai model use", 24)
	printHelpRow(w, style, "md <provider/model>", "等价于 askai model default", 24)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("说明:")))
	fmt.Fprintln(w, "  "+i18n.T("临时模型保存在当前任务中，只影响该任务后续追问。"))
	fmt.Fprintln(w, "  "+i18n.T("默认模型通过 models.json 中唯一的 default: true 标记。"))
	fmt.Fprintln(w, "  "+i18n.T("没有 default: true 时，新会话使用第一个 provider 的第一个模型。"))
	fmt.Fprintln(w, "  "+i18n.T("模型列表不会显示 API key。"))
	fmt.Fprintln(w, "  "+i18n.T("切换模型不会删除或改写已有对话正文。"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("参数:")))
	printHelpRow(w, outputStyle{}, "<provider/model>", "提供商 ID 和模型 ID，以 / 分隔", 24)
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("文件:")))
	fmt.Fprintln(w, "  ~/.config/ask-ai/models.json   "+i18n.T("提供商、模型和默认模型配置"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, style.bold(i18n.T("示例:")))
	fmt.Fprintln(w, "  askai ml")
	fmt.Fprintln(w, "  askai model use local/cn:glm-5.3-flash")
	fmt.Fprintln(w, "  askai md local/cn:deepseek-v4.1-flash")
}

func printHelpRow(w io.Writer, style outputStyle, command, description string, width int) {
	command = i18n.T(command)
	description = i18n.T(description)
	padding := width - displayWidth(command)
	if padding < 1 {
		padding = 1
	}
	fmt.Fprintf(w, "  %s%s %s\n", style.cyan(command), strings.Repeat(" ", padding), description)
}

func displayWidth(text string) int {
	width := 0
	for _, char := range text {
		switch {
		case char >= 0x1100 && char <= 0x115F,
			char >= 0x2E80 && char <= 0xA4CF,
			char >= 0xAC00 && char <= 0xD7A3,
			char >= 0xF900 && char <= 0xFAFF,
			char >= 0xFE10 && char <= 0xFE19,
			char >= 0xFE30 && char <= 0xFE6F,
			char >= 0xFF00 && char <= 0xFF60,
			char >= 0xFFE0 && char <= 0xFFE6,
			char >= 0x1F300 && char <= 0x1FAFF:
			width += 2
		default:
			width++
		}
	}
	return width
}
