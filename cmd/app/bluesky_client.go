package main

import (
	"fmt"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
)

func (b *Bot) createAuthenticatedBlueskyClient(config *Config) (*xrpc.Client, *atproto.ServerCreateSession_Output, error) {
	client := &xrpc.Client{Host: config.BlueskyHost}

	auth, err := atproto.ServerCreateSession(
		b.ctx,
		client,
		&atproto.ServerCreateSession_Input{
			Identifier: config.BlueskyIdentifier,
			Password:   config.BlueskyPassword,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("authentication failed: %w", err)
	}

	authClient := &xrpc.Client{
		Host: client.Host,
		Auth: &xrpc.AuthInfo{AccessJwt: auth.AccessJwt},
	}

	return authClient, auth, nil
}

func (b *Bot) checkNotifications(authClient *xrpc.Client) ([]*bsky.NotificationListNotifications_Notification, error) {
	limit := int64(10)
	reasons := []string{"mention"}
	cursor := ""
	var allUnreadNotifications []*bsky.NotificationListNotifications_Notification

	for {
		notificationsList, err := bsky.NotificationListNotifications(b.ctx, authClient, cursor, limit, false, reasons, "")
		if err != nil {
			return nil, fmt.Errorf("failed to list notifications: %w", err)
		}

		if len(notificationsList.Notifications) == 0 {
			break
		}

		hasReadMessages := false
		var unreadInBatch []*bsky.NotificationListNotifications_Notification

		for _, notif := range notificationsList.Notifications {
			if notif.IsRead {
				hasReadMessages = true
			} else {
				unreadInBatch = append(unreadInBatch, notif)
			}
		}

		allUnreadNotifications = append(allUnreadNotifications, unreadInBatch...)

		if hasReadMessages {
			break
		}

		if notificationsList.Cursor == nil {
			break
		}

		cursor = *notificationsList.Cursor
	}

	if len(allUnreadNotifications) > 0 {
		seenInput := &bsky.NotificationUpdateSeen_Input{
			SeenAt: time.Now().UTC().Format(time.RFC3339),
		}

		err := bsky.NotificationUpdateSeen(b.ctx, authClient, seenInput)
		if err != nil {
			return nil, fmt.Errorf("failed to mark notifications as seen: %w", err)
		}
	}

	return allUnreadNotifications, nil
}
