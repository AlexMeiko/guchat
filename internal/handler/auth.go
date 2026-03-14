package handler

import (
	"net/http"

	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/gin-gonic/gin"
)

func Register(c *gin.Context) {
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

func Login(c *gin.Context) {
	var req model.LoginRequest
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

func Me(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, model.ErrorResponse{
		Error: "not implemented",
	})
}
