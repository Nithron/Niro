package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/tickets"
	dc "github.com/nithron/niro/internal/platform/discord"
)

type fakeTickets struct {
	mu          sync.Mutex
	panels      []tickets.Panel
	list        []tickets.Ticket
	transcripts map[int64]tickets.Transcript
}

func (f *fakeTickets) ListTicketPanels(_ context.Context, g snowflake.ID) ([]tickets.Panel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tickets.Panel
	for _, p := range f.panels {
		if p.GuildID == g {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeTickets) GetTicketPanel(_ context.Context, g snowflake.ID, id int64) (tickets.Panel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.panels {
		if p.GuildID == g && p.ID == id {
			return p, nil
		}
	}
	return tickets.Panel{}, tickets.ErrPanelNotFound
}

func (f *fakeTickets) ListTickets(_ context.Context, g snowflake.ID, flt tickets.Filter) ([]tickets.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tickets.Ticket
	for i := len(f.list) - 1; i >= 0; i-- {
		t := f.list[i]
		if t.GuildID == g && (flt.Status == "" || t.Status == flt.Status) && (flt.Before == 0 || t.Number < flt.Before) {
			out = append(out, t)
		}
		if len(out) == flt.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeTickets) GetTicketByNumber(_ context.Context, g snowflake.ID, n int) (tickets.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.list {
		if t.GuildID == g && t.Number == n {
			return t, nil
		}
	}
	return tickets.Ticket{}, tickets.ErrNotFound
}

func (f *fakeTickets) GetTranscript(_ context.Context, g snowflake.ID, id int64) (tickets.Transcript, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tr, ok := f.transcripts[id]
	if !ok || tr.GuildID != g {
		return tickets.Transcript{}, tickets.ErrNoTranscript
	}
	return tr, nil
}

func setTicketMeta(h *harness) {
	h.rpc.handlers[dc.RPCGuildMeta] = func(_ any, out any) error {
		*(out.(*dc.GuildMeta)) = dc.GuildMeta{
			Channels: []dc.MetaChannel{{ID: 500, Name: "help", Type: "text"}, {ID: 510, Name: "Tickets", Type: "category"}},
			Roles:    []dc.MetaRole{{ID: 600, Name: "Staff", Position: 1}},
		}
		return nil
	}
}

func ticketPanelBody(edit func(m map[string]any)) string {
	doc := func(text string) map[string]any {
		return map[string]any{"version": 1, "blocks": []any{map[string]any{"type": "text", "text": text}}}
	}
	cat := map[string]any{"label": "Support", "mode": "channel", "parentId": "510", "staffRoleIds": []string{"600"}, "maxOpen": 1,
		"questions": []any{map[string]any{"label": "What's wrong?", "long": true, "required": true}}, "opening": doc("Thanks")}
	m := map[string]any{"name": "Help", "channelId": "500", "message": doc("Need help?"), "categories": []any{cat}}
	if edit != nil {
		edit(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestTicketPanelsAPI(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setTicketMeta(h)
	var sent []tickets.Panel
	h.rpc.handlers[dc.RPCTicketPanelSave] = func(params any, out any) error {
		p := params.(tickets.Panel)
		p.ID, p.MessageID = 4, 901
		sent = append(sent, p)
		*(out.(*tickets.Panel)) = p
		return nil
	}
	const path = "/api/v1/guilds/100/ticket-panels"
	cat := func(m map[string]any) map[string]any { return m["categories"].([]any)[0].(map[string]any) }
	cases := []struct {
		name string
		edit func(m map[string]any)
		want int
		msg  string
	}{
		{"unknown staff role", func(m map[string]any) { cat(m)["staffRoleIds"] = []string{"999"} }, 400, "staff role"},
		{"@everyone as staff", func(m map[string]any) { cat(m)["staffRoleIds"] = []string{"100"} }, 400, "staff role"},
		{"parent not a category", func(m map[string]any) { cat(m)["parentId"] = "500" }, 400, "channel category"},
		{"question too long", func(m map[string]any) {
			cat(m)["questions"] = []any{map[string]any{"label": strings.Repeat("q", 46), "long": false, "required": false}}
		}, 400, ""},
		{"valid", nil, 201, ""},
	}
	for _, c := range cases {
		rec := h.do("POST", path, ticketPanelBody(c.edit), sid, sameOrigin)
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.msg) {
			t.Errorf("%s: POST = %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	if len(sent) != 1 || sent[0].Categories[0].Questions[0].Label != "What's wrong?" || sent[0].Categories[0].ParentID != 510 {
		t.Fatalf("sent = %+v", sent)
	}
	if rec := h.do("PUT", path+"/4", ticketPanelBody(nil), sid, sameOrigin); rec.Code != 404 {
		t.Errorf("PUT unknown = %d", rec.Code)
	}
}

func TestTicketsAndTranscript(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	gz, err := tickets.RenderTranscript(tickets.TranscriptInfo{Server: "G", Ticket: tickets.Ticket{Number: 1}}, nil, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		h.tix.list = append(h.tix.list, tickets.Ticket{ID: int64(i), GuildID: managedGuild, Number: i, Status: tickets.StatusClosed,
			Mode: tickets.ModeThread, OpenerID: 7, CreatedAt: time.Now(), HasTranscript: i == 1})
	}
	h.tix.list = append(h.tix.list, tickets.Ticket{ID: 9, GuildID: 999, Number: 9})
	h.tix.transcripts = map[int64]tickets.Transcript{1: {TicketID: 1, GuildID: managedGuild, HTMLGzip: gz}, 9: {TicketID: 9, GuildID: 999, HTMLGzip: gz}}

	page := decode[map[string]any](t, h.do("GET", "/api/v1/guilds/100/tickets?limit=2", "", sid, nil))
	if items := page["items"].([]any); len(items) != 2 || items[0].(map[string]any)["number"] != float64(3) || page["nextBefore"] != float64(2) {
		t.Fatalf("page = %v", page)
	}
	rec := h.do("GET", "/api/v1/guilds/100/tickets/1/transcript", "", sid, nil)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") ||
		rec.Header().Get("Content-Security-Policy") != transcriptCSP || !strings.Contains(rec.Body.String(), "Ticket #1") {
		t.Errorf("transcript = %d %v", rec.Code, rec.Header())
	}
	if rec := h.do("GET", "/api/v1/guilds/100/tickets/2/transcript", "", sid, nil); rec.Code != 404 {
		t.Errorf("no transcript = %d", rec.Code)
	}
	if rec := h.do("GET", "/api/v1/guilds/100/tickets/9/transcript", "", sid, nil); rec.Code != 404 {
		t.Errorf("other guild's transcript = %d", rec.Code)
	}
	if rec := h.do("GET", "/api/v1/guilds/300/tickets/1/transcript", "", sid, nil); rec.Code != 403 {
		t.Errorf("unmanaged guild = %d", rec.Code)
	}
}

func TestTicketSettingsAPI(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	setTicketMeta(h)
	const path = "/api/v1/guilds/100/settings/tickets"
	if got := decode[map[string]any](t, h.do("GET", path, "", sid, nil)); got["retentionDays"] != float64(90) {
		t.Fatalf("defaults = %v", got)
	}
	for body, want := range map[string]int{
		`{"retentionDays":0,"transcriptChannelId":"0"}`:    400,
		`{"retentionDays":30,"transcriptChannelId":"510"}`: 400, // a category, not a text channel
		`{"retentionDays":30,"transcriptChannelId":"500"}`: 200,
	} {
		if rec := h.do("PUT", path, body, sid, sameOrigin); rec.Code != want {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
}
