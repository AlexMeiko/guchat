package memory

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
)

const (
	MemoryScopeUser         = "user"
	MemoryScopeConversation = "conversation"
	MemoryScopeGlobal       = "global"

	MemoryStatusActive   = "active"
	MemoryStatusDisabled = "disabled"
	MemoryStatusDeleted  = "deleted"

	// MemoryCategoryUserProfile 用户信息
	MemoryCategoryUserProfile = "user_profile"

	// MemoryCategoryPreference 偏好
	MemoryCategoryPreference = "preference"

	// MemoryCategoryFact 事实
	MemoryCategoryFact = "fact"

	// MemoryCategoryKnowledge 知识 / 文档条目
	MemoryCategoryKnowledge = "knowledge"

	// MemoryCategoryGoal 目标
	MemoryCategoryGoal = "goal"

	// MemoryCategoryRelationship 关系
	MemoryCategoryRelationship = "relationship"

	// MemoryCategoryExperience 经验
	MemoryCategoryExperience = "experience"

	// MemoryCategoryDailySummary 每日总结
	MemoryCategoryDailySummary = "daily_summary"

	// MemoryCategoryConstraint 约束
	MemoryCategoryConstraint = "constraint"

	// MemoryCategoryNegativePreference 禁忌 / 负偏好
	MemoryCategoryNegativePreference = "negative_preference"

	// MemoryCategorySituational 记录短期的语境
	MemoryCategorySituational = "situational"

	MemorySourceTypeNone         = "none"
	MemorySourceTypeConversation = "conversation"
	MemorySourceTypeWeb          = "web"
	MemorySourceTypeFile         = "file"
	MemorySourceTypeAPI          = "api"
	MemorySourceTypeRepo         = "repo"
	MemorySourceTypeManual       = "manual"

	MemoryOriginUserExplicit     = "user_explicit"
	MemoryOriginUserImported     = "user_imported"
	MemoryOriginBehaviorInferred = "behavior_inferred"
	MemoryOriginAssistantSummary = "assistant_summary"
	MemoryOriginSystemGenerated  = "system_generated"
	MemoryOriginToolGenerated    = "tool_generated"
)

type ListFilter struct {
	UserID     int64
	Statuses   []string
	Categories []string
	Scopes     []string
	Limit      int
	Offset     int
}

type SearchInput struct {
	UserID         int64
	ConversationID string
	Query          string
	Keywords       []string
	Categories     []string
	Scopes         []string
	Limit          int
}

type PromptFilter struct {
	UserID     int64
	Categories []string
	Limit      int
}

type Store interface {
	Create(ctx context.Context, item *entity.MemoryItem) error
	List(ctx context.Context, filter ListFilter) ([]entity.MemoryItem, error)
	ListPrompt(ctx context.Context, filter PromptFilter) ([]entity.MemoryItem, error)
	GetByID(ctx context.Context, userID int64, id int64) (*entity.MemoryItem, error)
	UpdateStatus(ctx context.Context, userID int64, id int64, status string) (bool, error)
	SoftDelete(ctx context.Context, userID int64, id int64) (bool, error)
	Search(ctx context.Context, input SearchInput) ([]entity.MemoryItem, error)
}
