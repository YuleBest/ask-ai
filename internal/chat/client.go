package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Client struct {
	APIURL     string
	APIKey     string
	HTTPClient *http.Client
}

type Request struct {
	Model           string
	ReasoningEffort string
	Messages        []Message
	Tools           []Tool
	ToolChoice      string
}

type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ImageAttachment struct {
	MIMEType string
	Data     []byte
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

func NewUserMessage(text string, images []ImageAttachment) Message {
	if len(images) == 0 {
		return Message{Role: "user", Content: text}
	}

	parts := make([]contentPart, 0, len(images)+1)
	parts = append(parts, contentPart{Type: "text", Text: text})
	for _, image := range images {
		parts = append(parts, contentPart{
			Type: "image_url",
			ImageURL: &imageURL{
				URL: "data:" + image.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(image.Data),
			},
		})
	}
	return Message{Role: "user", Content: parts}
}

type Delta struct {
	Content   string
	Reasoning string
	ToolCall  bool
}

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type Result struct {
	Usage        Usage
	HasUsage     bool
	ToolCalls    []ToolCall
	FinishReason string
}

type requestBody struct {
	Model           string         `json:"model"`
	Messages        []Message      `json:"messages"`
	Stream          bool           `json:"stream"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	StreamOptions   *streamOptions `json:"stream_options,omitempty"`
	Tools           []Tool         `json:"tools,omitempty"`
	ToolChoice      string         `json:"tool_choice,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Refusal          string `json:"refusal"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *Usage `json:"usage"`
}

const (
	maxStreamContentBytes = 16 << 20
	maxToolCalls          = 32
	maxToolArgumentBytes  = 1 << 20
	maxSSEEventBytes      = 1 << 20
	defaultRequestTimeout = 10 * time.Minute
	defaultHeaderTimeout  = 30 * time.Second
	defaultTLSIdleTimeout = 10 * time.Second
)

func NewHTTPClient() *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{Proxy: http.ProxyFromEnvironment}
	} else {
		transport = transport.Clone()
	}
	transport.ResponseHeaderTimeout = defaultHeaderTimeout
	transport.TLSHandshakeTimeout = defaultTLSIdleTimeout
	return &http.Client{
		Transport: transport,
		Timeout:   defaultRequestTimeout,
	}
}

