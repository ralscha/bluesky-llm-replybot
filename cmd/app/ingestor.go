package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/jackc/pgx/v5"

	database "github.com/ralscha/bluesky_llm_replybot/internal/database/generated"
)

func (b *Bot) runIngestor(config *Config) {
	b.logger.Info("Starting ingestor...")

	ticker := time.NewTicker(config.IngestorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ingestorCtx.Done():
			b.logger.Info("Ingestor shutting down...")
			return
		case <-ticker.C:
			if err := b.ingestNotifications(config); err != nil {
				b.logger.Error("Error ingesting notifications", "error", err)
			}
		}
	}
}

func (b *Bot) ingestNotifications(config *Config) error {
	authClient, _, err := b.createAuthenticatedBlueskyClient(config)
	if err != nil {
		return fmt.Errorf("failed to create authenticated client: %w", err)
	}

	notifications, err := b.checkNotifications(authClient)
	if err != nil {
		return fmt.Errorf("failed to check notifications: %w", err)
	}

	for _, notif := range notifications {
		if err := b.processNotificationForQueue(notif); err != nil {
			b.logger.Error("Error processing notification for queue", "error", err)
		}
	}

	return nil
}

func (b *Bot) processNotificationForQueue(notif *bsky.NotificationListNotifications_Notification) error {
	feedPost, ok := notif.Record.Val.(*bsky.FeedPost)
	if !ok {
		return fmt.Errorf("notification record is not a FeedPost")
	}

	if !strings.Contains(feedPost.Text, b.botHandle) {
		return nil
	}

	cleanedText := strings.ReplaceAll(feedPost.Text, b.botHandle, "")
	cleanedText = strings.TrimSpace(cleanedText)

	if cleanedText == "" {
		return nil
	}

	_, err := b.queries.InsertMessage(b.ctx, database.InsertMessageParams{
		MessageUri:   notif.Uri,
		MessageCid:   notif.Cid,
		AuthorDid:    notif.Author.Did,
		AuthorHandle: notif.Author.Handle,
		MessageText:  cleanedText,
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // Already queued, ignore silently
		}
		return fmt.Errorf("failed to insert message into queue: %w", err)
	}

	return nil
}
