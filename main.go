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
	"github.com/AlexMeiko/guchat/internal/sandbox"
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
	workspaceManager, err := sandbox.NewWorkspaceManager(cfg.SandboxDataRoot)
	if err != nil {
		log.Fatal(err)
	}

	var sandboxManager *sandbox.Manager
	if cfg.SandboxEnabled {
		dockerRunner := sandbox.NewDockerRunner(cfg.SandboxImage)

		checkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := dockerRunner.Check(checkCtx); err != nil {
			cancel()
			log.Fatal(err)
		}
		cancel()

		sandboxManager = sandbox.NewManager(
			workspaceManager,
			dockerRunner,
			time.Duration(cfg.SandboxIdleTimeoutSeconds)*time.Second,
		)
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			for range ticker.C {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				sandboxManager.CleanupExpired(cleanupCtx)
				cancel()
			}
		}()
	}

	conversationService := service.NewConversationService(conversationRepo, workspaceManager, sandboxManager)
	conversationHandler := handler.NewConversationHandler(conversationService)
	workspaceService := service.NewWorkspaceService(conversationService, workspaceManager)
	workspaceHandler := handler.NewWorkspaceHandler(workspaceService)

	memoryStore := memory.NewMySQLStore(mysqlDB)
	mysqlMemoryRetriever := memory.NewMySQLRetriever(memoryStore)

	memoryRAG, err := buildMemoryRAGComponents(context.Background(), client, cfg)
	if err != nil {
		log.Fatal(err)
	}

	memoryIndexer, err := buildMemoryIndexer(memoryRAG)
	if err != nil {
		log.Fatal(err)
	}

	memoryRetriever, err := buildMemoryRetriever(mysqlMemoryRetriever, memoryRAG)
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

	if sandboxManager != nil {
		toolProviders = append(toolProviders, tool.NewTerminalProvider(sandboxManager))
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
		cfg.ContextTokenLimit,
		cfg.ContextCompressRatio,
		cfg.GenerationMaxToolRounds,
		time.Duration(cfg.GenerationRetryInterval)*time.Second,
		int(cfg.GenerationRetryMax),
	)
	generationHandler := handler.NewGenerationHandler(generationService, messageService, toolCallService, runtimeManager)

	go generationService.RetryLoop(context.Background())

	r := router.New(
		authHandler,
		conversationHandler,
		messageHandler,
		memoryHandler,
		modelHandler,
		generationHandler,
		workspaceHandler,
		jwtService,
	)
	log.Printf("server starting on port %s", cfg.Port)

	if err := r.Run(":" + cfg.Port); err != nil {
		panic(err)
	}
}

type memoryRAGComponents struct {
	splitter            segment.Splitter
	embedder            embed.Embedder
	index               vector.Index
	similarityThreshold *float64
}

func buildMemoryRAGComponents(
	ctx context.Context,
	client *http.Client,
	cfg config.Config,
) (*memoryRAGComponents, error) {
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

	return &memoryRAGComponents{
		splitter:            splitter,
		embedder:            embedder,
		index:               index,
		similarityThreshold: cfg.MemorySimilarityThreshold,
	}, nil
}

func buildMemoryIndexer(components *memoryRAGComponents) (memory.Indexer, error) {
	if components == nil {
		return nil, nil
	}

	return memory.NewVectorIndexer(components.splitter, components.embedder, components.index)
}

func buildMemoryRetriever(
	fallback memory.Retriever,
	components *memoryRAGComponents,
) (memory.Retriever, error) {
	if components == nil {
		return fallback, nil
	}

	return memory.NewVectorRetriever(fallback, components.embedder, components.index, components.similarityThreshold)
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
	case "dashscope":
		return embed.NewDashScopeEmbedder(client, embed.DashScopeEmbedderConfig{
			BaseURL: cfg.EmbeddingBaseURL,
			APIKey:  cfg.EmbeddingAPIKey,
			Model:   cfg.EmbeddingModel,
			Dim:     cfg.EmbeddingDim,
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
