package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
	"github.com/AlexMeiko/guchat/internal/stream"
)

var (
	ErrModelDisabled          = errors.New("model disabled")
	ErrGenerationInProgress   = errors.New("generation already in progress")
	ErrGenerationTaskNotFound = errors.New("generation task not found")
)

const (
	ToolModeNone = "none"
	ToolModeAuto = "auto"

	promptMemoryLimit = 15
)

const compressedContextPrompt = "以下是此前对话的压缩摘要。请把它作为连续上下文使用，但如果它与后续原始消息冲突，以后续原始消息为准。\n\n"

type CreateGenerationInput struct {
	ConversationID  string
	SourceMessageID string
	ModelID         int64
	ToolMode        string
}

type generationRetryItem struct {
	UserID         int64
	ConversationID string
	MessageID      string
	RetryCount     int
}

type GenerationService struct {
	messageService       *MessageService
	modelService         *ModelService
	memoryService        *MemoryService
	generatorFactory     GeneratorFactory
	runtimeManager       *stream.Manager
	toolProviderManager  *ToolProviderManager
	assembler            *ContextAssembler
	toolCallRepo         *repository.ToolCallRepository
	generationRoundRepo  *repository.GenerationRoundRepository
	contextTokenLimit    int
	contextCompressRatio float64
	maxToolRounds        int
	visionAnalyzer       AnalyzeImageFunc
	retryInterval        time.Duration
	retryMax             int

	retryMu    sync.Mutex
	retryItems map[string]*generationRetryItem
}

// 用于在 provider 没有返回工具调用 ID 时生成稳定 ID，保证占位符、落库记录和工具结果能互相关联
func newGeneratedToolCallID(assistantMessageID string, round int, seq int) string {
	return fmt.Sprintf("call_%s_%d_%d", assistantMessageID, round, seq)
}

func NewGenerationService(
	messageService *MessageService,
	modelService *ModelService,
	memoryService *MemoryService,
	generatorFactory GeneratorFactory,
	runtimeManager *stream.Manager,
	toolProviderManager *ToolProviderManager,
	assembler *ContextAssembler,
	toolCallRepo *repository.ToolCallRepository,
	generationRoundRepo *repository.GenerationRoundRepository,
	contextTokenLimit int,
	contextCompressRatio float64,
	maxToolRounds int,
	visionFallbackModelID int64,
	retryInterval time.Duration,
	retryMax int,
) *GenerationService {
	return &GenerationService{
		messageService:       messageService,
		modelService:         modelService,
		memoryService:        memoryService,
		generatorFactory:     generatorFactory,
		runtimeManager:       runtimeManager,
		toolProviderManager:  toolProviderManager,
		assembler:            assembler,
		toolCallRepo:         toolCallRepo,
		generationRoundRepo:  generationRoundRepo,
		contextTokenLimit:    contextTokenLimit,
		contextCompressRatio: contextCompressRatio,
		maxToolRounds:        maxToolRounds,
		visionAnalyzer:       newVisionAnalyzer(modelService, generatorFactory, visionFallbackModelID),
		retryInterval:        retryInterval,
		retryMax:             retryMax,
		retryItems:           make(map[string]*generationRetryItem),
	}
}

func newSummaryGenerateMessage(summaryContent string) *GenerateMessage {
	summaryContent = strings.TrimSpace(summaryContent)
	if summaryContent == "" {
		return nil
	}

	return &GenerateMessage{
		Role:    entity.MessageRoleSystem,
		Content: compressedContextPrompt + summaryContent,
	}
}

func (s *GenerationService) contextCompressTriggerTokens() int {
	triggerTokens := int(float64(s.contextTokenLimit) * s.contextCompressRatio)
	if triggerTokens < 1 {
		return 1
	}
	return triggerTokens
}

func (s *GenerationService) Create(
	ctx context.Context,
	userID int64,
	input CreateGenerationInput,
) error {
	sourceMessage, err := s.messageService.GetByIDAndConversationID(
		ctx,
		userID,
		input.ConversationID,
		input.SourceMessageID,
	)
	if err != nil {
		return err
	}

	modelConfig, err := s.modelService.GetByID(ctx, input.ModelID)
	if err != nil {
		return err
	}

	if !modelConfig.IsEnabled {
		return ErrModelDisabled
	}
	if s.runtimeManager.Has(input.ConversationID) {
		return ErrGenerationInProgress
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
		return err
	}

	processCtx, cancel := context.WithCancel(context.Background())
	task := s.runtimeManager.Create(assistantMsg.ConversationID, assistantMsg.ID, cancel)
	if task == nil {
		cancel()
		if err := s.messageService.DeleteByIDAndConversationID(
			ctx,
			userID,
			input.ConversationID,
			assistantMsg.ID,
		); err != nil {
			return err
		}
		return ErrGenerationInProgress
	}

	s.runtimeManager.Publish(input.ConversationID, stream.Event{
		Type:      stream.EventTypeMessageCreated,
		MessageID: assistantMsg.ID,
		Message:   assistantMsg,
		Task:      task,
	})

	go func() {
		_ = s.Process(
			processCtx,
			userID,
			input.ConversationID,
			assistantMsg.ID,
			sourceMessage.Seq,
			input.ModelID,
			input.ToolMode,
		)
	}()

	return nil
}

