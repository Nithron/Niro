package api

import (
	"context"
	"net/http"
	"slices"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	dc "github.com/nithron/niro/internal/platform/discord"
)

// DeleteGuildData removes Niro from a server and deletes all of its data at
// once. Only the server's owner may do this: managers can remove Niro
// themselves, and the data then goes after the 30-day grace period.
//
// Ownership is checked against Discord directly, not the 60-second cache, so
// a former owner can't act right after transferring the server.
func (h handlers) DeleteGuildData(ctx context.Context, req apigen.DeleteGuildDataRequestObject) (apigen.DeleteGuildDataResponseObject, error) {
	id, err := snowflake.Parse(req.GuildId)
	if err != nil {
		return nil, guildErr(errForbidden)
	}
	fresh, err := h.oauth.CurrentUserGuilds(ctx, sessionFrom(ctx).AccessToken)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(fresh, func(g UserGuild) bool { return g.ID == id })
	if i < 0 {
		return nil, guildErr(errForbidden)
	}
	if !fresh[i].Owner {
		return nil, httpErr(http.StatusForbidden, apigen.ErrorCodeForbidden, "Only the server's owner can delete its data.")
	}
	if err := h.rpc.Call(ctx, dc.RPCGuildDeleteData, id, actorID(ctx), struct{}{}, nil); err != nil {
		return nil, rpcErr(err)
	}
	h.log.InfoContext(ctx, "server data deletion requested", "guild_id", id, "user_id", actorID(ctx))
	return apigen.DeleteGuildData204Response{}, nil
}
