package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	database "github.com/ralscha/bluesky_llm_replybot/internal/database/generated"
)

const (
	ModelFlash     = "gemini-2.5-flash"
	ModelFlashLite = "gemini-2.5-flash-lite"

	FlashRPM          = 10
	FlashRPD          = 250
	FlashTokensPerDay = 250000

	FlashLiteRPM          = 15
	FlashLiteRPD          = 1000
	FlashLiteTokensPerDay = 250000

	SharedGoogleSearchLimit = 500

	MaxConsecutiveMinuteFailures = 3
)

type ModelLimits struct {
	requestsThisMinute     int
	requestsToday          int
	tokensToday            int
	lastMinuteReset        time.Time
	lastDayReset           time.Time
	consecutiveMinuteFails int
	waitUntilMidnight      bool
	rpm                    int
	rpd                    int
	tokensPerDay           int
}

type RateLimiter struct {
	mu                sync.Mutex
	ctx               context.Context
	queries           *database.Queries
	flash             *ModelLimits
	flashLite         *ModelLimits
	googleSearchToday int
	logger            *slog.Logger
}

func NewRateLimiter(ctx context.Context, queries *database.Queries, logger *slog.Logger) *RateLimiter {
	rl := &RateLimiter{
		ctx:     ctx,
		queries: queries,
		logger:  logger,
		flash: &ModelLimits{
			rpm:          FlashRPM,
			rpd:          FlashRPD,
			tokensPerDay: FlashTokensPerDay,
		},
		flashLite: &ModelLimits{
			rpm:          FlashLiteRPM,
			rpd:          FlashLiteRPD,
			tokensPerDay: FlashLiteTokensPerDay,
		},
	}

	if err := rl.loadFromDatabase(); err != nil {
		logger.Error("Failed to load rate limiter stats from database, using defaults", "error", err)

		now := time.Now()
		rl.flash.lastMinuteReset = now
		rl.flash.lastDayReset = getMidnightPacific(now)
		rl.flashLite.lastMinuteReset = now
		rl.flashLite.lastDayReset = getMidnightPacific(now)
	}

	return rl
}

func (rl *RateLimiter) loadFromDatabase() error {
	flashStats, err := rl.queries.GetRateLimiterStats(rl.ctx, ModelFlash)
	if err != nil {
		return fmt.Errorf("failed to load flash stats: %w", err)
	}

	rl.flash.requestsThisMinute = int(flashStats.RequestsThisMinute)
	rl.flash.requestsToday = int(flashStats.RequestsToday)
	rl.flash.tokensToday = int(flashStats.TokensToday)
	rl.flash.lastMinuteReset = flashStats.LastMinuteReset.Time
	rl.flash.lastDayReset = flashStats.LastDayReset.Time
	rl.flash.consecutiveMinuteFails = int(flashStats.ConsecutiveMinuteFails)
	rl.flash.waitUntilMidnight = flashStats.WaitUntilMidnight

	flashLiteStats, err := rl.queries.GetRateLimiterStats(rl.ctx, ModelFlashLite)
	if err != nil {
		return fmt.Errorf("failed to load flash-lite stats: %w", err)
	}

	rl.flashLite.requestsThisMinute = int(flashLiteStats.RequestsThisMinute)
	rl.flashLite.requestsToday = int(flashLiteStats.RequestsToday)
	rl.flashLite.tokensToday = int(flashLiteStats.TokensToday)
	rl.flashLite.lastMinuteReset = flashLiteStats.LastMinuteReset.Time
	rl.flashLite.lastDayReset = flashLiteStats.LastDayReset.Time
	rl.flashLite.consecutiveMinuteFails = int(flashLiteStats.ConsecutiveMinuteFails)
	rl.flashLite.waitUntilMidnight = flashLiteStats.WaitUntilMidnight

	rl.googleSearchToday = max(int(flashStats.GoogleSearchToday), int(flashLiteStats.GoogleSearchToday))

	rl.logger.Info("Loaded rate limiter stats from database",
		"flash_rpm", rl.flash.requestsThisMinute,
		"flash_rpd", rl.flash.requestsToday,
		"flash_lite_rpm", rl.flashLite.requestsThisMinute,
		"flash_lite_rpd", rl.flashLite.requestsToday,
		"shared_google_search", rl.googleSearchToday)

	return nil
}

