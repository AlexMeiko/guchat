package handler

import (
	"net/http"

	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	jwtService *service.JWTService
}

func NewAuthHandler(jwtService *service.JWTService) *AuthHandler {
	return &AuthHandler{
		jwtService: jwtService,
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

	c.JSON(http.StatusNotImplemented, model.ErrorResponse{
		Error: "not implemented",
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: "invalid request body",
		})
		return
	}

	user := model.UserResponse{
		ID:       1,
		Username: req.Username,
		Role:     "user",
	}

	accessToken, accessExpiresIn, err := h.jwtService.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "failed to generate access token",
		})
		return
	}

	refreshToken, refreshExpiresIn, err := h.jwtService.GenerateRefreshToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: "failed to generate refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, model.LoginResponse{
		AccessToken:      accessToken,
		TokenType:        "Bearer",
		ExpiresIn:        accessExpiresIn,
		RefreshToken:     refreshToken,
		RefreshExpiresIn: refreshExpiresIn,
		User:             user,
	})
}

func (h *AuthHandler) Me(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, model.ErrorResponse{
		Error: "not implemented",
	})
}
