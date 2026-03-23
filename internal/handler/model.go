package handler

import (
	"encoding/json"
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

	extraBody, err := normalizeExtraBody(req.ExtraBody)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid extra_body",
		})
		return
	}

	modelConfig, err := h.modelService.Create(c.Request.Context(), service.CreateModelInput{
		Name:      req.Name,
		Provider:  req.Provider,
		ModelKey:  req.ModelKey,
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		ExtraBody: extraBody,
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

	var extraBody *string
	if req.ExtraBody != nil {
		normalizedExtraBody, err := normalizeExtraBody(*req.ExtraBody)
		if err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error: "invalid extra_body",
			})
			return
		}
		extraBody = &normalizedExtraBody
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
		ExtraBody: extraBody,
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

	c.Status(http.StatusNoContent)
}

func newModelDetailResponse(modelConfig *entity.ModelConfig) model.ModelDetailResponse {
	return model.ModelDetailResponse{
		ID:        modelConfig.ID,
		Name:      modelConfig.Name,
		Provider:  modelConfig.Provider,
		ModelKey:  modelConfig.ModelKey,
		BaseURL:   modelConfig.BaseURL,
		APIKey:    modelConfig.APIKey,
		ExtraBody: parseExtraBody(modelConfig.ExtraBody),
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

func normalizeExtraBody(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}

	object, ok := value.(map[string]any)
	if !ok {
		return "", errors.New("extra_body must be a json object")
	}

	normalized, err := json.Marshal(object)
	if err != nil {
		return "", err
	}

	return string(normalized), nil
}

func parseExtraBody(raw string) json.RawMessage {
	normalized, err := normalizeExtraBody(json.RawMessage(raw))
	if err != nil || normalized == "" {
		return json.RawMessage(`{}`)
	}

	return json.RawMessage(normalized)
}
