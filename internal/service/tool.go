package service

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrToolNotFound = errors.New("tool not found")

type UserContext struct {
	UserID         int64
	ConversationID string
	Role           string
	SupportsVision bool
	AnalyzeImage   AnalyzeImageFunc // 生成服务注入，nil 表示未配置 fallback
}

// AnalyzeImageFunc 把图片交给视觉模型描述，intent 必须非空；只用基础类型，避免 tool 反向依赖生成服务。
type AnalyzeImageFunc func(ctx context.Context, mimeType string, data []byte, intent string) (string, error)

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result"`
	Image      *GenerateImage  `json:"-"`
}

type ToolExchange struct {
	Round  int        `json:"round"`
	Seq    int        `json:"seq"`
	Call   ToolCall   `json:"call"`
	Result ToolResult `json:"result"`
}

type ToolProvider interface {
	Name() string
	ListTools(ctx context.Context, user UserContext) ([]ToolDefinition, error)
	CallTool(ctx context.Context, user UserContext, name string, args json.RawMessage) (ToolResult, error)
}

type ToolProviderManager struct {
	providers []ToolProvider
}

func NewToolProviderManager(providers ...ToolProvider) *ToolProviderManager {
	return &ToolProviderManager{providers: providers}
}

func (m *ToolProviderManager) ListTools(ctx context.Context, user UserContext) ([]ToolDefinition, error) {
	var result []ToolDefinition

	for _, provider := range m.providers {
		tools, err := provider.ListTools(ctx, user)
		if err != nil {
			return nil, err
		}
		result = append(result, tools...)
	}

	return result, nil
}

func (m *ToolProviderManager) CallTool(ctx context.Context, user UserContext, name string, args json.RawMessage) (ToolResult, error) {
	for _, provider := range m.providers {
		tools, err := provider.ListTools(ctx, user)
		if err != nil {
			return ToolResult{}, err
		}

		for _, tool := range tools {
			if tool.Name == name {
				return provider.CallTool(ctx, user, name, args)
			}
		}
	}

	return ToolResult{}, ErrToolNotFound
}
