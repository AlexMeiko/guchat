package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/AlexMeiko/guchat/internal/stream"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GenerationHandler struct {
	generationService *service.GenerationService
	messageService    *service.MessageService
	toolCallService   *service.ToolCallService
	runtimeManager    *stream.Manager
}

const (
	generationEventSnapshot        = "message.snapshot"
	generationEventDelta           = "message.delta"
	generationEventReasoningDelta  = "message.reasoning_delta"
	generationEventCompleted       = "message.completed"
	generationEventFailed          = "message.failed"
	generationEventToolCallCreated = "tool_call.created"
	generationEventToolCallUpdated = "tool_call.updated"
)

func NewGenerationHandler(
	generationService *service.GenerationService,
	messageService *service.MessageService,
	toolCallService *service.ToolCallService,
	runtimeManager *stream.Manager,
) *GenerationHandler {
	return &GenerationHandler{
		generationService: generationService,
		messageService:    messageService,
		toolCallService:   toolCallService,
		runtimeManager:    runtimeManager,
	}
}

func (h *GenerationHandler) Create(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	messageID := c.Param("message_id")
	if _, err := uuid.Parse(messageID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid message id"})
		return
	}

	var req model.CreateGenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid request body"})
		return
	}

	contextLimit := 0
	if req.ContextLimit != nil {
		if *req.ContextLimit <= 0 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid context limit"})
			return
		}
		contextLimit = *req.ContextLimit
	}

	toolMode := req.ToolMode
	if toolMode == "" {
		toolMode = service.ToolModeAuto
	}

	switch toolMode {
	case service.ToolModeNone, service.ToolModeAuto:
	default:
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid tool mode"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	message, err := h.generationService.Create(c.Request.Context(), user.UserID, service.CreateGenerationInput{
		ConversationID:  conversationID,
		SourceMessageID: messageID,
		ModelID:         req.ModelID,
		ContextLimit:    contextLimit,
		ToolMode:        toolMode,
	})
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
			return
		}

		if errors.Is(err, service.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "message not found"})
			return
		}

		if errors.Is(err, service.ErrModelNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "model not found"})
			return
		}

		if errors.Is(err, service.ErrModelDisabled) {
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: "model is disabled"})
			return
		}

		if errors.Is(err, service.ErrMessageSeqGapExhausted) {
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: "message position conflict"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, newMessageResponse(message, nil))
}

func (h *GenerationHandler) Events(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	messageID := c.Param("message_id")
	if _, err := uuid.Parse(messageID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid message id"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	message, err := h.messageService.GetByIDAndConversationID(
		c.Request.Context(),
		user.UserID,
		conversationID,
		messageID,
	)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
			return
		}
		if errors.Is(err, service.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "message not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	if _, ok := c.Writer.(http.Flusher); !ok {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "streaming unsupported"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	task, exists := h.runtimeManager.Get(message.ID)
	if !exists {
		toolCalls, err := h.toolCallService.ListByAssistantMessageID(c.Request.Context(), message.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
			return
		}

		_ = writeSSE(c, generationEventSnapshot, newGenerationSnapshotEvent(
			message.ID,
			message.Status,
			message.Content,
			message.ReasoningContent,
			message.ErrorMessage,
			newToolCallSnapshots(toolCalls),
		))
		return
	}

	snapshot := task.Snapshot()
	if err := writeSSE(c, generationEventSnapshot, newGenerationSnapshotEvent(
		snapshot.MessageID,
		snapshot.Status,
		snapshot.Content,
		snapshot.ReasoningContent,
		snapshot.ErrorMessage,
		snapshot.ToolCalls,
	)); err != nil {
		return
	}

	if snapshot.Status == entity.MessageStatusDone || snapshot.Status == entity.MessageStatusFailed {
		return
	}

	sentContentOffset := len(snapshot.Content)
	sentReasoningOffset := len(snapshot.ReasoningContent)
	sentToolCalls := indexToolCalls(snapshot.ToolCalls)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return

		case <-ticker.C:
			snapshot = task.Snapshot()

			// 检查文本是否有增量
			if len(snapshot.Content) > sentContentOffset {
				delta := snapshot.Content[sentContentOffset:]
				if err := writeSSE(c, generationEventDelta, newGenerationDeltaEvent(snapshot.MessageID, delta)); err != nil {
					return
				}
				sentContentOffset = len(snapshot.Content)
			}

			if len(snapshot.ReasoningContent) > sentReasoningOffset {
				delta := snapshot.ReasoningContent[sentReasoningOffset:]
				if err := writeSSE(c, generationEventReasoningDelta, newGenerationDeltaEvent(snapshot.MessageID, delta)); err != nil {
					return
				}
				sentReasoningOffset = len(snapshot.ReasoningContent)
			}

			//检查tool call状态是否有更新
			for _, call := range snapshot.ToolCalls {
				previous, exists := sentToolCalls[call.ID]
				if !exists {
					if err := writeSSE(c, generationEventToolCallCreated, newToolCallEvent(call)); err != nil {
						return
					}
					sentToolCalls[call.ID] = call
					continue
				}

				if previous.Status != call.Status ||
					previous.Result != call.Result ||
					previous.ErrorMessage != call.ErrorMessage {
					if err := writeSSE(c, generationEventToolCallUpdated, newToolCallEvent(call)); err != nil {
						return
					}
					sentToolCalls[call.ID] = call
				}
			}

			switch snapshot.Status {
			case entity.MessageStatusDone:
				_ = writeSSE(c, generationEventCompleted, newGenerationCompletedEvent(
					snapshot.MessageID,
					len(snapshot.Content),
					len(snapshot.ReasoningContent),
				))
				return

			case entity.MessageStatusFailed:
				_ = writeSSE(c, generationEventFailed, newGenerationFailedEvent(
					snapshot.MessageID,
					snapshot.ErrorMessage,
					len(snapshot.Content),
					len(snapshot.ReasoningContent),
				))
				return
			}
		}
	}

}