func (rl *RateLimiter) saveToDatabase() error {
	if err := rl.queries.UpsertRateLimiterStats(rl.ctx, database.UpsertRateLimiterStatsParams{
		ModelName:              ModelFlash,
		RequestsThisMinute:     int32(rl.flash.requestsThisMinute),
		RequestsToday:          int32(rl.flash.requestsToday),
		TokensToday:            int32(rl.flash.tokensToday),
		GoogleSearchToday:      int32(rl.googleSearchToday),
		LastMinuteReset:        pgtype.Timestamptz{Time: rl.flash.lastMinuteReset, Valid: true},
		LastDayReset:           pgtype.Timestamptz{Time: rl.flash.lastDayReset, Valid: true},
		ConsecutiveMinuteFails: int32(rl.flash.consecutiveMinuteFails),
		WaitUntilMidnight:      rl.flash.waitUntilMidnight,
	}); err != nil {
		return fmt.Errorf("failed to save flash stats: %w", err)
	}

	if err := rl.queries.UpsertRateLimiterStats(rl.ctx, database.UpsertRateLimiterStatsParams{
		ModelName:              ModelFlashLite,
		RequestsThisMinute:     int32(rl.flashLite.requestsThisMinute),
		RequestsToday:          int32(rl.flashLite.requestsToday),
		TokensToday:            int32(rl.flashLite.tokensToday),
		GoogleSearchToday:      int32(rl.googleSearchToday),
		LastMinuteReset:        pgtype.Timestamptz{Time: rl.flashLite.lastMinuteReset, Valid: true},
		LastDayReset:           pgtype.Timestamptz{Time: rl.flashLite.lastDayReset, Valid: true},
		ConsecutiveMinuteFails: int32(rl.flashLite.consecutiveMinuteFails),
		WaitUntilMidnight:      rl.flashLite.waitUntilMidnight,
	}); err != nil {
		return fmt.Errorf("failed to save flash-lite stats: %w", err)
	}

	return nil
}

func getMidnightPacific(t time.Time) time.Time {
	pacificLoc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		pacificLoc = time.FixedZone("PST", -8*60*60)
	}

	tPacific := t.In(pacificLoc)
	return time.Date(tPacific.Year(), tPacific.Month(), tPacific.Day(), 0, 0, 0, 0, pacificLoc)
}

func getNextMidnightPacific(t time.Time) time.Time {
	return getMidnightPacific(t).Add(24 * time.Hour)
}

func (rl *RateLimiter) resetIfNeeded() {
	now := time.Now()
	needsSave := false

	if now.Sub(rl.flash.lastMinuteReset) >= time.Minute {
		rl.flash.requestsThisMinute = 0
		rl.flash.lastMinuteReset = now
		needsSave = true

		if rl.flash.consecutiveMinuteFails > 0 && !rl.flash.waitUntilMidnight {
			rl.flash.consecutiveMinuteFails = 0
		}
	}

	if now.Sub(rl.flashLite.lastMinuteReset) >= time.Minute {
		rl.flashLite.requestsThisMinute = 0
		rl.flashLite.lastMinuteReset = now
		needsSave = true

		if rl.flashLite.consecutiveMinuteFails > 0 && !rl.flashLite.waitUntilMidnight {
			rl.flashLite.consecutiveMinuteFails = 0
		}
	}

	currentMidnight := getMidnightPacific(now)

	if currentMidnight.After(rl.flash.lastDayReset) {
		rl.flash.requestsToday = 0
		rl.flash.tokensToday = 0
		rl.flash.lastDayReset = currentMidnight
		rl.flash.consecutiveMinuteFails = 0
		rl.flash.waitUntilMidnight = false
		rl.googleSearchToday = 0
		needsSave = true
		rl.logger.Info("Reset flash model daily limits", "model", ModelFlash)
	}

	if currentMidnight.After(rl.flashLite.lastDayReset) {
		rl.flashLite.requestsToday = 0
		rl.flashLite.tokensToday = 0
		rl.flashLite.lastDayReset = currentMidnight
		rl.flashLite.consecutiveMinuteFails = 0
		rl.flashLite.waitUntilMidnight = false
		rl.googleSearchToday = 0
		needsSave = true
		rl.logger.Info("Reset flash-lite model daily limits", "model", ModelFlashLite)
	}

	if needsSave {
		if err := rl.saveToDatabase(); err != nil {
			rl.logger.Error("Failed to save rate limiter stats after reset", "error", err)
		}
	}
}

