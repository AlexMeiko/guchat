package handler

import (
	"errors"
	"net/http"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/AlexMeiko/guchat/internal/stream"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MessageHandler struct {
	messageService *service.MessageService
	runtimeManager *stream.Manager
}

func NewMessageHandler(messageService *service.MessageService, runtimeManager *stream.Manager) *MessageHandler {
	return &MessageHandler{
		messageService: messageService,
		runtimeManager: runtimeManager,
	}
}

func newMessageResponse(message *entity.Message) model.MessageResponse {
	return model.MessageResponse{
		ID:               message.ID,
		ConversationID:   message.ConversationID,
		Role:             message.Role,
		Content:          message.Content,
		ReasoningContent: message.ReasoningContent,
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

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	messages, err := h.messageService.ListByConversationID(c.Request.Context(), user.UserID, conversationID)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	response := make([]model.MessageResponse, len(messages))
	for i := range messages {
		response[i] = newMessageResponse(&messages[i])
	}

	c.JSON(http.StatusOK, response)
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

	c.JSON(http.StatusCreated, newMessageResponse(message))
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

	c.JSON(http.StatusOK, newMessageResponse(message))
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

	c.JSON(http.StatusOK, newMessageResponse(message))
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
