package main

import (
	"errors"
	"log"
	"os"
	"stockflow/backend/internal/config"

	"github.com/joho/godotenv"
)

func main() {
	// Load local environment variables
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatalf("failed to load .env: %v", err)
		}
	}

	// Load and validate application configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	log.Printf(
		"StockFlow API configuration loaded | env=%s | port=%s",
		cfg.AppEnv,
		cfg.Port,
	)
}
