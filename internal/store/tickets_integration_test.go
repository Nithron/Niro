package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/tickets"
)

func TestTickets(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	for _, g := range []core.GuildInfo{{ID: 1, Name: "G", OwnerID: 1}, {ID: 2, Name: "H", OwnerID: 1}} {
		if err := s.UpsertGuild(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.CreateTicketPanel(ctx, tickets.Panel{GuildID: 1, Name: "Help", ChannelID: 50, Message: message.Simple("Need help?"),
		Categories: []tickets.Category{{ID: "abc", Label: "Support", Mode: tickets.ModeThread, StaffRoleIDs: []snowflake.ID{60}, MaxOpen: 1}}})
	if err != nil || p.ID == 0 || p.Categories[0].ID != "abc" {
		t.Fatalf("panel = %+v, %v", p, err)
	}
	if n, _ := s.CountTicketPanels(ctx, 1); n != 1 {
		t.Errorf("count = %d", n)
	}

	a, err := s.CreateTicket(ctx, tickets.NewTicket{GuildID: 1, PanelID: p.ID, CategoryID: "abc", CategoryLabel: "Support", Mode: tickets.ModeThread, OpenerID: 100, OpenerName: "ana"})
	if err != nil || a.Number != 1 || a.Status != tickets.StatusOpen {
		t.Fatalf("ticket = %+v, %v", a, err)
	}
	b, _ := s.CreateTicket(ctx, tickets.NewTicket{GuildID: 1, PanelID: p.ID, CategoryID: "abc", Mode: tickets.ModeThread, OpenerID: 101})
	c, _ := s.CreateTicket(ctx, tickets.NewTicket{GuildID: 2, PanelID: 9, CategoryID: "x", Mode: tickets.ModeChannel, OpenerID: 100})
	if b.Number != 2 || c.Number != 1 {
		t.Errorf("numbers = %d, %d", b.Number, c.Number)
	}
	if err := s.SetTicketChannel(ctx, 1, a.ID, 700); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetTicketByChannel(ctx, 1, 700); err != nil || got.ID != a.ID {
		t.Errorf("by channel = %+v, %v", got, err)
	}
	if _, err := s.GetTicketByChannel(ctx, 2, 700); !errors.Is(err, tickets.ErrNotFound) {
		t.Errorf("other guild = %v", err)
	}
	if n, _ := s.CountOpenTickets(ctx, 1, 100, p.ID, "abc"); n != 1 {
		t.Errorf("open count = %d", n)
	}

	if _, err := s.ReopenTicket(ctx, 1, a.ID); !errors.Is(err, tickets.ErrWrongStatus) {
		t.Errorf("reopen open = %v", err)
	}
	if got, err := s.ClaimTicket(ctx, 1, a.ID, 200, "mo"); err != nil || got.Status != tickets.StatusClaimed || got.ClaimedByName != "mo" {
		t.Errorf("claim = %+v, %v", got, err)
	}
	if _, err := s.ClaimTicket(ctx, 1, a.ID, 201, "x"); !errors.Is(err, tickets.ErrWrongStatus) {
		t.Errorf("claim twice = %v", err)
	}
	closed, err := s.CloseTicket(ctx, 1, a.ID, 200, "mo", "done")
	if err != nil || closed.Status != tickets.StatusClosed || closed.ClosedAt == nil || closed.CloseReason != "done" {
		t.Fatalf("close = %+v, %v", closed, err)
	}
	if n, _ := s.CountOpenTickets(ctx, 1, 100, p.ID, "abc"); n != 0 {
		t.Errorf("open count after close = %d", n)
	}
	if err := s.SaveTranscript(ctx, tickets.Transcript{TicketID: a.ID, GuildID: 1, HTMLGzip: []byte{1, 2}, Messages: 3, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if tr, err := s.GetTranscript(ctx, 1, a.ID); err != nil || tr.Messages != 3 || len(tr.HTMLGzip) != 2 {
		t.Errorf("transcript = %+v, %v", tr, err)
	}
	if _, err := s.GetTranscript(ctx, 2, a.ID); !errors.Is(err, tickets.ErrNoTranscript) {
		t.Errorf("transcript from other guild = %v", err)
	}
	if got, _ := s.GetTicketByNumber(ctx, 1, 1); !got.HasTranscript {
		t.Error("has_transcript not set")
	}
	// Reopening keeps the claim.
	if got, err := s.ReopenTicket(ctx, 1, a.ID); err != nil || got.Status != tickets.StatusClaimed || got.ClosedAt != nil {
		t.Errorf("reopen = %+v, %v", got, err)
	}
	list, err := s.ListTickets(ctx, 1, tickets.Filter{Limit: 10})
	if err != nil || len(list) != 2 || list[0].Number != 2 {
		t.Errorf("list = %+v, %v", list, err)
	}
	if list, _ := s.ListTickets(ctx, 1, tickets.Filter{Status: tickets.StatusOpen, Limit: 10}); len(list) != 1 || list[0].ID != b.ID {
		t.Errorf("open list = %+v", list)
	}
	if list, _ := s.ListTickets(ctx, 1, tickets.Filter{Before: 2, Limit: 10}); len(list) != 1 || list[0].Number != 1 {
		t.Errorf("page 2 = %+v", list)
	}

	if err := s.SaveTranscript(ctx, tickets.Transcript{TicketID: b.ID, GuildID: 1, HTMLGzip: []byte{1}, ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTranscript(ctx, 1, b.ID); !errors.Is(err, tickets.ErrNoTranscript) {
		t.Errorf("expired transcript served: %v", err)
	}
	if n, err := s.PurgeTranscripts(ctx, time.Now()); err != nil || n != 1 {
		t.Errorf("purge = %d, %v", n, err)
	}
	if err := s.DeleteTicket(ctx, 1, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTicket(ctx, 1, b.ID); !errors.Is(err, tickets.ErrNotFound) {
		t.Errorf("deleted ticket = %v", err)
	}
}