func (c Client) Stream(ctx context.Context, req Request, onDelta func(Delta) error) (Result, error) {
	if onDelta == nil {
		return Result{}, errors.New("流式内容回调不能为空")
	}

	payload, err := json.Marshal(requestBody{
		Model:           req.Model,
		Messages:        req.Messages,
		Stream:          true,
		ReasoningEffort: req.ReasoningEffort,
		StreamOptions:   &streamOptions{IncludeUsage: true},
		Tools:           req.Tools,
		ToolChoice:      req.ToolChoice,
	})
	if err != nil {
		return Result{}, fmt.Errorf("构造请求失败: %w", err)
	}

	endpoint := strings.TrimRight(c.APIURL, "/") + "/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("创建请求失败: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = NewHTTPClient()
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("请求失败: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return Result{}, fmt.Errorf("接口返回 %s: %s", response.Status, message)
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	wroteContent := false
	var usage Usage
	hasUsage := false
	toolCallsByIndex := make(map[int]*ToolCall)
	toolCallNotified := false
	var finishReason string
	var refusal string
	doneSeen := false
	contentBytes := 0
	toolArgumentBytes := 0
	var dataLines []string
	dataBytes := 0

	processEvent := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		dataBytes = 0
		if data == "[DONE]" {
			doneSeen = true
			return true, nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return false, fmt.Errorf("解析流式响应失败: %w", err)
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return false, errors.New(chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
			hasUsage = true
		}
		if len(chunk.Choices) == 0 {
			return false, nil
		}

		choice := chunk.Choices[0]
		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}
		delta := choice.Delta
		if delta.Refusal != "" {
			refusal = delta.Refusal
		}
		hasToolDelta := false
		for _, toolDelta := range delta.ToolCalls {
			hasToolDelta = true
			call, exists := toolCallsByIndex[toolDelta.Index]
			if !exists {
				if len(toolCallsByIndex) >= maxToolCalls {
					return false, errors.New("工具调用数量超过限制")
				}
				call = &ToolCall{}
				toolCallsByIndex[toolDelta.Index] = call
			}
			if toolDelta.ID != "" {
				call.ID = toolDelta.ID
			}
			if toolDelta.Type != "" {
				call.Type = toolDelta.Type
			}
			if toolDelta.Function.Name != "" {
				call.Function.Name = toolDelta.Function.Name
			}
			toolArgumentBytes += len(toolDelta.Function.Arguments)
			if toolArgumentBytes > maxToolArgumentBytes {
				return false, errors.New("工具调用参数超过限制")
			}
			call.Function.Arguments += toolDelta.Function.Arguments
		}
		notifyToolCall := hasToolDelta && !toolCallNotified
		if delta.Content == "" && delta.ReasoningContent == "" && !notifyToolCall {
			return false, nil
		}
		contentBytes += len(delta.Content) + len(delta.ReasoningContent)
		if contentBytes > maxStreamContentBytes {
			return false, errors.New("流式响应内容超过限制")
		}
		if err := onDelta(Delta{
			Content:   delta.Content,
			Reasoning: delta.ReasoningContent,
			ToolCall:  notifyToolCall,
		}); err != nil {
			return false, fmt.Errorf("处理流式内容失败: %w", err)
		}
		toolCallNotified = toolCallNotified || hasToolDelta
		if delta.Content != "" {
			wroteContent = true
		}
		return false, nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			done, err := processEvent()
			if err != nil {
				return Result{}, err
			}
			if done {
				break
			}
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		value := strings.TrimPrefix(line, "data:")
		value = strings.TrimPrefix(value, " ")
		dataBytes += len(value) + 1
		if dataBytes > maxSSEEventBytes {
			return Result{}, errors.New("流式事件超过大小限制")
		}
		dataLines = append(dataLines, value)
	}
	if err := scanner.Err(); err != nil {
		return Result{}, fmt.Errorf("读取流式响应失败: %w", err)
	}
	if !doneSeen && len(dataLines) > 0 {
		done, err := processEvent()
		if err != nil {
			return Result{}, err
		}
		if done {
			doneSeen = true
		}
	}
	if refusal != "" {
		return Result{}, fmt.Errorf("模型拒绝回答: %s", refusal)
	}
	switch finishReason {
	case "length":
		return Result{}, errors.New("模型响应因长度限制被截断")
	case "content_filter":
		return Result{}, errors.New("模型响应被内容过滤器截断")
	}
	if !doneSeen && finishReason == "" {
		return Result{}, errors.New("流式响应未正常结束")
	}
	if !wroteContent && len(toolCallsByIndex) == 0 {
		return Result{}, errors.New("响应结束但未收到正文或工具调用")
	}

	indexes := make([]int, 0, len(toolCallsByIndex))
	for index := range toolCallsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	toolCalls := make([]ToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := *toolCallsByIndex[index]
		if call.Type == "" {
			call.Type = "function"
		}
		if call.ID == "" || call.Function.Name == "" {
			return Result{}, errors.New("工具调用缺少 id 或函数名")
		}
		if call.Type != "function" {
			return Result{}, fmt.Errorf("不支持的工具调用类型: %s", call.Type)
		}
		if call.Function.Arguments == "" {
			call.Function.Arguments = "{}"
		}
		if !json.Valid([]byte(call.Function.Arguments)) {
			return Result{}, fmt.Errorf("工具调用参数不是有效 JSON: %s", call.Function.Name)
		}
		toolCalls = append(toolCalls, call)
	}
	return Result{
		Usage:        usage,
		HasUsage:     hasUsage,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}, nil
}
