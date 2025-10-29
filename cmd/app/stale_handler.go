package main

import (
	"fmt"
	"time"
)

func (b *Bot) runStaleMessageHandler() {
	b.logger.Info("Starting stale message handler...")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Info("Stale message handler shutting down...")
			return
		case <-ticker.C:
			if err := b.handleStaleMessages(); err != nil {
				b.logger.Error("Error handling stale messages", "error", err)
			}
		}
	}
}

func (b *Bot) handleStaleMessages() error {
	staleMessages, err := b.queries.GetStaleProcessingMessages(b.ctx)
	if err != nil {
		return fmt.Errorf("failed to get stale processing messages: %w", err)
	}

	for _, message := range staleMessages {
		b.logger.Warn("Resetting stale message",
			"message_id", message.ID,
			"retry_count", message.RetryCount)

		if err := b.queries.ResetStaleMessage(b.ctx, message.ID); err != nil {
			b.logger.Error("Failed to reset stale message",
				"message_id", message.ID,
				"error", err)
		}
	}

	if len(staleMessages) > 0 {
		b.logger.Info("Reset stale messages", "count", len(staleMessages))
	}

	return nil
}
