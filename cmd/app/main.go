package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
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

	pool, err := initDatabase(config.DatabaseURL, logger)
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	chatModel, err := NewChatModel(context.Background(), config.ChatModel)
	if err != nil {
		logger.Error("Failed to initialize chat model", "error", err)
		os.Exit(1)
	}

	spendingLimiter := NewSpendingLimiter(pool, config.UsagePricing, config.DailySpendingLimit)
	requestLimiter := NewRequestLimiter(config.LLMRequestsPerMinute)
	bot := NewBot(pool, chatModel, config.ChatModel.Model, config.ChatModel.MaxOutputTokens, spendingLimiter, requestLimiter, config.BotHandle, config.MaxRetries, logger)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	bot.Start(config)
	<-sigChan
	bot.Stop()
}
