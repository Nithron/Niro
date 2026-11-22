package store_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/moderation"
)

func TestGuardianEvents(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	if err := s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	ev, err := s.CreateEvent(ctx, guardian.NewEvent{
		GuildID: 1, UserID: 100, UserName: "victim", Kind: guardian.KindCompromised, Score: 3,
		Signals:  []guardian.Signal{{Kind: guardian.SignalBurst, Detail: "l", Weight: 3}},
		Response: guardian.ResponseQuarantine, Status: guardian.StatusOpen, CaseNumber: 4,
		Evidence: guardian.Evidence{Channels: []snowflake.ID{11, 12, 13}, Messages: 3, Domains: []string{"evil.ru"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == 0 || ev.CaseNumber != 4 || len(ev.Signals) != 1 || len(ev.Evidence.Channels) != 3 {
		t.Fatalf("created = %+v", ev)
	}
	if err := s.SetEventAlert(ctx, 1, ev.ID, 77, 88); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEvent(ctx, 1, ev.ID)
	if err != nil || got.AlertChannelID != 77 || got.AlertMessageID != 88 {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := s.GetEvent(ctx, 2, ev.ID); !errors.Is(err, guardian.ErrEventNotFound) {
		t.Fatalf("other guild: %v", err)
	}

	info, _ := s.CreateEvent(ctx, guardian.NewEvent{GuildID: 1, Kind: guardian.KindScam, Response: guardian.ResponseAlert, Status: guardian.StatusInfo})
	open, _ := s.ListEvents(ctx, 1, guardian.EventFilter{Status: guardian.StatusOpen, Limit: 10})
	if len(open) != 1 || open[0].ID != ev.ID {
		t.Fatalf("open = %+v", open)
	}
	all, _ := s.ListEvents(ctx, 1, guardian.EventFilter{Limit: 10})
	if len(all) != 2 || all[0].ID != info.ID {
		t.Fatalf("all = %+v", all)
	}
	page, _ := s.ListEvents(ctx, 1, guardian.EventFilter{Before: info.ID, Limit: 10})
	if len(page) != 1 || page[0].ID != ev.ID {
		t.Fatalf("cursor = %+v", page)
	}

	r, err := s.ResolveEvent(ctx, 1, ev.ID, guardian.StatusKept, 5, "Mod")
	if err != nil || r.Status != guardian.StatusKept || r.ResolvedBy != 5 || r.ResolvedAt == nil {
		t.Fatalf("keep = %+v, %v", r, err)
	}
	if r, err = s.ResolveEvent(ctx, 1, ev.ID, guardian.StatusRestored, 5, "Mod"); err != nil || r.Status != guardian.StatusRestored {
		t.Fatalf("restore after keep = %+v, %v", r, err)
	}
	if _, err := s.ResolveEvent(ctx, 1, ev.ID, guardian.StatusBanned, 5, "Mod"); !errors.Is(err, guardian.ErrAlreadyResolved) {
		t.Fatalf("second resolve: %v", err)
	}
	if _, err := s.ResolveEvent(ctx, 1, 99999, guardian.StatusBanned, 5, "Mod"); !errors.Is(err, guardian.ErrEventNotFound) {
		t.Fatalf("missing event: %v", err)
	}

	// Retention keeps events that still need a decision.
	stillOpen, _ := s.CreateEvent(ctx, guardian.NewEvent{GuildID: 1, Kind: guardian.KindScam, Response: guardian.ResponseQuarantine, Status: guardian.StatusOpen})
	n, err := s.PurgeEventsBefore(ctx, time.Now().Add(time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("purged %d, %v", n, err)
	}
	if _, err := s.GetEvent(ctx, 1, stillOpen.ID); err != nil {
		t.Fatalf("open event purged: %v", err)
	}
}

func TestQuarantineCaseAction(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	_ = s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 1})
	for _, a := range []moderation.Action{moderation.ActionQuarantine, moderation.ActionUnquarantine} {
		if _, err := s.CreateCase(ctx, moderation.NewCase{GuildID: 1, Action: a, TargetID: 2, ModeratorID: 3, Source: moderation.SourceGuardian}); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
}

func TestQuarantineRecords(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	_ = s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 1})

	if _, err := s.ActiveQuarantine(ctx, 1, 100); !errors.Is(err, guardian.ErrNotQuarantined) {
		t.Fatalf("none yet: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	q := guardian.Quarantine{GuildID: 1, UserID: 100, Mode: guardian.QuarantineRole, RoleID: 9, SavedRoleIDs: []snowflake.ID{21, 22}, CreatedAt: now}
	if err := s.SaveQuarantine(ctx, q); err != nil {
		t.Fatal(err)
	}
	// A second quarantine must not overwrite the roles saved by the first
	// (by then the member only has the quarantine role).
	q2 := q
	q2.SavedRoleIDs = nil
	if err := s.SaveQuarantine(ctx, q2); err != nil {
		t.Fatal(err)
	}
	got, err := s.ActiveQuarantine(ctx, 1, 100)
	if err != nil || !slices.Equal(got.SavedRoleIDs, []snowflake.ID{21, 22}) || got.RoleID != 9 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := s.ReleaseQuarantine(ctx, 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseQuarantine(ctx, 1, 100); !errors.Is(err, guardian.ErrNotQuarantined) {
		t.Fatalf("release twice: %v", err)
	}
}

func TestRaids(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	_ = s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 1})
	now := time.Now().UTC()

	r, started, err := s.StartRaid(ctx, guardian.Raid{GuildID: 1, StartedAt: now, EndsAt: now.Add(-time.Second), Joins: 12})
	if err != nil || !started || r.ID == 0 {
		t.Fatalf("start = %+v %v %v", r, started, err)
	}
	again, started, err := s.StartRaid(ctx, guardian.Raid{GuildID: 1, StartedAt: now, EndsAt: now.Add(time.Hour)})
	if err != nil || started || again.ID != r.ID {
		t.Fatalf("second start = %+v %v %v", again, started, err)
	}
	lvl := 1
	st := guardian.RaidState{InvitesPaused: true, PrevVerification: &lvl, SetVerification: 3, PrevSlowmodes: map[snowflake.ID]int{11: 0}}
	if err := s.UpdateRaid(ctx, 1, r.ID, 0, st); err != nil {
		t.Fatal(err)
	}
	active, err := s.ActiveRaid(ctx, 1)
	if err != nil || !active.State.InvitesPaused || *active.State.PrevVerification != 1 || active.State.PrevSlowmodes[11] != 0 {
		t.Fatalf("active = %+v, %v", active, err)
	}
	due, _ := s.DueRaids(ctx, now)
	if len(due) != 1 {
		t.Fatalf("due = %+v", due)
	}
	ended, err := s.EndRaid(ctx, 1, 5)
	if err != nil || ended.EndedAt == nil || ended.EndedBy != 5 {
		t.Fatalf("end = %+v, %v", ended, err)
	}
	if _, err := s.EndRaid(ctx, 1, 5); !errors.Is(err, guardian.ErrNoRaid) {
		t.Fatalf("end twice: %v", err)
	}
	if _, err := s.ActiveRaid(ctx, 1); !errors.Is(err, guardian.ErrNoRaid) {
		t.Fatalf("active after end: %v", err)
	}
	if _, started, _ := s.StartRaid(ctx, guardian.Raid{GuildID: 1, StartedAt: now, EndsAt: now.Add(time.Hour)}); !started {
		t.Fatal("a new raid can start after the last one ended")
	}
}

func TestLockdownsAndFeeds(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	_ = s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 1})
	now := time.Now()
	if err := s.SaveLockdowns(ctx, []guardian.LockState{
		{GuildID: 1, ChannelID: 11, HadOverwrite: true, Allow: 1024, Deny: 2048, By: 5, CreatedAt: now},
		{GuildID: 1, ChannelID: 12, CreatedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	// Locking again keeps the first saved state.
	_ = s.SaveLockdowns(ctx, []guardian.LockState{{GuildID: 1, ChannelID: 11, Allow: 0, Deny: 4096, CreatedAt: now}})
	locks, _ := s.Lockdowns(ctx, 1)
	if len(locks) != 2 || locks[0].Deny != 2048 || !locks[0].HadOverwrite {
		t.Fatalf("locks = %+v", locks)
	}
	_ = s.DeleteLockdowns(ctx, 1, []snowflake.ID{11})
	if locks, _ = s.Lockdowns(ctx, 1); len(locks) != 1 || locks[0].ChannelID != 12 {
		t.Fatalf("after delete = %+v", locks)
	}

	if at, err := s.FeedUpdatedAt(ctx, "a"); err != nil || !at.IsZero() {
		t.Fatalf("never fetched = %v %v", at, err)
	}
	if n, err := s.ReplaceFeedDomains(ctx, "a", []string{"evil.ru", "bad.com"}); err != nil || n != 2 {
		t.Fatalf("replace = %d %v", n, err)
	}
	_, _ = s.ReplaceFeedDomains(ctx, "b", []string{"evil.ru", "other.net"})
	_, _ = s.ReplaceFeedDomains(ctx, "a", []string{"evil.ru"}) // bad.com removed from feed a
	domains, _ := s.ScamDomains(ctx)
	slices.Sort(domains)
	if !slices.Equal(domains, []string{"evil.ru", "other.net"}) {
		t.Fatalf("domains = %v", domains)
	}
	if at, _ := s.FeedUpdatedAt(ctx, "a"); at.IsZero() {
		t.Fatal("feed time not recorded")
	}
}
