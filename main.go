package main

import (
	"log"

	"github.com/AlexMeiko/guchat/internal/config"
	"github.com/AlexMeiko/guchat/internal/handler"
	"github.com/AlexMeiko/guchat/internal/router"
	"github.com/AlexMeiko/guchat/internal/service"
)

func main() {
	cfg, err := config.Load()

	if err != nil {
		log.Fatal(err)
	}

	jwtService := service.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessTTL,
		cfg.JWTRefreshTTL,
	)

	authHandler := handler.NewAuthHandler(jwtService)

	r := router.New(authHandler, jwtService)
	log.Printf("server starting on port %s", cfg.Port)

	if err := r.Run(":" + cfg.Port); err != nil {
		panic(err)
	}
}
