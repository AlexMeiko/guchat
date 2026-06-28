package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/memory"
	"github.com/AlexMeiko/guchat/internal/service"
)

type searchMemoryArgs struct {
	Query      string   `json:"query"`
	Keywords   []string `json:"keywords"`
	Categories []string `json:"categories"`
	Scopes     []string `json:"scopes"`
	Limit      int      `json:"limit"`
}

type searchMemoryResult struct {
	Items []memoryToolItem `json:"items"`
}

type addMemoryResult struct {
	OK bool `json:"ok"`
}

type addMemoryArgs struct {
	Scope       string  `json:"scope"`
	Category    string  `json:"category"`
	SourceType  string  `json:"source_type"`
	SourceRef   string  `json:"source_ref"`
	SourceTitle string  `json:"source_title"`
	Content     string  `json:"content"`
	Confidence  float64 `json:"confidence"`
	ExpiresAt   string  `json:"expires_at"`
}

type disableMemoryResult struct {
	OK bool `json:"ok"`
}

type disableMemoryArgs struct {
	ID int64 `json:"id"`
}

type memoryToolItem struct {
	ID          int64     `json:"id"`
	Scope       string    `json:"scope"`
	Category    string    `json:"category"`
	SourceType  string    `json:"source_type"`
	SourceRef   string    `json:"source_ref,omitempty"`
	SourceTitle string    `json:"source_title,omitempty"`
	Content     string    `json:"content"`
	Confidence  float64   `json:"confidence"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (p *BuiltinProvider) memoryToolDefinitions() []service.ToolDefinition {
	return []service.ToolDefinition{
		{
			Name:        ToolSearchMemory,
			Description: "搜索当前用户和当前会话可用的长期记忆、偏好、事实、约束、总结、知识和代码片段。当用户问题可能涉及个人信息、偏好、项目知识、文档、代码或先前记录，或你不确定是否保存过相关信息时使用。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "自然语言检索问题，用于表达这次要查找什么记忆、知识或代码。可以描述意图、功能、逻辑或问题，不需要提供原文或代码片段。"
					},
					"keywords": {
						"type": "array",
						"description": "可选的少量精确关键词或短语，只用于辅助匹配专有名词、函数名、文件名、项目名、错误码、API 名称等。不要堆词，也不要把自然语言整句拆成关键词；语义检索主要依赖 query，不确定时可以不传。",
						"items": { "type": "string" }
					},
					"categories": {
						"type": "array",
						"description": "可选的记忆分类过滤，例如 user_profile、preference、fact、knowledge、goal、relationship、experience、daily_summary、constraint、negative_preference、situational。检索文档、知识或代码时通常使用 knowledge。",
						"items": { "type": "string" }
					},
					"scopes": {
						"type": "array",
						"description": "可选的范围过滤，只能使用 user、conversation、global。",
						"items": { "type": "string" }
					},
					"limit": {
						"type": "integer",
						"description": "返回的最大条数。未传或小于等于 0 时使用后端默认值；当前最多返回 20 条。"
					}
				},
				"required": ["query"],
				"additionalProperties": false
			}`),
		},
		{
			Name:        ToolAddMemory,
			Description: "写入一条 active 记忆。仅当用户明确要求记住某件事，或当前对话产生了后续明显有用的长期信息时使用。不能创建 global 记忆，不能传 user_id、conversation_id、origin 或 status。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"scope": {
						"type": "string",
						"description": "记忆范围，只能是 user 或 conversation。缺省值 user。"
					},
					"category": {
						"type": "string",
						"description": "记忆分类，例如 user_profile、preference、fact、knowledge、goal、relationship、experience、daily_summary、constraint、negative_preference、situational。缺省值 fact。文档、知识点、代码片段、实现方案等可复用资料应使用 knowledge。constraint、negative_preference、user_profile、preference 的 user 记忆可能会在后续会话默认提供给模型，只用于长期稳定、跨会话普遍有用的信息；普通事实、知识、总结、短期状态不要放入这些分类。"
					},
					"source_type": {
						"type": "string",
						"description": "来源类型，例如 none、conversation、web、file、api、repo、manual。在对话中未传时，add_memory 默认记录为当前会话来源，相关来源引用由后端自动处理。"
					},
					"source_ref": {
						"type": "string",
						"description": "来源引用，例如 URL、文件 key、repo path。source_type=conversation 时不需要传，当前会话来源由后端自动处理。"
					},
					"source_title": {
						"type": "string",
						"description": "来源标题，例如网页标题、文件名、文档标题。"
					},
					"content": {
						"type": "string",
						"description": "要保存的记忆正文，应简洁、明确、可复用。保存代码时保留关键标识符、语言信息和代码块格式。"
					},
					"confidence": {
						"type": "number",
						"description": "可信度，范围 0 到 1。未传时后端默认处理。"
					},
					"expires_at": {
						"type": "string",
						"description": "可选过期时间，RFC3339 格式。长期有效的记忆不要传。"
					}
				},
				"required": ["content"],
				"additionalProperties": false
			}`),
		},
		{
			Name:        ToolDisableMemory,
			Description: "禁用一条记忆，使其不再被后续检索到。只能禁用 user 自己的记忆。禁用后记忆状态变为 disabled，但数据仍保留在数据库中。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {
						"type": "integer",
						"description": "要禁用的记忆 ID。"
					}
				},
				"required": ["id"],
				"additionalProperties": false
			}`),
		},
	}
}

