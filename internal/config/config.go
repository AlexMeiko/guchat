package config

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	JWTSecret               string
	JWTAccessTTL            int64
	JWTRefreshTTL           int64
	GenerationContextLimit  int
	GenerationMaxToolRounds int
	GenerationRetryInterval int64
	GenerationRetryMax      int64
	TavilyAPIKey            string
	TavilyBaseURL           string

	MCPName      string
	MCPURL       string
	MCPAuthType  string
	MCPAuthField string
	MCPAuthKey   string
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

	return Config{
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             databaseURL,
		JWTSecret:               jwtSecret,
		JWTAccessTTL:            getEnvInt64("JWT_ACCESS_TTL_SECONDS", 3600),
		JWTRefreshTTL:           getEnvInt64("JWT_REFRESH_TTL_SECONDS", 2592000),
		GenerationContextLimit:  max(int(getEnvInt64("GENERATION_CONTEXT_LIMIT", 25)), 1),
		GenerationMaxToolRounds: max(int(getEnvInt64("GENERATION_MAX_TOOL_ROUNDS", 12)), 1),
		GenerationRetryInterval: max(getEnvInt64("GENERATION_RETRY_INTERVAL_SECONDS", 30), 1),
		GenerationRetryMax:      max(getEnvInt64("GENERATION_RETRY_MAX", 5), 1),
		TavilyAPIKey:            strings.TrimSpace(os.Getenv("TAVILY_API_KEY")),
		TavilyBaseURL:           strings.TrimRight(strings.TrimSpace(getEnv("TAVILY_BASE_URL", defaultTavilyBaseURL)), "/"),

		MCPName:      strings.TrimSpace(getEnv("MCP_NAME", "mcp")),
		MCPURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("MCP_URL")), "/"),
		MCPAuthType:  strings.TrimSpace(getEnv("MCP_AUTH_TYPE", "none")),
		MCPAuthField: strings.TrimSpace(os.Getenv("MCP_AUTH_FIELD")),
		MCPAuthKey:   strings.TrimSpace(os.Getenv("MCP_AUTH_KEY")),
	}, nil
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
