package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/AlexMeiko/guchat/internal/config"
	"github.com/AlexMeiko/guchat/internal/db"
	"github.com/AlexMeiko/guchat/internal/generator"
	"github.com/AlexMeiko/guchat/internal/handler"
	"github.com/AlexMeiko/guchat/internal/repository"
	"github.com/AlexMeiko/guchat/internal/router"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/AlexMeiko/guchat/internal/stream"
	"github.com/AlexMeiko/guchat/internal/tool"
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

	runtimeManager := stream.NewManager()

	toolCallRepo := repository.NewToolCallRepository(mysqlDB)
	toolCallService := service.NewToolCallService(toolCallRepo)

	toolProviders := []service.ToolProvider{
		tool.NewBuiltinProvider(tool.BuiltinProviderConfig{
			TavilyAPIKey:  cfg.TavilyAPIKey,
			TavilyBaseURL: cfg.TavilyBaseURL,
		}),
	}

	for _, server := range cfg.MCPServers {
		toolProviders = append(toolProviders, tool.NewMCPProvider(tool.MCPProviderConfig{
			Name:      server.Name,
			URL:       server.URL,
			AuthType:  server.AuthType,
			AuthField: server.AuthField,
			AuthKey:   server.AuthKey,
			Transport: server.Transport,
			Command:   server.Command,
			Args:      server.Args,
			Env:       server.Env,
		}))
	}

	toolProviderManager := service.NewToolProviderManager(toolProviders...)

	messageHandler := handler.NewMessageHandler(messageService, toolCallService, runtimeManager)

	modelRepo := repository.NewModelRepository(mysqlDB)
	modelService := service.NewModelService(modelRepo)

	recovered, err := messageService.RecoverInterruptedGenerations(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("recovered %d interrupted generations", recovered)

	modelHandler := handler.NewModelHandler(modelService)

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     60 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0,
	}

	generatorFactory := generator.NewFactory(map[string]service.Generator{
		"openai":           generator.NewOpenAIChatCompletionsGenerator(client),
		"openai_responses": generator.NewOpenAIResponsesGenerator(client),
		"fake":             generator.NewFakeGenerator(),
	})

	generationService := service.NewGenerationService(
		messageService,
		modelService,
		generatorFactory,
		runtimeManager,
		toolProviderManager,
		toolCallRepo,
		cfg.GenerationContextLimit,
		cfg.GenerationMaxToolRounds,
		time.Duration(cfg.GenerationRetryInterval)*time.Second,
		int(cfg.GenerationRetryMax),
	)
	generationHandler := handler.NewGenerationHandler(generationService, messageService, toolCallService, runtimeManager)

	go generationService.RetryLoop(context.Background())

	r := router.New(authHandler, conversationHandler, messageHandler, modelHandler, generationHandler, jwtService)
	log.Printf("server starting on port %s", cfg.Port)

	if err := r.Run(":" + cfg.Port); err != nil {
		panic(err)
	}
}
