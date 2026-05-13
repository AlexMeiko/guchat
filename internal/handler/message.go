package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/AlexMeiko/guchat/internal/stream"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MessageHandler struct {
	messageService  *service.MessageService
	toolCallService *service.ToolCallService
	runtimeManager  *stream.Manager
}

func NewMessageHandler(
	messageService *service.MessageService,
	toolCallService *service.ToolCallService,
	runtimeManager *stream.Manager,
) *MessageHandler {
	return &MessageHandler{
		messageService:  messageService,
		toolCallService: toolCallService,
		runtimeManager:  runtimeManager,
	}
}

func newMessageResponse(message *entity.Message, toolCalls []model.ToolCallEvent) model.MessageResponse {
	return model.MessageResponse{
		ID:               message.ID,
		ConversationID:   message.ConversationID,
		Role:             message.Role,
		Content:          message.Content,
		ReasoningContent: message.ReasoningContent,
		ToolCalls:        toolCalls,
		Status:           message.Status,
		ErrorMessage:     message.ErrorMessage,
		CreatedAt:        message.CreatedAt,
	}
}

func (h *MessageHandler) ListByConversationID(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	limit := 20
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid limit"})
			return
		}
		limit = parsed
	}

	var beforeSeq *int
	if raw := c.Query("before_seq"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid before_seq"})
			return
		}
		beforeSeq = &parsed
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	result, err := h.messageService.ListPageByConversationID(c.Request.Context(), user.UserID, service.ListMessagesPageInput{
		ConversationID: conversationID,
		BeforeSeq:      beforeSeq,
		Limit:          limit,
	})
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	assistantMessageIDs := make([]string, 0)
	for i := range result.Messages {
		if result.Messages[i].Role == entity.MessageRoleAssistant {
			assistantMessageIDs = append(assistantMessageIDs, result.Messages[i].ID)
		}
	}

	toolCalls, err := h.toolCallService.ListByAssistantMessageIDs(c.Request.Context(), assistantMessageIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}
	toolCallsByMessageID := groupToolCallEventsByMessageID(toolCalls)

	items := make([]model.MessageResponse, len(result.Messages))
	for i := range result.Messages {
		message := &result.Messages[i]
		items[i] = newMessageResponse(message, toolCallsByMessageID[message.ID])
	}

	c.JSON(http.StatusOK, model.ListMessagesResponse{
		Items:         items,
		HasMore:       result.HasMore,
		NextBeforeSeq: result.NextBeforeSeq,
	})
}

func (h *MessageHandler) Create(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	var req model.CreateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.PrevID != "" {
		if _, err := uuid.Parse(req.PrevID); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid prev id"})
			return
		}
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	message, err := h.messageService.CreateMessage(c.Request.Context(), user.UserID, service.CreateMessageInput{
		ConversationID:   conversationID,
		Role:             "user",
		Content:          req.Content,
		ReasoningContent: "",
		Status:           "done",
		ErrorMessage:     "",
		PrevID:           req.PrevID,
	})

	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
			return
		}

		if errors.Is(err, service.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "prev message not found"})
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

func (h *MessageHandler) GetByIDAndConversationID(c *gin.Context) {
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

	message, err := h.messageService.GetByIDAndConversationID(c.Request.Context(), user.UserID, conversationID, messageID)
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

	var toolCalls []model.ToolCallEvent
	if message.Role == entity.MessageRoleAssistant {
		records, err := h.toolCallService.ListByAssistantMessageID(c.Request.Context(), message.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
			return
		}

		toolCalls = groupToolCallEventsByMessageID(records)[message.ID]
	}

	c.JSON(http.StatusOK, newMessageResponse(message, toolCalls))
}

func (h *MessageHandler) UpdateContentByIDAndConversationID(c *gin.Context) {
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

	var req model.UpdateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid request body"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	message, err := h.messageService.UpdateContentByIDAndConversationID(c.Request.Context(), user.UserID, conversationID, messageID, req.Content)
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

	c.JSON(http.StatusOK, newMessageResponse(message, nil))
}

func (h *MessageHandler) DeleteByIDAndConversationID(c *gin.Context) {
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

	err := h.messageService.DeleteByIDAndConversationID(c.Request.Context(), user.UserID, conversationID, messageID)
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

	if task, ok := h.runtimeManager.Get(messageID); ok {
		task.Cancel("message deleted by user")
	}
	c.Status(http.StatusNoContent)
}

// 将工具调用记录按 assistant_message_id 分组
func groupToolCallEventsByMessageID(toolCalls []entity.ToolCall) map[string][]model.ToolCallEvent {
	result := make(map[string][]model.ToolCallEvent)

	for _, call := range toolCalls {
		item := model.ToolCallEvent{
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
		result[call.AssistantMessageID] = append(result[call.AssistantMessageID], item)
	}

	return result
}
