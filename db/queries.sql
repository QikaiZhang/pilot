-- name: InsertMessage :exec
INSERT INTO conversation_history (
    message_id,
    user_id,
    session_id,
    role,
    content,
    metadata,
    created_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetMessageByID :one
SELECT
    id,
    message_id,
    user_id,
    session_id,
    role,
    content,
    metadata,
    created_at
FROM conversation_history
WHERE user_id = ?
  AND session_id = ?
  AND message_id = ?;

-- name: ListHistory :many
SELECT
    id,
    message_id,
    user_id,
    session_id,
    role,
    content,
    metadata,
    created_at
FROM conversation_history
WHERE user_id = ?
  AND session_id = ?
ORDER BY created_at ASC, id ASC
LIMIT ?;

-- name: DeleteHistory :exec
DELETE FROM conversation_history
WHERE user_id = ?
  AND session_id = ?;
