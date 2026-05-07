package service

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
)

type ToolCallService struct {
	toolCallRepo *repository.ToolCallRepository
}

func NewToolCallService(toolCallRepo *repository.ToolCallRepository) *ToolCallService {
	return &ToolCallService{toolCallRepo: toolCallRepo}
}

func (s *ToolCallService) ListByAssistantMessageID(
	ctx context.Context,
	assistantMessageID string,
) ([]entity.ToolCall, error) {
	return s.toolCallRepo.ListByAssistantMessageID(ctx, assistantMessageID)
}

func (s *ToolCallService) ListByAssistantMessageIDs(
	ctx context.Context,
	assistantMessageIDs []string,
) ([]entity.ToolCall, error) {
	return s.toolCallRepo.ListByAssistantMessageIDs(ctx, assistantMessageIDs)
}
