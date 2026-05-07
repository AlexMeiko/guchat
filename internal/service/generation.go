package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
	"github.com/AlexMeiko/guchat/internal/stream"
)

var (
	ErrModelDisabled          = errors.New("model disabled")
	ErrGenerationTaskNotFound = errors.New("generation task not found")
)

const (
	ToolModeNone = "none"
	ToolModeAuto = "auto"
)

// TODO: 默认上下文限制，后面放到config里
const defaultGenerationContextLimit = 25

// TODO: 调用工具最大轮次，每轮可以调用多个工具，后面放到config里
const defaultMaxToolRounds = 8

type CreateGenerationInput struct {
	ConversationID  string
	SourceMessageID string
	ModelID         int64
	ContextLimit    int
	ToolMode        string
}

type generationRetryItem struct {
	UserID         int64
	ConversationID string
	MessageID      string
	RetryCount     int
}

type GenerationService struct {
	messageService      *MessageService
	modelService        *ModelService
	generatorFactory    GeneratorFactory
	runtimeManager      *stream.Manager
	toolProviderManager *ToolProviderManager
	toolCallRepo        *repository.ToolCallRepository
	retryInterval       time.Duration
	retryMax            int

	retryMu    sync.Mutex
	retryItems map[string]*generationRetryItem
}

