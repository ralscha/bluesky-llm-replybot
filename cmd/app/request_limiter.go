package main

import (
	"context"
	"time"
)

type RequestLimiter struct {
	requestsPerMinute int
	tokens            chan struct{}
}

func NewRequestLimiter(requestsPerMinute int) *RequestLimiter {
	if requestsPerMinute <= 0 {
		return &RequestLimiter{}
	}

	limiter := &RequestLimiter{
		requestsPerMinute: requestsPerMinute,
		tokens:            make(chan struct{}, requestsPerMinute),
	}
	for range requestsPerMinute {
		limiter.tokens <- struct{}{}
	}
	go limiter.refill()
	return limiter
}

func (l *RequestLimiter) Wait(ctx context.Context) error {
	if l == nil || l.requestsPerMinute <= 0 {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.tokens:
		return nil
	}
}

func (l *RequestLimiter) refill() {
	interval := time.Minute / time.Duration(l.requestsPerMinute)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case l.tokens <- struct{}{}:
		default:
		}
	}
}
