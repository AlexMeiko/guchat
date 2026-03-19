package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelHandler struct {
	modelService *service.ModelService
}

func NewModelHandler(modelService *service.ModelService) *ModelHandler {
	return &ModelHandler{
		modelService: modelService,
	}
}

func (h *ModelHandler) ListEnabled(c *gin.Context) {
	models, err := h.modelService.ListEnabled(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	response := make([]model.ModelSimpleResponse, len(models))
	for i := range models {
		response[i] = newModelSimpleResponse(&models[i])
	}

	c.JSON(http.StatusOK, response)
}

func (h *ModelHandler) Create(c *gin.Context) {
	var req model.CreateModelRequest
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
	if user.Role != "admin" {
		c.JSON(http.StatusForbidden, model.ErrorResponse{
			Error: "forbidden",
		})
		return
	}

	modelConfig, err := h.modelService.Create(c.Request.Context(), service.CreateModelInput{
		Name:      req.Name,
		Provider:  req.Provider,
		ModelKey:  req.ModelKey,
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		IsEnabled: req.IsEnabled,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusCreated, newModelDetailResponse(modelConfig))
}

func (h *ModelHandler) Get(c *gin.Context) {
	id, ok := parseModelID(c)
	if !ok {
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}
	if user.Role != "admin" {
		c.JSON(http.StatusForbidden, model.ErrorResponse{
			Error: "forbidden",
		})
		return
	}

	modelConfig, err := h.modelService.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrModelNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: "model not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, newModelDetailResponse(modelConfig))
}

func (h *ModelHandler) Update(c *gin.Context) {
	id, ok := parseModelID(c)
	if !ok {
		return
	}

	var req model.UpdateModelRequest
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
	if user.Role != "admin" {
		c.JSON(http.StatusForbidden, model.ErrorResponse{
			Error: "forbidden",
		})
		return
	}

	modelConfig, err := h.modelService.UpdateByID(c.Request.Context(), id, service.UpdateModelInput{
		Name:      req.Name,
		Provider:  req.Provider,
		ModelKey:  req.ModelKey,
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		IsEnabled: req.IsEnabled,
	})

	if err != nil {
		if errors.Is(err, service.ErrModelNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: "model not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, newModelDetailResponse(modelConfig))
}

func (h *ModelHandler) Delete(c *gin.Context) {
	id, ok := parseModelID(c)
	if !ok {
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}
	if user.Role != "admin" {
		c.JSON(http.StatusForbidden, model.ErrorResponse{
			Error: "forbidden",
		})
		return
	}

	err := h.modelService.DeleteByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrModelNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: "model not found",
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

func newModelDetailResponse(modelConfig *entity.ModelConfig) model.ModelDetailResponse {
	return model.ModelDetailResponse{
		ID:        modelConfig.ID,
		Name:      modelConfig.Name,
		Provider:  modelConfig.Provider,
		ModelKey:  modelConfig.ModelKey,
		BaseURL:   modelConfig.BaseURL,
		APIKey:    modelConfig.APIKey,
		IsEnabled: modelConfig.IsEnabled,
		CreatedAt: modelConfig.CreatedAt,
		UpdatedAt: modelConfig.UpdatedAt,
	}
}

func newModelSimpleResponse(modelConfig *entity.ModelConfig) model.ModelSimpleResponse {
	return model.ModelSimpleResponse{
		ID:   modelConfig.ID,
		Name: modelConfig.Name,
	}
}

func parseModelID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid model id",
		})
		return 0, false
	}

	return id, true
}
