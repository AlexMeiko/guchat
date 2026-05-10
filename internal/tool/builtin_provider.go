package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/service"
)

const (
	ToolGetCurrentTime = "get_current_time"
	ToolTavilySearch   = "tavily_search"
	ToolReadWebPage    = "read_web_page"

	defaultTavilyBaseURL = "https://api.tavily.com"
)

type BuiltinProviderConfig struct {
	TavilyAPIKey  string
	TavilyBaseURL string
}

type BuiltinProvider struct {
	tavilyAPIKey  string
	tavilyBaseURL string
}

// 时间工具
type currentTimeArgs struct {
	Timezone string `json:"timezone"`
}

type currentTimeResult struct {
	Timezone string `json:"timezone"`
	Time     string `json:"time"`
	Unix     int64  `json:"unix"`
}

// Tavily 搜索工具
type tavilySearchArgs struct {
	Query string `json:"query"`
}

type tavilySearchRequest struct {
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth"`
	MaxResults  int    `json:"max_results"`
}

func NewBuiltinProvider(cfg BuiltinProviderConfig) *BuiltinProvider {
	tavilyBaseURL := strings.TrimRight(strings.TrimSpace(cfg.TavilyBaseURL), "/")
	if tavilyBaseURL == "" {
		tavilyBaseURL = defaultTavilyBaseURL
	}

	return &BuiltinProvider{
		tavilyAPIKey:  strings.TrimSpace(cfg.TavilyAPIKey),
		tavilyBaseURL: tavilyBaseURL,
	}
}

func (p *BuiltinProvider) Name() string {
	return "builtin"
}

func (p *BuiltinProvider) ListTools(ctx context.Context, user service.UserContext) ([]service.ToolDefinition, error) {
	tools := []service.ToolDefinition{
		{
			Name:        ToolGetCurrentTime,
			Description: "获取指定时区的当前时间",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"timezone": {
						"type": "string",
						"description": "IANA 时区名称，例如 Asia/Shanghai"
					}
				},
				"required": ["timezone"],
				"additionalProperties": false
			}`),
		},
		{
			Name:        ToolReadWebPage,
			Description: "读取指定公开网页 URL 的文本内容。当用户提供具体链接，或者搜索结果中已有目标链接且需要查看页面正文时使用。不要用于搜索未知网页。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "要读取的网页 URL"
					},
					"max_chars": {
						"type": "integer",
						"description": "要提取的最大字符数，多出的截断。默认为 10000"
					}
				},
				"required": ["url"],
				"additionalProperties": false
			}`),
		},
	}

	if p.tavilyAPIKey != "" {
		tools = append(tools, service.ToolDefinition{
			Name:        ToolTavilySearch,
			Description: "搜索互联网信息并返回结果摘要与来源链接。当问题涉及最新信息、网页资料、新闻或需要外部来源时使用。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "要搜索的关键词或问题"
					}
				},
				"required": ["query"],
				"additionalProperties": false
			}`),
		})
	}

	return tools, nil
}

func (p *BuiltinProvider) CallTool(ctx context.Context, user service.UserContext, name string, args json.RawMessage) (service.ToolResult, error) {
	switch name {
	case ToolGetCurrentTime:
		return p.getCurrentTime(args)
	case ToolTavilySearch:
		return p.tavilySearch(ctx, args)
	case ToolReadWebPage:
		return p.readWebPage(ctx, args)
	default:
		return service.ToolResult{}, service.ErrToolNotFound
	}
}

func (p *BuiltinProvider) getCurrentTime(args json.RawMessage) (service.ToolResult, error) {
	var input currentTimeArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}

	loc, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return service.ToolResult{}, err
	}

	now := time.Now().In(loc)

	payload, err := json.Marshal(currentTimeResult{
		Timezone: input.Timezone,
		Time:     now.Format(time.RFC3339),
		Unix:     now.Unix(),
	})
	if err != nil {
		return service.ToolResult{}, err
	}

	return service.ToolResult{
		Name:   ToolGetCurrentTime,
		Result: payload,
	}, nil
}

func (p *BuiltinProvider) tavilySearch(ctx context.Context, args json.RawMessage) (service.ToolResult, error) {
	var input tavilySearchArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}

	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		return service.ToolResult{}, fmt.Errorf("query is required")
	}

	apiKey := p.tavilyAPIKey
	if apiKey == "" {
		return service.ToolResult{}, fmt.Errorf("TAVILY_API_KEY is required")
	}

	reqBody := tavilySearchRequest{
		Query:       input.Query,
		SearchDepth: "advanced",
		MaxResults:  10,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return service.ToolResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tavilyBaseURL+"/search", bytes.NewReader(payload))
	if err != nil {
		return service.ToolResult{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return service.ToolResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return service.ToolResult{}, fmt.Errorf("tavily api error: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return service.ToolResult{}, err
	}
	if !json.Valid(body) {
		return service.ToolResult{}, fmt.Errorf("invalid tavily json response")
	}

	return service.ToolResult{
		Name:   ToolTavilySearch,
		Result: body,
	}, nil
}
