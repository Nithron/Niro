-- name: InsertRolePanel :one
INSERT INTO role_panels (guild_id, name, channel_id, message, style, mode, roles, max_roles, required_role_id, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateRolePanel :one
UPDATE role_panels SET name = $3, channel_id = $4, message = $5, style = $6, mode = $7, roles = $8,
    max_roles = $9, required_role_id = $10, updated_by = $11, updated_at = now()
WHERE guild_id = $1 AND id = $2
RETURNING *;

-- name: GetRolePanel :one
SELECT * FROM role_panels WHERE guild_id = $1 AND id = $2;

-- name: ListRolePanels :many
SELECT * FROM role_panels WHERE guild_id = $1 ORDER BY id;

-- name: CountRolePanels :one
SELECT count(*) FROM role_panels WHERE guild_id = $1;

-- name: DeleteRolePanel :exec
DELETE FROM role_panels WHERE guild_id = $1 AND id = $2;

-- name: SetRolePanelMessage :exec
UPDATE role_panels SET channel_id = $3, message_id = $4 WHERE guild_id = $1 AND id = $2;
