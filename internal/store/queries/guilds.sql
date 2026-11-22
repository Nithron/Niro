-- name: UpsertGuild :exec
-- Called when the bot sees a guild (startup or join). A guild that was left
-- and re-added gets a fresh joined_at.
INSERT INTO guilds (id, name, icon, owner_id, locale, joined_at, left_at, updated_at)
VALUES ($1, $2, $3, $4, $5, now(), NULL, now())
ON CONFLICT (id) DO UPDATE SET
    name       = EXCLUDED.name,
    icon       = EXCLUDED.icon,
    owner_id   = EXCLUDED.owner_id,
    locale     = EXCLUDED.locale,
    joined_at  = CASE WHEN guilds.left_at IS NOT NULL THEN now() ELSE guilds.joined_at END,
    left_at    = NULL,
    updated_at = now();

-- name: MarkGuildLeft :exec
UPDATE guilds SET left_at = now(), updated_at = now()
WHERE id = $1 AND left_at IS NULL;

-- name: FilterActiveGuildIDs :many
-- Which of the given guilds currently have the bot in them.
SELECT id FROM guilds
WHERE id = ANY (@ids::bigint[]) AND left_at IS NULL;

-- name: IsGuildActive :one
SELECT EXISTS (SELECT 1 FROM guilds WHERE id = $1 AND left_at IS NULL);
