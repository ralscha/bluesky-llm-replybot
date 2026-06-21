-- name: InsertMessage :one
INSERT INTO message_queue (
    message_uri,
    message_cid,
    author_did,
    author_handle,
    message_text
) VALUES (
    $1, $2, $3, $4, $5
)
ON CONFLICT (message_uri) DO NOTHING
RETURNING id;

-- name: ClaimNextMessage :one
UPDATE message_queue
SET
    status = 'processing',
    processing_started_at = NOW()
WHERE id = (
    SELECT id FROM message_queue
    WHERE status = 'pending'
      AND (deferred_until IS NULL OR deferred_until <= NOW())
    ORDER BY created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, message_uri, message_cid, author_did, author_handle, message_text, retry_count;

-- name: UpdateMessageWithLLMResponse :exec
UPDATE message_queue
SET
    status = 'ready_to_send',
    llm_response = $2,
    model_name = $3,
    retry_count = 0
WHERE id = $1;

-- name: UpdateMessageFailed :exec
UPDATE message_queue
SET
    status = CASE
        WHEN retry_count + 1 < $2 THEN 'pending'
        ELSE 'failed'
    END,
    retry_count = retry_count + 1,
    processing_started_at = NULL
WHERE id = $1;

-- name: GetReadyToSendMessages :many
SELECT id, message_uri, message_cid, author_did, author_handle, message_text, llm_response,
       model_name, created_at, processing_started_at, retry_count, status, deferred_until, spending_notice_sent
FROM message_queue
WHERE status = 'ready_to_send'
ORDER BY created_at ASC
LIMIT $1;

-- name: GetStaleProcessingMessages :many
SELECT id, message_uri, message_cid, author_did, author_handle, message_text, retry_count
FROM message_queue
WHERE status = 'processing'
  AND processing_started_at < NOW() - INTERVAL '5 minutes'
ORDER BY created_at ASC;

-- name: ResetStaleMessage :exec
UPDATE message_queue
SET
    status = 'pending',
    processing_started_at = NULL,
    retry_count = retry_count + 1
WHERE id = $1;

-- name: DeleteMessageFromQueue :exec
DELETE FROM message_queue
WHERE id = $1;

-- name: UpdateReadyToSendMessageFailed :exec
UPDATE message_queue
SET
    status = CASE
        WHEN retry_count + 1 < $2 THEN 'ready_to_send'
        ELSE 'failed'
    END,
    retry_count = retry_count + 1
WHERE id = $1;

-- name: UpdateMessageDeferredWithNotice :exec
UPDATE message_queue
SET
    status = 'ready_to_send',
    llm_response = $2,
    model_name = NULL,
    processing_started_at = NULL,
    deferred_until = $3,
    spending_notice_sent = TRUE
WHERE id = $1;

-- name: MarkDeferredNoticeSent :exec
UPDATE message_queue
SET
    status = 'pending',
    llm_response = NULL,
    model_name = NULL,
    processing_started_at = NULL
WHERE id = $1;
