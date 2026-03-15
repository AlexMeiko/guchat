package config

import (
	"errors"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	JWTSecret     string
	JWTAccessTTL  int64
	JWTRefreshTTL int64
}

func Load() (Config, error) {
	_ = godotenv.Load()

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	return Config{
		Port:          getEnv("PORT", "8080"),
		JWTSecret:     jwtSecret,
		JWTAccessTTL:  getEnvInt64("JWT_ACCESS_TTL_SECONDS", 3600),
		JWTRefreshTTL: getEnvInt64("JWT_REFRESH_TTL_SECONDS", 2592000),
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
