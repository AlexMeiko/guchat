package handler

import (
	"net/http"

	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	resp := model.HealthResponse{
		Status: "ok",
	}

	c.JSON(http.StatusOK, resp)
}
