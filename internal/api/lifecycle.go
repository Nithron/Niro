package api

import (
	"context"
	"net/http"

	"github.com/nithron/niro/internal/api/apigen"
	dc "github.com/nithron/niro/internal/platform/discord"
)

// DeleteGuildData removes Niro from a server and deletes all of its data at
// once. Only the server's owner may do this: managers can remove Niro
// themselves, and the data then goes after the 30-day grace period.
func (h handlers) DeleteGuildData(ctx context.Context, req apigen.DeleteGuildDataRequestObject) (apigen.DeleteGuildDataResponseObject, error) {
	g, _, err := h.authorizeGuild(ctx, req.GuildId, false)
	if err != nil {
		return nil, guildErr(err)
	}
	if !g.Owner {
		return nil, httpErr(http.StatusForbidden, apigen.ErrorCodeForbidden, "Only the server's owner can delete its data.")
	}
	if err := h.rpc.Call(ctx, dc.RPCGuildDeleteData, g.ID, actorID(ctx), struct{}{}, nil); err != nil {
		return nil, rpcErr(err)
	}
	h.log.InfoContext(ctx, "server data deletion requested", "guild_id", g.ID, "user_id", actorID(ctx))
	return apigen.DeleteGuildData204Response{}, nil
}
