-- name: NextCaseNumber :one
INSERT INTO guild_case_counters (guild_id, last_case) VALUES ($1, 1)
ON CONFLICT (guild_id) DO UPDATE SET last_case = guild_case_counters.last_case + 1
RETURNING last_case;

-- name: InsertCase :one
INSERT INTO mod_cases (
    guild_id, case_number, action, target_id, target_name, moderator_id, moderator_name,
    reason, duration_seconds, expires_at, source, details, dm_sent
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetCase :one
SELECT * FROM mod_cases
WHERE guild_id = $1 AND case_number = $2 AND deleted_at IS NULL;

-- name: ListCases :many
SELECT * FROM mod_cases
WHERE guild_id = @guild_id
  AND deleted_at IS NULL
  AND (sqlc.narg('target_id')::bigint IS NULL OR target_id = sqlc.narg('target_id'))
  AND (sqlc.narg('moderator_id')::bigint IS NULL OR moderator_id = sqlc.narg('moderator_id'))
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('before')::integer IS NULL OR case_number < sqlc.narg('before'))
ORDER BY case_number DESC
LIMIT @lim;

-- name: CountCases :one
SELECT count(*) FROM mod_cases
WHERE guild_id = @guild_id
  AND deleted_at IS NULL
  AND (sqlc.narg('target_id')::bigint IS NULL OR target_id = sqlc.narg('target_id'))
  AND (sqlc.narg('moderator_id')::bigint IS NULL OR moderator_id = sqlc.narg('moderator_id'))
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'));

-- name: UpdateCaseReason :one
UPDATE mod_cases SET reason = $3, updated_at = now()
WHERE guild_id = $1 AND case_number = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCase :one
UPDATE mod_cases SET deleted_at = now(), deleted_by = $3, updated_at = now()
WHERE guild_id = $1 AND case_number = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SetCaseLogMessage :exec
UPDATE mod_cases SET log_channel_id = $3, log_message_id = $4
WHERE guild_id = $1 AND case_number = $2;

-- name: InsertScheduledAction :exec
INSERT INTO scheduled_actions (guild_id, kind, target_id, case_number, run_at)
VALUES ($1, $2, $3, $4, $5);

-- name: CancelScheduledActions :exec
-- Cancels pending actions of a kind for a member (e.g. a manual unban
-- supersedes the pending temp-ban expiry).
UPDATE scheduled_actions SET done_at = now(), last_error = 'cancelled'
WHERE guild_id = $1 AND kind = $2 AND target_id = $3 AND done_at IS NULL;

-- name: ClaimDueActions :many
-- Claims due actions by pushing run_at forward (a lease); if the process dies
-- mid-way they become due again after the lease.
UPDATE scheduled_actions SET run_at = now() + interval '5 minutes', attempts = attempts + 1
WHERE id IN (
    SELECT id FROM scheduled_actions
    WHERE done_at IS NULL AND run_at <= now()
    ORDER BY run_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, guild_id, kind, target_id, case_number, attempts;

-- name: CompleteScheduledAction :exec
UPDATE scheduled_actions SET done_at = now(), last_error = sqlc.narg('last_error')
WHERE id = $1;

-- name: FailScheduledAction :exec
UPDATE scheduled_actions SET last_error = $2, run_at = now() + make_interval(mins => $3::integer)
WHERE id = $1;
