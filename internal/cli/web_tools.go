package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/i18n"
	"github.com/YuleBest/ask-ai/internal/terminaltext"
	"github.com/YuleBest/ask-ai/internal/websearch"
)

const chineseWebToolInstructions = `你可以使用 web_search 和 web_fetch 查询公开网络。
- 面对新闻、最新版本、价格、发布信息等时效性问题时，主动搜索，不要只依赖已有知识。
- 搜索结果不足或需要核对原文时，使用 web_fetch 抓取页面。
- 基于工具结果回答，不要编造来源、链接或页面内容。`

const englishWebToolInstructions = `You can use web_search and web_fetch to access the public web.
- Proactively search for time-sensitive topics such as news, latest versions, prices, and releases instead of relying only on existing knowledge.
- Use web_fetch when search results are insufficient or the primary source needs to be checked.
- Base answers on tool results. Never invent sources, links, or page content.`

type webToolExecutor struct {
	client   websearch.ExaClient
	language i18n.Language
}

func webToolDefinitions() []chat.Tool {
	tools, _ := webToolDefinitionsFromMeta(config.DefaultMeta(), i18n.PromptLanguage())
	return tools
}

func webToolDefinitionsFromMeta(meta config.Meta, language i18n.Language) ([]chat.Tool, error) {
	tool, ok := meta.Tools["webSearch"]
	if !ok {
		return nil, i18n.New("配置元数据缺少 webSearch 工具")
	}
	if len(tool.Definitions) == 0 {
		return nil, i18n.New("配置元数据缺少 webSearch 工具定义")
	}

	tools := make([]chat.Tool, 0, len(tool.Definitions))
	for _, definition := range tool.Definitions {
		definition = localizeDefaultToolDefinition(definition, language)
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

func localizeDefaultToolDefinition(definition config.MetaToolDefinition, language i18n.Language) config.MetaToolDefinition {
	if language != i18n.ChineseSimplified {
		return definition
	}
	defaults := config.DefaultMeta().Tools["webSearch"].Definitions
	for _, defaultDefinition := range defaults {
		if definition.Name != defaultDefinition.Name ||
			definition.Description != defaultDefinition.Description ||
			!sameJSON(definition.Parameters, defaultDefinition.Parameters) {
			continue
		}
		switch definition.Name {
		case "web_search":
			definition.Description = "使用 Exa 搜索公开网络。面对新闻、最新版本、价格、发布信息等时效性问题时主动使用。"
			definition.Parameters = json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "搜索词"},
					"num_results": {"type": "integer", "minimum": 1, "maximum": 10, "description": "结果数量，默认 5"}
				},
				"required": ["query"],
				"additionalProperties": false
			}`)
		case "web_fetch":
			definition.Description = "使用 Exa 抓取公开 URL 的可读正文。当搜索结果不足或需要核对原始来源时使用。"
			definition.Parameters = json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {"type": "string", "description": "要抓取的 HTTP 或 HTTPS URL"}
				},
				"required": ["url"],
				"additionalProperties": false
			}`)
		}
		return definition
	}
	return definition
}