func newGenerationSnapshotEvent(
	messageID, status, content, reasoningContent, errMsg string,
	toolCalls []stream.ToolCallSnapshot,
) model.GenerationEvent {
	return model.GenerationEvent{
		MessageID:        messageID,
		Status:           status,
		Content:          content,
		ReasoningContent: reasoningContent,
		ToolCalls:        newToolCallEvents(toolCalls),
		Error:            errMsg,
	}
}

func newToolCallSnapshots(toolCalls []entity.ToolCall) []stream.ToolCallSnapshot {
	if len(toolCalls) == 0 {
		return nil
	}

	result := make([]stream.ToolCallSnapshot, len(toolCalls))
	for i, call := range toolCalls {
		result[i] = stream.ToolCallSnapshot{
			ID:           call.ID,
			ProviderID:   call.ProviderCallID,
			Name:         call.ToolName,
			Arguments:    call.ArgumentsJSON,
			Result:       call.ResultJSON,
			Status:       call.Status,
			ErrorMessage: call.ErrorMessage,
			Round:        call.Round,
			Seq:          call.Seq,
		}
	}

	return result
}

func newToolCallEvents(toolCalls []stream.ToolCallSnapshot) []model.ToolCallEvent {
	if len(toolCalls) == 0 {
		return nil
	}

	result := make([]model.ToolCallEvent, len(toolCalls))
	for i, call := range toolCalls {
		result[i] = newToolCallEvent(call)
	}

	return result
}

func newGenerationDeltaEvent(messageID, delta string) model.GenerationEvent {
	return model.GenerationEvent{
		MessageID: messageID,
		Delta:     delta,
	}
}

func newGenerationCompletedEvent(messageID string, contentBytes, reasoningBytes int) model.GenerationEvent {
	return model.GenerationEvent{
		MessageID:      messageID,
		Status:         entity.MessageStatusDone,
		ContentBytes:   contentBytes,
		ReasoningBytes: reasoningBytes,
	}
}

func newGenerationFailedEvent(messageID, errMsg string, contentBytes, reasoningBytes int) model.GenerationEvent {
	return model.GenerationEvent{
		MessageID:      messageID,
		Status:         entity.MessageStatusFailed,
		Error:          errMsg,
		ContentBytes:   contentBytes,
		ReasoningBytes: reasoningBytes,
	}
}

func writeSSE(c *gin.Context, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if _, err := c.Writer.Write([]byte("event: " + event + "\n")); err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("data: " + string(payload) + "\n\n")); err != nil {
		return err
	}

	c.Writer.Flush()
	return nil
}

// 把toolCalls切片转换为以toolCall ID为key的map
func indexToolCalls(toolCalls []stream.ToolCallSnapshot) map[int64]stream.ToolCallSnapshot {
	result := make(map[int64]stream.ToolCallSnapshot, len(toolCalls))
	for _, call := range toolCalls {
		result[call.ID] = call
	}
	return result
}

func newToolCallEvent(call stream.ToolCallSnapshot) model.ToolCallEvent {
	return model.ToolCallEvent{
		ID:           call.ID,
		ProviderID:   call.ProviderID,
		Name:         call.Name,
		Arguments:    call.Arguments,
		Result:       call.Result,
		Status:       call.Status,
		ErrorMessage: call.ErrorMessage,
		Round:        call.Round,
		Seq:          call.Seq,
	}
}
