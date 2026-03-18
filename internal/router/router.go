package router

import (
	"github.com/AlexMeiko/guchat/internal/handler"
	"github.com/AlexMeiko/guchat/internal/middleware"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
)

func New(
	authHandler *handler.AuthHandler,
	conversationHandler *handler.ConversationHandler,
	jwtService *service.JWTService,
) *gin.Engine {
	r := gin.Default()

	r.GET("/health", handler.Health)

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}
		api.GET("/me", middleware.Auth(jwtService), authHandler.Me)

		conversation := api.Group("/conversations", middleware.Auth(jwtService))
		{
			conversation.GET("", conversationHandler.List)
			conversation.POST("", conversationHandler.Create)
			conversation.GET("/:id", conversationHandler.Get)
			conversation.PATCH("/:id", conversationHandler.Update)
			conversation.DELETE("/:id", conversationHandler.Delete)
		}
	}

	return r
}
