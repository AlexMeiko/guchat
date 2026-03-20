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
	runtimeManager    *stream.Manager
}

const (
	generationEventSnapshot       = "message.snapshot"
	generationEventDelta          = "message.delta"
	generationEventReasoningDelta = "message.reasoning_delta"
	generationEventCompleted      = "message.completed"
	generationEventFailed         = "message.failed"
)

func NewGenerationHandler(generationService *service.GenerationService, messageService *service.MessageService, runtimeManager *stream.Manager) *GenerationHandler {
	return &GenerationHandler{
		generationService: generationService,
		messageService:    messageService,
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

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	message, err := h.generationService.Create(c.Request.Context(), user.UserID, service.CreateGenerationInput{
		conversationID,
		messageID,
		req.ModelID,
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

	c.JSON(http.StatusCreated, newMessageResponse(message))
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
		_ = writeSSE(c, generationEventSnapshot, newGenerationSnapshotEvent(
			message.ID,
			message.Status,
			message.Content,
			message.ReasoningContent,
			message.ErrorMessage,
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
	)); err != nil {
		return
	}

	if snapshot.Status == entity.MessageStatusDone || snapshot.Status == entity.MessageStatusFailed {
		return
	}

	sentContentOffset := len(snapshot.Content)
	sentReasoningOffset := len(snapshot.ReasoningContent)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return

		case <-ticker.C:
			snapshot = task.Snapshot()

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

func newGenerationSnapshotEvent(messageID, status, content, reasoningContent, errMsg string) model.GenerationEvent {
	return model.GenerationEvent{
		MessageID:        messageID,
		Status:           status,
		Content:          content,
		ReasoningContent: reasoningContent,
		Error:            errMsg,
	}
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
