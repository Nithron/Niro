-- +goose Up

-- Role panels (M4). The message is a message document (JSON, see
-- internal/core/message); roles is the ordered list of panel roles.
CREATE TABLE role_panels (
    id               bigserial   PRIMARY KEY,
    guild_id         bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    name             text        NOT NULL,
    channel_id       bigint      NOT NULL,
    message_id       bigint,
    message          jsonb       NOT NULL,
    style            text        NOT NULL CHECK (style IN ('buttons', 'select')),
    mode             text        NOT NULL CHECK (mode IN ('toggle', 'unique', 'verify')),
    roles            jsonb       NOT NULL,
    max_roles        integer     NOT NULL DEFAULT 0,
    required_role_id bigint,
    updated_by       bigint,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX role_panels_guild_idx ON role_panels (guild_id, id);

-- +goose Down
DROP TABLE role_panels;
