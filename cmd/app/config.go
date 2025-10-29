package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL         string
	BlueskyIdentifier   string
	BlueskyPassword     string
	BlueskyHost         string
	BotHandle           string
	IngestorInterval    time.Duration
	WorkerInterval      time.Duration
	ReplySenderInterval time.Duration
	MaxRetries          int
}

func LoadConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		slog.Warn("Error loading .env file", "error", err)
	}

	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbSSLMode := os.Getenv("DB_SSLMODE")

	if dbUser == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" || dbSSLMode == "" {
		return nil, fmt.Errorf("all database environment variables (DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, DB_NAME, DB_SSLMODE) are required")
	}

	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		dbUser, dbPassword, dbHost, dbPort, dbName, dbSSLMode)

	blueskyIdentifier := os.Getenv("BLUESKY_IDENTIFIER")
	blueskyPassword := os.Getenv("BLUESKY_PASSWORD")
	blueskyHost := os.Getenv("BLUESKY_HOST")
	botHandle := os.Getenv("BOT_HANDLE")

	if blueskyIdentifier == "" || blueskyPassword == "" {
		return nil, fmt.Errorf("BLUESKY_IDENTIFIER and BLUESKY_PASSWORD environment variables are required")
	}

	if blueskyHost == "" {
		return nil, fmt.Errorf("BLUESKY_HOST environment variable is required")
	}

	if botHandle == "" {
		return nil, fmt.Errorf("BOT_HANDLE environment variable is required")
	}

	maxRetries := 3
	if maxRetriesEnv := os.Getenv("MAX_RETRIES"); maxRetriesEnv != "" {
		if parsed, err := strconv.Atoi(maxRetriesEnv); err == nil && parsed > 0 {
			maxRetries = parsed
		}
	}

	config := &Config{
		DatabaseURL:         dbURL,
		BlueskyIdentifier:   blueskyIdentifier,
		BlueskyPassword:     blueskyPassword,
		BlueskyHost:         blueskyHost,
		BotHandle:           botHandle,
		IngestorInterval:    1 * time.Minute,
		WorkerInterval:      5 * time.Second,
		ReplySenderInterval: 10 * time.Second,
		MaxRetries:          maxRetries,
	}

	return config, nil
}
