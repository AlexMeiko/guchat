package router

import (
	"github.com/AlexMeiko/guchat/internal/handler"
	"github.com/AlexMeiko/guchat/internal/middleware"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
)

func New(authHandler *handler.AuthHandler, jwtService *service.JWTService) *gin.Engine {
	r := gin.Default()

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/register", authHandler.Register)
		}
		api.GET("/me", middleware.Auth(jwtService), authHandler.Me)
	}

	return r
}
