package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/nithro/internal/modules/moderation"
	dc "github.com/nithron/nithro/internal/platform/discord"
	"github.com/nithron/nithro/internal/rpc"
)

type rpcCall struct {
	Method string
	Guild  snowflake.ID
	Actor  snowflake.ID
	Params string
}

type fakeRPC struct {
	mu       sync.Mutex
	calls    []rpcCall
	handlers map[string]func(params any, out any) error
}

func (f *fakeRPC) Call(_ context.Context, method string, g, actor snowflake.ID, params, out any) error {
	raw, _ := json.Marshal(params)
	f.mu.Lock()
	f.calls = append(f.calls, rpcCall{method, g, actor, string(raw)})
	h, ok := f.handlers[method]
	f.mu.Unlock()
	if !ok {
		return &rpc.Error{Code: rpc.CodeUnavailable, Message: "no bot"}
	}
	return h(params, out)
}

func (f *fakeRPC) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c.Method)
	}
	return out
}

// fakeCases implements the read side of moderation.CaseStore.
type fakeCases struct {
	moderation.CaseStore
	cases []moderation.Case
}

func (f *fakeCases) GetCase(_ context.Context, g snowflake.ID, n int) (moderation.Case, error) {
	for _, c := range f.cases {
		if c.GuildID == g && c.Number == n {
			return c, nil
		}
	}
	return moderation.Case{}, moderation.ErrCaseNotFound
}

