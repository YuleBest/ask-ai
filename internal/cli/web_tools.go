package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/config"
	"github.com/YuleBest/ask-ai/internal/terminaltext"
	"github.com/YuleBest/ask-ai/internal/websearch"
)

const webToolInstructions = `你可以使用 web_search 和 web_fetch 查询公开网络。
- 面对新闻、最新版本、价格、发布信息等时效性问题时，主动搜索，不要只依赖已有知识。
- 搜索结果不足或需要核对原文时，使用 web_fetch 抓取页面。
- 基于工具结果回答，不要编造来源、链接或页面内容。`

type webToolExecutor struct {
	client websearch.ExaClient
}

func webToolDefinitions() []chat.Tool {
	tools, _ := webToolDefinitionsFromMeta(config.DefaultMeta())
	return tools
}

func webToolDefinitionsFromMeta(meta config.Meta) ([]chat.Tool, error) {
	tool, ok := meta.Tools["webSearch"]
	if !ok {
		return nil, errors.New("配置元数据缺少 webSearch 工具")
	}
	if len(tool.Definitions) == 0 {
		return nil, errors.New("配置元数据缺少 webSearch 工具定义")
	}

	tools := make([]chat.Tool, 0, len(tool.Definitions))
	for _, definition := range tool.Definitions {
		if definition.Name == "" {
			return nil, errors.New("工具定义缺少 name")
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

func addWebToolInstructions(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return webToolInstructions
	}
	return prompt + "\n\n" + webToolInstructions
}

func (e webToolExecutor) Execute(ctx context.Context, call chat.ToolCall) string {
	switch call.Function.Name {
	case "web_search":
		var arguments struct {
			Query      string `json:"query"`
			NumResults int    `json:"num_results"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
			return toolErrorJSON("解析搜索参数失败: " + err.Error())
		}
		results, err := e.client.Search(ctx, arguments.Query, arguments.NumResults)
		if err != nil {
			return toolErrorJSON(err.Error())
		}
		return marshalToolResult(map[string]any{
			"query":   arguments.Query,
			"results": results,
		})
	case "web_fetch":
		var arguments struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
			return toolErrorJSON("解析抓取参数失败: " + err.Error())
		}
		result, err := e.client.Fetch(ctx, arguments.URL)
		if err != nil {
			return toolErrorJSON(err.Error())
		}
		return marshalToolResult(result)
	default:
		return toolErrorJSON("未知工具: " + call.Function.Name)
	}
}

func marshalToolResult(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return toolErrorJSON("序列化工具结果失败: " + err.Error())
	}
	return string(data)
}

func toolErrorJSON(message string) string {
	data, _ := json.Marshal(map[string]string{"error": message})
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
			text = "搜索: " + arguments.Query
		} else {
			text = "搜索 Web"
		}
	case "web_fetch":
		var arguments struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(call.Function.Arguments), &arguments) == nil {
			text = "抓取: " + arguments.URL
		} else {
			text = "抓取网页"
		}
	default:
		text = fmt.Sprintf("调用工具: %s", call.Function.Name)
	}
	return terminaltext.Clean(text)
}

func printToolResultPreview(writer io.Writer, call chat.ToolCall, rawResult string) {
	style := newOutputStyle(writer)
	var failure struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(rawResult), &failure) == nil && failure.Error != "" {
		fmt.Fprintln(writer, style.red("  错误: "+previewText(failure.Error, 240)))
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
			fmt.Fprintln(writer, style.gray("  未返回搜索结果"))
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
			fmt.Fprintf(writer, "%s\n", style.gray("  标题: "+previewText(result.Title, 160)))
		}
		if result.Text != "" {
			fmt.Fprintf(writer, "%s\n", style.gray("  正文: "+previewText(result.Text, 320)))
		} else {
			fmt.Fprintln(writer, style.gray("  未返回页面正文"))
		}
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
