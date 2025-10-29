package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/genai"

	database "github.com/ralscha/bluesky_llm_replybot/internal/database/generated"
)

type Bot struct {
	ctx            context.Context
	cancel         context.CancelFunc
	ingestorCtx    context.Context
	ingestorCancel context.CancelFunc
	wg             sync.WaitGroup
	pool           *pgxpool.Pool
	queries        *database.Queries
	genaiClient    *genai.Client
	rateLimiter    *RateLimiter
	botHandle      string
	maxRetries     int
	logger         *slog.Logger
}

func NewBot(pool *pgxpool.Pool, genaiClient *genai.Client, botHandle string, maxRetries int, logger *slog.Logger) *Bot {
	ctx, cancel := context.WithCancel(context.Background())
	ingestorCtx, ingestorCancel := context.WithCancel(ctx)

	queries := database.New(pool)

	return &Bot{
		ctx:            ctx,
		cancel:         cancel,
		ingestorCtx:    ingestorCtx,
		ingestorCancel: ingestorCancel,
		pool:           pool,
		queries:        queries,
		genaiClient:    genaiClient,
		rateLimiter:    NewRateLimiter(ctx, queries, logger),
		botHandle:      botHandle,
		maxRetries:     maxRetries,
		logger:         logger,
	}
}

func (b *Bot) Start(config *Config) {
	b.logger.Info("Starting Bluesky Reply Bot...")

	b.wg.Go(func() {
		b.runIngestor(config)
	})

	b.wg.Go(func() {
		b.runWorker(config)
	})

	b.wg.Go(func() {
		b.runReplySender(config)
	})

	b.wg.Go(func() {
		b.runStaleMessageHandler()
	})

	b.logger.Info("Bot started successfully")
}

func (b *Bot) Stop() {
	b.logger.Info("Shutting down bot...")

	b.logger.Info("Stopping ingestion...")
	b.ingestorCancel()

	b.logger.Info("Waiting for workers to complete", "timeout", "2m")

	done := make(chan struct{})
	go func() {
		b.cancel()
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		b.logger.Info("All workers completed gracefully")
	case <-time.After(2 * time.Minute):
		b.logger.Warn("Shutdown timeout reached, forcing shutdown", "timeout", "2m")
	}

	b.logger.Info("Bot stopped")
}