func (f *fakeCases) ListCases(_ context.Context, g snowflake.ID, flt moderation.CaseFilter) ([]moderation.Case, error) {
	var out []moderation.Case
	for i := len(f.cases) - 1; i >= 0; i-- {
		c := f.cases[i]
		if c.GuildID == g && (flt.TargetID == 0 || c.TargetID == flt.TargetID) && (flt.Before == 0 || c.Number < flt.Before) {
			out = append(out, c)
		}
		if len(out) == flt.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeCases) CountCases(_ context.Context, g snowflake.ID, flt moderation.CaseFilter) (int, error) {
	n := 0
	for _, c := range f.cases {
		if c.GuildID == g && (flt.TargetID == 0 || c.TargetID == flt.TargetID) {
			n++
		}
	}
	return n, nil
}

func setMeta(h *harness) {
	h.rpc.handlers[dc.RPCGuildMeta] = func(_ any, out any) error {
		*(out.(*dc.GuildMeta)) = dc.GuildMeta{
			Channels: []dc.MetaChannel{{ID: 500, Name: "mod-log", Type: "text"}, {ID: 501, Name: "Voice", Type: "voice"}},
			Roles:    []dc.MetaRole{{ID: 600, Name: "Mods", Position: 3}},
		}
		return nil
	}
}

func TestModerationSettings(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setMeta(h)
	const path = "/api/v1/guilds/100/settings/moderation"

	got := decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	if got["logChannelId"] != "0" || got["dmUsers"] != true || got["requireReason"] != false {
		t.Fatalf("defaults = %v", got)
	}
	for body, want := range map[string]int{
		`{"logChannelId":"501","dmUsers":true,"requireReason":false}`: 400, // voice channel
		`{"logChannelId":"999","dmUsers":true,"requireReason":false}`: 400, // not in server
		`{"logChannelId":"abc","dmUsers":true,"requireReason":false}`: 400,
		`{"logChannelId":"500","dmUsers":false,"requireReason":true}`: 200,
	} {
		if rec := h.do("PUT", path, body, sid, sameOrigin); rec.Code != want {
			t.Errorf("PUT %s = %d, want %d (%s)", body, rec.Code, want, rec.Body.String())
		}
	}
	got = decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	if got["logChannelId"] != "500" || got["dmUsers"] != false || got["requireReason"] != true {
		t.Fatalf("saved = %v", got)
	}
	if rec := h.do("GET", "/api/v1/guilds/200/settings/moderation", "", sid, nil); rec.Code != 409 {
		t.Fatalf("bot absent = %d", rec.Code)
	}
}

func TestGuildMetaIsCached(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setMeta(h)
	for range 2 {
		meta := decode[map[string]any](t, h.do("GET", "/api/v1/guilds/100/meta", "", sid, nil))
		if len(meta["channels"].([]any)) != 2 || len(meta["roles"].([]any)) != 1 {
			t.Fatalf("meta = %v", meta)
		}
	}
	if n := len(h.rpc.methods()); n != 1 {
		t.Fatalf("bot asked %d times, want 1 (cached)", n)
	}
}

func TestCases(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	now := time.Unix(1700000000, 0).UTC()
	for i := 1; i <= 30; i++ {
		target := snowflake.ID(1000)
		if i%2 == 0 {
			target = 2000
		}
		h.cases.cases = append(h.cases.cases, moderation.Case{GuildID: managedGuild, Number: i, Action: moderation.ActionWarn,
			TargetID: target, TargetName: "u", ModeratorID: 7, ModeratorName: "ana", Source: moderation.SourceManual, CreatedAt: now, UpdatedAt: now})
	}

	page := decode[map[string]any](t, h.do("GET", "/api/v1/guilds/100/cases", "", sid, nil))
	items := page["items"].([]any)
	if len(items) != 25 || page["total"] != float64(30) || page["nextBefore"] != float64(6) {
		t.Fatalf("page 1: %d items, total %v, next %v", len(items), page["total"], page["nextBefore"])
	}
	if first := items[0].(map[string]any); first["number"] != float64(30) || first["targetId"] != "2000" {
		t.Fatalf("first item = %v", first)
	}
	page = decode[map[string]any](t, h.do("GET", "/api/v1/guilds/100/cases?before=6", "", sid, nil))
	if n := len(page["items"].([]any)); n != 5 || page["nextBefore"] != nil {
		t.Fatalf("last page: %d items, next %v", n, page["nextBefore"])
	}
	page = decode[map[string]any](t, h.do("GET", "/api/v1/guilds/100/cases?user=1000&limit=5", "", sid, nil))
	if page["total"] != float64(15) {
		t.Fatalf("user filter total = %v", page["total"])
	}
	if rec := h.do("GET", "/api/v1/guilds/100/cases?action=explode", "", sid, nil); rec.Code != 400 {
		t.Fatalf("bad action = %d", rec.Code)
	}
	if rec := h.do("GET", "/api/v1/guilds/100/cases/99", "", sid, nil); rec.Code != 404 {
		t.Fatalf("missing case = %d", rec.Code)
	}
	if rec := h.do("GET", "/api/v1/guilds/300/cases", "", sid, nil); rec.Code != 403 {
		t.Fatalf("other server = %d", rec.Code)
	}

	// Editing goes through the bot (which also edits the log message).
	h.rpc.handlers[dc.RPCCaseReason] = func(params, _ any) error {
		p := params.(dc.CaseReasonParams)
		h.cases.cases[p.Number-1].Reason = p.Reason
		return nil
	}
	rec := h.do("PATCH", "/api/v1/guilds/100/cases/3", `{"reason":"edited"}`, sid, sameOrigin)
	if rec.Code != 200 || decode[map[string]any](t, rec)["reason"] != "edited" {
		t.Fatalf("patch = %d", rec.Code)
	}
	last := h.rpc.calls[len(h.rpc.calls)-1]
	if last.Actor != 7 || last.Guild != managedGuild {
		t.Fatalf("rpc call = %+v (actor must be the dashboard user)", last)
	}
	h.rpc.handlers[dc.RPCCaseDelete] = func(any, any) error { return nil }
	if rec := h.do("DELETE", "/api/v1/guilds/100/cases/3", "", sid, sameOrigin); rec.Code != 204 {
		t.Fatalf("delete = %d", rec.Code)
	}
}

func TestAutoMod(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	const path = "/api/v1/guilds/100/automod/rules"

	// Bot offline → 502 with a clear code.
	rec := h.do("GET", path, "", sid, nil)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "bot_unavailable") {
		t.Fatalf("offline = %d %s", rec.Code, rec.Body.String())
	}

	h.rpc.handlers[dc.RPCAutoModList] = func(_ any, out any) error {
		*(out.(*[]moderation.AutoModRule)) = []moderation.AutoModRule{{ID: 42, Name: "Links", Enabled: true, Trigger: "keyword", Keywords: []string{"x"}, BlockMessage: true}}
		return nil
	}
	rules := decode[[]map[string]any](t, h.do("GET", path, "", sid, nil))
	if len(rules) != 1 || rules[0]["id"] != "42" || rules[0]["name"] != "Links" {
		t.Fatalf("rules = %v", rules)
	}

	// Invalid rules are rejected before reaching the bot.
	before := len(h.rpc.methods())
	rec = h.do("POST", path, `{"name":"x","enabled":true,"trigger":"keyword","blockMessage":true}`, sid, sameOrigin)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "keyword") {
		t.Fatalf("invalid rule = %d %s", rec.Code, rec.Body.String())
	}
	if len(h.rpc.methods()) != before {
		t.Fatal("invalid rule was sent to the bot")
	}

	h.rpc.handlers[dc.RPCAutoModCreate] = func(params, out any) error {
		r := params.(moderation.AutoModRule)
		r.ID, r.CreatedByBot = 77, true
		*(out.(*moderation.AutoModRule)) = r
		return nil
	}
	rec = h.do("POST", path, `{"name":"Scams","enabled":true,"trigger":"keyword","keywords":["free nitro"],"blockMessage":true}`, sid, sameOrigin)
	if rec.Code != 201 {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]any](t, rec); got["id"] != "77" || got["createdByBot"] != true {
		t.Fatalf("created = %v", got)
	}

	h.rpc.handlers[dc.RPCAutoModUpdate] = func(any, any) error {
		return &rpc.Error{Code: rpc.CodeDiscord, Message: "Maximum number of rules reached"}
	}
	rec = h.do("PUT", path+"/77", `{"name":"Scams","enabled":false,"trigger":"spam","blockMessage":true}`, sid, sameOrigin)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "Maximum number of rules") {
		t.Fatalf("discord error = %d %s", rec.Code, rec.Body.String())
	}

	presets := decode[[]map[string]any](t, h.do("GET", "/api/v1/automod/presets", "", sid, nil))
	if len(presets) != len(moderation.AutoModPresets()) {
		t.Fatalf("presets = %d", len(presets))
	}
}
