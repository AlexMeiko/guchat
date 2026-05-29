package handler

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/memory"
	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
)

type MemoryHandler struct {
	memoryService *service.MemoryService
}

func NewMemoryHandler(memoryService *service.MemoryService) *MemoryHandler {
	return &MemoryHandler{
		memoryService: memoryService,
	}
}

const (
	noMin int64 = math.MinInt64
	noMax int64 = math.MaxInt64
)

func (h *MemoryHandler) List(c *gin.Context) {
	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	limit, ok := parseInt64Value(c.Query("limit"), 50, 1, 100)
	if !ok {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid limit"})
		return
	}

	offset, ok := parseInt64Value(c.Query("offset"), 0, 0, noMax)
	if !ok {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid offset"})
		return
	}

	items, err := h.memoryService.ListOwned(
		c.Request.Context(),
		user.UserID,
		splitCSV(c.Query("status")),
		splitCSV(c.Query("category")),
		splitCSV(c.Query("scope")),
		int(limit),
		int(offset),
	)
	if err != nil {
		if errors.Is(err, service.ErrInvalidMemoryInput) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid memory filter"})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	respItems := make([]model.MemoryItemResponse, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, toMemoryItemResponse(item))
	}

	c.JSON(http.StatusOK, model.ListMemoryItemsResponse{Items: respItems})
}

func (h *MemoryHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseInt64Value(c.Param("id"), 0, 1, noMax)
	if !ok {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid memory id"})
		return
	}

	var req model.UpdateMemoryStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid request body"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	var err error
	switch req.Status {
	case memory.MemoryStatusActive:
		err = h.memoryService.Enable(c.Request.Context(), user.UserID, id)
	case memory.MemoryStatusDisabled:
		err = h.memoryService.Disable(c.Request.Context(), user.UserID, id)
	case memory.MemoryStatusDeleted:
		err = h.memoryService.Delete(c.Request.Context(), user.UserID, id)
	default:
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid memory status"})
		return
	}

	if err != nil {
		if errors.Is(err, service.ErrMemoryItemNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "memory item not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *MemoryHandler) Delete(c *gin.Context) {
	id, ok := parseInt64Value(c.Param("id"), 0, 1, noMax)
	if !ok {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid memory id"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	err := h.memoryService.Delete(c.Request.Context(), user.UserID, id)
	if err != nil {
		if errors.Is(err, service.ErrMemoryItemNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "memory item not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	c.Status(http.StatusNoContent)
}

func toMemoryItemResponse(item entity.MemoryItem) model.MemoryItemResponse {
	metadata := json.RawMessage(item.MetadataJSON)
	if len(metadata) == 0 || !json.Valid(metadata) {
		metadata = json.RawMessage(`{}`)
	}

	resp := model.MemoryItemResponse{
		ID:         item.ID,
		Scope:      item.Scope,
		Category:   item.Category,
		SourceType: item.SourceType,
		Origin:     item.Origin,
		Content:    item.Content,
		Metadata:   metadata,
		Confidence: item.Confidence,
		Status:     item.Status,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}

	if item.ConversationID.Valid {
		resp.ConversationID = item.ConversationID.String
	}

	if item.SourceRef.Valid {
		resp.SourceRef = item.SourceRef.String
	}

	if item.SourceTitle.Valid {
		resp.SourceTitle = item.SourceTitle.String
	}

	if item.ExpiresAt.Valid {
		resp.ExpiresAt = &item.ExpiresAt.Time
	}

	return resp
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

func parseInt64Value(raw string, defaultValue, min, max int64) (int64, bool) {
	raw = strings.TrimSpace(raw)

	value := defaultValue
	if raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, false
		}
		value = parsed
	}

	if min != noMin && value < min {
		return 0, false
	}
	if max != noMax && value > max {
		return 0, false
	}

	return value, true
}
