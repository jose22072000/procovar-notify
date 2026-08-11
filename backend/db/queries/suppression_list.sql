-- name: IsSuppressed :one
SELECT EXISTS (
    SELECT 1 FROM suppression_list
    WHERE application_id = $1 AND channel = $2 AND recipient = $3
);

-- name: ListSuppressions :many
SELECT * FROM suppression_list
WHERE application_id = $1 ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: AddSuppression :one
INSERT INTO suppression_list (application_id, channel, recipient, reason)
VALUES ($1, $2, $3, $4)
ON CONFLICT (application_id, channel, recipient) DO UPDATE SET reason = EXCLUDED.reason
RETURNING *;

-- name: DeleteSuppression :exec
DELETE FROM suppression_list WHERE id = $1 AND application_id = $2;

-- name: DeleteSuppressionByRecipient :exec
DELETE FROM suppression_list WHERE application_id = $1 AND channel = $2 AND recipient = $3;
