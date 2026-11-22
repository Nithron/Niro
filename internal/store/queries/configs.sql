-- name: ListModuleStates :many
SELECT module, enabled, version, updated_at
FROM guild_configs
WHERE guild_id = $1;

-- name: GetModuleEnabledForUpdate :one
SELECT enabled FROM guild_configs
WHERE guild_id = $1 AND module = $2
FOR UPDATE;

-- name: UpsertModuleEnabled :one
INSERT INTO guild_configs (guild_id, module, enabled, updated_by, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (guild_id, module) DO UPDATE SET
    enabled    = EXCLUDED.enabled,
    version    = guild_configs.version + 1,
    updated_by = EXCLUDED.updated_by,
    updated_at = now()
RETURNING module, enabled, version, updated_at;

-- name: InsertConfigAudit :exec
INSERT INTO config_audit (guild_id, module, actor_id, source, change)
VALUES ($1, $2, $3, $4, $5);

-- name: ListConfigAudit :many
SELECT id, module, actor_id, source, change, created_at
FROM config_audit
WHERE guild_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;
