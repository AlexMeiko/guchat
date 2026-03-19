package main

import (
	"log"

	"github.com/AlexMeiko/guchat/internal/config"
	"github.com/AlexMeiko/guchat/internal/db"
	"github.com/AlexMeiko/guchat/internal/generator"
	"github.com/AlexMeiko/guchat/internal/handler"
	"github.com/AlexMeiko/guchat/internal/repository"
	"github.com/AlexMeiko/guchat/internal/router"
	"github.com/AlexMeiko/guchat/internal/service"
)

func main() {
	cfg, err := config.Load()

	if err != nil {
		log.Fatal(err)
	}

	mysqlDB, err := db.NewMySQL(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	defer mysqlDB.Close()

	jwtService := service.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessTTL,
		cfg.JWTRefreshTTL,
	)

	userRepo := repository.NewUserRepository(mysqlDB)
	refreshTokenRepo := repository.NewRefreshTokenRepository(mysqlDB)
	authService := service.NewAuthService(
		userRepo,
		refreshTokenRepo,
		jwtService,
	)
	authHandler := handler.NewAuthHandler(authService)

	conversationRepo := repository.NewConversationRepository(mysqlDB)
	conversationService := service.NewConversationService(conversationRepo)
	conversationHandler := handler.NewConversationHandler(conversationService)

	messageRepo := repository.NewMessageRepository(mysqlDB)
	messageService := service.NewMessageService(conversationRepo, messageRepo)
	messageHandler := handler.NewMessageHandler(messageService)

	modelRepo := repository.NewModelRepository(mysqlDB)
	modelService := service.NewModelService(modelRepo)
	modelHandler := handler.NewModelHandler(modelService)

	fakeGenerator := generator.NewFakeGenerator()
	generatorFactory := generator.NewFactory(map[string]service.Generator{
		"openai": fakeGenerator,
	})

	generationService := service.NewGenerationService(messageService, modelService, generatorFactory)
	generationHandler := handler.NewGenerationHandler(generationService)

	r := router.New(authHandler, conversationHandler, messageHandler, modelHandler, generationHandler, jwtService)
	log.Printf("server starting on port %s", cfg.Port)

	if err := r.Run(":" + cfg.Port); err != nil {
		panic(err)
	}
}
