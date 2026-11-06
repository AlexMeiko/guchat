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
	generationService   *service.GenerationService
	conversationService *service.ConversationService
	runtimeManager      *stream.Manager
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
	conversationService *service.ConversationService,
	runtimeManager *stream.Manager,
) *GenerationHandler {
	return &GenerationHandler{
		generationService:   generationService,
		conversationService: conversationService,
		runtimeManager:      runtimeManager,
	}
}

func (h *GenerationHandler) Cancel(c *gin.Context) {
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

	if _, err := h.conversationService.Get(
		c.Request.Context(),
		user.UserID,
		conversationID,
	); err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	// 幂等：生成可能刚好在这个请求之前结束、内存任务已被回收，
	// 此时没有可取消的任务，同样按成功处理，由前端重新拉取消息状态即可。
	curMsgID, matched := h.runtimeManager.CancelIfCurrent(
		conversationID,
		messageID,
		"generation cancelled by user",
	)
	if curMsgID != "" && !matched {
		c.JSON(http.StatusConflict, model.GenerationConflictResponse{
			Error:     "generation changed",
			MessageID: curMsgID,
		})
		return
	}

	c.Status(http.StatusNoContent)
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

	err := h.generationService.Create(c.Request.Context(), user.UserID, service.CreateGenerationInput{
		ConversationID:  conversationID,
		SourceMessageID: messageID,
		ModelID:         req.ModelID,
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

		if errors.Is(err, service.ErrGenerationInProgress) {
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: "generation already in progress"})
			return
		}

		if errors.Is(err, service.ErrMessageSeqGapExhausted) {
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: "message position conflict"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	c.Status(http.StatusAccepted)
}

func (h *GenerationHandler) ConversationEvents(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	if _, err := h.conversationService.Get(c.Request.Context(), user.UserID, conversationID); err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
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

	current, events, unsubscribe := h.runtimeManager.Subscribe(conversationID)
	defer unsubscribe()

	var task *stream.Task
	var taskTicker *time.Ticker
	var taskTick <-chan time.Time
	var sentContentOffset int
	var sentReasoningOffset int
	var sentToolCalls map[int64]stream.ToolCallSnapshot

	stopTask := func() {
		if taskTicker != nil {
			taskTicker.Stop()
			taskTicker = nil
			taskTick = nil
		}
		task = nil
	}

	startTask := func(next *stream.Task) error {
		if next == nil {
			return nil
		}

		stopTask()
		task = next
		snapshot := task.Snapshot()
		sentContentOffset = len(snapshot.Content)
		sentReasoningOffset = len(snapshot.ReasoningContent)
		sentToolCalls = indexToolCalls(snapshot.ToolCalls)

		if err := writeSSE(c, generationEventSnapshot, newGenerationSnapshotEvent(
			snapshot.MessageID,
			snapshot.Status,
			snapshot.Content,
			snapshot.ReasoningContent,
			snapshot.ErrorMessage,
			snapshot.ToolCalls,
		)); err != nil {
			return err
		}

		if snapshot.Status == entity.MessageStatusDone || snapshot.Status == entity.MessageStatusFailed {
			stopTask()
			return nil
		}

		taskTicker = time.NewTicker(100 * time.Millisecond)
		taskTick = taskTicker.C
		return nil
	}

	if err := startTask(current); err != nil {
		return
	}
	c.Writer.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return

		case event, ok := <-events:
			if !ok {
				return
			}

			switch event.Type {
			case stream.EventTypeMessageCreated:
				if event.Message != nil {
					if err := writeSSE(c, string(event.Type), newMessageResponse(event.Message, nil)); err != nil {
						return
					}
				}
				if err := startTask(event.Task); err != nil {
					return
				}

			case stream.EventTypeMessageUpdated:
				if event.Message == nil {
					continue
				}
				if err := writeSSE(c, string(event.Type), newMessageResponse(event.Message, nil)); err != nil {
					return
				}

			case stream.EventTypeMessageDeleted:
				if err := writeSSE(c, string(event.Type), map[string]string{
					"message_id": event.MessageID,
				}); err != nil {
					return
				}
				if task != nil && task.Snapshot().MessageID == event.MessageID {
					stopTask()
				}

			case stream.EventTypeGenerationCompleted:
				if task != event.Task {
					continue
				}
				finished, err := writeTaskProgress(
					c,
					task,
					&sentContentOffset,
					&sentReasoningOffset,
					sentToolCalls,
				)
				if err != nil {
					return
				}
				if finished {
					stopTask()
				}
			}

		case <-taskTick:
			if task == nil {
				continue
			}

			finished, err := writeTaskProgress(
				c,
				task,
				&sentContentOffset,
				&sentReasoningOffset,
				sentToolCalls,
			)
			if err != nil {
				return
			}
			if finished {
				stopTask()
			}

		case <-heartbeat.C:
			if err := writeSSEComment(c); err != nil {
				return
			}
		}
	}
}

