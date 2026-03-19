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
	messageHandler *handler.MessageHandler,
	modelHandler *handler.ModelHandler,
	generationHandler *handler.GenerationHandler,
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

			conversation.GET("/:conversation_id", conversationHandler.Get)
			conversation.PATCH("/:conversation_id", conversationHandler.Update)
			conversation.DELETE("/:conversation_id", conversationHandler.Delete)

			conversation.GET("/:conversation_id/messages", messageHandler.ListByConversationID)
			conversation.POST("/:conversation_id/messages", messageHandler.Create)
			conversation.GET("/:conversation_id/messages/:message_id", messageHandler.GetByIDAndConversationID)
			conversation.PATCH("/:conversation_id/messages/:message_id", messageHandler.UpdateContentByIDAndConversationID)
			conversation.DELETE("/:conversation_id/messages/:message_id", messageHandler.DeleteByIDAndConversationID)

			conversation.POST("/:conversation_id/messages/:message_id/generation", generationHandler.Create)

		}

		models := api.Group("/models", middleware.Auth(jwtService))
		{
			models.GET("", modelHandler.ListEnabled)
			models.POST("", modelHandler.Create)
			models.GET("/:id", modelHandler.Get)
			models.PATCH("/:id", modelHandler.Update)
			models.DELETE("/:id", modelHandler.Delete)
		}
	}

	return r
}
