-- name: InsertGuardianEvent :one
INSERT INTO guardian_events (guild_id, user_id, user_name, kind, signals, score, response, evidence, case_number, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetGuardianEvent :one
SELECT * FROM guardian_events WHERE guild_id = $1 AND id = $2;

-- name: ListGuardianEvents :many
SELECT * FROM guardian_events
WHERE guild_id = @guild_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('user_id')::bigint IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('before')::bigint IS NULL OR id < sqlc.narg('before'))
ORDER BY id DESC
LIMIT @lim;

-- name: SetGuardianEventAlert :exec
UPDATE guardian_events SET alert_channel_id = $3, alert_message_id = $4
WHERE guild_id = $1 AND id = $2;

-- name: ResolveGuardianEvent :one
-- Only open or kept events can be resolved; a second click finds no row.
UPDATE guardian_events
SET status = $3, resolved_by = $4, resolved_by_name = $5, resolved_at = now()
WHERE guild_id = $1 AND id = $2 AND status IN ('open', 'kept')
RETURNING *;

-- name: SetGuardianEventStatus :one
UPDATE guardian_events
SET status = $3, resolved_by = $4, resolved_by_name = $5, resolved_at = now()
WHERE guild_id = $1 AND id = $2
RETURNING *;

-- name: PurgeGuardianEvents :execrows
-- Evidence older than the cutoff (decision 28), except events a moderator
-- chose to keep and ones still in effect: an open event holds a quarantine
-- waiting for a decision, an active one a raid or lockdown. They go at the
-- first purge after they are resolved.
DELETE FROM guardian_events WHERE created_at < $1 AND status NOT IN ('open', 'kept', 'active');

-- name: UpsertQuarantine :exec
INSERT INTO guardian_quarantines (guild_id, user_id, event_id, mode, saved_role_ids, role_id, until, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (guild_id, user_id) DO UPDATE SET
    -- A second quarantine keeps the roles saved by the first one.
    event_id = EXCLUDED.event_id, mode = EXCLUDED.mode,
    saved_role_ids = CASE WHEN guardian_quarantines.mode = 'role' THEN guardian_quarantines.saved_role_ids ELSE EXCLUDED.saved_role_ids END,
    role_id = EXCLUDED.role_id, until = EXCLUDED.until;

-- name: GetQuarantine :one
SELECT * FROM guardian_quarantines WHERE guild_id = $1 AND user_id = $2;

-- name: DeleteQuarantine :execrows
DELETE FROM guardian_quarantines WHERE guild_id = $1 AND user_id = $2;

-- name: InsertRaid :one
INSERT INTO guardian_raids (guild_id, started_at, ends_at, manual, started_by, started_name, joins)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (guild_id) WHERE ended_at IS NULL DO NOTHING
RETURNING *;

-- name: UpdateRaid :exec
UPDATE guardian_raids SET event_id = $3, state = $4 WHERE guild_id = $1 AND id = $2;

-- name: GetActiveRaid :one
SELECT * FROM guardian_raids WHERE guild_id = $1 AND ended_at IS NULL;

-- name: EndActiveRaid :one
UPDATE guardian_raids SET ended_at = now(), ended_by = $2
WHERE guild_id = $1 AND ended_at IS NULL
RETURNING *;

-- name: DueRaids :many
SELECT * FROM guardian_raids WHERE ended_at IS NULL AND ends_at <= $1 ORDER BY ends_at LIMIT 50;

-- name: UpsertLockdown :exec
-- Locking an already locked channel keeps the original saved state.
INSERT INTO channel_lockdowns (guild_id, channel_id, had_overwrite, allow_bits, deny_bits, locked_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (guild_id, channel_id) DO NOTHING;

-- name: ListLockdowns :many
SELECT * FROM channel_lockdowns WHERE guild_id = $1 ORDER BY created_at;

-- name: DeleteLockdowns :exec
DELETE FROM channel_lockdowns WHERE guild_id = $1 AND channel_id = ANY(@channel_ids::bigint[]);

-- name: DeleteFeedDomains :exec
DELETE FROM scam_domains WHERE source = $1;

-- name: InsertFeedDomains :copyfrom
INSERT INTO scam_domains (domain, source) VALUES ($1, $2);

-- name: UpsertFeed :exec
INSERT INTO scam_feeds (source, domains, updated_at) VALUES ($1, $2, $3)
ON CONFLICT (source) DO UPDATE SET domains = EXCLUDED.domains, updated_at = EXCLUDED.updated_at;

-- name: GetFeedUpdatedAt :one
SELECT updated_at FROM scam_feeds WHERE source = $1;

-- name: ListScamDomains :many
SELECT DISTINCT domain FROM scam_domains;
