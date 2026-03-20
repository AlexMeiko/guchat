package service

import (
	"context"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/stream"
)

var (
	ErrModelDisabled          = errors.New("model disabled")
	ErrGenerationTaskNotFound = errors.New("generation task not found")
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
	runtimeManager   *stream.Manager
}

func NewGenerationService(
	messageService *MessageService,
	modelService *ModelService,
	generatorFactory GeneratorFactory,
	runtimeManager *stream.Manager,
) *GenerationService {
	return &GenerationService{
		messageService:   messageService,
		modelService:     modelService,
		generatorFactory: generatorFactory,
		runtimeManager:   runtimeManager,
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

	s.runtimeManager.Create(assistantMsg.ID)

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
	task, ok := s.runtimeManager.Get(assistantMessageID)
	if !ok {
		return ErrGenerationTaskNotFound
	}

	fail := func(cause error) error {
		task.Failed(cause.Error())
		snapshot := task.Snapshot()

		if err := s.messageService.UpdateGeneratedMessage(ctx, userID, UpdateGeneratedMessageInput{
			ConversationID:   conversationID,
			MessageID:        assistantMessageID,
			Content:          snapshot.Content,
			ReasoningContent: snapshot.ReasoningContent,
			Status:           snapshot.Status,
			ErrorMessage:     snapshot.ErrorMessage,
		}); err != nil {
			return errors.Join(cause, err)
		}

		return cause
	}

	modelConfig, err := s.modelService.GetByID(ctx, modelID)
	if err != nil {
		return fail(err)
	}

	if !modelConfig.IsEnabled {
		return fail(ErrModelDisabled)
	}

	messages, err := s.messageService.ListByConversationIDBeforeOrEqualSeq(
		ctx,
		userID,
		conversationID,
		sourceMessageSeq,
	)
	if err != nil {
		return fail(err)
	}

	generator, err := s.generatorFactory.Get(modelConfig)
	if err != nil {
		return fail(err)
	}

	task.Start()

	if err := s.messageService.UpdateGeneratedMessage(ctx, userID, UpdateGeneratedMessageInput{
		ConversationID:   conversationID,
		MessageID:        assistantMessageID,
		Content:          "",
		ReasoningContent: "",
		Status:           entity.MessageStatusStreaming,
		ErrorMessage:     "",
	}); err != nil {
		return fail(err)
	}

	err = generator.Generate(ctx, GenerateInput{
		Model:    modelConfig,
		Messages: messages,
	}, GenerateCallbacks{
		task.AppendContent,
		task.AppendReasoningContent,
	})
	if err != nil {
		return fail(err)
	}

	task.Done()
	snapshot := task.Snapshot()

	if err := s.messageService.UpdateGeneratedMessage(ctx, userID, UpdateGeneratedMessageInput{
		ConversationID:   conversationID,
		MessageID:        assistantMessageID,
		Content:          snapshot.Content,
		ReasoningContent: snapshot.ReasoningContent,
		Status:           snapshot.Status,
		ErrorMessage:     snapshot.ErrorMessage,
	}); err != nil {
		return fail(err)
	}

	s.runtimeManager.Delete(assistantMessageID)
	return nil
}
