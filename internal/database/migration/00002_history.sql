-- +goose Up
-- +goose StatementBegin
CREATE TABLE message_history (
    id BIGSERIAL PRIMARY KEY,
    message_uri TEXT NOT NULL,
    message_cid TEXT NOT NULL,
    author_did TEXT NOT NULL,
    author_handle TEXT NOT NULL,
    message_text TEXT NOT NULL,
    llm_response TEXT NOT NULL,
    reply_uri TEXT,
    reply_cid TEXT,
    status VARCHAR(20) NOT NULL,
    retry_count INT DEFAULT 0,
    error_message TEXT,
    used_google_search_grounding BOOLEAN,
    model_name TEXT,
    received_at TIMESTAMPTZ NOT NULL,
    processing_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS message_history;
-- +goose StatementEnd
