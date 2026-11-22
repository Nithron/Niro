-- +goose Up

-- Guardian records quarantines and releases as moderation cases.
ALTER TABLE mod_cases DROP CONSTRAINT mod_cases_action_check;
ALTER TABLE mod_cases ADD CONSTRAINT mod_cases_action_check
    CHECK (action IN ('warn', 'timeout', 'untimeout', 'kick', 'ban', 'unban', 'note', 'purge', 'quarantine', 'unquarantine'));

-- Detections and what happened next. Evidence is a summary (hashes, channel
-- IDs, counts, domains), never message text. Kept 90 days (plan §5).
CREATE TABLE guardian_events (
    id                bigserial   PRIMARY KEY,
    guild_id          bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    user_id           bigint,
    user_name         text        NOT NULL DEFAULT '',
    kind              text        NOT NULL CHECK (kind IN ('compromised', 'scam', 'raid', 'account_age')),
    signals           jsonb       NOT NULL DEFAULT '[]'::jsonb,
    score             integer     NOT NULL DEFAULT 0,
    response          text        NOT NULL CHECK (response IN ('alert', 'delete', 'quarantine', 'kick', 'raid_mode')),
    evidence          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    case_number       integer,
    status            text        NOT NULL CHECK (status IN ('open', 'kept', 'restored', 'banned', 'info', 'active', 'ended')),
    resolved_by       bigint,
    resolved_by_name  text        NOT NULL DEFAULT '',
    resolved_at       timestamptz,
    alert_channel_id  bigint,
    alert_message_id  bigint,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX guardian_events_guild_idx ON guardian_events (guild_id, id DESC);
CREATE INDEX guardian_events_status_idx ON guardian_events (guild_id, status, id DESC);
CREATE INDEX guardian_events_created_idx ON guardian_events (created_at);

-- Active quarantines, with what is needed to undo them.
CREATE TABLE guardian_quarantines (
    guild_id        bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    user_id         bigint      NOT NULL,
    event_id        bigint      REFERENCES guardian_events (id) ON DELETE SET NULL,
    mode            text        NOT NULL CHECK (mode IN ('timeout', 'role')),
    saved_role_ids  bigint[]    NOT NULL DEFAULT '{}',
    role_id         bigint,
    until           timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

-- Raid-mode periods. At most one active raid per guild.
CREATE TABLE guardian_raids (
    id            bigserial   PRIMARY KEY,
    guild_id      bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    event_id      bigint      REFERENCES guardian_events (id) ON DELETE SET NULL,
    started_at    timestamptz NOT NULL DEFAULT now(),
    ends_at       timestamptz NOT NULL,
    ended_at      timestamptz,
    ended_by      bigint,
    manual        boolean     NOT NULL DEFAULT false,
    started_by    bigint,
    started_name  text        NOT NULL DEFAULT '',
    joins         integer     NOT NULL DEFAULT 0,
    state         jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE UNIQUE INDEX guardian_raids_active_idx ON guardian_raids (guild_id) WHERE ended_at IS NULL;
CREATE INDEX guardian_raids_due_idx ON guardian_raids (ends_at) WHERE ended_at IS NULL;

-- Channel permissions saved by /lockdown so /unlock restores them exactly.
CREATE TABLE channel_lockdowns (
    guild_id       bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    channel_id     bigint      NOT NULL,
    had_overwrite  boolean     NOT NULL,
    allow_bits     bigint      NOT NULL DEFAULT 0,
    deny_bits      bigint      NOT NULL DEFAULT 0,
    locked_by      bigint,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, channel_id)
);

-- Scam-domain blocklist, one row per domain per feed.
CREATE TABLE scam_domains (
    domain    text        NOT NULL,
    source    text        NOT NULL,
    added_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (domain, source)
);

CREATE TABLE scam_feeds (
    source      text        PRIMARY KEY,
    domains     integer     NOT NULL,
    updated_at  timestamptz NOT NULL
);

-- +goose Down
DROP TABLE scam_feeds;
DROP TABLE scam_domains;
DROP TABLE channel_lockdowns;
DROP TABLE guardian_raids;
DROP TABLE guardian_quarantines;
DROP TABLE guardian_events;
DELETE FROM mod_cases WHERE action IN ('quarantine', 'unquarantine');
ALTER TABLE mod_cases DROP CONSTRAINT mod_cases_action_check;
ALTER TABLE mod_cases ADD CONSTRAINT mod_cases_action_check
    CHECK (action IN ('warn', 'timeout', 'untimeout', 'kick', 'ban', 'unban', 'note', 'purge'));
