package handler

import (
	"errors"
	"net/http"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/middleware"
	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ConversationHandler struct {
	conversationService *service.ConversationService
}

func NewConversationHandler(conversationService *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{conversationService: conversationService}
}

func newConversationResponse(conversation *entity.Conversation) model.ConversationResponse {
	return model.ConversationResponse{
		ID:        conversation.ID,
		Title:     conversation.Title,
		CreatedAt: conversation.CreatedAt,
		UpdatedAt: conversation.UpdatedAt,
	}
}

func requireCurrentUser(c *gin.Context) (service.AccessIdentity, bool) {
	value, exists := c.Get(middleware.CurrentUserKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Error: "current user not found",
		})
		return service.AccessIdentity{}, false
	}

	user, ok := value.(service.AccessIdentity)
	if !ok {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "invalid current user context",
		})
		return service.AccessIdentity{}, false
	}

	return user, true
}

func (h *ConversationHandler) Create(c *gin.Context) {
	var req model.CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	conversation, err := h.conversationService.Create(c.Request.Context(), user.UserID, req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusCreated, newConversationResponse(conversation))
}

func (h *ConversationHandler) List(c *gin.Context) {
	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	conversations, err := h.conversationService.List(c.Request.Context(), user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	response := make([]model.ConversationResponse, len(conversations))
	for i := range conversations {
		response[i] = newConversationResponse(&conversations[i])
	}

	c.JSON(http.StatusOK, response)
}

func (h *ConversationHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid conversation id",
		})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	conversation, err := h.conversationService.Get(c.Request.Context(), user.UserID, id)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: "conversation not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, newConversationResponse(conversation))
}

func (h *ConversationHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid conversation id",
		})
		return
	}

	var req model.UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	err := h.conversationService.UpdateTitle(c.Request.Context(), user.UserID, id, req.Title)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: "conversation not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, model.OKResponse{OK: true})
}

func (h *ConversationHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid conversation id",
		})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	err := h.conversationService.Delete(c.Request.Context(), user.UserID, id)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: "conversation not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, model.OKResponse{OK: true})
}
