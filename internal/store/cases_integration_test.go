package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/modules/moderation"
)

func TestCaseLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	for _, id := range []int64{1, 2} {
		if err := s.UpsertGuild(ctx, core.GuildInfo{ID: snow(id), Name: "G", OwnerID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	mk := func(g int64, a moderation.Action, target int64) moderation.Case {
		t.Helper()
		c, err := s.CreateCase(ctx, moderation.NewCase{GuildID: snow(g), Action: a, TargetID: snow(target),
			TargetName: "u", ModeratorID: 9, ModeratorName: "mod", Reason: "r", Source: moderation.SourceManual})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := mk(1, moderation.ActionWarn, 100); c.Number != 1 {
		t.Fatalf("first case = %d", c.Number)
	}
	mk(1, moderation.ActionNote, 100)
	mk(1, moderation.ActionWarn, 200)
	if c := mk(2, moderation.ActionWarn, 100); c.Number != 1 {
		t.Fatalf("numbering must be per guild, got %d", c.Number)
	}

	list, err := s.ListCases(ctx, snow(1), moderation.CaseFilter{TargetID: 100, Limit: 10})
	if err != nil || len(list) != 2 || list[0].Number != 2 {
		t.Fatalf("list by target = %+v, %v", list, err)
	}
	if n, _ := s.CountCases(ctx, snow(1), moderation.CaseFilter{Action: moderation.ActionWarn}); n != 2 {
		t.Fatalf("count warns = %d", n)
	}
	page, _ := s.ListCases(ctx, snow(1), moderation.CaseFilter{Before: 3, Limit: 1})
	if len(page) != 1 || page[0].Number != 2 {
		t.Fatalf("cursor page = %+v", page)
	}

	c, err := s.UpdateCaseReason(ctx, snow(1), 1, "better reason")
	if err != nil || c.Reason != "better reason" {
		t.Fatalf("update reason = %+v, %v", c, err)
	}
	if err := s.SetCaseLogMessage(ctx, snow(1), 1, 55, 66); err != nil {
		t.Fatal(err)
	}
	if c, _ = s.GetCase(ctx, snow(1), 1); c.LogChannelID != 55 || c.LogMessageID != 66 {
		t.Fatalf("log refs = %+v", c)
	}
	if c, err = s.DeleteCase(ctx, snow(1), 1, 9); err != nil || !c.Deleted {
		t.Fatalf("delete = %+v, %v", c, err)
	}
	if _, err := s.GetCase(ctx, snow(1), 1); !errors.Is(err, moderation.ErrCaseNotFound) {
		t.Fatalf("deleted case visible: %v", err)
	}
	if _, err := s.DeleteCase(ctx, snow(1), 1, 9); !errors.Is(err, moderation.ErrCaseNotFound) {
		t.Fatalf("double delete: %v", err)
	}
}

func TestScheduledActions(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	if err := s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Hour)
	for _, at := range []time.Time{past, future} {
		if _, err := s.CreateCase(ctx, moderation.NewCase{GuildID: 1, Action: moderation.ActionBan, TargetID: 100,
			ModeratorID: 9, Source: moderation.SourceManual, Duration: time.Hour, ExpiresAt: &at, ScheduleUnbanAt: &at}); err != nil {
			t.Fatal(err)
		}
	}
	due, err := s.ClaimDueActions(ctx, 10)
	if err != nil || len(due) != 1 || due[0].Kind != "unban" || due[0].TargetID != 100 || due[0].CaseNumber != 1 || due[0].Attempts != 1 {
		t.Fatalf("claim = %+v, %v", due, err)
	}
	if again, _ := s.ClaimDueActions(ctx, 10); len(again) != 0 {
		t.Fatalf("claimed action must be leased, got %+v", again)
	}
	if err := s.CompleteScheduledAction(ctx, due[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelScheduledUnbans(ctx, 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryScheduledAction(ctx, due[0].ID, "boom", time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestModuleSettingsAudit(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	if err := s.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	st, err := s.SetModuleSettings(ctx, 1, core.ModuleModeration, []byte(`{"dm_users":false}`), true, nil, core.SourceDashboard)
	if err != nil || !st.Enabled || string(st.Config) != `{"dm_users": false}` && string(st.Config) != `{"dm_users":false}` {
		t.Fatalf("settings = %+v (%s), %v", st, st.Config, err)
	}
	audit, _ := s.ListConfigAudit(ctx, 1, 5)
	if len(audit) != 1 {
		t.Fatalf("audit entries = %d", len(audit))
	}
}