func NewGenerationService(
	messageService *MessageService,
	modelService *ModelService,
	generatorFactory GeneratorFactory,
	runtimeManager *stream.Manager,
	toolProviderManager *ToolProviderManager,
	toolCallRepo *repository.ToolCallRepository,
	retryInterval time.Duration,
	retryMax int,
) *GenerationService {
	return &GenerationService{
		messageService:      messageService,
		modelService:        modelService,
		generatorFactory:    generatorFactory,
		runtimeManager:      runtimeManager,
		toolProviderManager: toolProviderManager,
		toolCallRepo:        toolCallRepo,
		retryInterval:       retryInterval,
		retryMax:            retryMax,
		retryItems:          make(map[string]*generationRetryItem),
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

	processCtx, cancel := context.WithCancel(context.Background())
	s.runtimeManager.Create(assistantMsg.ID, cancel)

	if input.ContextLimit <= 0 {
		input.ContextLimit = defaultGenerationContextLimit
	}

	go func() {
		_ = s.Process(
			processCtx,
			userID,
			input.ConversationID,
			assistantMsg.ID,
			sourceMessage.Seq,
			input.ModelID,
			input.ContextLimit,
			input.ToolMode,
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
	userContextLimit int,
	toolMode string,
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
			if s.shouldRetry(err) {
				s.addRetry(userID, conversationID, assistantMessageID)
			} else {
				s.runtimeManager.Delete(assistantMessageID)
			}
			return errors.Join(cause, err)
		}

		s.runtimeManager.Delete(assistantMessageID)
		return cause
	}

	modelConfig, err := s.modelService.GetByID(ctx, modelID)
	if err != nil {
		return fail(err)
	}

	if !modelConfig.IsEnabled {
		return fail(ErrModelDisabled)
	}

	messages, err := s.messageService.ListGenerationContextByConversationID(
		ctx,
		userID,
		conversationID,
		sourceMessageSeq,
		userContextLimit,
	)
	if err != nil {
		return fail(err)
	}

	generator, err := s.generatorFactory.Get(modelConfig)
	if err != nil {
		return fail(err)
	}

	var tools []ToolDefinition
	if toolMode == ToolModeAuto {
		tools, err = s.toolProviderManager.ListTools(ctx, UserContext{
			UserID: userID,
		})
		if err != nil {
			return fail(err)
		}
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

	var toolResults []ToolResult
	completed := false

	for round := 1; round <= defaultMaxToolRounds; round++ {
		var calls []ToolCall

		err = generator.Generate(ctx, GenerateInput{
			Model:       modelConfig,
			Messages:    messages,
			Tools:       tools,
			ToolResults: toolResults,
		}, GenerateCallbacks{
			ContentDelta:   task.AppendContent,
			ReasoningDelta: task.AppendReasoningContent,
			ToolCallCreated: func(call ToolCall) {
				calls = append(calls, call)
			},
		})
		if err != nil {
			return fail(err)
		}

		if len(calls) == 0 {
			task.Done()
			completed = true
			break
		}

		for i, modelCall := range calls {
			record := &entity.ToolCall{
				ConversationID:     conversationID,
				AssistantMessageID: assistantMessageID,
				ProviderCallID:     modelCall.ID,
				ToolName:           modelCall.Name,
				ArgumentsJSON:      string(modelCall.Arguments),
				ResultJSON:         "",
				Status:             entity.ToolCallStatusPending,
				ErrorMessage:       "",
				Round:              round,
				Seq:                i + 1,
			}
			
			//在数据库创建工具调用记录
			if err := s.toolCallRepo.Create(ctx, record); err != nil {
				return fail(err)
			}

			task.AddToolCall(stream.ToolCallSnapshot{
				ID:         record.ID,
				ProviderID: record.ProviderCallID,
				Name:       record.ToolName,
				Arguments:  record.ArgumentsJSON,
				Result:     record.ResultJSON,
				Status:     record.Status,
				Round:      record.Round,
				Seq:        record.Seq,
			})

			//添加占位符，保持文本和工具调用的顺序性方便前端展示
			task.AppendContent("\n\n<!--tool_call:" + strconv.FormatInt(record.ID, 10) + "-->\n\n")

			err := s.toolCallRepo.MarkRunning(ctx, record.ID)
			if err != nil {
				return fail(err)
			}
			task.UpdateToolCallRunning(record.ID)

			toolResult, err := s.toolProviderManager.CallTool(ctx, UserContext{UserID: userID}, modelCall.Name, modelCall.Arguments)
			if err != nil {
				//工具执行失败
				payload, marshalErr := json.Marshal(map[string]any{
					"ok":    false,
					"error": err.Error(),
				})
				if marshalErr != nil {
					return fail(errors.Join(err, marshalErr))
				}

				//把失败结果交给AI处理
				toolResults = append(toolResults, ToolResult{
					ToolCallID: modelCall.ID,
					Name:       modelCall.Name,
					Result:     payload,
				})

				err = s.toolCallRepo.MarkFailed(ctx, record.ID, string(payload), err.Error())
				if err != nil {
					return fail(err)
				}
				task.UpdateToolCallFailed(record.ID, string(payload), err.Error())

				continue
			}

			toolResult.ToolCallID = modelCall.ID

			if err := s.toolCallRepo.MarkDone(ctx, record.ID, string(toolResult.Result)); err != nil {
				return fail(err)
			}
			task.UpdateToolCallDone(record.ID, string(toolResult.Result))

			toolResults = append(toolResults, toolResult)
		}
	}

	if !completed {
		return fail(errors.New("max tool rounds exceeded"))
	}

	snapshot := task.Snapshot()

	if err := s.messageService.UpdateGeneratedMessage(ctx, userID, UpdateGeneratedMessageInput{
		ConversationID:   conversationID,
		MessageID:        assistantMessageID,
		Content:          snapshot.Content,
		ReasoningContent: snapshot.ReasoningContent,
		Status:           snapshot.Status,
		ErrorMessage:     snapshot.ErrorMessage,
	}); err != nil {
		if s.shouldRetry(err) {
			s.addRetry(userID, conversationID, assistantMessageID)
		} else {
			s.runtimeManager.Delete(assistantMessageID)
		}
		return err
	}

	s.runtimeManager.Delete(assistantMessageID)
	return nil
}

func (s *GenerationService) RetryLoop(ctx context.Context) {
	ticker := time.NewTicker(s.retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Retry()
		}
	}
}

func (s *GenerationService) addRetry(userID int64, conversationID, messageID string) {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()

	if _, exists := s.retryItems[messageID]; exists {
		return
	}

	s.retryItems[messageID] = &generationRetryItem{
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      messageID,
		RetryCount:     0,
	}
}

func (s *GenerationService) Retry() {
	// 切片主要是为了分离数据库操作，避免在持锁状态下进行数据库读写，导致长时间占用锁致使其他业务无法读取或写入map
	s.retryMu.Lock()
	items := make([]generationRetryItem, 0, len(s.retryItems))
	for _, item := range s.retryItems {
		items = append(items, *item)
	}
	s.retryMu.Unlock()

	for _, item := range items {
		if item.RetryCount >= s.retryMax {
			s.removeRetry(item.MessageID)
			s.runtimeManager.Delete(item.MessageID)
			continue
		}

		task, ok := s.runtimeManager.Get(item.MessageID)
		if !ok {
			s.removeRetry(item.MessageID)
			continue
		}

		snapshot := task.Snapshot()
		err := s.messageService.UpdateGeneratedMessage(context.Background(), item.UserID, UpdateGeneratedMessageInput{
			ConversationID:   item.ConversationID,
			MessageID:        item.MessageID,
			Content:          snapshot.Content,
			ReasoningContent: snapshot.ReasoningContent,
			Status:           snapshot.Status,
			ErrorMessage:     snapshot.ErrorMessage,
		})
		if err != nil {
			if !s.shouldRetry(err) {
				s.removeRetry(item.MessageID)
				s.runtimeManager.Delete(item.MessageID)
				continue
			}

			s.retryMu.Lock()
			if current, exists := s.retryItems[item.MessageID]; exists {
				current.RetryCount++
			}
			s.retryMu.Unlock()
			continue
		}

		s.removeRetry(item.MessageID)
		s.runtimeManager.Delete(item.MessageID)
	}
}

func (s *GenerationService) removeRetry(messageID string) {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()

	delete(s.retryItems, messageID)
}

func (s *GenerationService) shouldRetry(err error) bool {
	return !errors.Is(err, ErrConversationNotFound) && !errors.Is(err, ErrMessageNotFound)
}
