package api

import (
	"encoding/json"
	"testing"

	"github.com/nithron/niro/internal/modules/logging"
)

func defaultLoggingBody(t *testing.T, edit func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{}
	b, _ := json.Marshal(toAPILoggingSettings(logging.DefaultSettings()))
	_ = json.Unmarshal(b, &m)
	edit(m)
	out, _ := json.Marshal(m)
	return string(out)
}

func TestLoggingSettings(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setMeta(h)
	const path = "/api/v1/guilds/100/settings/logging"

	got := decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	events := got["events"].(map[string]any)
	if got["defaultChannelId"] != "0" || got["ignoreBots"] != true || events["message_delete"] != true || events["voice_join"] != false ||
		len(events) != len(logging.Kinds) {
		t.Fatalf("defaults = %v", got)
	}

	channels := func(m map[string]any) map[string]any { return m["channels"].(map[string]any) }
	cases := []struct {
		name string
		edit func(m map[string]any)
		want int
	}{
		{"voice channel as log", func(m map[string]any) { m["defaultChannelId"] = "501" }, 400},
		{"unknown log channel", func(m map[string]any) { channels(m)["messages"] = "999" }, 400},
		{"bad id", func(m map[string]any) { channels(m)["members"] = "abc" }, 400},
		{"unknown ignored role", func(m map[string]any) { m["ignoreRoleIds"] = []string{"999"} }, 400},
		{"unknown ignored channel", func(m map[string]any) { m["ignoreChannelIds"] = []string{"999"} }, 400},
		{"unknown event", func(m map[string]any) { m["events"] = map[string]bool{"message_explode": true} }, 400},
		{"log channel ignored", func(m map[string]any) {
			m["defaultChannelId"] = "500"
			m["ignoreChannelIds"] = []string{"500"}
		}, 400},
		{"valid", func(m map[string]any) {
			m["defaultChannelId"] = "500"
			channels(m)["moderation"] = "500"
			m["ignoreChannelIds"] = []string{"501"}
			m["ignoreRoleIds"] = []string{"600"}
			m["noMessageContent"] = true
			m["events"] = map[string]bool{"voice_join": true, "member_join": false}
		}, 200},
	}
	for _, c := range cases {
		rec := h.do("PUT", path, defaultLoggingBody(t, c.edit), sid, sameOrigin)
		if rec.Code != c.want {
			t.Errorf("%s: PUT = %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}
	got = decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	events = got["events"].(map[string]any)
	if got["defaultChannelId"] != "500" || got["noMessageContent"] != true || events["voice_join"] != true ||
		events["member_join"] != false || events["message_edit"] != true || len(events) != len(logging.Kinds) {
		t.Fatalf("saved = %v", got)
	}
	if channels(got)["moderation"] != "500" || channels(got)["messages"] != "0" {
		t.Fatalf("channels = %v", got["channels"])
	}
}
