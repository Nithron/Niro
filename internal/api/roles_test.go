package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/roles"
	dc "github.com/nithron/niro/internal/platform/discord"
)

type fakePanels struct {
	mu     sync.Mutex
	panels []roles.Panel
}

func (f *fakePanels) ListPanels(_ context.Context, g snowflake.ID) ([]roles.Panel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []roles.Panel
	for _, p := range f.panels {
		if p.GuildID == g {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakePanels) GetPanel(_ context.Context, g snowflake.ID, id int64) (roles.Panel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.panels {
		if p.GuildID == g && p.ID == id {
			return p, nil
		}
	}
	return roles.Panel{}, roles.ErrNotFound
}

func setPanelMeta(h *harness) {
	h.rpc.handlers[dc.RPCGuildMeta] = func(_ any, out any) error {
		*(out.(*dc.GuildMeta)) = dc.GuildMeta{
			Channels: []dc.MetaChannel{{ID: 500, Name: "roles", Type: "text"}, {ID: 501, Name: "Voice", Type: "voice"}},
			Roles: []dc.MetaRole{{ID: 600, Name: "Red", Position: 1}, {ID: 601, Name: "Admins", Position: 2, Dangerous: true},
				{ID: 602, Name: "Bot", Position: 3, Managed: true}, {ID: 603, Name: "VIP", Position: 1}},
		}
		return nil
	}
}

func panelBody(edit func(m map[string]any)) string {
	m := map[string]any{
		"name": "Colours", "channelId": "500", "style": "buttons", "mode": "toggle", "maxRoles": 0, "requiredRoleId": "0",
		"message": map[string]any{"version": 1, "blocks": []any{map[string]any{"type": "text", "text": "Pick a colour"}}},
		"roles":   []any{map[string]any{"roleId": "600", "label": "Red", "emoji": "🟥"}},
	}
	if edit != nil {
		edit(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestRolePanels(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setPanelMeta(h)
	var saved []roles.Panel
	h.rpc.handlers[dc.RPCRolePanelSave] = func(params any, out any) error {
		p := params.(roles.Panel)
		if p.ID == 0 {
			p.ID = 7
		}
		p.MessageID = 900
		saved = append(saved, p)
		*(out.(*roles.Panel)) = p
		return nil
	}
	const path = "/api/v1/guilds/100/role-panels"

	cases := []struct {
		name string
		edit func(m map[string]any)
		want int
		msg  string
	}{
		{"voice channel", func(m map[string]any) { m["channelId"] = "501" }, 400, "text or announcement"},
		{"dangerous role", func(m map[string]any) { m["roles"] = []any{map[string]any{"roleId": "601", "label": "A"}} }, 400, "dangerous permissions"},
		{"managed role", func(m map[string]any) { m["roles"] = []any{map[string]any{"roleId": "602", "label": "B"}} }, 400, "managed"},
		{"@everyone", func(m map[string]any) { m["roles"] = []any{map[string]any{"roleId": "100", "label": "E"}} }, 400, "doesn't exist"},
		{"unknown role", func(m map[string]any) { m["roles"] = []any{map[string]any{"roleId": "999", "label": "E"}} }, 400, "doesn't exist"},
		{"bad message", func(m map[string]any) {
			m["message"] = map[string]any{"version": 1, "blocks": []any{map[string]any{"type": "section", "text": "x", "thumbnail": "http://insecure"}}}
		}, 400, "Message:"},
		{"duplicate role", func(m map[string]any) {
			m["roles"] = []any{map[string]any{"roleId": "600", "label": "a"}, map[string]any{"roleId": "600", "label": "b"}}
		}, 400, "twice"},
		{"valid", nil, 201, ""},
	}
	for _, c := range cases {
		rec := h.do("POST", path, panelBody(c.edit), sid, sameOrigin)
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.msg) {
			t.Errorf("%s: POST = %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	if len(saved) != 1 || saved[0].GuildID != managedGuild || saved[0].Roles[0].Emoji != "🟥" || saved[0].Message.Blocks[0].Text != "Pick a colour" {
		t.Fatalf("sent to the bot: %+v", saved)
	}
	created := decode[map[string]any](t, h.do("POST", path, panelBody(nil), sid, sameOrigin))
	if created["id"] != float64(7) || created["messageId"] != "900" {
		t.Errorf("created = %v", created)
	}

	// Updating a panel that doesn't exist (or belongs to another server) is a 404.
	if rec := h.do("PUT", path+"/7", panelBody(nil), sid, sameOrigin); rec.Code != 404 {
		t.Errorf("PUT unknown = %d", rec.Code)
	}
	h.panels.panels = append(h.panels.panels, saved[0])
	if rec := h.do("PUT", path+"/7", panelBody(func(m map[string]any) { m["name"] = "New" }), sid, sameOrigin); rec.Code != 200 {
		t.Errorf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	if last := saved[len(saved)-1]; last.ID != 7 || last.Name != "New" {
		t.Errorf("update sent %+v", last)
	}

	list := decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	if items := list["items"].([]any); len(items) != 1 || items[0].(map[string]any)["name"] != "Colours" {
		t.Errorf("list = %v", list)
	}

	h.rpc.handlers[dc.RPCRolePanelDelete] = func(params any, _ any) error {
		if params.(dc.RolePanelIDParams).ID != 7 {
			t.Errorf("delete params = %v", params)
		}
		return nil
	}
	if rec := h.do("DELETE", path+"/7", "", sid, sameOrigin); rec.Code != 204 {
		t.Errorf("DELETE = %d", rec.Code)
	}
	if rec := h.do("POST", path, panelBody(nil), sid, nil); rec.Code != 403 {
		t.Errorf("cross-site POST = %d", rec.Code)
	}
}
