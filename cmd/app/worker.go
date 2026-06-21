package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	database "github.com/ralscha/bluesky_llm_replybot/internal/database/generated"
)

const fallbackResponseText = "I apologize, but I'm unable to generate a response at this time. Please try again later."

func (b *Bot) runWorker(config *Config) {
	b.logger.Info("Starting worker...")

	ticker := time.NewTicker(config.WorkerInterval)
	defer ticker.Stop()

	b.processNextMessageAndLog()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Info("Worker shutting down...")
			return
		case <-ticker.C:
			b.processNextMessageAndLog()
		}
	}
}

func (b *Bot) processNextMessageAndLog() {
	if err := b.processNextMessage(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		b.logger.Error("Worker error", "error", err)
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

	response, modelName, err := b.generateLLMResponse(message.MessageText)
	if err != nil {
		var spendingErr *SpendingLimitExceededError
		if errors.As(err, &spendingErr) {
			return b.deferAfterSpendingLimit(message.ID, spendingErr.Status)
		}
		return b.handleLLMGenerationError(message, err)
	}

	if err := b.queries.UpdateMessageWithLLMResponse(b.ctx, database.UpdateMessageWithLLMResponseParams{
		ID:          message.ID,
		LlmResponse: &response,
		ModelName:   &modelName,
	}); err != nil {
		return fmt.Errorf("failed to update message with LLM response: %w", err)
	}

	b.logger.Info("Completed message", "message_id", message.ID)
	return nil
}

func (b *Bot) deferAfterSpendingLimit(messageID int64, status SpendingStatus) error {
	now := time.Now()
	notice := spendingLimitNotice(status, now)
	if err := b.queries.UpdateMessageDeferredWithNotice(b.ctx, database.UpdateMessageDeferredWithNoticeParams{
		ID:          messageID,
		LlmResponse: &notice,
		DeferredUntil: pgtype.Timestamptz{
			Time:  status.ResetAt,
			Valid: true,
		},
	}); err != nil {
		return fmt.Errorf("failed to defer message after spending limit reached: %w", err)
	}

	b.logger.Info("Deferred message until daily spending reset",
		"message_id", messageID,
		"reset_at", status.ResetAt.Format(time.RFC3339),
		"spent_micros", status.SpentMicros,
		"reserved_micros", status.ReservedMicros,
		"limit_micros", status.LimitMicros)

	return nil
}
func spendingLimitNotice(status SpendingStatus, now time.Time) string {
	hours := hoursUntil(status.ResetAt, now)
	if hours == 1 {
		return "I've reached my daily LLM budget. Your message will be processed after the daily reset in about 1 hour."
	}
	return fmt.Sprintf("I've reached my daily LLM budget. Your message will be processed after the daily reset in about %d hours.", hours)
}

func (b *Bot) handleLLMGenerationError(message database.ClaimNextMessageRow, err error) error {
	errorMsg := err.Error()
	currentRetryCount := message.RetryCount

	if currentRetryCount+1 >= int32(b.maxRetries) {
		return b.handleMaxRetriesReached(message)
	}

	return b.handleRetryableFailure(message, errorMsg, err)
}

func (b *Bot) handleMaxRetriesReached(message database.ClaimNextMessageRow) error {
	b.logger.Warn("Max retries reached for message, sending fallback response",
		"message_id", message.ID,
		"retry_count", message.RetryCount)

	if updateErr := b.queries.UpdateMessageWithLLMResponse(b.ctx, database.UpdateMessageWithLLMResponseParams{
		ID:          message.ID,
		LlmResponse: new(fallbackResponseText),
	}); updateErr != nil {
		b.logger.Error("Failed to update message with fallback response",
			"message_id", message.ID,
			"error", updateErr)
		return fmt.Errorf("failed to update message with fallback response: %w", updateErr)
	}

	b.logger.Info("Set fallback response for failed message", "message_id", message.ID)
	return nil
}

//go:fix inline
func stringPtr(s string) *string {
	return new(s)
}

func (b *Bot) handleRetryableFailure(message database.ClaimNextMessageRow, errorMsg string, originalErr error) error {
	maxRetries := int32(b.maxRetries)
	if updateErr := b.queries.UpdateMessageFailed(b.ctx, database.UpdateMessageFailedParams{
		ID:         message.ID,
		RetryCount: maxRetries,
	}); updateErr != nil {
		b.logger.Error("Failed to update message as failed",
			"message_id", message.ID,
			"error", updateErr)
	}
	return fmt.Errorf("failed to generate LLM response: %w", originalErr)
}

func (b *Bot) generateLLMResponse(userMessage string) (string, string, error) {
	prompt := fmt.Sprintf(`You are a helpful AI assistant responding to a message on Bluesky (microblogging social media service).
Please provide a thoughtful, engaging, and helpful response to the following user message.
Keep your response concise and appropriate for social media (maximum 500 characters).

User message:
%s
`, userMessage)

	messages := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}

	modelName := b.currentModelName()
	b.logger.Info("Attempting to generate response", "model", modelName)

	if err := b.waitForLLMRequestSlot(); err != nil {
		return "", "", err
	}

	reservation, status, allowed, err := b.reserveLLMSpend(prompt)
	if err != nil {
		return "", "", err
	}
	if !allowed {
		return "", "", &SpendingLimitExceededError{Status: status}
	}

	resp, err := b.chatModel.Generate(b.ctx, messages)
	if err != nil {
		return "", "", err
	}

	responseText := extractText(resp)
	if responseText == "" {
		return "", "", fmt.Errorf("received empty response from model")
	}

	if reservation.IsValid() {
		usage := usageFromResponse(resp)
		if usage == nil {
			usage = b.spendingLimiter.EstimateUsage(prompt, responseText)
		}
		status, err := b.spendingLimiter.FinalizeReservation(b.ctx, reservation, usage)
		if err != nil {
			return "", "", err
		}
		if status.CommittedAndReservedMicros() >= status.LimitMicros {
			b.logger.Warn("Daily LLM spending limit reached",
				"spent_micros", status.SpentMicros,
				"reserved_micros", status.ReservedMicros,
				"limit_micros", status.LimitMicros,
				"reset_at", status.ResetAt.Format(time.RFC3339))
		}
	}

	return responseText, modelName, nil
}

type SpendingLimitExceededError struct {
	Status SpendingStatus
}

func (e *SpendingLimitExceededError) Error() string {
	return "daily LLM spending limit reached"
}

func (b *Bot) reserveLLMSpend(prompt string) (SpendingReservation, SpendingStatus, bool, error) {
	if b.spendingLimiter == nil || !b.spendingLimiter.IsEnabled() {
		return SpendingReservation{}, SpendingStatus{}, true, nil
	}
	return b.spendingLimiter.Reserve(b.ctx, time.Now(), prompt, b.chatModelMaxOutputTokens())
}

func (b *Bot) chatModelMaxOutputTokens() int {
	if b.maxOutputTokens <= 0 {
		return 1
	}
	return b.maxOutputTokens
}

func (b *Bot) waitForLLMRequestSlot() error {
	if b.requestLimiter == nil {
		return nil
	}
	return b.requestLimiter.Wait(b.ctx)
}

func usageFromResponse(resp *schema.Message) *schema.TokenUsage {
	if resp == nil || resp.ResponseMeta == nil || resp.ResponseMeta.Usage == nil {
		return nil
	}
	return resp.ResponseMeta.Usage
}

func (b *Bot) currentModelName() string {
	return b.llmModelName
}
