package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"

	database "github.com/ralscha/bluesky_llm_replybot/internal/database/generated"
)

func (b *Bot) runReplySender(config *Config) {
	b.logger.Info("Starting reply sender...")

	ticker := time.NewTicker(config.ReplySenderInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Info("Reply sender shutting down...")
			return
		case <-ticker.C:
			if err := b.sendPendingReplies(config); err != nil {
				b.logger.Error("Error sending pending replies", "error", err)
			}
		}
	}
}

func (b *Bot) sendPendingReplies(config *Config) error {
	messages, err := b.queries.GetReadyToSendMessages(b.ctx, 10)
	if err != nil {
		return fmt.Errorf("failed to get ready to send messages: %w", err)
	}

	if len(messages) == 0 {
		return nil
	}

	authClient, auth, err := b.createAuthenticatedBlueskyClient(config)
	if err != nil {
		return fmt.Errorf("failed to create authenticated client: %w", err)
	}

	for _, message := range messages {
		select {
		case <-b.ctx.Done():
			b.logger.Info("Reply sender cancelled during processing")
			return nil
		default:
		}

		replyURI, replyCID, err := b.sendReply(authClient, auth, message)
		if err != nil {
			b.logger.Error("Failed to send reply",
				"message_id", message.ID,
				"error", err)

			errorMsg := err.Error()
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

			b.finalizeMessage(message, replyURI, replyCID, "failed", &errorMsg)
		} else {
			b.logger.Info("Successfully sent reply", "message_id", message.ID)
			b.finalizeMessage(message, replyURI, replyCID, "completed", nil)
		}

		select {
		case <-b.ctx.Done():
			b.logger.Info("Reply sender cancelled during sleep")
			return nil
		case <-time.After(5 * time.Second):
			// continue
		}
	}

	return nil
}

func (b *Bot) sendReply(authClient *xrpc.Client, auth *atproto.ServerCreateSession_Output, message database.GetReadyToSendMessagesRow) (string, string, error) {
	var responseText string
	if message.LlmResponse != nil {
		responseText = *message.LlmResponse
	} else {
		return "", "", fmt.Errorf("no LLM response available for message ID %d", message.ID)
	}

	var signature string
	if message.ModelName != nil && *message.ModelName != "" {
		signature = fmt.Sprintf("\n🤖 %s", *message.ModelName)
	}

	fullText := responseText
	if signature != "" {
		fullText += signature
	}

	const maxGraphemes = 300
	graphemeCount := countGraphemes(fullText)

	if graphemeCount <= maxGraphemes {
		return b.sendSinglePost(authClient, auth, message, fullText)
	}

	if signature != "" && countGraphemes(responseText) <= maxGraphemes {
		return b.sendSinglePost(authClient, auth, message, responseText)
	}

	return b.sendThreadedReply(authClient, auth, message, responseText, signature)
}

func (b *Bot) sendSinglePost(authClient *xrpc.Client, auth *atproto.ServerCreateSession_Output, message database.GetReadyToSendMessagesRow, text string) (string, string, error) {
	replyRecord := bsky.FeedPost{
		Text:      text,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Reply: &bsky.FeedPost_ReplyRef{
			Root: &atproto.RepoStrongRef{
				Uri: message.MessageUri,
				Cid: message.MessageCid,
			},
			Parent: &atproto.RepoStrongRef{
				Uri: message.MessageUri,
				Cid: message.MessageCid,
			},
		},
	}

	encodedRecord := &util.LexiconTypeDecoder{Val: &replyRecord}

	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()

	resp, err := atproto.RepoCreateRecord(
		ctx,
		authClient,
		&atproto.RepoCreateRecord_Input{
			Repo:       auth.Did,
			Collection: "app.bsky.feed.post",
			Record:     encodedRecord,
		},
	)

	if err != nil {
		return "", "", err
	}

	return resp.Uri, resp.Cid, nil
}

