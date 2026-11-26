package store_test

import (
	"testing"
	"time"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/modules/roles"
)

func TestLeftGuildsArePurgedAfterTheGracePeriod(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	for _, id := range []int64{1, 2, 3} {
		if err := s.UpsertGuild(ctx, core.GuildInfo{ID: snow(id), Name: "G", OwnerID: 9}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreatePanel(ctx, roles.Panel{GuildID: snow(id), Name: "P", ChannelID: 5, Message: message.Simple("x"),
			Style: roles.StyleButtons, Mode: roles.ModeToggle, Roles: []roles.PanelRole{{RoleID: 10, Label: "R"}}}); err != nil {
			t.Fatal(err)
		}
	}
	// Niro leaves 1 and 3; 3 comes back.
	for _, id := range []int64{1, 3} {
		if err := s.MarkGuildLeft(ctx, snow(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertGuild(ctx, core.GuildInfo{ID: 3, Name: "G", OwnerID: 9}); err != nil {
		t.Fatal(err)
	}

	// Still inside the grace period: nothing goes.
	if n, err := s.PurgeLeftGuilds(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("purge inside the grace period = (%d, %v)", n, err)
	}
	// Past it (the cutoff is after the leave): server 1 and its panels go.
	if n, err := s.PurgeLeftGuilds(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("purge = (%d, %v), want 1", n, err)
	}
	for id, want := range map[int64]int{1: 0, 2: 1, 3: 1} {
		panels, err := s.ListPanels(ctx, snow(id))
		if err != nil || len(panels) != want {
			t.Errorf("server %d has %d panels (%v), want %d", id, len(panels), err, want)
		}
	}
}

func TestDeleteGuildNow(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	if err := s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 9}); err != nil {
		t.Fatal(err)
	}
	actor := snow(9)
	if _, err := s.SetModuleEnabled(ctx, 1, core.ModuleLogging, true, &actor, core.SourceDashboard); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.DeleteGuild(ctx, 1); err != nil || !ok {
		t.Fatalf("delete = (%v, %v)", ok, err)
	}
	if ok, err := s.DeleteGuild(ctx, 1); err != nil || ok {
		t.Fatalf("second delete = (%v, %v), want (false, nil)", ok, err)
	}
	if audit, err := s.ListConfigAudit(ctx, 1, 10); err != nil || len(audit) != 0 {
		t.Fatalf("audit left behind: %d (%v)", len(audit), err)
	}
}

func TestRetentionPurges(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	if err := s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 9}); err != nil {
		t.Fatal(err)
	}
	actor := snow(9)
	if _, err := s.SetModuleEnabled(ctx, 1, core.ModuleLogging, true, &actor, core.SourceDashboard); err != nil {
		t.Fatal(err)
	}
	if n, err := s.PurgeConfigAudit(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("recent audit purged: (%d, %v)", n, err)
	}
	if n, err := s.PurgeConfigAudit(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("old audit purge = (%d, %v), want 1", n, err)
	}
	for _, purge := range []func() (int64, error){
		func() (int64, error) { return s.PurgeDoneScheduledActions(ctx, time.Now()) },
		func() (int64, error) { return s.PurgeEndedRaids(ctx, time.Now()) },
	} {
		if _, err := purge(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSchedulerSkipsServersNiroLeft(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	due := time.Now().Add(-time.Minute)
	for _, id := range []int64{1, 2} {
		if err := s.UpsertGuild(ctx, core.GuildInfo{ID: snow(id), Name: "G", OwnerID: 9}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateCase(ctx, moderation.NewCase{GuildID: snow(id), Action: moderation.ActionBan, TargetID: 50,
			ModeratorID: 9, Source: moderation.SourceManual, ScheduleUnbanAt: &due}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkGuildLeft(ctx, 2); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimDueActions(ctx, 10)
	if err != nil || len(claimed) != 1 || claimed[0].GuildID != 1 {
		t.Fatalf("claimed = %+v (%v), want only server 1's action", claimed, err)
	}
}
