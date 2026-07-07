package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
	"github.com/AlexMeiko/guchat/internal/sandbox"
	"github.com/google/uuid"
)

var ErrConversationNotFound = errors.New("conversation not found")

type ConversationService struct {
	conversationRepo *repository.ConversationRepository
	workspaceManager *sandbox.WorkspaceManager
	sandboxManager   *sandbox.Manager
}

func NewConversationService(
	conversationRepo *repository.ConversationRepository,
	workspaceManager *sandbox.WorkspaceManager,
	sandboxManager *sandbox.Manager,
) *ConversationService {
	return &ConversationService{
		conversationRepo: conversationRepo,
		workspaceManager: workspaceManager,
		sandboxManager:   sandboxManager,
	}
}

func (s *ConversationService) Create(ctx context.Context, userID int64, title string) (*entity.Conversation, error) {
	conversation := &entity.Conversation{
		ID:     uuid.NewString(),
		UserID: userID,
		Title:  title,
	}

	if err := s.conversationRepo.Create(ctx, conversation); err != nil {
		return nil, err
	}

	result, err := s.conversationRepo.GetByIDAndUserID(ctx, conversation.ID, userID)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ConversationService) List(ctx context.Context, userID int64) ([]entity.Conversation, error) {
	return s.conversationRepo.ListByUserID(ctx, userID)
}

func (s *ConversationService) Get(ctx context.Context, userID int64, conversationID string) (*entity.Conversation, error) {
	conversation, err := s.conversationRepo.GetByIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	return conversation, nil
}

func (s *ConversationService) UpdateTitle(ctx context.Context, userID int64, conversationID string, title string) error {
	updated, err := s.conversationRepo.UpdateTitleByIDAndUserID(ctx, conversationID, userID, title)
	if err != nil {
		return err
	}

	if !updated {
		return ErrConversationNotFound
	}

	return nil
}

func (s *ConversationService) Delete(ctx context.Context, userID int64, conversationID string) error {
	if _, err := s.Get(ctx, userID, conversationID); err != nil {
		return err
	}

	if s.sandboxManager != nil {
		if err := s.sandboxManager.Destroy(ctx, userID, conversationID); err != nil {
			return err
		}
	}

	if s.workspaceManager != nil {
		if err := s.workspaceManager.DeleteConversation(ctx, userID, conversationID); err != nil {
			return err
		}
	}

	deleted, err := s.conversationRepo.DeleteByIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return err
	}

	if !deleted {
		return ErrConversationNotFound
	}

	return nil
}
