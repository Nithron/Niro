-- +goose Up

-- Per-guild case counter so case numbers are 1, 2, 3… within each server.
CREATE TABLE guild_case_counters (
    guild_id  bigint  PRIMARY KEY REFERENCES guilds (id) ON DELETE CASCADE,
    last_case integer NOT NULL DEFAULT 0
);

-- Moderation cases. Names are snapshots at the time of the action so the
-- history stays readable after users rename or leave. Deleting a case is a
-- soft delete (kept for the audit trail, hidden from lists).
CREATE TABLE mod_cases (
    guild_id         bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    case_number      integer     NOT NULL,
    action           text        NOT NULL CHECK (action IN ('warn', 'timeout', 'untimeout', 'kick', 'ban', 'unban', 'note', 'purge')),
    target_id        bigint,
    target_name      text        NOT NULL DEFAULT '',
    moderator_id     bigint      NOT NULL,
    moderator_name   text        NOT NULL DEFAULT '',
    reason           text        NOT NULL DEFAULT '',
    duration_seconds integer,
    expires_at       timestamptz,
    source           text        NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'automod', 'guardian', 'dashboard', 'system')),
    details          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    dm_sent          boolean     NOT NULL DEFAULT false,
    log_channel_id   bigint,
    log_message_id   bigint,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz,
    deleted_by       bigint,
    PRIMARY KEY (guild_id, case_number)
);

CREATE INDEX mod_cases_target_idx ON mod_cases (guild_id, target_id, case_number DESC) WHERE deleted_at IS NULL;
CREATE INDEX mod_cases_recent_idx ON mod_cases (guild_id, case_number DESC) WHERE deleted_at IS NULL;

-- Actions to run later (e.g. lifting a temporary ban). Claimed with
-- FOR UPDATE SKIP LOCKED so several bot processes can share the work.
CREATE TABLE scheduled_actions (
    id          bigserial   PRIMARY KEY,
    guild_id    bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    kind        text        NOT NULL CHECK (kind IN ('unban')),
    target_id   bigint      NOT NULL,
    case_number integer,
    run_at      timestamptz NOT NULL,
    attempts    integer     NOT NULL DEFAULT 0,
    last_error  text,
    done_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX scheduled_actions_due_idx ON scheduled_actions (run_at) WHERE done_at IS NULL;

-- +goose Down
DROP TABLE scheduled_actions;
DROP TABLE mod_cases;
DROP TABLE guild_case_counters;
