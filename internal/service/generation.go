package service

import (
	"context"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
)

var (
	ErrModelDisabled = errors.New("model disabled")
)

type CreateGenerationInput struct {
	ConversationID  string
	SourceMessageID string
	ModelID         int64
}

type GenerationService struct {
	messageService   *MessageService
	modelService     *ModelService
	generatorFactory GeneratorFactory
}

func NewGenerationService(
	messageService *MessageService,
	modelService *ModelService,
	generatorFactory GeneratorFactory,
) *GenerationService {
	return &GenerationService{
		messageService:   messageService,
		modelService:     modelService,
		generatorFactory: generatorFactory,
	}
}

func (s *GenerationService) Create(
	ctx context.Context,
	userID int64,
	input CreateGenerationInput,
) (*entity.Message, error) {
	sourceMessage, err := s.messageService.GetByIDAndConversationID(
		ctx,
		userID,
		input.ConversationID,
		input.SourceMessageID,
	)
	if err != nil {
		return nil, err
	}

	modelConfig, err := s.modelService.GetByID(ctx, input.ModelID)
	if err != nil {
		return nil, err
	}

	if !modelConfig.IsEnabled {
		return nil, ErrModelDisabled
	}

	assistantMsg, err := s.messageService.CreateMessage(ctx, userID, CreateMessageInput{
		ConversationID:   input.ConversationID,
		Role:             entity.MessageRoleAssistant,
		Content:          "",
		ReasoningContent: "",
		Status:           entity.MessageStatusPending,
		ErrorMessage:     "",
		PrevID:           input.SourceMessageID,
	})
	if err != nil {
		return nil, err
	}

	go func() {
		_ = s.Process(
			context.Background(),
			userID,
			input.ConversationID,
			assistantMsg.ID,
			sourceMessage.Seq,
			input.ModelID,
		)
	}()

	return assistantMsg, nil
}

func (s *GenerationService) Process(
	ctx context.Context,
	userID int64,
	conversationID string,
	assistantMessageID string,
	sourceMessageSeq int,
	modelID int64,
) error {
	fail := func(result *GenerateResult, cause error) error {
		input := UpdateGeneratedMessageInput{
			ConversationID:   conversationID,
			MessageID:        assistantMessageID,
			Content:          "",
			ReasoningContent: "",
			Status:           entity.MessageStatusFailed,
			ErrorMessage:     cause.Error(),
		}

		if result != nil {
			input.Content = result.Content
			input.ReasoningContent = result.ReasoningContent
		}

		if err := s.messageService.UpdateGeneratedMessage(ctx, userID, input); err != nil {
			return errors.Join(cause, err)
		}

		return cause
	}

	modelConfig, err := s.modelService.GetByID(ctx, modelID)
	if err != nil {
		return fail(nil, err)
	}

	if !modelConfig.IsEnabled {
		return fail(nil, ErrModelDisabled)
	}

	messages, err := s.messageService.ListByConversationIDBeforeOrEqualSeq(
		ctx,
		userID,
		conversationID,
		sourceMessageSeq,
	)
	if err != nil {
		return fail(nil, err)
	}

	generator, err := s.generatorFactory.Get(modelConfig)
	if err != nil {
		return fail(nil, err)
	}

	result, err := generator.Generate(ctx, GenerateInput{
		Model:    modelConfig,
		Messages: messages,
	})
	if err != nil {
		return fail(result, err)
	}

	if result == nil {
		return fail(nil, errors.New("generator returned nil result"))
	}

	return s.messageService.UpdateGeneratedMessage(ctx, userID, UpdateGeneratedMessageInput{
		ConversationID:   conversationID,
		MessageID:        assistantMessageID,
		Content:          result.Content,
		ReasoningContent: result.ReasoningContent,
		Status:           entity.MessageStatusDone,
		ErrorMessage:     "",
	})
}