func (rl *RateLimiter) CanUseModel(model string) (bool, string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.resetIfNeeded()

	var limits *ModelLimits
	switch model {
	case ModelFlash:
		limits = rl.flash
	case ModelFlashLite:
		limits = rl.flashLite
	default:
		return false, fmt.Sprintf("unknown model: %s", model)
	}

	if limits.waitUntilMidnight {
		nextMidnight := getNextMidnightPacific(time.Now())
		return false, fmt.Sprintf("model %s waiting until midnight Pacific (%s)", model, nextMidnight.Format(time.RFC3339))
	}

	if limits.requestsThisMinute >= limits.rpm {
		return false, fmt.Sprintf("model %s RPM limit reached (%d/%d)", model, limits.requestsThisMinute, limits.rpm)
	}

	if limits.requestsToday >= limits.rpd {
		return false, fmt.Sprintf("model %s RPD limit reached (%d/%d)", model, limits.requestsToday, limits.rpd)
	}

	if limits.tokensToday >= limits.tokensPerDay {
		return false, fmt.Sprintf("model %s token limit reached (%d/%d)", model, limits.tokensToday, limits.tokensPerDay)
	}

	return true, ""
}

func (rl *RateLimiter) RecordRequest(model string, tokensUsed int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.resetIfNeeded()

	var limits *ModelLimits
	switch model {
	case ModelFlash:
		limits = rl.flash
	case ModelFlashLite:
		limits = rl.flashLite
	default:
		return
	}

	limits.requestsThisMinute++
	limits.requestsToday++
	limits.tokensToday += tokensUsed

	limits.consecutiveMinuteFails = 0

	if err := rl.saveToDatabase(); err != nil {
		rl.logger.Error("Failed to save rate limiter stats after recording request", "error", err)
	}
}

func (rl *RateLimiter) RecordGrounding(model string, groundingType string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.resetIfNeeded()

	switch groundingType {
	case "google_search":
		rl.googleSearchToday++
	}

	if err := rl.saveToDatabase(); err != nil {
		rl.logger.Error("Failed to save rate limiter stats after recording grounding", "error", err)
	}
}

func (rl *RateLimiter) CanUseGrounding(model string, groundingType string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.resetIfNeeded()

	switch groundingType {
	case "google_search":
		return rl.googleSearchToday < SharedGoogleSearchLimit
	default:
		return false
	}
}

func (rl *RateLimiter) HandleRateLimitError(model string, err error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.resetIfNeeded()

	var limits *ModelLimits
	switch model {
	case ModelFlash:
		limits = rl.flash
	case ModelFlashLite:
		limits = rl.flashLite
	default:
		return
	}

	if isRateLimitError(err) {
		limits.consecutiveMinuteFails++

		rl.logger.Warn("Rate limit error detected",
			"model", model,
			"consecutive_fails", limits.consecutiveMinuteFails,
			"error", err)

		if limits.consecutiveMinuteFails >= MaxConsecutiveMinuteFailures {
			limits.waitUntilMidnight = true
			nextMidnight := getNextMidnightPacific(time.Now())

			rl.logger.Warn("Model disabled until midnight Pacific",
				"model", model,
				"consecutive_fails", limits.consecutiveMinuteFails,
				"next_midnight", nextMidnight.Format(time.RFC3339))
		}

		if err := rl.saveToDatabase(); err != nil {
			rl.logger.Error("Failed to save rate limiter stats after handling error", "error", err)
		}
	}
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	rateLimitIndicators := []string{
		"rate limit",
		"quota exceeded",
		"too many requests",
		"429",
		"resource exhausted",
		"resource_exhausted",
	}

	for _, indicator := range rateLimitIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}
