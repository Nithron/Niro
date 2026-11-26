-- name: InsertTicketPanel :one
INSERT INTO ticket_panels (guild_id, name, channel_id, message, categories, updated_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateTicketPanel :one
UPDATE ticket_panels SET name = $3, channel_id = $4, message = $5, categories = $6, updated_by = $7, updated_at = now()
WHERE guild_id = $1 AND id = $2
RETURNING *;

-- name: GetTicketPanel :one
SELECT * FROM ticket_panels WHERE guild_id = $1 AND id = $2;

-- name: ListTicketPanels :many
SELECT * FROM ticket_panels WHERE guild_id = $1 ORDER BY id;

-- name: CountTicketPanels :one
SELECT count(*) FROM ticket_panels WHERE guild_id = $1;

-- name: DeleteTicketPanel :exec
DELETE FROM ticket_panels WHERE guild_id = $1 AND id = $2;

-- name: SetTicketPanelMessage :exec
UPDATE ticket_panels SET channel_id = $3, message_id = $4 WHERE guild_id = $1 AND id = $2;

-- name: NextTicketNumber :one
INSERT INTO guild_ticket_counters (guild_id, last_ticket) VALUES ($1, 1)
ON CONFLICT (guild_id) DO UPDATE SET last_ticket = guild_ticket_counters.last_ticket + 1
RETURNING last_ticket;

-- name: InsertTicket :one
INSERT INTO tickets (guild_id, number, panel_id, category_id, category_label, mode, opener_id, opener_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: DeleteTicket :exec
DELETE FROM tickets WHERE guild_id = $1 AND id = $2;

-- name: SetTicketChannel :exec
UPDATE tickets SET channel_id = $3 WHERE guild_id = $1 AND id = $2;

-- name: GetTicket :one
SELECT sqlc.embed(t), (tr.ticket_id IS NOT NULL)::boolean AS has_transcript
FROM tickets t LEFT JOIN ticket_transcripts tr ON tr.ticket_id = t.id
WHERE t.guild_id = $1 AND t.id = $2;

-- name: GetTicketByNumber :one
SELECT sqlc.embed(t), (tr.ticket_id IS NOT NULL)::boolean AS has_transcript
FROM tickets t LEFT JOIN ticket_transcripts tr ON tr.ticket_id = t.id
WHERE t.guild_id = $1 AND t.number = $2;

-- name: GetTicketByChannel :one
SELECT sqlc.embed(t), (tr.ticket_id IS NOT NULL)::boolean AS has_transcript
FROM tickets t LEFT JOIN ticket_transcripts tr ON tr.ticket_id = t.id
WHERE t.guild_id = $1 AND t.channel_id = $2;

-- name: ListTickets :many
SELECT sqlc.embed(t), (tr.ticket_id IS NOT NULL)::boolean AS has_transcript
FROM tickets t LEFT JOIN ticket_transcripts tr ON tr.ticket_id = t.id
WHERE t.guild_id = @guild_id
  AND (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status'))
  AND (sqlc.narg('before')::integer IS NULL OR t.number < sqlc.narg('before'))
ORDER BY t.number DESC
LIMIT @lim;

-- name: CountOpenTickets :one
SELECT count(*) FROM tickets
WHERE guild_id = $1 AND opener_id = $2 AND panel_id = $3 AND category_id = $4 AND status <> 'closed';

-- name: ClaimTicket :one
UPDATE tickets SET status = 'claimed', claimed_by = $3, claimed_by_name = $4
WHERE guild_id = $1 AND id = $2 AND status = 'open'
RETURNING *;

-- name: CloseTicket :one
UPDATE tickets SET status = 'closed', closed_by = $3, closed_by_name = $4, close_reason = $5, closed_at = now()
WHERE guild_id = $1 AND id = $2 AND status IN ('open', 'claimed')
RETURNING *;

-- name: ReopenTicket :one
UPDATE tickets SET status = CASE WHEN claimed_by IS NULL THEN 'open' ELSE 'claimed' END, closed_at = NULL
WHERE guild_id = $1 AND id = $2 AND status = 'closed'
RETURNING *;

-- name: UpsertTranscript :exec
INSERT INTO ticket_transcripts (ticket_id, guild_id, html_gzip, message_count, expires_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (ticket_id) DO UPDATE SET html_gzip = EXCLUDED.html_gzip, message_count = EXCLUDED.message_count,
    expires_at = EXCLUDED.expires_at, created_at = now();

-- name: GetTranscript :one
SELECT * FROM ticket_transcripts WHERE guild_id = $1 AND ticket_id = $2 AND expires_at > now();

-- name: PurgeTranscripts :execrows
DELETE FROM ticket_transcripts WHERE expires_at < $1;
