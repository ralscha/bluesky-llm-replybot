-- +goose Up
-- +goose StatementBegin
CREATE TABLE llm_usage_daily (
    usage_date DATE PRIMARY KEY,
    input_cache_tokens BIGINT NOT NULL DEFAULT 0,
    input_miss_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    estimated_spend_micros BIGINT NOT NULL DEFAULT 0,
    reserved_spend_micros BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS llm_usage_daily;
-- +goose StatementEnd
