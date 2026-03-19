package handler

import (
	"errors"
	"net/http"

	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GenerationHandler struct {
	generationService *service.GenerationService
}

func NewGenerationHandler(generationService *service.GenerationService) *GenerationHandler {
	return &GenerationHandler{
		generationService: generationService,
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