func (p *BuiltinProvider) searchMemory(ctx context.Context, user service.UserContext, args json.RawMessage) (service.ToolResult, error) {
	if p.memoryService == nil {
		return service.ToolResult{}, fmt.Errorf("memory service is not configured")
	}
	if user.UserID <= 0 {
		return service.ToolResult{}, fmt.Errorf("invalid user context")
	}

	var input searchMemoryArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}

	input.Query = strings.TrimSpace(input.Query)
	input.Keywords = trimStringList(input.Keywords)
	input.Categories = trimStringList(input.Categories)
	input.Scopes = trimStringList(input.Scopes)

	if input.Query == "" {
		return service.ToolResult{}, fmt.Errorf("query is required")
	}

	hits, err := p.memoryService.SearchActive(
		ctx,
		user.UserID,
		user.ConversationID,
		input.Query,
		input.Keywords,
		input.Limit,
		input.Categories,
		input.Scopes,
	)
	if err != nil {
		return service.ToolResult{}, err
	}

	resultItems := make([]memoryToolItem, 0, len(hits))
	for _, hit := range hits {
		item := hit.Item
		if hit.Content != "" {
			item.Content = hit.Content
		}
		resultItems = append(resultItems, toMemoryToolItem(item))
	}

	payload, err := json.Marshal(searchMemoryResult{Items: resultItems})
	if err != nil {
		return service.ToolResult{}, err
	}

	return service.ToolResult{
		Name:   ToolSearchMemory,
		Result: payload,
	}, nil
}

func (p *BuiltinProvider) addMemory(ctx context.Context, user service.UserContext, args json.RawMessage) (service.ToolResult, error) {
	if p.memoryService == nil {
		return service.ToolResult{}, fmt.Errorf("memory service is not configured")
	}
	if user.UserID <= 0 {
		return service.ToolResult{}, fmt.Errorf("invalid user context")
	}

	var input addMemoryArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}

	scope := strings.TrimSpace(input.Scope)
	sourceType := strings.TrimSpace(input.SourceType)
	sourceRef := strings.TrimSpace(input.SourceRef)

	if sourceType == "" && user.ConversationID != "" {
		sourceType = memory.MemorySourceTypeConversation
		sourceRef = user.ConversationID
	}
	if sourceType == memory.MemorySourceTypeConversation {
		if user.ConversationID == "" {
			return service.ToolResult{}, fmt.Errorf("conversation is required")
		}
		sourceRef = user.ConversationID
	}

	expiresAt, err := parseMemoryExpiresAt(input.ExpiresAt)
	if err != nil {
		return service.ToolResult{}, err
	}

	_, err = p.memoryService.Create(ctx, user.UserID, service.CreateMemoryInput{
		ConversationID: user.ConversationID,
		Scope:          scope,
		Category:       strings.TrimSpace(input.Category),
		Origin:         memoryOriginForAdd(sourceType),
		SourceType:     sourceType,
		SourceRef:      sourceRef,
		SourceTitle:    strings.TrimSpace(input.SourceTitle),
		Content:        strings.TrimSpace(input.Content),
		MetadataJSON:   "{}",
		Confidence:     input.Confidence,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return service.ToolResult{}, err
	}

	payload, err := json.Marshal(addMemoryResult{
		OK: true,
	})
	if err != nil {
		return service.ToolResult{}, err
	}

	return service.ToolResult{
		Name:   ToolAddMemory,
		Result: payload,
	}, nil
}

func (p *BuiltinProvider) disableMemory(ctx context.Context, user service.UserContext, args json.RawMessage) (service.ToolResult, error) {
	if p.memoryService == nil {
		return service.ToolResult{}, fmt.Errorf("memory service is not configured")
	}
	if user.UserID <= 0 {
		return service.ToolResult{}, fmt.Errorf("invalid user context")
	}

	var input disableMemoryArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}

	if input.ID <= 0 {
		return service.ToolResult{}, fmt.Errorf("invalid memory id")
	}

	err := p.memoryService.Disable(ctx, user.UserID, input.ID)
	if err != nil {
		return service.ToolResult{}, err
	}

	payload, err := json.Marshal(disableMemoryResult{
		OK: true,
	})
	if err != nil {
		return service.ToolResult{}, err
	}

	return service.ToolResult{
		Name:   ToolDisableMemory,
		Result: payload,
	}, nil
}

func trimStringList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}

func toMemoryToolItem(item entity.MemoryItem) memoryToolItem {
	result := memoryToolItem{
		ID:         item.ID,
		Scope:      item.Scope,
		Category:   item.Category,
		SourceType: item.SourceType,
		Content:    item.Content,
		Confidence: item.Confidence,
		UpdatedAt:  item.UpdatedAt,
	}

	if item.SourceRef.Valid {
		result.SourceRef = item.SourceRef.String
	}
	if item.SourceTitle.Valid {
		result.SourceTitle = item.SourceTitle.String
	}

	return result
}

func parseMemoryExpiresAt(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("expires_at must be RFC3339")
	}

	return &parsed, nil
}

func memoryOriginForAdd(sourceType string) string {
	switch sourceType {
	case memory.MemorySourceTypeWeb,
		memory.MemorySourceTypeAPI,
		memory.MemorySourceTypeRepo,
		memory.MemorySourceTypeFile:
		return memory.MemoryOriginToolGenerated
	default:
		return memory.MemoryOriginAssistantSummary
	}
}