func writeTaskProgress(
	c *gin.Context,
	task *stream.Task,
	sentContentOffset, sentReasoningOffset *int,
	sentToolCalls map[int64]stream.ToolCallSnapshot,
) (bool, error) {
	snapshot := task.Snapshot()

	if len(snapshot.Content) > *sentContentOffset {
		if err := writeSSE(c, generationEventDelta, newGenerationDeltaEvent(
			snapshot.MessageID,
			snapshot.Content[*sentContentOffset:],
		)); err != nil {
			return false, err
		}
		*sentContentOffset = len(snapshot.Content)
	}

	if len(snapshot.ReasoningContent) > *sentReasoningOffset {
		if err := writeSSE(c, generationEventReasoningDelta, newGenerationDeltaEvent(
			snapshot.MessageID,
			snapshot.ReasoningContent[*sentReasoningOffset:],
		)); err != nil {
			return false, err
		}
		*sentReasoningOffset = len(snapshot.ReasoningContent)
	}

	for _, call := range snapshot.ToolCalls {
		previous, exists := sentToolCalls[call.ID]
		if !exists {
			if err := writeSSE(c, generationEventToolCallCreated, newToolCallEvent(snapshot.MessageID, call)); err != nil {
				return false, err
			}
			sentToolCalls[call.ID] = call
			continue
		}

		if previous.Status != call.Status ||
			previous.Result != call.Result ||
			previous.ErrorMessage != call.ErrorMessage {
			if err := writeSSE(c, generationEventToolCallUpdated, newToolCallEvent(snapshot.MessageID, call)); err != nil {
				return false, err
			}
			sentToolCalls[call.ID] = call
		}
	}

	switch snapshot.Status {
	case entity.MessageStatusDone:
		return true, writeSSE(c, generationEventCompleted, newGenerationCompletedEvent(
			snapshot.MessageID,
			len(snapshot.Content),
			len(snapshot.ReasoningContent),
		))
	case entity.MessageStatusFailed:
		return true, writeSSE(c, generationEventFailed, newGenerationFailedEvent(
			snapshot.MessageID,
			snapshot.ErrorMessage,
			len(snapshot.Content),
			len(snapshot.ReasoningContent),
		))
	default:
		return false, nil
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
		ToolCalls:        newToolCallEvents(messageID, toolCalls),
		Error:            errMsg,
	}
}

func newToolCallEvents(messageID string, toolCalls []stream.ToolCallSnapshot) []model.ToolCallEvent {
	if len(toolCalls) == 0 {
		return nil
	}

	result := make([]model.ToolCallEvent, len(toolCalls))
	for i, call := range toolCalls {
		result[i] = newToolCallEvent(messageID, call)
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

func writeSSEComment(c *gin.Context) error {
	if _, err := c.Writer.Write([]byte(": ping\n\n")); err != nil {
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

func newToolCallEvent(messageID string, call stream.ToolCallSnapshot) model.ToolCallEvent {
	return model.ToolCallEvent{
		ID:           call.ID,
		MessageID:    messageID,
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
