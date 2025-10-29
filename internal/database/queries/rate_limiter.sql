-- name: GetRateLimiterStats :one
SELECT model_name, requests_this_minute, requests_today, tokens_today, 
       google_search_today,
       last_minute_reset, last_day_reset, consecutive_minute_fails, wait_until_midnight, updated_at
FROM rate_limiter_stats
WHERE model_name = $1;

-- name: UpsertRateLimiterStats :exec
INSERT INTO rate_limiter_stats (
    model_name, 
    requests_this_minute, 
    requests_today, 
    tokens_today, 
    google_search_today,
    last_minute_reset, 
    last_day_reset, 
    consecutive_minute_fails, 
    wait_until_midnight,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, NOW()
)
ON CONFLICT (model_name) 
DO UPDATE SET
    requests_this_minute = EXCLUDED.requests_this_minute,
    requests_today = EXCLUDED.requests_today,
    tokens_today = EXCLUDED.tokens_today,
    google_search_today = EXCLUDED.google_search_today,
    last_minute_reset = EXCLUDED.last_minute_reset,
    last_day_reset = EXCLUDED.last_day_reset,
    consecutive_minute_fails = EXCLUDED.consecutive_minute_fails,
    wait_until_midnight = EXCLUDED.wait_until_midnight,
    updated_at = NOW();
