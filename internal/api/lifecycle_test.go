package api

import (
	"net/http"
	"testing"

	dc "github.com/nithron/niro/internal/platform/discord"
)

func TestDeleteGuildDataIsOwnerOnly(t *testing.T) {
	h := newHarness(t)
	h.rpc.handlers[dc.RPCGuildDeleteData] = func(any, any) error { return nil }
	sid := h.login()
	// Manage Server is not enough, and neither is being a member.
	for _, g := range []string{"100", "300", "999"} {
		if rec := h.do("DELETE", "/api/v1/guilds/"+g+"/data", "", sid, sameOrigin); rec.Code != http.StatusForbidden {
			t.Errorf("delete data of %s = %d, want 403", g, rec.Code)
		}
	}
	if len(h.rpc.calls) != 0 {
		t.Fatalf("refused deletes reached the bot: %v", h.rpc.calls)
	}
	// The owner may, even after Niro has left (the data is still there in the grace period).
	if rec := h.do("DELETE", "/api/v1/guilds/200/data", "", sid, sameOrigin); rec.Code != http.StatusNoContent {
		t.Fatalf("owner delete = %d %s", rec.Code, rec.Body.String())
	}
	if len(h.rpc.calls) != 1 || h.rpc.calls[0].Method != dc.RPCGuildDeleteData || h.rpc.calls[0].Guild != noBotGuild {
		t.Fatalf("rpc calls = %+v", h.rpc.calls)
	}
	// Ownership is read fresh from Discord: after a transfer, the old owner
	// is refused at once, even with the server list still cached.
	before := h.oauth.guildCalls
	h.oauth.guilds[1].Owner = false
	if rec := h.do("DELETE", "/api/v1/guilds/200/data", "", sid, sameOrigin); rec.Code != http.StatusForbidden {
		t.Fatalf("delete after losing ownership = %d, want 403", rec.Code)
	}
	if h.oauth.guildCalls == before {
		t.Fatal("ownership was not checked with Discord")
	}
	h.oauth.guilds[1].Owner = true
	// Cross-site requests are refused like any other write.
	if rec := h.do("DELETE", "/api/v1/guilds/200/data", "", sid, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site delete = %d", rec.Code)
	}
}
