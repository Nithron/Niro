-- Retention jobs (M5). Every guild table cascades from guilds, so deleting a
-- guild row deletes all of that server's data.

-- name: PurgeLeftGuilds :execrows
-- Servers Niro left before the cutoff (the 30-day grace period is over).
DELETE FROM guilds WHERE left_at IS NOT NULL AND left_at < $1;

-- name: CountMissingGuilds :one
-- Servers recorded as active that the gateway did not report: Niro was
-- removed while it was offline, so no leave event ever came.
SELECT count(*) FROM guilds WHERE left_at IS NULL AND NOT (id = ANY (@present::bigint[]));

-- name: MarkMissingGuildsLeft :execrows
UPDATE guilds SET left_at = now(), updated_at = now()
WHERE left_at IS NULL AND NOT (id = ANY (@present::bigint[]));

-- name: CountActiveGuilds :one
SELECT count(*) FROM guilds WHERE left_at IS NULL;

-- name: DeleteGuild :execrows
-- A server owner asked for their server's data to be deleted now.
DELETE FROM guilds WHERE id = $1;

-- name: PurgeDoneScheduledActions :execrows
DELETE FROM scheduled_actions WHERE done_at IS NOT NULL AND done_at < $1;

-- name: PurgeEndedRaids :execrows
DELETE FROM guardian_raids WHERE ended_at IS NOT NULL AND ended_at < $1;

-- name: PurgeConfigAudit :execrows
DELETE FROM config_audit WHERE created_at < $1;
