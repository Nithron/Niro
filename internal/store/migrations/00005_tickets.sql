-- +goose Up

-- Ticket panels (M4): a message with a button per category (JSON, see
-- internal/modules/tickets).
CREATE TABLE ticket_panels (
    id          bigserial   PRIMARY KEY,
    guild_id    bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    channel_id  bigint      NOT NULL,
    message_id  bigint,
    message     jsonb       NOT NULL,
    categories  jsonb       NOT NULL,
    updated_by  bigint,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ticket_panels_guild_idx ON ticket_panels (guild_id, id);

-- Per-guild ticket numbers (1, 2, 3…), like case numbers.
CREATE TABLE guild_ticket_counters (
    guild_id    bigint  PRIMARY KEY REFERENCES guilds (id) ON DELETE CASCADE,
    last_ticket integer NOT NULL DEFAULT 0
);

-- Tickets. No message text: that is only in the transcript. Names and the
-- close reason are scrubbed when the transcript expires. panel_id has no
-- foreign key: tickets outlive deleted panels.
CREATE TABLE tickets (
    id              bigserial   PRIMARY KEY,
    guild_id        bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    number          integer     NOT NULL,
    panel_id        bigint      NOT NULL,
    category_id     text        NOT NULL,
    category_label  text        NOT NULL DEFAULT '',
    mode            text        NOT NULL CHECK (mode IN ('thread', 'channel')),
    channel_id      bigint,
    opener_id       bigint      NOT NULL,
    opener_name     text        NOT NULL DEFAULT '',
    claimed_by      bigint,
    claimed_by_name text        NOT NULL DEFAULT '',
    status          text        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'claimed', 'closed')),
    close_reason    text        NOT NULL DEFAULT '',
    closed_by       bigint,
    closed_by_name  text        NOT NULL DEFAULT '',
    closed_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (guild_id, number)
);

CREATE INDEX tickets_open_idx ON tickets (guild_id, opener_id) WHERE status <> 'closed';
CREATE INDEX tickets_channel_idx ON tickets (guild_id, channel_id);

-- Transcripts: the one table with message text (decision 25), gzip HTML
-- that expires (deleted by the bot's retention job).
CREATE TABLE ticket_transcripts (
    ticket_id     bigint      PRIMARY KEY REFERENCES tickets (id) ON DELETE CASCADE,
    guild_id      bigint      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    html_gzip     bytea       NOT NULL,
    message_count integer     NOT NULL DEFAULT 0,
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ticket_transcripts_expiry_idx ON ticket_transcripts (expires_at);

-- +goose Down
DROP TABLE ticket_transcripts;
DROP TABLE tickets;
DROP TABLE guild_ticket_counters;
DROP TABLE ticket_panels;
