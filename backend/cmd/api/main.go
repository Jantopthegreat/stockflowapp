package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"stockflow/backend/internal/config"
	"stockflow/backend/internal/database"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("failed to load .env: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	log.Printf(
		"Connected to PostgreSQL | env=%s | port=%s",
		cfg.AppEnv,
		cfg.Port,
	)
}
