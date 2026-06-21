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
    retry_count INT NOT NULL DEFAULT 0,
    llm_response TEXT,
    model_name TEXT,
    deferred_until TIMESTAMPTZ,
    spending_notice_sent BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_status_created ON message_queue (status, created_at);
CREATE INDEX idx_processing ON message_queue (processing_started_at)
    WHERE status = 'processing';
CREATE INDEX idx_message_queue_deferred_until ON message_queue (deferred_until)
    WHERE deferred_until IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS message_queue;
-- +goose StatementEnd
