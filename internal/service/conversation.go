package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
	"github.com/google/uuid"
)

var ErrConversationNotFound = errors.New("conversation not found")

type ConversationService struct {
	conversationRepo *repository.ConversationRepository
}

func NewConversationService(conversationRepo *repository.ConversationRepository) *ConversationService {
	return &ConversationService{
		conversationRepo: conversationRepo,
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
	deleted, err := s.conversationRepo.DeleteByIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return err
	}

	if !deleted {
		return ErrConversationNotFound
	}

	return nil
}