func (b *Bot) sendThreadedReply(authClient *xrpc.Client, auth *atproto.ServerCreateSession_Output, message database.GetReadyToSendMessagesRow, responseText, signature string) (string, string, error) {
	const maxGraphemes = 280

	chunks := splitTextIntoChunks(responseText, maxGraphemes)

	if signature != "" {
		lastChunk := chunks[len(chunks)-1]
		if countGraphemes(lastChunk+signature) <= 300 {
			chunks[len(chunks)-1] = lastChunk + signature
		}
	}

	rootURI := message.MessageUri
	rootCID := message.MessageCid
	parentURI := message.MessageUri
	parentCID := message.MessageCid

	var firstPostURI, firstPostCID string

	for i, chunk := range chunks {
		replyRecord := bsky.FeedPost{
			Text:      chunk,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Reply: &bsky.FeedPost_ReplyRef{
				Root: &atproto.RepoStrongRef{
					Uri: rootURI,
					Cid: rootCID,
				},
				Parent: &atproto.RepoStrongRef{
					Uri: parentURI,
					Cid: parentCID,
				},
			},
		}

		encodedRecord := &util.LexiconTypeDecoder{Val: &replyRecord}

		ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
		resp, err := atproto.RepoCreateRecord(
			ctx,
			authClient,
			&atproto.RepoCreateRecord_Input{
				Repo:       auth.Did,
				Collection: "app.bsky.feed.post",
				Record:     encodedRecord,
			},
		)
		cancel()

		if err != nil {
			return firstPostURI, firstPostCID, fmt.Errorf("failed to send chunk %d/%d: %w", i+1, len(chunks), err)
		}

		b.logger.Info("Sent reply chunk",
			"message_id", message.ID,
			"chunk", i+1,
			"total", len(chunks),
			"graphemes", countGraphemes(chunk))

		if i == 0 {
			firstPostURI = resp.Uri
			firstPostCID = resp.Cid
		}

		parentURI = resp.Uri
		parentCID = resp.Cid

		if i < len(chunks)-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return firstPostURI, firstPostCID, nil
}

func (b *Bot) finalizeMessage(message database.GetReadyToSendMessagesRow, replyURI, replyCID, status string, errorMessage *string) {
	if histErr := b.insertMessageHistory(message, replyURI, replyCID, status, errorMessage); histErr != nil {
		b.logger.Error("Failed to insert message into history",
			"message_id", message.ID,
			"status", status,
			"error", histErr)
	} else {
		if deleteErr := b.queries.DeleteMessageFromQueue(b.ctx, message.ID); deleteErr != nil {
			b.logger.Error("Failed to delete message from queue after moving to history",
				"message_id", message.ID,
				"error", deleteErr)
		} else {
			b.logger.Info("Finalized message and moved to history",
				"message_id", message.ID,
				"status", status)
		}
	}
}

func (b *Bot) insertMessageHistory(message database.GetReadyToSendMessagesRow, replyURI, replyCID, status string, errorMessage *string) error {
	llmResponse := ""
	if message.LlmResponse != nil {
		llmResponse = *message.LlmResponse
	}

	var replyURIPtr *string
	if replyURI != "" {
		replyURIPtr = &replyURI
	}

	var replyCIDPtr *string
	if replyCID != "" {
		replyCIDPtr = &replyCID
	}

	_, err := b.queries.InsertMessageHistory(b.ctx, database.InsertMessageHistoryParams{
		MessageUri:                message.MessageUri,
		MessageCid:                message.MessageCid,
		AuthorDid:                 message.AuthorDid,
		AuthorHandle:              message.AuthorHandle,
		MessageText:               message.MessageText,
		LlmResponse:               llmResponse,
		ReplyUri:                  replyURIPtr,
		ReplyCid:                  replyCIDPtr,
		Status:                    status,
		RetryCount:                nil,
		ErrorMessage:              errorMessage,
		UsedGoogleSearchGrounding: message.UsedGoogleSearchGrounding,
		ModelName:                 message.ModelName,
		ReceivedAt:                message.CreatedAt,
		ProcessingStartedAt:       message.ProcessingStartedAt,
	})

	return err
}
