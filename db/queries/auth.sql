-- name: GetProjectIDByTokenHash :one
UPDATE ingest_tokens
SET last_used_at = now()
WHERE token_hash = $1
  AND enabled = true
  AND (expires_at IS NULL OR expires_at > now())
RETURNING project_id;
