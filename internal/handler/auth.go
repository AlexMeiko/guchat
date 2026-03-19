package handler

import (
	"errors"
	"net/http"

	"github.com/AlexMeiko/guchat/internal/middleware"
	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	err := h.authService.Register(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUsernameAlreadyExists) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error: "username already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusCreated, model.OKResponse{OK: true})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	result, err := h.authService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error: "invalid username or password",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, model.LoginResponse{
		AccessToken:      result.AccessToken.Token,
		TokenType:        "Bearer",
		ExpiresIn:        result.AccessToken.ExpiresIn,
		RefreshToken:     result.RefreshToken.Token,
		RefreshExpiresIn: result.RefreshToken.ExpiresIn,
		User: model.UserResponse{
			ID:       result.User.UserID,
			Username: result.User.Username,
			Role:     result.User.Role,
		},
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	result, err := h.authService.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRefreshToken) {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error: "invalid refresh token",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, model.RefreshResponse{
		AccessToken: result.AccessToken.Token,
		TokenType:   "Bearer",
		ExpiresIn:   result.AccessToken.ExpiresIn,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req model.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	err := h.authService.Logout(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRefreshToken) {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error: "invalid refresh token",
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

func (h *AuthHandler) Me(c *gin.Context) {
	value, exists := c.Get(middleware.CurrentUserKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Error: "current user not found",
		})
		return
	}

	accessIdentity, ok := value.(service.AccessIdentity)
	if !ok {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "invalid current user context",
		})
		return
	}

	c.JSON(http.StatusOK, model.UserResponse{
		ID:       accessIdentity.UserID,
		Username: accessIdentity.Username,
		Role:     accessIdentity.Role,
	})
}
