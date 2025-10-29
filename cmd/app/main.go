package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/genai"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	config, err := LoadConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		logger.Error("GEMINI_API_KEY environment variable is required")
		os.Exit(1)
	}

	pool, err := initDatabase(config.DatabaseURL, logger)
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	genaiClient, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: geminiAPIKey,
	})
	if err != nil {
		logger.Error("Failed to initialize Google GenAI client", "error", err)
		os.Exit(1)
	}

	bot := NewBot(pool, genaiClient, config.BotHandle, config.MaxRetries, logger)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	bot.Start(config)
	<-sigChan
	bot.Stop()
}
