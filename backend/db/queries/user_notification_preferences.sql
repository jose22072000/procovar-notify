-- name: GetUserPreference :one
SELECT * FROM user_notification_preferences
WHERE application_id = $1 AND user_id = $2;

-- name: UpsertUserPreference :one
INSERT INTO user_notification_preferences
    (application_id, user_id, email_enabled, push_enabled, sms_enabled, in_app_enabled)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (application_id, user_id) DO UPDATE SET
    email_enabled  = EXCLUDED.email_enabled,
    push_enabled   = EXCLUDED.push_enabled,
    sms_enabled    = EXCLUDED.sms_enabled,
    in_app_enabled = EXCLUDED.in_app_enabled
RETURNING *;
