package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/genai"

	database "github.com/ralscha/bluesky_llm_replybot/internal/database/generated"
)

func (b *Bot) runWorker(config *Config) {
	b.logger.Info("Starting worker...")

	ticker := time.NewTicker(config.WorkerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Info("Worker shutting down...")
			return
		case <-ticker.C:
			if err := b.processNextMessage(); err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					b.logger.Error("Worker error", "error", err)
				}
			}
		}
	}
}

func (b *Bot) processNextMessage() error {
	message, err := b.queries.ClaimNextMessage(b.ctx)
	if err != nil {
		return err
	}

	b.logger.Info("Processing message",
		"message_id", message.ID,
		"author_handle", message.AuthorHandle)

	response, usedGoogleSearchGrounding, modelName, err := b.generateLLMResponse(message.MessageText)
	if err != nil {
		return b.handleLLMGenerationError(message, err)
	}

	if err := b.queries.UpdateMessageWithLLMResponse(b.ctx, database.UpdateMessageWithLLMResponseParams{
		ID:                        message.ID,
		LlmResponse:               &response,
		UsedGoogleSearchGrounding: usedGoogleSearchGrounding,
		ModelName:                 &modelName,
	}); err != nil {
		return fmt.Errorf("failed to update message with LLM response: %w", err)
	}

	b.logger.Info("Completed message", "message_id", message.ID)
	return nil
}

func (b *Bot) handleLLMGenerationError(message database.ClaimNextMessageRow, err error) error {
	errorMsg := err.Error()
	currentRetryCount := int32(0)
	if message.RetryCount != nil {
		currentRetryCount = *message.RetryCount
	}

	if currentRetryCount+1 >= int32(b.maxRetries) {
		return b.handleMaxRetriesReached(message)
	}

	return b.handleRetryableFailure(message, errorMsg, err)
}

func (b *Bot) handleMaxRetriesReached(message database.ClaimNextMessageRow) error {
	b.logger.Warn("Max retries reached for message, sending fallback response",
		"message_id", message.ID,
		"retry_count", message.RetryCount)

	fallbackResponse := "I apologize, but I'm unable to generate a response at this time. Please try again later."

	if updateErr := b.queries.UpdateMessageWithLLMResponse(b.ctx, database.UpdateMessageWithLLMResponseParams{
		ID:          message.ID,
		LlmResponse: &fallbackResponse,
	}); updateErr != nil {
		b.logger.Error("Failed to update message with fallback response",
			"message_id", message.ID,
			"error", updateErr)
		return fmt.Errorf("failed to update message with fallback response: %w", updateErr)
	}

	b.logger.Info("Set fallback response for failed message", "message_id", message.ID)
	return nil
}

func (b *Bot) handleRetryableFailure(message database.ClaimNextMessageRow, errorMsg string, originalErr error) error {
	maxRetries := int32(b.maxRetries)
	if updateErr := b.queries.UpdateMessageFailed(b.ctx, database.UpdateMessageFailedParams{
		ID:           message.ID,
		RetryCount:   &maxRetries,
		ErrorMessage: &errorMsg,
	}); updateErr != nil {
		b.logger.Error("Failed to update message as failed",
			"message_id", message.ID,
			"error", updateErr)
	}
	return fmt.Errorf("failed to generate LLM response: %w", originalErr)
}

func (b *Bot) generateLLMResponse(userMessage string) (string, *bool, string, error) {
	prompt := fmt.Sprintf(`You are a helpful AI assistant responding to a message on Bluesky (microblogging social media service).
Please provide a thoughtful, engaging, and helpful response to the following user message.
Keep your response concise and appropriate for social media (maximum 300 characters).

User message: 
%s
`, userMessage)

	content := genai.Text(prompt)

	models := []string{ModelFlash, ModelFlashLite}

	for _, model := range models {
		canUse, reason := b.rateLimiter.CanUseModel(model)
		if !canUse {
			b.logger.Info("Model not available, trying next",
				"model", model,
				"reason", reason)
			continue
		}

		enableGoogleSearch := b.rateLimiter.CanUseGrounding(model, "google_search")

		config := &genai.GenerateContentConfig{}

		if enableGoogleSearch {
			tools := []*genai.Tool{}
			if enableGoogleSearch {
				tools = append(tools, &genai.Tool{
					GoogleSearch: &genai.GoogleSearch{},
				})
			}
			tools = append(tools, &genai.Tool{
				CodeExecution: &genai.ToolCodeExecution{},
			})
			config.Tools = tools
		}

		b.logger.Info("Attempting to generate response",
			"model", model,
			"google_search_enabled", enableGoogleSearch)

		resp, err := b.genaiClient.Models.GenerateContent(b.ctx, model, content, config)
		if err != nil {
			b.rateLimiter.HandleRateLimitError(model, err)

			b.logger.Warn("Failed to generate response with model",
				"model", model,
				"error", err)

			continue
		}

		if len(resp.Candidates) == 0 {
			b.logger.Warn("No candidates returned from model", "model", model)
			continue
		}

		candidate := resp.Candidates[0]
		if len(candidate.Content.Parts) == 0 {
			b.logger.Warn("No parts in candidate content", "model", model)
			continue
		}

		var responseText string
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				responseText += part.Text
			}
		}

		if responseText == "" {
			b.logger.Warn("Received empty response from model", "model", model)
			continue
		}

		tokensUsed := 0
		if resp.UsageMetadata != nil {
			tokensUsed = int(resp.UsageMetadata.TotalTokenCount)
		} else {
			// Fallback if usage metadata is not available
			tokensUsed = (len(prompt) + len(responseText)) / 4
		}

		b.rateLimiter.RecordRequest(model, tokensUsed)

		if enableGoogleSearch {
			b.rateLimiter.RecordGrounding(model, "google_search")
		}

		return responseText, &enableGoogleSearch, model, nil
	}

	return "", nil, "", fmt.Errorf("failed to generate response: all models exhausted or rate limited")
}
