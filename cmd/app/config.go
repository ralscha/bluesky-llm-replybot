package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL          string
	BlueskyIdentifier    string
	BlueskyPassword      string
	BlueskyHost          string
	BotHandle            string
	IngestorInterval     time.Duration
	WorkerInterval       time.Duration
	ReplySenderInterval  time.Duration
	MaxRetries           int
	ChatModel            ChatModelConfig
	UsagePricing         UsagePricing
	DailySpendingLimit   float64
	LLMRequestsPerMinute int
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

	dbURL := buildDatabaseURL(dbUser, dbPassword, dbHost, dbPort, dbName, dbSSLMode)

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

	chatModelConfig, err := loadChatModelConfig()
	if err != nil {
		return nil, err
	}

	usagePricing, err := loadUsagePricing()
	if err != nil {
		return nil, err
	}

	dailySpendingLimit, err := getOptionalFloat("LLM_DAILY_SPENDING_LIMIT", 0)
	if err != nil {
		return nil, err
	}
	if dailySpendingLimit > 0 && usagePricing.Total() <= 0 {
		return nil, fmt.Errorf("at least one LLM price must be greater than zero when LLM_DAILY_SPENDING_LIMIT is enabled")
	}

	llmRequestsPerMinute, err := getOptionalInt("LLM_REQUESTS_PER_MINUTE", 0)
	if err != nil {
		return nil, err
	}

	maxRetries := 3
	if maxRetriesEnv := os.Getenv("MAX_RETRIES"); maxRetriesEnv != "" {
		if parsed, err := strconv.Atoi(maxRetriesEnv); err == nil && parsed > 0 {
			maxRetries = parsed
		}
	}

	config := &Config{
		DatabaseURL:          dbURL,
		BlueskyIdentifier:    blueskyIdentifier,
		BlueskyPassword:      blueskyPassword,
		BlueskyHost:          blueskyHost,
		BotHandle:            botHandle,
		IngestorInterval:     1 * time.Minute,
		WorkerInterval:       5 * time.Second,
		ReplySenderInterval:  10 * time.Second,
		MaxRetries:           maxRetries,
		ChatModel:            chatModelConfig,
		UsagePricing:         usagePricing,
		DailySpendingLimit:   dailySpendingLimit,
		LLMRequestsPerMinute: llmRequestsPerMinute,
	}

	return config, nil
}

func loadChatModelConfig() (ChatModelConfig, error) {
	apiKey := os.Getenv("LLM_API_KEY")
	modelName := os.Getenv("LLM_MODEL")
	if apiKey == "" || modelName == "" {
		return ChatModelConfig{}, fmt.Errorf("LLM_API_KEY and LLM_MODEL environment variables are required")
	}

	temperature, err := getOptionalFloat("LLM_TEMPERATURE", 0.7)
	if err != nil {
		return ChatModelConfig{}, err
	}

	maxOutputTokens, err := getOptionalPositiveInt("LLM_MAX_OUTPUT_TOKENS", 250)
	if err != nil {
		return ChatModelConfig{}, err
	}

	return ChatModelConfig{
		Provider:        getOptionalString("LLM_PROVIDER", "openai"),
		APIKey:          apiKey,
		BaseURL:         os.Getenv("LLM_BASE_URL"),
		Model:           modelName,
		Temperature:     float32(temperature),
		MaxOutputTokens: maxOutputTokens,
		Timeout:         60 * time.Second,
	}, nil
}

func loadUsagePricing() (UsagePricing, error) {
	inputCache, err := getOptionalFloat("LLM_PRICE_INPUT_CACHE_PER_MILLION", 0)
	if err != nil {
		return UsagePricing{}, err
	}
	inputMiss, err := getOptionalFloat("LLM_PRICE_INPUT_MISS_PER_MILLION", 0)
	if err != nil {
		return UsagePricing{}, err
	}
	output, err := getOptionalFloat("LLM_PRICE_OUTPUT_PER_MILLION", 0)
	if err != nil {
		return UsagePricing{}, err
	}
	return UsagePricing{InputCachePerMillion: inputCache, InputMissPerMillion: inputMiss, OutputPerMillion: output}, nil
}

func getOptionalString(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func getOptionalPositiveInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}
func getOptionalInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return parsed, nil
}

func getOptionalFloat(name string, fallback float64) (float64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number", name)
	}
	return parsed, nil
}

func buildDatabaseURL(dbUser, dbPassword, dbHost, dbPort, dbName, dbSSLMode string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(dbUser, dbPassword),
		Host:   net.JoinHostPort(dbHost, dbPort),
		Path:   dbName,
	}

	query := u.Query()
	query.Set("sslmode", dbSSLMode)
	u.RawQuery = query.Encode()

	return u.String()
}
