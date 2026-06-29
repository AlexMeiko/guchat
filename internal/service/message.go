package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
	"github.com/google/uuid"
)

const messageSeqStep = 1024

var (
	ErrMessageNotFound        = errors.New("message not found")
	ErrMessageSeqGapExhausted = errors.New("message seq gap exhausted")
)

type CreateMessageInput struct {
	ConversationID   string
	Role             string
	Content          string
	ReasoningContent string
	SummaryContent   string
	Status           string
	ErrorMessage     string
	PrevID           string
}

type GenerationContextResult struct {
	SummaryContent string
	Messages       []entity.Message
}

type ListMessagesPageInput struct {
	ConversationID string
	BeforeSeq      *int
	Limit          int
}

type ListMessagesPageResult struct {
	Messages      []entity.Message
	HasMore       bool
	NextBeforeSeq *int
}

type UpdateGeneratedMessageInput struct {
	ConversationID   string
	MessageID        string
	Content          string
	ReasoningContent string
	Status           string
	ErrorMessage     string
}

type MessageService struct {
	conversationRepo *repository.ConversationRepository
	messageRepo      *repository.MessageRepository
}

func NewMessageService(
	conversationRepo *repository.ConversationRepository,
	messageRepo *repository.MessageRepository,
) *MessageService {
	return &MessageService{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
	}
}

func (s *MessageService) CreateMessage(ctx context.Context, userID int64, input CreateMessageInput) (*entity.Message, error) {
	if err := s.ensureConversationOwned(ctx, userID, input.ConversationID); err != nil {
		return nil, err
	}

	seq, err := s.nextSeqForCreate(ctx, input.ConversationID, input.PrevID)
	if err != nil {
		return nil, err
	}

	if input.Status == "" {
		input.Status = "done"
	}

	message := &entity.Message{
		ID:               uuid.NewString(),
		ConversationID:   input.ConversationID,
		Role:             input.Role,
		Content:          input.Content,
		ReasoningContent: input.ReasoningContent,
		SummaryContent:   input.SummaryContent,
		HasSummary:       strings.TrimSpace(input.SummaryContent) != "",
		Status:           input.Status,
		ErrorMessage:     input.ErrorMessage,
		Seq:              seq,
	}

	if err := s.messageRepo.Create(ctx, message); err != nil {
		return nil, err
	}

	if input.PrevID != "" {
		if _, found, err := s.messageRepo.GetNextSeqByConversationIDAndSeq(ctx, input.ConversationID, message.Seq); err != nil {
			return nil, err
		} else if found {
			if err := s.messageRepo.ClearSummaryContentAfterSeq(ctx, input.ConversationID, message.Seq); err != nil {
				return nil, err
			}
		}
	}

	if err := s.conversationRepo.TouchByIDAndUserID(ctx, input.ConversationID, userID); err != nil {
		return nil, err
	}

	created, err := s.messageRepo.GetByIDAndConversationID(ctx, message.ID, message.ConversationID)
	if err != nil {
		return nil, err
	}

	return created, nil
}

//func (s *MessageService) ListByConversationID(ctx context.Context, userID int64, conversationID string) ([]entity.Message, error) {
//	if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
//		return nil, err
//	}
//
//	return s.messageRepo.ListByConversationID(ctx, conversationID)
//}

func (s *MessageService) ListPageByConversationID(
	ctx context.Context,
	userID int64,
	input ListMessagesPageInput,
) (*ListMessagesPageResult, error) {
	if err := s.ensureConversationOwned(ctx, userID, input.ConversationID); err != nil {
		return nil, err
	}

	messages, hasMore, err := s.messageRepo.ListPageByConversationID(
		ctx,
		input.ConversationID,
		input.BeforeSeq,
		input.Limit,
	)
	if err != nil {
		return nil, err
	}

	var nextBeforeSeq *int
	if hasMore && len(messages) > 0 {
		seq := messages[0].Seq
		nextBeforeSeq = &seq
	}

	return &ListMessagesPageResult{
		Messages:      messages,
		HasMore:       hasMore,
		NextBeforeSeq: nextBeforeSeq,
	}, nil
}

func (s *MessageService) ListByConversationIDBeforeOrEqualSeq(
	ctx context.Context,
	userID int64,
	conversationID string,
	seq int,
) ([]entity.Message, error) {
	if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	return s.messageRepo.ListByConversationIDBeforeOrEqualSeq(ctx, conversationID, seq)
}

