package discord

import (
	"context"
	"errors"

	"github.com/disgoorg/disgo/rest"

	"github.com/nithron/niro/internal/rpc"
)

// RPCGuildDeleteData removes Niro from a server and deletes all of that
// server's data at once (GDPR erasure, decision 28). The API only sends it
// for the server's owner.
const RPCGuildDeleteData = "guild.delete_data"

func (b *Bot) registerLifecycleRPC(srv *rpc.Server) {
	srv.Handle(RPCGuildDeleteData, func(ctx context.Context, r rpc.Request) (any, error) {
		if b.deps.Lifecycle == nil {
			return nil, rpc.Errorf(rpc.CodeUnavailable, "data deletion is not configured")
		}
		// Leave first: if Niro stayed, new data would keep arriving.
		if err := b.client.Rest.LeaveGuild(r.GuildID, rest.WithCtx(ctx)); err != nil && !rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownGuild) {
			return nil, discordRPCError(err)
		}
		if _, err := b.deps.Lifecycle.DeleteGuild(ctx, r.GuildID); err != nil {
			b.log.ErrorContext(ctx, "deleting server data failed; the 30-day purge will retry", "guild_id", r.GuildID, "err", err)
			return nil, errors.New("deleting the server's data failed")
		}
		if b.deps.GuildCache != nil {
			if err := b.deps.GuildCache.ForgetGuild(ctx, r.GuildID); err != nil {
				b.log.WarnContext(ctx, "clearing the server's cached keys failed", "guild_id", r.GuildID, "err", err)
			}
		}
		b.log.InfoContext(ctx, "server data deleted at the owner's request", "guild_id", r.GuildID, "user_id", r.ActorID)
		return nil, nil
	})
}
