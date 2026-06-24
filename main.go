package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/AlexMeiko/guchat/internal/config"
	"github.com/AlexMeiko/guchat/internal/db"
	"github.com/AlexMeiko/guchat/internal/generator"
	"github.com/AlexMeiko/guchat/internal/handler"
	"github.com/AlexMeiko/guchat/internal/memory"
	"github.com/AlexMeiko/guchat/internal/memory/embed"
	"github.com/AlexMeiko/guchat/internal/memory/segment"
	"github.com/AlexMeiko/guchat/internal/memory/vector"
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

	memoryStore := memory.NewMySQLStore(mysqlDB)
	memoryRetriever := memory.NewMySQLRetriever(memoryStore)

	memoryIndexer, err := buildMemoryIndexer(context.Background(), client, cfg)
	if err != nil {
		log.Fatal(err)
	}

	memoryService := service.NewMemoryService(memoryStore, memoryRetriever, conversationRepo, memoryIndexer)
	memoryHandler := handler.NewMemoryHandler(memoryService)

	messageRepo := repository.NewMessageRepository(mysqlDB)
	messageService := service.NewMessageService(conversationRepo, messageRepo)

	runtimeManager := stream.NewManager()

	toolCallRepo := repository.NewToolCallRepository(mysqlDB)
	toolCallService := service.NewToolCallService(toolCallRepo)
	generationRoundRepo := repository.NewGenerationRoundRepository(mysqlDB)

	toolProviders := []service.ToolProvider{
		tool.NewBuiltinProvider(tool.BuiltinProviderConfig{
			TavilyAPIKey:  cfg.TavilyAPIKey,
			TavilyBaseURL: cfg.TavilyBaseURL,
			MemoryService: memoryService,
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

	generatorFactory := generator.NewFactory(map[string]service.Generator{
		"openai":           generator.NewOpenAIChatCompletionsGenerator(client),
		"openai_responses": generator.NewOpenAIResponsesGenerator(client),
		"fake":             generator.NewFakeGenerator(),
	})

	generationService := service.NewGenerationService(
		messageService,
		modelService,
		memoryService,
		generatorFactory,
		runtimeManager,
		toolProviderManager,
		toolCallRepo,
		generationRoundRepo,
		cfg.GenerationContextLimit,
		cfg.GenerationMaxToolRounds,
		time.Duration(cfg.GenerationRetryInterval)*time.Second,
		int(cfg.GenerationRetryMax),
	)
	generationHandler := handler.NewGenerationHandler(generationService, messageService, toolCallService, runtimeManager)

	go generationService.RetryLoop(context.Background())

	r := router.New(authHandler, conversationHandler, messageHandler, memoryHandler, modelHandler, generationHandler, jwtService)
	log.Printf("server starting on port %s", cfg.Port)

	if err := r.Run(":" + cfg.Port); err != nil {
		panic(err)
	}
}

func buildMemoryIndexer(ctx context.Context, client *http.Client, cfg config.Config) (memory.Indexer, error) {
	if !cfg.MemoryRAGEnabled() {
		return nil, nil
	}

	splitter, err := buildMemorySplitter(client, cfg)
	if err != nil {
		return nil, err
	}

	embedder, err := buildMemoryEmbedder(client, cfg)
	if err != nil {
		return nil, err
	}

	index, err := buildMemoryVectorIndex(ctx, client, cfg)
	if err != nil {
		return nil, err
	}

	return memory.NewVectorIndexer(splitter, embedder, index)
}

func buildMemorySplitter(client *http.Client, cfg config.Config) (segment.Splitter, error) {
	switch cfg.RAGSplitterProvider {
	case "external_api":
		var headers map[string]string
		if cfg.RAGSplitterAPIHeaders != "" {
			if err := json.Unmarshal([]byte(cfg.RAGSplitterAPIHeaders), &headers); err != nil {
				return nil, err
			}
		}

		return &segment.ExternalAPISplitter{
			URL:          cfg.RAGSplitterAPIURL,
			Headers:      headers,
			SegmentsPath: cfg.RAGSplitterSegmentsPath,
			Client:       client,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported rag splitter provider: %s", cfg.RAGSplitterProvider)
	}
}

func buildMemoryEmbedder(client *http.Client, cfg config.Config) (embed.Embedder, error) {
	switch cfg.EmbeddingProvider {
	case "openai":
		return embed.NewOpenAIEmbedder(client, embed.OpenAIEmbedderConfig{
			BaseURL: cfg.EmbeddingBaseURL,
			APIKey:  cfg.EmbeddingAPIKey,
			Model:   cfg.EmbeddingModel,
		})

	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.EmbeddingProvider)
	}
}

func buildMemoryVectorIndex(ctx context.Context, client *http.Client, cfg config.Config) (vector.Index, error) {
	index, err := vector.NewQdrantIndex(client, vector.QdrantConfig{
		BaseURL:    cfg.QdrantURL,
		APIKey:     cfg.QdrantAPIKey,
		Collection: cfg.QdrantCollection,
		VectorSize: cfg.EmbeddingDim,
		Distance:   cfg.QdrantDistance,
	})
	if err != nil {
		return nil, err
	}

	if err := index.EnsureCollection(ctx); err != nil {
		return nil, err
	}
	if err := index.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	return index, nil
}