func (s *MessageService) ListGenerationContextWithSummaryByConversationID(
	ctx context.Context,
	userID int64,
	conversationID string,
	seq int,
) (*GenerationContextResult, error) {
	if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	anchor, found, err := s.messageRepo.GetLatestSummaryBeforeOrEqualSeq(ctx, conversationID, seq)
	if err != nil {
		return nil, err
	}

	afterSeq := 0
	summaryContent := ""
	if found {
		afterSeq = anchor.Seq
		summaryContent = anchor.SummaryContent
	}

	messages, err := s.messageRepo.ListByConversationIDAfterSeqBeforeOrEqualSeq(ctx, conversationID, afterSeq, seq)
	if err != nil {
		return nil, err
	}

	return &GenerationContextResult{
		SummaryContent: summaryContent,
		Messages:       messages,
	}, nil
}

func (s *MessageService) GetByIDAndConversationID(ctx context.Context, userID int64, conversationID, messageID string) (*entity.Message, error) {
	if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	message, err := s.messageRepo.GetByIDAndConversationID(ctx, messageID, conversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMessageNotFound
		}

		return nil, err
	}

	return message, nil
}

func (s *MessageService) UpdateContentByIDAndConversationID(ctx context.Context, userID int64, conversationID, messageID, content string) (*entity.Message, error) {
	if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	updated, err := s.messageRepo.UpdateContentByIDAndConversationID(ctx, messageID, conversationID, content)
	if err != nil {
		return nil, err
	}

	if !updated {
		return nil, ErrMessageNotFound
	}

	if err := s.messageRepo.ClearSummaryContentFromMessageID(ctx, conversationID, messageID); err != nil {
		return nil, err
	}

	result, err := s.messageRepo.GetByIDAndConversationID(ctx, messageID, conversationID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *MessageService) UpdateSummaryContentByIDAndConversationID(ctx context.Context, userID int64, conversationID, messageID, summaryContent string) error {
	if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
		return err
	}

	updated, err := s.messageRepo.UpdateSummaryContentByIDAndConversationID(ctx, messageID, conversationID, summaryContent)
	if err != nil {
		return err
	}
	if !updated {
		return ErrMessageNotFound
	}

	return nil
}

func (s *MessageService) DeleteByIDAndConversationID(ctx context.Context, userID int64, conversationID, messageID string) error {
	if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
		return err
	}

	if err := s.messageRepo.ClearSummaryContentAfterMessageID(ctx, conversationID, messageID); err != nil {
		return err
	}

	deleted, err := s.messageRepo.DeleteByIDAndConversationID(ctx, messageID, conversationID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrMessageNotFound
	}

	return s.conversationRepo.TouchByIDAndUserID(ctx, conversationID, userID)
}

// 供generation使用
func (s *MessageService) UpdateGeneratedMessage(
	ctx context.Context,
	userID int64,
	input UpdateGeneratedMessageInput,
) error {
	if err := s.ensureConversationOwned(ctx, userID, input.ConversationID); err != nil {
		return err
	}

	message := &entity.Message{
		ID:               input.MessageID,
		ConversationID:   input.ConversationID,
		Content:          input.Content,
		ReasoningContent: input.ReasoningContent,
		Status:           input.Status,
		ErrorMessage:     input.ErrorMessage,
	}

	updated, err := s.messageRepo.UpdateByIDAndConversationID(ctx, message)
	if err != nil {
		return err
	}
	if !updated {
		return ErrMessageNotFound
	}

	return nil
}

func (s *MessageService) RecoverInterruptedGenerations(ctx context.Context) (int64, error) {
	return s.messageRepo.FailUnfinishedMsg(
		ctx,
		"generation interrupted by server restart",
	)
}

func (s *MessageService) nextSeqForCreate(ctx context.Context, conversationID, prevID string) (int, error) {
	if prevID == "" {
		lastSeq, err := s.messageRepo.GetLastSeqByConversationID(ctx, conversationID)
		if err != nil {
			return 0, err
		}
		return lastSeq + messageSeqStep, nil
	}

	prevMessage, err := s.messageRepo.GetByIDAndConversationID(ctx, prevID, conversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrMessageNotFound
		}
		return 0, err
	}

	nextSeq, found, err := s.messageRepo.GetNextSeqByConversationIDAndSeq(ctx, conversationID, prevMessage.Seq)
	if err != nil {
		return 0, err
	}

	if !found {
		return prevMessage.Seq + messageSeqStep, nil
	}

	seq := prevMessage.Seq + ((nextSeq - prevMessage.Seq) >> 1)
	if seq == prevMessage.Seq || seq == nextSeq {
		return 0, ErrMessageSeqGapExhausted
	}

	return seq, nil
}

func (s *MessageService) ensureConversationOwned(ctx context.Context, userID int64, conversationID string) error {
	_, err := s.conversationRepo.GetByIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConversationNotFound
		}

		return err
	}

	return nil
}
