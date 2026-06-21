package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type ChatModelConfig struct {
	Provider        string
	APIKey          string
	BaseURL         string
	Model           string
	Temperature     float32
	MaxOutputTokens int
	Timeout         time.Duration
}

func NewChatModel(ctx context.Context, config ChatModelConfig) (model.BaseChatModel, error) {
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	if provider == "" {
		provider = "openai"
	}

	switch provider {
	case "openai", "openai-compatible":
		modelConfig := &openai.ChatModelConfig{
			APIKey:              config.APIKey,
			BaseURL:             config.BaseURL,
			Model:               config.Model,
			Temperature:         &config.Temperature,
			MaxCompletionTokens: &config.MaxOutputTokens,
			Timeout:             config.Timeout,
		}
		return openai.NewChatModel(ctx, modelConfig)
	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER %q", config.Provider)
	}
}

func extractText(message *schema.Message) string {
	if message == nil {
		return ""
	}
	if message.Content != "" {
		return message.Content
	}

	var builder strings.Builder
	for _, part := range message.AssistantGenMultiContent {
		if part.Text != "" {
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}
