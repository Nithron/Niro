-- +goose Up

-- Servers Niro has been added to. Snowflake IDs fit in a signed bigint.
-- left_at is set when the bot is removed; data is purged after the
-- retention grace period (M5), not immediately.
CREATE TABLE guilds (
    id         bigint      PRIMARY KEY,
    name       text        NOT NULL,
    icon       text,
    owner_id   bigint      NOT NULL,
    locale     text        NOT NULL DEFAULT 'en-US',
    joined_at  timestamptz NOT NULL DEFAULT now(),
    left_at    timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- One row per (guild, module) once the module has been changed from its
-- default. Module-specific settings go in config (validated by the module).
CREATE TABLE guild_configs (
    guild_id   bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    module     text        NOT NULL,
    enabled    boolean     NOT NULL,
    config     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    version    bigint      NOT NULL DEFAULT 1,
    updated_by bigint,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, module)
);

-- Who changed what, from where. Written in the same transaction as the change.
CREATE TABLE config_audit (
    id         bigserial   PRIMARY KEY,
    guild_id   bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    module     text        NOT NULL,
    actor_id   bigint,
    source     text        NOT NULL CHECK (source IN ('dashboard', 'command', 'system')),
    change     jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX config_audit_guild_created_idx ON config_audit (guild_id, created_at DESC);

-- +goose Down
DROP TABLE config_audit;
DROP TABLE guild_configs;
DROP TABLE guilds;
