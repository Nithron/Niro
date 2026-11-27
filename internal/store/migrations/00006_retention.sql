-- +goose Up

-- Retention (M5): indexes for the jobs that delete old rows, and for the
-- purge of servers Niro left more than 30 days ago (plan §5, decision 28).
CREATE INDEX guilds_left_idx ON guilds (left_at) WHERE left_at IS NOT NULL;
CREATE INDEX scheduled_actions_done_idx ON scheduled_actions (done_at) WHERE done_at IS NOT NULL;
CREATE INDEX guardian_raids_ended_idx ON guardian_raids (ended_at) WHERE ended_at IS NOT NULL;
CREATE INDEX config_audit_created_idx ON config_audit (created_at);

-- +goose Down
DROP INDEX config_audit_created_idx;
DROP INDEX guardian_raids_ended_idx;
DROP INDEX scheduled_actions_done_idx;
DROP INDEX guilds_left_idx;
