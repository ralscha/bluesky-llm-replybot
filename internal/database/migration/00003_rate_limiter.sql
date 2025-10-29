-- +goose Up
-- +goose StatementBegin
CREATE TABLE rate_limiter_stats (
    model_name VARCHAR(50) PRIMARY KEY,
    requests_this_minute INT NOT NULL DEFAULT 0,
    requests_today INT NOT NULL DEFAULT 0,
    tokens_today INT NOT NULL DEFAULT 0,
    google_search_today INT NOT NULL DEFAULT 0,
    last_minute_reset TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_day_reset TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    consecutive_minute_fails INT NOT NULL DEFAULT 0,
    wait_until_midnight BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO rate_limiter_stats (model_name, last_minute_reset, last_day_reset) 
VALUES 
    ('gemini-2.5-flash', NOW(), NOW()),
    ('gemini-2.5-flash-lite', NOW(), NOW());

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rate_limiter_stats;
-- +goose StatementEnd
