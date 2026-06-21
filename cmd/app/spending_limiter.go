package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const microsPerUnit = 1_000_000

type UsagePricing struct {
	InputCachePerMillion float64
	InputMissPerMillion  float64
	OutputPerMillion     float64
}

func (p UsagePricing) Total() float64 {
	return p.InputCachePerMillion + p.InputMissPerMillion + p.OutputPerMillion
}

type SpendingLimiter struct {
	pool             *pgxpool.Pool
	pricing          UsagePricing
	dailyLimitMicros int64
}

type SpendingStatus struct {
	SpentMicros    int64
	ReservedMicros int64
	LimitMicros    int64
	ResetAt        time.Time
}

type SpendingReservation struct {
	UsageDate string
	Micros    int64
}

func (r SpendingReservation) IsValid() bool {
	return r.UsageDate != "" && r.Micros > 0
}

func NewSpendingLimiter(pool *pgxpool.Pool, pricing UsagePricing, dailyLimit float64) *SpendingLimiter {
	return &SpendingLimiter{
		pool:             pool,
		pricing:          pricing,
		dailyLimitMicros: currencyToMicros(dailyLimit),
	}
}

func (s *SpendingLimiter) IsEnabled() bool {
	return s != nil && s.dailyLimitMicros > 0
}

func (s *SpendingLimiter) CurrentStatus(ctx context.Context, now time.Time) (SpendingStatus, error) {
	status := SpendingStatus{LimitMicros: s.dailyLimitMicros, ResetAt: nextDailyResetUTC(now)}
	if !s.IsEnabled() {
		return status, nil
	}

	usageDate := usageDate(now)
	err := s.pool.QueryRow(ctx, `
SELECT estimated_spend_micros, reserved_spend_micros
FROM llm_usage_daily
WHERE usage_date = $1`, usageDate).Scan(&status.SpentMicros, &status.ReservedMicros)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return status, nil
		}
		return status, fmt.Errorf("failed to load LLM daily usage: %w", err)
	}

	return status, nil
}

func (s *SpendingLimiter) LimitReached(ctx context.Context, now time.Time) (SpendingStatus, bool, error) {
	status, err := s.CurrentStatus(ctx, now)
	if err != nil || !s.IsEnabled() {
		return status, false, err
	}
	return status, status.CommittedAndReservedMicros() >= status.LimitMicros, nil
}

func (s *SpendingLimiter) Reserve(ctx context.Context, now time.Time, prompt string, maxOutputTokens int) (SpendingReservation, SpendingStatus, bool, error) {
	status := SpendingStatus{LimitMicros: s.dailyLimitMicros, ResetAt: nextDailyResetUTC(now)}
	if !s.IsEnabled() {
		return SpendingReservation{}, status, true, nil
	}

	inputTokens := estimateTokens(prompt)
	reservationMicros := s.calculateSpendMicros(0, inputTokens, maxOutputTokens)
	date := usageDate(now)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SpendingReservation{}, status, false, fmt.Errorf("failed to begin spending reservation: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
INSERT INTO llm_usage_daily (usage_date)
VALUES ($1)
ON CONFLICT (usage_date) DO NOTHING`, date); err != nil {
		return SpendingReservation{}, status, false, fmt.Errorf("failed to initialize LLM daily usage: %w", err)
	}

	if err := tx.QueryRow(ctx, `
SELECT estimated_spend_micros, reserved_spend_micros
FROM llm_usage_daily
WHERE usage_date = $1
FOR UPDATE`, date).Scan(&status.SpentMicros, &status.ReservedMicros); err != nil {
		return SpendingReservation{}, status, false, fmt.Errorf("failed to lock LLM daily usage: %w", err)
	}

	if status.CommittedAndReservedMicros()+reservationMicros > status.LimitMicros {
		return SpendingReservation{}, status, false, nil
	}

	if _, err := tx.Exec(ctx, `
UPDATE llm_usage_daily
SET reserved_spend_micros = reserved_spend_micros + $2,
    updated_at = NOW()
WHERE usage_date = $1`, date, reservationMicros); err != nil {
		return SpendingReservation{}, status, false, fmt.Errorf("failed to reserve LLM spend: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return SpendingReservation{}, status, false, fmt.Errorf("failed to commit spending reservation: %w", err)
	}

	status.ReservedMicros += reservationMicros
	return SpendingReservation{UsageDate: date, Micros: reservationMicros}, status, true, nil
}

func (s *SpendingLimiter) FinalizeReservation(ctx context.Context, reservation SpendingReservation, usage *schema.TokenUsage) (SpendingStatus, error) {
	status := SpendingStatus{LimitMicros: s.dailyLimitMicros, ResetAt: nextDailyResetUTC(time.Now())}
	if !s.IsEnabled() || !reservation.IsValid() || usage == nil {
		return status, nil
	}

	cachedInput := max(usage.PromptTokenDetails.CachedTokens, 0)
	missInput := max(usage.PromptTokens-cachedInput, 0)
	output := max(usage.CompletionTokens, 0)
	actualMicros := s.calculateSpendMicros(cachedInput, missInput, output)

	err := s.pool.QueryRow(ctx, `
UPDATE llm_usage_daily
SET
    input_cache_tokens = input_cache_tokens + $2,
    input_miss_tokens = input_miss_tokens + $3,
    output_tokens = output_tokens + $4,
    estimated_spend_micros = estimated_spend_micros + $5,
    reserved_spend_micros = GREATEST(reserved_spend_micros - $6, 0),
    updated_at = NOW()
WHERE usage_date = $1
RETURNING estimated_spend_micros, reserved_spend_micros`, reservation.UsageDate, cachedInput, missInput, output, actualMicros, reservation.Micros).Scan(&status.SpentMicros, &status.ReservedMicros)
	if err != nil {
		return status, fmt.Errorf("failed to finalize LLM usage reservation: %w", err)
	}

	return status, nil
}

func (s *SpendingLimiter) EstimateUsage(prompt, response string) *schema.TokenUsage {
	promptTokens := estimateTokens(prompt)
	completionTokens := estimateTokens(response)
	return &schema.TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
}

func (s *SpendingLimiter) calculateSpendMicros(inputCacheTokens, inputMissTokens, outputTokens int) int64 {
	spend := (float64(inputCacheTokens)*s.pricing.InputCachePerMillion +
		float64(inputMissTokens)*s.pricing.InputMissPerMillion +
		float64(outputTokens)*s.pricing.OutputPerMillion) / 1_000_000
	return currencyToMicros(spend)
}

func (s SpendingStatus) CommittedAndReservedMicros() int64 {
	return s.SpentMicros + s.ReservedMicros
}

func currencyToMicros(amount float64) int64 {
	if amount <= 0 {
		return 0
	}
	return int64(math.Ceil(amount * microsPerUnit))
}

func nextDailyResetUTC(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day()+1, 0, 0, 0, 0, time.UTC)
}

func usageDate(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func hoursUntil(t time.Time, now time.Time) int {
	d := t.Sub(now.UTC())
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(d.Hours()))
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return max(len([]byte(text)), 1)
}
