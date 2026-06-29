package config

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                      string
	DatabaseURL               string
	JWTSecret                 string
	JWTAccessTTL              int64
	JWTRefreshTTL             int64
	ContextTokenLimit         int
	ContextCompressRatio      float64
	GenerationMaxToolRounds   int
	GenerationRetryInterval   int64
	GenerationRetryMax        int64
	TavilyAPIKey              string
	TavilyBaseURL             string
	EmbeddingProvider         string
	EmbeddingBaseURL          string
	EmbeddingAPIKey           string
	EmbeddingModel            string
	EmbeddingDim              int
	QdrantURL                 string
	QdrantAPIKey              string
	QdrantCollection          string
	QdrantDistance            string
	MemorySimilarityThreshold *float64
	RAGSplitterProvider       string
	RAGSplitterAPIURL         string
	RAGSplitterAPIHeaders     string
	RAGSplitterSegmentsPath   string

	MCPServers []MCPServerConfig
}

type MCPServerConfig struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	AuthType  string   `json:"auth_type"`
	AuthField string   `json:"auth_field"`
	AuthKey   string   `json:"auth_key"`
	Transport string   `json:"transport"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Env       []string `json:"env"`
}

const defaultTavilyBaseURL = "https://api.tavily.com"

func Load() (Config, error) {
	_ = godotenv.Load()

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	mcpServers, err := loadMCPServers()
	if err != nil {
		return Config{}, err
	}
	memorySimilarityThreshold := getOptionalEnvFloat64("MEMORY_SIMILARITY_THRESHOLD")
	contextCompressRatio := getEnvFloat64("CONTEXT_COMPRESS_RATIO", 0.8)
	if contextCompressRatio <= 0 || contextCompressRatio > 1 {
		log.Printf(
			"warning: config CONTEXT_COMPRESS_RATIO=%f is invalid, expected a value in (0, 1], using fallback value 0.800000",
			contextCompressRatio,
		)
		contextCompressRatio = 0.8
	}

	return Config{
		Port:                      getEnv("PORT", "8080"),
		DatabaseURL:               databaseURL,
		JWTSecret:                 jwtSecret,
		JWTAccessTTL:              getEnvInt64("JWT_ACCESS_TTL_SECONDS", 3600),
		JWTRefreshTTL:             getEnvInt64("JWT_REFRESH_TTL_SECONDS", 2592000),
		ContextTokenLimit:         int(max(getEnvInt64("CONTEXT_TOKEN_LIMIT", 32000), 1)),
		ContextCompressRatio:      contextCompressRatio,
		GenerationMaxToolRounds:   max(int(getEnvInt64("GENERATION_MAX_TOOL_ROUNDS", 12)), 1),
		GenerationRetryInterval:   max(getEnvInt64("GENERATION_RETRY_INTERVAL_SECONDS", 30), 1),
		GenerationRetryMax:        max(getEnvInt64("GENERATION_RETRY_MAX", 5), 1),
		TavilyAPIKey:              strings.TrimSpace(os.Getenv("TAVILY_API_KEY")),
		TavilyBaseURL:             strings.TrimRight(strings.TrimSpace(getEnv("TAVILY_BASE_URL", defaultTavilyBaseURL)), "/"),
		EmbeddingProvider:         strings.TrimSpace(os.Getenv("EMBEDDING_PROVIDER")),
		EmbeddingBaseURL:          strings.TrimRight(strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL")), "/"),
		EmbeddingAPIKey:           strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY")),
		EmbeddingModel:            strings.TrimSpace(os.Getenv("EMBEDDING_MODEL")),
		EmbeddingDim:              int(max(getEnvInt64("EMBEDDING_DIM", 0), 0)),
		QdrantURL:                 strings.TrimRight(strings.TrimSpace(os.Getenv("QDRANT_URL")), "/"),
		QdrantAPIKey:              strings.TrimSpace(os.Getenv("QDRANT_API_KEY")),
		QdrantCollection:          strings.TrimSpace(os.Getenv("QDRANT_COLLECTION")),
		QdrantDistance:            strings.TrimSpace(getEnv("QDRANT_DISTANCE", "Cosine")),
		MemorySimilarityThreshold: memorySimilarityThreshold,
		RAGSplitterProvider:       strings.TrimSpace(os.Getenv("RAG_SPLITTER_PROVIDER")),
		RAGSplitterAPIURL:         strings.TrimSpace(os.Getenv("RAG_SPLITTER_API_URL")),
		RAGSplitterAPIHeaders:     strings.TrimSpace(getEnv("RAG_SPLITTER_API_HEADERS_JSON", "{}")),
		RAGSplitterSegmentsPath:   strings.TrimSpace(getEnv("RAG_SPLITTER_API_SEGMENTS_PATH", "chunks")),

		MCPServers: mcpServers,
	}, nil
}

func (c Config) MemoryRAGEnabled() bool {
	return c.EmbeddingProvider != "" &&
		c.EmbeddingBaseURL != "" &&
		c.EmbeddingModel != "" &&
		c.EmbeddingDim > 0 &&
		c.QdrantURL != "" &&
		c.QdrantCollection != "" &&
		c.RAGSplitterProvider != "" &&
		c.RAGSplitterAPIURL != "" &&
		c.RAGSplitterSegmentsPath != ""
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		log.Printf(
			"warning: config %s=%q is invalid, expected an integer, using fallback value %d",
			key,
			value,
			fallback,
		)
		return fallback
	}

	return parsed
}

func getEnvFloat64(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("warning: config %s=%q is invalid, expected a float, using fallback value %f", key, value, fallback)
		return fallback
	}

	return parsed
}

func getOptionalEnvFloat64(key string) *float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("warning: config %s=%q is invalid, expected a float, ignoring it", key, value)
		return nil
	}

	return &parsed
}

func loadMCPServers() ([]MCPServerConfig, error) {
	raw := strings.TrimSpace(os.Getenv("MCP_SERVERS"))
	if raw == "" {
		return nil, nil
	}

	var servers []MCPServerConfig
	if err := json.Unmarshal([]byte(raw), &servers); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(servers))
	for i := range servers {
		servers[i].Name = strings.TrimSpace(servers[i].Name)
		servers[i].URL = strings.TrimRight(strings.TrimSpace(servers[i].URL), "/")
		servers[i].AuthType = strings.TrimSpace(servers[i].AuthType)
		servers[i].AuthField = strings.TrimSpace(servers[i].AuthField)
		servers[i].AuthKey = strings.TrimSpace(servers[i].AuthKey)
		servers[i].Transport = strings.TrimSpace(servers[i].Transport)
		servers[i].Command = strings.TrimSpace(servers[i].Command)

		if servers[i].Name == "" {
			return nil, errors.New("MCP_SERVERS server name is required")
		}
		if _, ok := seen[servers[i].Name]; ok {
			return nil, errors.New("MCP_SERVERS contains duplicate name")
		}
		seen[servers[i].Name] = struct{}{}

		switch servers[i].Transport {
		case "http":
			if servers[i].URL == "" {
				return nil, errors.New("MCP_SERVERS http server url is required")
			}
		case "stdio":
			if servers[i].Command == "" {
				return nil, errors.New("MCP_SERVERS stdio server command is required")
			}
		default:
			return nil, errors.New("MCP_SERVERS contains unsupported transport")
		}

		if servers[i].AuthType == "" {
			servers[i].AuthType = "none"
		}
	}

	return servers, nil
}