func sameJSON(left, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func addWebToolInstructions(prompt string, language i18n.Language) string {
	prompt = strings.TrimSpace(prompt)
	instructions := chineseWebToolInstructions
	if language == i18n.English {
		instructions = englishWebToolInstructions
	}
	if prompt == "" {
		return instructions
	}
	return prompt + "\n\n" + instructions
}

func (e webToolExecutor) Execute(ctx context.Context, call chat.ToolCall) string {
	switch call.Function.Name {
	case "web_search":
		var arguments struct {
			Query      string `json:"query"`
			NumResults int    `json:"num_results"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
			return toolErrorJSON(i18n.TFor(e.language, "解析搜索参数失败: %s", err), "parse_search_arguments")
		}
		results, err := e.client.Search(ctx, arguments.Query, arguments.NumResults)
		if err != nil {
			return toolErrorJSON(err.Error(), "search_failed")
		}
		return marshalToolResult(map[string]any{
			"query":   arguments.Query,
			"results": results,
		}, e.language)
	case "web_fetch":
		var arguments struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
			return toolErrorJSON(i18n.TFor(e.language, "解析抓取参数失败: %s", err), "parse_fetch_arguments")
		}
		result, err := e.client.Fetch(ctx, arguments.URL)
		if err != nil {
			return toolErrorJSON(err.Error(), "fetch_failed")
		}
		return marshalToolResult(result, e.language)
	default:
		return toolErrorJSON(i18n.TFor(e.language, "未知工具: %s", call.Function.Name), "unknown_tool")
	}
}

func marshalToolResult(value any, language i18n.Language) string {
	data, err := json.Marshal(value)
	if err != nil {
		return toolErrorJSON(i18n.TFor(language, "序列化工具结果失败: %s", err), "serialize_result")
	}
	return string(data)
}

func toolErrorJSON(message, code string) string {
	data, _ := json.Marshal(map[string]string{
		"error":      message,
		"error_code": code,
	})
	return string(data)
}

func toolStatusText(call chat.ToolCall) string {
	var text string
	switch call.Function.Name {
	case "web_search":
		var arguments struct {
			Query string `json:"query"`
		}
		if json.Unmarshal([]byte(call.Function.Arguments), &arguments) == nil {
			text = i18n.T("搜索: %s", arguments.Query)
		} else {
			text = i18n.T("搜索 Web")
		}
	case "web_fetch":
		var arguments struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(call.Function.Arguments), &arguments) == nil {
			text = i18n.T("抓取: %s", arguments.URL)
		} else {
			text = i18n.T("抓取网页")
		}
	default:
		text = i18n.T("调用工具: %s", call.Function.Name)
	}
	return terminaltext.Clean(text)
}

func printToolResultPreview(writer io.Writer, call chat.ToolCall, rawResult string) {
	style := newOutputStyle(writer)
	var failure struct {
		Error string `json:"error"`
		Code  string `json:"error_code"`
	}
	if json.Unmarshal([]byte(rawResult), &failure) == nil && failure.Error != "" {
		message := toolErrorOutputText(failure.Code)
		if message == "" {
			message = previewText(failure.Error, 240)
		}
		fmt.Fprintln(writer, style.red("  "+i18n.T("错误: %s", message)))
		return
	}

	switch call.Function.Name {
	case "web_search":
		var result struct {
			Results []websearch.SearchResult `json:"results"`
		}
		if json.Unmarshal([]byte(rawResult), &result) != nil {
			return
		}
		if len(result.Results) == 0 {
			fmt.Fprintln(writer, style.gray("  "+i18n.T("未返回搜索结果")))
			return
		}
		limit := min(len(result.Results), 3)
		for index := 0; index < limit; index++ {
			item := result.Results[index]
			title := strings.TrimSpace(item.Title)
			if title == "" {
				title = item.URL
			}
			fmt.Fprintf(writer, "%s\n", style.gray(fmt.Sprintf("  %d. %s", index+1, previewText(title, 160))))
			if item.URL != "" {
				fmt.Fprintf(writer, "%s\n", style.gray("     "+previewText(item.URL, 240)))
			}
			if len(item.Highlights) > 0 {
				fmt.Fprintf(writer, "%s\n", style.gray("     "+previewText(item.Highlights[0], 200)))
			}
		}
	case "web_fetch":
		var result websearch.FetchResult
		if json.Unmarshal([]byte(rawResult), &result) != nil {
			return
		}
		if result.Title != "" {
			fmt.Fprintf(writer, "%s\n", style.gray("  "+i18n.T("标题: %s", previewText(result.Title, 160))))
		}
		if result.Text != "" {
			fmt.Fprintf(writer, "%s\n", style.gray("  "+i18n.T("正文: %s", previewText(result.Text, 320))))
		} else {
			fmt.Fprintln(writer, style.gray("  "+i18n.T("未返回页面正文")))
		}
	}
}

func toolErrorOutputText(code string) string {
	switch code {
	case "parse_search_arguments":
		return i18n.T("解析搜索参数失败")
	case "parse_fetch_arguments":
		return i18n.T("解析抓取参数失败")
	case "search_failed":
		return i18n.T("搜索失败")
	case "fetch_failed":
		return i18n.T("网页抓取失败")
	case "unknown_tool":
		return i18n.T("未知工具")
	case "serialize_result":
		return i18n.T("序列化工具结果失败")
	default:
		return ""
	}
}

func previewText(text string, limit int) string {
	text = strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}
		return char
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	if limit < 4 {
		limit = 4
	}
	if displayWidth(text) <= limit {
		return text
	}

	var truncated strings.Builder
	width := 0
	for _, char := range text {
		charWidth := displayWidth(string(char))
		if width+charWidth > limit-3 {
			break
		}
		truncated.WriteRune(char)
		width += charWidth
	}
	return truncated.String() + "..."
}
