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
    used_google_search_grounding = $3,
    model_name = $4,
    retry_count = 0,
    completed_at = NOW()
WHERE id = $1;

-- name: UpdateMessageFailed :exec
UPDATE message_queue 
SET 
    status = CASE 
        WHEN retry_count + 1 < $2 THEN 'pending'
        ELSE 'failed'
    END,
    retry_count = retry_count + 1,
    error_message = $3,
    processing_started_at = NULL,
    completed_at = CASE 
        WHEN retry_count + 1 >= $2 THEN NOW()
        ELSE NULL
    END
WHERE id = $1;

-- name: GetReadyToSendMessages :many
SELECT id, message_uri, message_cid, author_did, author_handle, message_text, llm_response, 
       used_google_search_grounding, model_name, created_at, processing_started_at
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
