package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nithron/niro/internal/modules/welcome"
)

func welcomeBody(t *testing.T, edit func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{}
	b, _ := json.Marshal(toAPIWelcome(welcome.DefaultSettings()))
	_ = json.Unmarshal(b, &m)
	edit(m)
	out, _ := json.Marshal(m)
	return string(out)
}

func TestWelcomeSettings(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setPanelMeta(h)
	const path = "/api/v1/guilds/100/settings/welcome"

	got := decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	if got["welcomeEnabled"] != false || got["dmEnabled"] != false || got["pingMember"] != true {
		t.Fatalf("defaults = %v", got)
	}
	cases := []struct {
		name string
		edit func(m map[string]any)
		want int
		msg  string
	}{
		{"enabled without channel", func(m map[string]any) { m["welcomeEnabled"] = true }, 400, "choose a channel"},
		{"voice channel", func(m map[string]any) { m["welcomeChannelId"] = "501" }, 400, "text or announcement"},
		{"dangerous auto-role", func(m map[string]any) { m["autoRoleIds"] = []string{"601"} }, 400, "dangerous"},
		{"managed auto-role", func(m map[string]any) { m["autoRoleIds"] = []string{"602"} }, 400, "managed"},
		{"empty message", func(m map[string]any) { m["goodbyeMessage"] = map[string]any{"version": 1, "blocks": []any{}} }, 400, "empty"},
		{"valid", func(m map[string]any) {
			m["welcomeEnabled"], m["welcomeChannelId"], m["dmEnabled"] = true, "500", true
			m["autoRoleIds"] = []string{"600", "603"}
		}, 200, ""},
	}
	for _, c := range cases {
		rec := h.do("PUT", path, welcomeBody(t, c.edit), sid, sameOrigin)
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.msg) {
			t.Errorf("%s: PUT = %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	got = decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	if got["welcomeChannelId"] != "500" || got["dmEnabled"] != true || len(got["autoRoleIds"].([]any)) != 2 {
		t.Fatalf("saved = %v", got)
	}
}
