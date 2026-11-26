package discord

import (
	"context"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/rpc"
)

// RPCGuildDeleteData removes Niro from a server and deletes all of that
// server's data at once (GDPR erasure, decision 28). The API only sends it
// for the server's owner.
const RPCGuildDeleteData = "guild.delete_data"

// eraseSweepDelay is when the second erasure pass runs: by then the leave
// event has been handled and in-flight handlers for the server have finished.
const eraseSweepDelay = 15 * time.Second

func (b *Bot) registerLifecycleRPC(srv *rpc.Server) {
	srv.Handle(RPCGuildDeleteData, func(ctx context.Context, r rpc.Request) (any, error) {
		if b.deps.Lifecycle == nil || b.deps.GuildCache == nil {
			return nil, rpc.Errorf(rpc.CodeUnavailable, "data deletion is not configured")
		}
		// The channel list is needed for the message cache, and disgo drops
		// it when Niro leaves.
		var channels []snowflake.ID
		b.client.Caches.ChannelsForGuild(r.GuildID)(func(c discord.GuildChannel) bool {
			channels = append(channels, c.ID())
			return true
		})
		// Leave first: if Niro stayed, new data would keep arriving.
		if err := b.client.Rest.LeaveGuild(r.GuildID, rest.WithCtx(ctx)); err != nil && !rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownGuild) {
			return nil, discordRPCError(err)
		}
		if err := b.eraseGuild(ctx, r.GuildID, channels); err != nil {
			b.log.ErrorContext(ctx, "deleting server data failed", "guild_id", r.GuildID, "err", err)
			return nil, rpc.Errorf(rpc.CodeUnavailable, "Niro left the server, but deleting its data failed. Please try again.")
		}
		b.log.InfoContext(ctx, "server data deleted at the owner's request", "guild_id", r.GuildID, "user_id", r.ActorID)
		// A second pass catches anything a handler still running for this
		// server wrote in the meantime. Rows can't come back after it: every
		// server table references guilds, and only a new join recreates that
		// row. On shutdown the pass runs at once instead of being lost.
		go func() {
			select {
			case <-time.After(eraseSweepDelay):
			case <-b.stopped:
			}
			sweepCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := b.eraseGuild(sweepCtx, r.GuildID, channels); err != nil {
				b.log.Warn("second erasure pass failed", "guild_id", r.GuildID, "err", err)
			}
		}()
		return nil, nil
	})
}

// eraseGuild deletes a server's cached message text and Valkey keys, then
// its rows (the cascade takes every table). The message text goes first: it
// needs the channel list, which a retry after the leave can no longer get.
func (b *Bot) eraseGuild(ctx context.Context, id snowflake.ID, channels []snowflake.ID) error {
	if err := b.deps.GuildCache.ForgetMessages(ctx, channels); err != nil {
		return err
	}
	if err := b.deps.GuildCache.ForgetGuild(ctx, id); err != nil {
		return err
	}
	_, err := b.deps.Lifecycle.DeleteGuild(ctx, id)
	return err
}
