package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/guardian"
	dc "github.com/nithron/niro/internal/platform/discord"
	"github.com/nithron/niro/internal/rpc"
)

// fakeGuardian is the read side of the Guardian store.
type fakeGuardian struct {
	mu     sync.Mutex
	events []guardian.Event
	raid   *guardian.Raid
}

func (f *fakeGuardian) GetEvent(_ context.Context, g snowflake.ID, id int64) (guardian.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.events {
		if e.GuildID == g && e.ID == id {
			return e, nil
		}
	}
	return guardian.Event{}, guardian.ErrEventNotFound
}

func (f *fakeGuardian) ListEvents(_ context.Context, g snowflake.ID, flt guardian.EventFilter) ([]guardian.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []guardian.Event
	for i := len(f.events) - 1; i >= 0; i-- {
		e := f.events[i]
		if e.GuildID == g && (flt.Status == "" || e.Status == flt.Status) && (flt.Before == 0 || e.ID < flt.Before) {
			out = append(out, e)
		}
		if len(out) == flt.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeGuardian) ActiveRaid(context.Context, snowflake.ID) (guardian.Raid, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.raid == nil {
		return guardian.Raid{}, guardian.ErrNoRaid
	}
	return *f.raid, nil
}

func defaultGuardianBody(t *testing.T, edit func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{}
	b, _ := json.Marshal(toAPIGuardianSettings(guardian.DefaultSettings()))
	_ = json.Unmarshal(b, &m)
	edit(m)
	out, _ := json.Marshal(m)
	return string(out)
}

func TestGuardianSettings(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setMeta(h)
	const path = "/api/v1/guilds/100/settings/guardian"

	got := decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	if got["mode"] != "enforce" || got["burstChannels"] != float64(3) || got["quarantineMode"] != "timeout" || got["alertChannelId"] != "0" {
		t.Fatalf("defaults = %v", got)
	}

	cases := []struct {
		name string
		edit func(m map[string]any)
		want int
	}{
		{"voice alert channel", func(m map[string]any) { m["alertChannelId"] = "501" }, 400},
		{"role mode without role", func(m map[string]any) { m["quarantineMode"] = "role" }, 400},
		{"unknown quarantine role", func(m map[string]any) { m["quarantineMode"] = "role"; m["quarantineRoleId"] = "999" }, 400},
		{"@everyone as verify role", func(m map[string]any) { m["verifyRoleId"] = "100" }, 400},
		{"out of range", func(m map[string]any) { m["burstChannels"] = 1 }, 400},
		{"bad domain", func(m map[string]any) { m["denyDomains"] = []string{"not a domain"} }, 400},
		{"valid", func(m map[string]any) {
			m["alertChannelId"] = "500"
			m["quarantineMode"] = "role"
			m["quarantineRoleId"] = "600"
			m["denyDomains"] = []string{"https://Grabify.link/abc", "grabify.link"}
			m["mode"] = "monitor"
		}, 200},
	}
	for _, c := range cases {
		rec := h.do("PUT", path, defaultGuardianBody(t, c.edit), sid, sameOrigin)
		if rec.Code != c.want {
			t.Errorf("%s: PUT = %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}
	got = decode[map[string]any](t, h.do("GET", path, "", sid, nil))
	if got["mode"] != "monitor" || got["quarantineRoleId"] != "600" || got["alertChannelId"] != "500" {
		t.Fatalf("saved = %v", got)
	}
	if d := got["denyDomains"].([]any); len(d) != 1 || d[0] != "grabify.link" {
		t.Fatalf("domains not normalized: %v", d)
	}
}

func TestGuardianEventsAndResolve(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	now := time.Now().UTC()
	for i := int64(1); i <= 3; i++ {
		h.guard.events = append(h.guard.events, guardian.Event{
			ID: i, GuildID: managedGuild, UserID: 42, UserName: "victim", Kind: guardian.KindScam,
			Signals:  []guardian.Signal{{Kind: guardian.SignalBlockedDomain, Detail: "evil.ru", Weight: 3}},
			Response: guardian.ResponseQuarantine, Status: guardian.StatusOpen, CaseNumber: int(i), CreatedAt: now,
			Evidence: guardian.Evidence{Channels: []snowflake.ID{11}, Deleted: 1, Domains: []string{"evil.ru"}},
		})
	}
	h.guard.events = append(h.guard.events, guardian.Event{ID: 9, GuildID: 999, Status: guardian.StatusOpen})

	list := decode[map[string]any](t, h.do("GET", "/api/v1/guilds/100/guardian/events?limit=2", "", sid, nil))
	items := list["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["id"] != float64(3) || list["nextBefore"] != float64(2) {
		t.Fatalf("page 1 = %v", list)
	}
	first := items[0].(map[string]any)
	if first["userId"] != "42" || first["domains"].([]any)[0] != "evil.ru" || first["signals"].([]any)[0].(map[string]any)["kind"] != "blocked_domain" {
		t.Fatalf("event = %v", first)
	}
	if rec := h.do("GET", "/api/v1/guilds/100/guardian/events/9", "", sid, nil); rec.Code != 404 {
		t.Fatalf("other guild's event = %d", rec.Code)
	}

	h.rpc.handlers[dc.RPCGuardianResolve] = func(params any, _ any) error {
		p := params.(dc.GuardianResolveParams)
		h.guard.mu.Lock()
		defer h.guard.mu.Unlock()
		for i := range h.guard.events {
			if h.guard.events[i].ID == p.EventID {
				if h.guard.events[i].Status != guardian.StatusOpen {
					return rpc.Errorf(rpc.CodeInvalid, "another moderator already handled this event")
				}
				h.guard.events[i].Status, h.guard.events[i].ResolvedByName = guardian.StatusRestored, "Mod"
			}
		}
		return nil
	}
	rec := h.do("POST", "/api/v1/guilds/100/guardian/events/3/resolve", `{"action":"restore"}`, sid, sameOrigin)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"restored"`) {
		t.Fatalf("resolve = %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.do("POST", "/api/v1/guilds/100/guardian/events/3/resolve", `{"action":"restore"}`, sid, sameOrigin); rec.Code != 400 {
		t.Fatalf("second resolve = %d", rec.Code)
	}
	if rec := h.do("POST", "/api/v1/guilds/100/guardian/events/3/resolve", `{"action":"nuke"}`, sid, sameOrigin); rec.Code != 400 {
		t.Fatalf("bad action = %d", rec.Code)
	}
	// Without the Origin check the request is refused (CSRF).
	if rec := h.do("POST", "/api/v1/guilds/100/guardian/events/2/resolve", `{"action":"keep"}`, sid, nil); rec.Code != 403 {
		t.Fatalf("cross-origin resolve = %d", rec.Code)
	}
	var calls []rpcCall
	for _, c := range h.rpc.calls {
		if c.Method == dc.RPCGuardianResolve {
			calls = append(calls, c)
		}
	}
	if len(calls) != 2 || calls[0].Actor == 0 || !strings.Contains(calls[0].Params, `"event_id":3`) {
		t.Fatalf("rpc calls = %+v", calls)
	}
}

func TestRaidMode(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	const path = "/api/v1/guilds/100/guardian/raid"
	if got := decode[map[string]any](t, h.do("GET", path, "", sid, nil)); got["active"] != false {
		t.Fatalf("status = %v", got)
	}
	if rec := h.do("POST", path, "", sid, sameOrigin); rec.Code != 502 {
		t.Fatalf("start with bot offline = %d", rec.Code)
	}
	h.rpc.handlers[dc.RPCGuardianRaidStart] = func(any, any) error {
		now := time.Now()
		h.guard.raid = &guardian.Raid{StartedAt: now, EndsAt: now.Add(30 * time.Minute), Manual: true}
		return nil
	}
	h.rpc.handlers[dc.RPCGuardianRaidEnd] = func(any, any) error {
		h.guard.raid = nil
		return nil
	}
	if got := decode[map[string]any](t, h.do("POST", path, "", sid, sameOrigin)); got["active"] != true || got["manual"] != true {
		t.Fatalf("after start = %v", got)
	}
	if got := decode[map[string]any](t, h.do("DELETE", path, "", sid, sameOrigin)); got["active"] != false {
		t.Fatalf("after end = %v", got)
	}
}

func TestAuditIsCached(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	h.rpc.handlers[dc.RPCGuardianAudit] = func(_ any, out any) error {
		*(out.(*dc.AuditResult)) = dc.AuditResult{Findings: []guardian.Finding{
			{Severity: guardian.SeverityHigh, Code: guardian.FindRoleAdmin, SubjectID: 7, Subject: "Admin", Count: -1},
		}}
		return nil
	}
	for range 2 {
		got := decode[map[string]any](t, h.do("GET", "/api/v1/guilds/100/guardian/audit", "", sid, nil))
		f := got["findings"].([]any)[0].(map[string]any)
		if f["code"] != "role_admin" || f["subjectId"] != "7" || f["count"] != float64(-1) || got["webhooksChecked"] != false {
			t.Fatalf("audit = %v", got)
		}
	}
	if n := len(h.rpc.methods()); n != 1 {
		t.Fatalf("audit ran %d times, want 1 (cached)", n)
	}
	if rec := h.do("GET", "/api/v1/guilds/300/guardian/audit", "", sid, nil); rec.Code != 403 {
		t.Fatalf("member-only guild = %d", rec.Code)
	}
}
