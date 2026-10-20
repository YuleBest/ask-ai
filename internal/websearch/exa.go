package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/YuleBest/ask-ai/internal/i18n"
)

const (
	defaultExaBaseURL  = "https://api.exa.ai"
	defaultResultCount = 5
	maxResultCount     = 10
	maxFetchedChars    = 12000
)

type ExaClient struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Language   i18n.Language
}

type SearchResult struct {
	Title         string   `json:"title,omitempty"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"published_date,omitempty"`
	Author        string   `json:"author,omitempty"`
	Highlights    []string `json:"highlights,omitempty"`
}

type FetchResult struct {
	Title         string `json:"title,omitempty"`
	URL           string `json:"url"`
	PublishedDate string `json:"published_date,omitempty"`
	Author        string `json:"author,omitempty"`
	Text          string `json:"text"`
}

type exaSearchResponse struct {
	Results []exaResult     `json:"results"`
	Error   json.RawMessage `json:"error"`
}

type exaContentsResponse struct {
	Results  []exaResult     `json:"results"`
	Statuses []exaURLStatus  `json:"statuses"`
	Error    json.RawMessage `json:"error"`
}

type exaResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate"`
	Author        string   `json:"author"`
	Highlights    []string `json:"highlights"`
	Text          string   `json:"text"`
}

type exaURLStatus struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Error  json.RawMessage `json:"error"`
}

func (c ExaClient) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, i18n.NewFor(c.Language, "搜索词不能为空")
	}
	if numResults <= 0 {
		numResults = defaultResultCount
	}
	if numResults > maxResultCount {
		numResults = maxResultCount
	}

	payload := map[string]any{
		"query":      query,
		"type":       "auto",
		"numResults": numResults,
		"contents": map[string]any{
			"highlights": true,
		},
	}
	var response exaSearchResponse
	if err := c.post(ctx, "/search", payload, &response); err != nil {
		return nil, err
	}
	if message := exaErrorText(response.Error); message != "" {
		return nil, errors.New(message)
	}

	results := make([]SearchResult, 0, len(response.Results))
	for _, result := range response.Results {
		results = append(results, SearchResult{
			Title:         result.Title,
			URL:           result.URL,
			PublishedDate: result.PublishedDate,
			Author:        result.Author,
			Highlights:    result.Highlights,
		})
	}
	return results, nil
}

func (c ExaClient) Fetch(ctx context.Context, rawURL string) (FetchResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return FetchResult{}, i18n.ErrorfFor(c.Language, "无效的 URL: %s", rawURL)
	}

	payload := map[string]any{
		"urls": []string{rawURL},
		"text": map[string]any{
			"maxCharacters": maxFetchedChars,
		},
	}
	var response exaContentsResponse
	if err := c.post(ctx, "/contents", payload, &response); err != nil {
		return FetchResult{}, err
	}
	if message := exaErrorText(response.Error); message != "" {
		return FetchResult{}, errors.New(message)
	}
	if len(response.Results) > 0 {
		result := response.Results[0]
		return FetchResult{
			Title:         result.Title,
			URL:           result.URL,
			PublishedDate: result.PublishedDate,
			Author:        result.Author,
			Text:          result.Text,
		}, nil
	}
	for _, status := range response.Statuses {
		if status.Status == "error" {
			if message := exaErrorText(status.Error); message != "" {
				return FetchResult{}, errors.New(message)
			}
		}
	}
	return FetchResult{}, i18n.NewFor(c.Language, "exa 未返回页面内容")
}

func (c ExaClient) post(ctx context.Context, path string, payload, output any) error {
	if strings.TrimSpace(c.APIKey) == "" {
		return i18n.NewFor(c.Language, "exa API key 不能为空")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return i18n.ErrorfFor(c.Language, "构造 Exa 请求失败: %w", err)
	}

	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultExaBaseURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(data))
	if err != nil {
		return i18n.ErrorfFor(c.Language, "创建 Exa 请求失败: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "ask-ai")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return i18n.ErrorfFor(c.Language, "请求 Exa 失败: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return i18n.ErrorfFor(c.Language, "读取 Exa 响应失败: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var errorBody struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &errorBody) == nil {
			if errorBody.Error != "" {
				return i18n.ErrorfFor(c.Language, "exa 返回 %s: %s", response.Status, errorBody.Error)
			}
			if errorBody.Message != "" {
				return i18n.ErrorfFor(c.Language, "exa 返回 %s: %s", response.Status, errorBody.Message)
			}
		}
		return i18n.ErrorfFor(c.Language, "exa 返回 %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, output); err != nil {
		return i18n.ErrorfFor(c.Language, "解析 Exa 响应失败: %w", err)
	}
	return nil
}

func exaErrorText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var object struct {
		Tag     string `json:"tag"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &object) == nil {
		if object.Message != "" {
			return object.Message
		}
		return object.Tag
	}
	return strings.TrimSpace(string(raw))
}
