package tool

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AlexMeiko/guchat/internal/service"
)

const ToolGetCurrentTime = "get_current_time"

type BuiltinProvider struct{}

type currentTimeArgs struct {
	Timezone string `json:"timezone"`
}

type currentTimeResult struct {
	Timezone string `json:"timezone"`
	Time     string `json:"time"`
	Unix     int64  `json:"unix"`
}

func NewBuiltinProvider() *BuiltinProvider {
	return &BuiltinProvider{}
}

func (p *BuiltinProvider) Name() string {
	return "builtin"
}

func (p *BuiltinProvider) ListTools(ctx context.Context, user service.UserContext) ([]service.ToolDefinition, error) {
	return []service.ToolDefinition{
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
	}, nil
}

func (p *BuiltinProvider) CallTool(ctx context.Context, user service.UserContext, name string, args json.RawMessage) (service.ToolResult, error) {
	switch name {
	case ToolGetCurrentTime:
		return p.getCurrentTime(args)
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