func (s *GenerationService) Process(
	ctx context.Context,
	userID int64,
	conversationID string,
	assistantMessageID string,
	sourceMessageSeq int,
	modelID int64,
	toolMode string,
) error {
	task, ok := s.runtimeManager.Get(conversationID, assistantMessageID)
	if !ok {
		return ErrGenerationTaskNotFound
	}

	// ctx 会被「中止生成」和「删除消息」cancel 掉，但「把最终状态写回数据库」必须照常完成：
	// 用被取消的 ctx 写库时 database/sql 会直接返回 context.Canceled、SQL 根本不发出，
	// 消息就会一直停在 streaming，要等重试循环（最长一个重试间隔）才补上。
	// WithoutCancel 保留 ctx 上的值，只去掉取消信号和 deadline。
	terminalCtx := context.WithoutCancel(ctx)

	fail := func(cause error) error {
		task.Failed(cause.Error())
		snapshot := task.Snapshot()

		if err := s.messageService.UpdateGeneratedMessage(terminalCtx, userID, UpdateGeneratedMessageInput{
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
				s.runtimeManager.Delete(conversationID, assistantMessageID)
			}
			return errors.Join(cause, err)
		}

		s.runtimeManager.Delete(conversationID, assistantMessageID)
		return cause
	}

	modelConfig, err := s.modelService.GetByID(ctx, modelID)
	if err != nil {
		return fail(err)
	}

	if !modelConfig.IsEnabled {
		return fail(ErrModelDisabled)
	}

	generationContext, err := s.messageService.ListGenerationContextWithSummaryByConversationID(
		ctx,
		userID,
		conversationID,
		sourceMessageSeq,
	)
	if err != nil {
		return fail(err)
	}

	user := UserContext{
		UserID:         userID,
		ConversationID: conversationID,
		SupportsVision: modelConfig.SupportsVision,
		AnalyzeImage:   s.visionAnalyzer,
	}
	session := s.assembler.Session(user)

	assistantMessageIDs := make([]string, 0, len(generationContext.Messages))
	for _, message := range generationContext.Messages {
		if message.Role == entity.MessageRoleAssistant {
			assistantMessageIDs = append(assistantMessageIDs, message.ID)
		}
	}

	toolCallRecords, err := s.toolCallRepo.ListByAssistantMessageIDs(ctx, assistantMessageIDs)
	if err != nil {
		return fail(err)
	}

	roundRecords, err := s.generationRoundRepo.ListByAssistantMessageIDs(ctx, assistantMessageIDs)
	if err != nil {
		return fail(err)
	}

	generateMessages := s.assembler.BindHistory(
		generationContext.Messages,
		toolCallRecords,
		roundRecords,
	)
	generateMessages, err = session.View(ctx, generateMessages)
	if err != nil {
		return fail(err)
	}

	workingSummary := generationContext.SummaryContent

	// 记忆注入
	var promptMemoryMessage *GenerateMessage

	if s.memoryService != nil {
		promptMemories, err := s.memoryService.ListPromptMemories(ctx, userID, promptMemoryLimit)
		if err != nil {
			return fail(err)
		}

		promptMemoryMessage = newPromptMemoryMessage(promptMemories, toolMode)
	}

	generator, err := s.generatorFactory.Get(modelConfig)
	if err != nil {
		return fail(err)
	}

	var tools []ToolDefinition
	if toolMode == ToolModeAuto {
		tools, err = s.toolProviderManager.ListTools(ctx, user)
		if err != nil {
			return fail(err)
		}
	}

	triggerTokens := s.contextCompressTriggerTokens()
	toolsTokens := estimateToolDefinitionsTokens(tools)
	memoryTokens := 0
	if promptMemoryMessage != nil {
		memoryTokens = estimateGenerateMessageTokens(*promptMemoryMessage)
	}

	summaryTokens := 0
	if summaryMessage := newSummaryGenerateMessage(workingSummary); summaryMessage != nil {
		summaryTokens = estimateGenerateMessageTokens(*summaryMessage)
	}

	buffer := make([]GenerateMessage, 0, len(generateMessages))
	bufferTokens := 0

	for _, nextMessage := range generateMessages {
		nextTokens := estimateGenerateMessageTokens(nextMessage)

		if toolsTokens+memoryTokens+summaryTokens+bufferTokens+nextTokens > triggerTokens && len(buffer) > 0 {
			summaryEnd := len(buffer)
			if buffer[summaryEnd-1].Role != entity.MessageRoleAssistant {
				summaryEnd = 0
				for i := len(buffer) - 1; i >= 0; i-- {
					if buffer[i].Role == entity.MessageRoleAssistant {
						summaryEnd = i + 1
						break
					}
				}
			}

			if summaryEnd > 0 {
				summaryMessages := s.assembler.Prefix(workingSummary, promptMemoryMessage, buffer[:summaryEnd])
				summary, err := generateSummary(ctx, generator, modelConfig, summaryMessages, tools)
				if err != nil {
					return fail(err)
				}

				last := buffer[summaryEnd-1]
				if err := s.messageService.UpdateSummaryContentByIDAndConversationID(
					ctx,
					userID,
					conversationID,
					last.ID,
					summary,
				); err != nil {
					return fail(err)
				}

				workingSummary = summary
				summaryTokens = 0
				if summaryMessage := newSummaryGenerateMessage(workingSummary); summaryMessage != nil {
					summaryTokens = estimateGenerateMessageTokens(*summaryMessage)
				}

				buffer = append(buffer[:0], buffer[summaryEnd:]...)
				bufferTokens = 0
				for _, retainedMessage := range buffer {
					bufferTokens += estimateGenerateMessageTokens(retainedMessage)
				}
			}
		}

		buffer = append(buffer, nextMessage)
		bufferTokens += nextTokens
	}

	generateMessages = s.assembler.Prefix(workingSummary, promptMemoryMessage, buffer)

	if !task.Start() {
		return fail(context.Canceled)
	}

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

	streamingMessage, err := s.messageService.GetByIDAndConversationID(
		ctx,
		userID,
		conversationID,
		assistantMessageID,
	)
	if err != nil {
		return fail(err)
	}
	s.runtimeManager.Publish(conversationID, stream.Event{
		Type:      stream.EventTypeMessageUpdated,
		MessageID: assistantMessageID,
		Message:   streamingMessage,
	})

	var toolExchanges []ToolExchange
	var generationRounds []GenerationRound
	completed := false

	for round := 1; round <= s.maxToolRounds; round++ {
		var calls []ToolCall

		snapshot := task.Snapshot()
		contentStartOffset := len(snapshot.Content)
		reasoningStartOffset := len(snapshot.ReasoningContent)

		roundMessages := s.assembler.Round(generateMessages, GenerateMessage{
			ID:               assistantMessageID,
			Role:             entity.MessageRoleAssistant,
			Content:          snapshot.Content,
			ReasoningContent: snapshot.ReasoningContent,
			ToolExchanges:    toolExchanges,
			GenerationRounds: generationRounds,
		})

		err = generator.Generate(ctx, GenerateInput{
			Model:    modelConfig,
			Messages: roundMessages,
			Tools:    tools,
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

		snapshot = task.Snapshot()
		contentEndOffset := len(snapshot.Content)
		reasoningEndOffset := len(snapshot.ReasoningContent)

		if contentEndOffset > contentStartOffset || reasoningEndOffset > reasoningStartOffset || len(calls) > 0 {
			roundRecord := &entity.GenerationRound{
				ConversationID:       conversationID,
				AssistantMessageID:   assistantMessageID,
				Round:                round,
				ContentStartOffset:   contentStartOffset,
				ContentEndOffset:     contentEndOffset,
				ReasoningStartOffset: reasoningStartOffset,
				ReasoningEndOffset:   reasoningEndOffset,
			}

			if err := s.generationRoundRepo.Create(ctx, roundRecord); err != nil {
				return fail(err)
			}

			generationRounds = append(generationRounds, GenerationRound{
				Round:                roundRecord.Round,
				ContentStartOffset:   roundRecord.ContentStartOffset,
				ContentEndOffset:     roundRecord.ContentEndOffset,
				ReasoningStartOffset: roundRecord.ReasoningStartOffset,
				ReasoningEndOffset:   roundRecord.ReasoningEndOffset,
			})
		}

		if len(calls) == 0 {
			task.Done()
			completed = true
			break
		}

		for i, modelCall := range calls {
			if modelCall.ID == "" {
				modelCall.ID = newGeneratedToolCallID(assistantMessageID, round, i+1)
			}

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
			task.AppendContent("\n\n<!--tool_call:" + record.ProviderCallID + "-->\n\n")

			err := s.toolCallRepo.MarkRunning(ctx, record.ID)
			if err != nil {
				return fail(err)
			}
			task.UpdateToolCallRunning(record.ID)

			toolResult, toolErr := s.toolProviderManager.CallTool(ctx, user, modelCall.Name, modelCall.Arguments)

			if toolErr != nil {
				toolErrorMessage := toolErr.Error()
				//工具执行失败
				payload, marshalErr := json.Marshal(map[string]any{
					"error": toolErrorMessage,
				})
				if marshalErr != nil {
					return fail(errors.Join(toolErr, marshalErr))
				}

				markErr := s.toolCallRepo.MarkFailed(ctx, record.ID, string(payload), toolErrorMessage)
				if markErr != nil {
					return fail(errors.Join(toolErr, markErr))
				}
				task.UpdateToolCallFailed(record.ID, string(payload), toolErrorMessage)

				viewed, viewErr := session.ViewResult(ctx, ToolResult{
					ToolCallID: modelCall.ID,
					Name:       modelCall.Name,
					Result:     payload,
				})
				if viewErr != nil {
					return fail(viewErr)
				}

				toolExchanges = append(toolExchanges, ToolExchange{
					Round:  round,
					Seq:    i + 1,
					Call:   modelCall,
					Result: viewed,
				})
				continue
			}

			toolResult.ToolCallID = modelCall.ID

			if err := s.toolCallRepo.MarkDone(ctx, record.ID, string(toolResult.Result)); err != nil {
				return fail(err)
			}
			task.UpdateToolCallDone(record.ID, string(toolResult.Result))

			viewed, viewErr := session.ViewResult(ctx, toolResult)
			if viewErr != nil {
				return fail(viewErr)
			}

			toolExchanges = append(toolExchanges, ToolExchange{
				Round:  round,
				Seq:    i + 1,
				Call:   modelCall,
				Result: viewed,
			})
		}
	}

	if !completed {
		return fail(errors.New("max tool rounds exceeded"))
	}

	snapshot := task.Snapshot()

	// 同样用 terminalCtx：取消可能刚好落在「模型已正常收尾」和这次写库之间，
	// 那时 ctx 已被 cancel，用 ctx 会让这条 done/failed 落库静默失败。
	if err := s.messageService.UpdateGeneratedMessage(terminalCtx, userID, UpdateGeneratedMessageInput{
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
			s.runtimeManager.Delete(conversationID, assistantMessageID)
		}
		return err
	}

	s.runtimeManager.Delete(conversationID, assistantMessageID)
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
			s.runtimeManager.Delete(item.ConversationID, item.MessageID)
			continue
		}

		task, ok := s.runtimeManager.Get(item.ConversationID, item.MessageID)
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
				s.runtimeManager.Delete(item.ConversationID, item.MessageID)
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
		s.runtimeManager.Delete(item.ConversationID, item.MessageID)
	}
}

func newPromptMemoryMessage(items []entity.MemoryItem, toolMode string) *GenerateMessage {
	if len(items) == 0 && toolMode != ToolModeAuto {
		return nil
	}

	var lines []string

	if len(items) > 0 {
		lines = append(lines, "以下是当前用户的长期偏好和用户画像记忆。请在回答时自然遵守，不要主动说明这些记忆的存在。")
		for _, item := range items {
			lines = append(lines, "- "+strings.TrimSpace(item.Content))
		}
	}

	if toolMode == ToolModeAuto {
		lines = append(lines, "如果需要更多历史记忆、偏好、事实、总结或知识，可以调用 search_memory。")
		lines = append(lines, "当用户明确要求记住信息，或当前对话产生了长期有用的信息时，可以调用 add_memory。")
		lines = append(lines, "当用户明确要求忘记、不要再记住、禁用某条记忆，或明确纠正已保存记忆时，先用 search_memory 找到对应私有记忆 ID，再调用 disable_memory；调用后不要再依据或复述该记忆。")
	}

	if len(lines) == 0 {
		return nil
	}

	return &GenerateMessage{
		Role:    entity.MessageRoleSystem,
		Content: strings.Join(lines, "\n"),
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
