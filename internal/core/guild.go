package core

import (
	"context"

	"github.com/disgoorg/snowflake/v2"
)

// GuildInfo is what Niro records about a server it is in.
type GuildInfo struct {
	ID      snowflake.ID
	Name    string
	Icon    string // icon hash; empty if the server has no icon
	OwnerID snowflake.ID
	Locale  string // the server's preferred locale, e.g. "en-US"
}

// GuildDirectory tracks which servers Niro is currently in.
type GuildDirectory interface {
	UpsertGuild(ctx context.Context, g GuildInfo) error
	MarkGuildLeft(ctx context.Context, id snowflake.ID) error
	// ActiveGuilds returns the subset of ids that currently have Niro.
	ActiveGuilds(ctx context.Context, ids []snowflake.ID) (map[snowflake.ID]bool, error)
	IsGuildActive(ctx context.Context, id snowflake.ID) (bool, error)
}
