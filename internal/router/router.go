package router

import (
	"github.com/AlexMeiko/guchat/internal/handler"

	"github.com/gin-gonic/gin"
)

func New() *gin.Engine {
	r := gin.Default()

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/login", handler.Login)
			auth.POST("/register", handler.Register)
		}
		api.GET("/me", handler.Me)
	}

	return r
}
