-- name: InsertMessageHistory :one
INSERT INTO message_history (
    message_uri,
    message_cid,
    author_did,
    author_handle,
    message_text,
    llm_response,
    reply_uri,
    reply_cid,
    status,
    retry_count,
    error_message,
    used_google_search_grounding,
    model_name,
    received_at,
    processing_started_at,
    completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW()
) RETURNING *;
