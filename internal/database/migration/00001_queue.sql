-- +goose Up
-- +goose StatementBegin
CREATE TABLE message_queue (
    id BIGSERIAL PRIMARY KEY,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    message_uri TEXT NOT NULL UNIQUE,
    message_cid TEXT NOT NULL,
    author_did TEXT NOT NULL,
    author_handle TEXT NOT NULL,
    message_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    retry_count INT DEFAULT 0,
    error_message TEXT,
    llm_response TEXT,
    used_google_search_grounding BOOLEAN,
    model_name TEXT
);

-- Index for efficient queue processing
CREATE INDEX idx_status_created ON message_queue (status, created_at);

-- Index for processing queries
CREATE INDEX idx_processing ON message_queue (processing_started_at) 
    WHERE status = 'processing';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS message_queue;
-- +goose StatementEnd