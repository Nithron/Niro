package guardian

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/moderation"
)

var modActor = moderation.Actor{Rank: core.Rank{ID: 5, TopRolePosition: 40}, Name: "Mod"}

func scamMessage(h *harness, user snowflake.ID) Message {
	return Message{
		GuildID: testGuild, ChannelID: 11, ID: snowflake.New(h.now), AuthorID: user, AuthorName: "victim",
		Content: "https://evil.ru/nitro", CreatedAt: h.now,
	}
}

func quarantined(t *testing.T, h *harness, user snowflake.ID) Event {
	t.Helper()
	h.svc.d.Blocklist.Replace([]string{"evil.ru"})
	v, err := h.svc.HandleMessage(context.Background(), scamMessage(h, user))
	if err != nil || v.Response != ResponseQuarantine {
		t.Fatalf("verdict %+v, err %v", v, err)
	}
	ev := h.store.events[len(h.store.events)-1]
	if ev.Status != StatusOpen {
		t.Fatalf("event status %s", ev.Status)
	}
	return ev
}

func TestQuarantineRecordsCaseDMsAndAlerts(t *testing.T) {
	h := newHarness(DefaultSettings())
	u := h.addMember("victim", 400, 1)
	ev := quarantined(t, h, u)

	if until, ok := h.platform.timeouts[u]; !ok || until.Sub(h.now) != 7*24*time.Hour {
		t.Errorf("timeout = %v (%v)", until, ok)
	}
	if len(h.mod.cases) != 1 || h.mod.cases[0].Action != moderation.ActionQuarantine || h.mod.cases[0].Source != moderation.SourceGuardian {
		t.Errorf("cases = %+v", h.mod.cases)
	}
	if !strings.Contains(h.mod.cases[0].Reason, "blocked_domain (evil.ru)") {
		t.Errorf("reason = %q", h.mod.cases[0].Reason)
	}
	if ev.CaseNumber != 1 || len(h.alerts.dms) != 1 || len(h.alerts.posted) != 1 {
		t.Errorf("event %+v, dms %v, alerts %v", ev, h.alerts.dms, h.alerts.posted)
	}
	if got := h.store.events[0]; got.AlertChannelID != 77 || got.AlertMessageID == 0 {
		t.Errorf("alert reference not saved: %+v", got)
	}
	// Evidence never contains message text.
	b, _ := json.Marshal(h.store.events[0].Evidence)
	if strings.Contains(string(b), "nitro") {
		t.Errorf("evidence leaks message text: %s", b)
	}
}

func TestRestoreLiftsTimeout(t *testing.T) {
	h := newHarness(DefaultSettings())
	u := h.addMember("victim", 400, 1)
	ev := quarantined(t, h, u)

	got, err := h.svc.Resolve(context.Background(), testGuild, ev.ID, ResolveRestore, modActor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRestored || got.ResolvedBy != modActor.ID {
		t.Errorf("event = %+v", got)
	}
	if _, still := h.platform.timeouts[u]; still {
		t.Error("timeout not removed")
	}
	if _, err := h.store.ActiveQuarantine(context.Background(), testGuild, u); !errors.Is(err, ErrNotQuarantined) {
		t.Error("quarantine record not released")
	}
	last := h.mod.cases[len(h.mod.cases)-1]
	if last.Action != moderation.ActionUnquarantine || last.ModeratorID != modActor.ID {
		t.Errorf("restore case = %+v", last)
	}
	if len(h.alerts.edited) != 1 || h.alerts.edited[0].Status != StatusRestored {
		t.Errorf("alert not updated: %+v", h.alerts.edited)
	}
	if _, err := h.svc.Resolve(context.Background(), testGuild, ev.ID, ResolveBan, modActor); !errors.Is(err, ErrAlreadyResolved) {
		t.Errorf("second resolution: %v", err)
	}
}

func TestRoleQuarantineSavesAndRestoresRoles(t *testing.T) {
	s := DefaultSettings()
	s.QuarantineMode, s.QuarantineRoleID = QuarantineRole, 999
	h := newHarness(s)
	u := h.addMember("victim", 400, 1, 21, 22)
	ev := quarantined(t, h, u)

	if got := h.platform.members[u].RoleIDs; !slices.Equal(got, []snowflake.ID{999}) {
		t.Fatalf("roles while quarantined = %v", got)
	}
	if _, timedOut := h.platform.timeouts[u]; timedOut {
		t.Error("role mode must not time out")
	}
	if _, err := h.svc.Resolve(context.Background(), testGuild, ev.ID, ResolveRestore, modActor); err != nil {
		t.Fatal(err)
	}
	if got := h.platform.members[u].RoleIDs; !slices.Equal(got, []snowflake.ID{21, 22}) {
		t.Errorf("roles after restore = %v", got)
	}
}

func TestKeepThenBan(t *testing.T) {
	h := newHarness(DefaultSettings())
	u := h.addMember("victim", 400, 1)
	ev := quarantined(t, h, u)
	ctx := context.Background()

	if got, err := h.svc.Resolve(ctx, testGuild, ev.ID, ResolveKeep, modActor); err != nil || got.Status != StatusKept {
		t.Fatalf("keep: %+v %v", got, err)
	}
	if _, err := h.svc.Resolve(ctx, testGuild, ev.ID, ResolveKeep, modActor); !errors.Is(err, ErrAlreadyResolved) {
		t.Errorf("keeping twice: %v", err)
	}
	got, err := h.svc.Resolve(ctx, testGuild, ev.ID, ResolveBan, modActor)
	if err != nil || got.Status != StatusBanned {
		t.Fatalf("ban: %+v %v", got, err)
	}
	if len(h.mod.executed) != 1 || h.mod.executed[0].Action != moderation.ActionBan || h.mod.executed[0].TargetID != u ||
		h.mod.executed[0].Actor.ID != modActor.ID {
		t.Errorf("ban request = %+v", h.mod.executed)
	}
}

func TestCannotQuarantineHigherMember(t *testing.T) {
	h := newHarness(DefaultSettings())
	u := h.addMember("admin-ish", 400, 60) // above Niro's role (50)
	h.svc.d.Blocklist.Replace([]string{"evil.ru"})
	if _, err := h.svc.HandleMessage(context.Background(), scamMessage(h, u)); err != nil {
		t.Fatal(err)
	}
	if _, timedOut := h.platform.timeouts[u]; timedOut {
		t.Error("member above the bot was timed out")
	}
	ev := h.store.events[0]
	if ev.Status != StatusInfo || !slices.Contains(ev.Evidence.Problems, "bot_role_too_low") {
		t.Errorf("event = %+v", ev)
	}
	if len(h.platform.deleted) != 1 {
		t.Error("the message should still be deleted")
	}
}

func TestDeleteFailureIsReported(t *testing.T) {
	h := newHarness(DefaultSettings())
	h.platform.failDelete = true
	u := h.addMember("victim", 400, 1)
	quarantined(t, h, u)
	if ev := h.store.events[0]; !slices.Contains(ev.Evidence.Problems, "delete_failed") || ev.Evidence.Deleted != 0 {
		t.Errorf("evidence = %+v", ev.Evidence)
	}
}

func TestModuleOffDoesNothing(t *testing.T) {
	h := newHarness(DefaultSettings())
	h.enabled = false
	u := h.addMember("victim", 400, 1)
	h.svc.d.Blocklist.Replace([]string{"evil.ru"})
	v, err := h.svc.HandleMessage(context.Background(), scamMessage(h, u))
	if err != nil || v.Response != ResponseNone || len(h.store.events) != 0 {
		t.Errorf("verdict %+v err %v events %d", v, err, len(h.store.events))
	}
}

func TestExemptRole(t *testing.T) {
	s := DefaultSettings()
	s.ExemptRoleIDs = []snowflake.ID{42}
	h := newHarness(s)
	u := h.addMember("trusted", 400, 1, 42)
	m := scamMessage(h, u)
	m.AuthorRoleIDs = []snowflake.ID{42}
	h.svc.d.Blocklist.Replace([]string{"evil.ru"})
	if v, _ := h.svc.HandleMessage(context.Background(), m); v.Response != ResponseNone {
		t.Errorf("exempt member flagged: %+v", v)
	}
}

func TestRaidModeEndRestoresState(t *testing.T) {
	s := DefaultSettings()
	s.RaidSlowmodeSeconds = 30
	h := newHarness(s)
	h.platform.verification = 1
	h.platform.channels = []Channel{{ID: 11}, {ID: 12, Slowmode: 60}, {ID: 13, Slowmode: 5}}
	ctx := context.Background()

	r, err := h.svc.StartRaid(ctx, testGuild, modActor)
	if err != nil {
		t.Fatal(err)
	}
	if h.platform.verification != VerificationHigh || h.platform.invitesUntil.IsZero() {
		t.Errorf("raid not applied: verification %d invites %v", h.platform.verification, h.platform.invitesUntil)
	}
	if h.platform.invitesUntil.Sub(h.now) != 30*time.Minute {
		t.Errorf("invites paused until %v", h.platform.invitesUntil)
	}
	want := []Channel{{ID: 11, Slowmode: 30}, {ID: 12, Slowmode: 60}, {ID: 13, Slowmode: 30}}
	if !slices.Equal(h.platform.channels, want) {
		t.Errorf("slowmodes = %+v", h.platform.channels)
	}
	if _, err := h.svc.StartRaid(ctx, testGuild, modActor); !errors.Is(err, ErrRaidActive) {
		t.Errorf("second start: %v", err)
	}
	if r.EventID == 0 || h.store.events[0].Status != StatusActive {
		t.Errorf("raid event: %+v", h.store.events)
	}

	// An admin changes one channel by hand during the raid: leave it alone.
	h.platform.channels[0].Slowmode = 10

	// Raid mode ends on its own after its duration.
	h.now = h.now.Add(31 * time.Minute)
	h.svc.endDueRaids(ctx)
	if h.platform.verification != 1 || !h.platform.invitesUntil.IsZero() {
		t.Errorf("not restored: verification %d invites %v", h.platform.verification, h.platform.invitesUntil)
	}
	want = []Channel{{ID: 11, Slowmode: 10}, {ID: 12, Slowmode: 60}, {ID: 13, Slowmode: 5}}
	if !slices.Equal(h.platform.channels, want) {
		t.Errorf("slowmodes after end = %+v", h.platform.channels)
	}
	if h.store.events[0].Status != StatusEnded {
		t.Errorf("raid event status %s", h.store.events[0].Status)
	}
	if _, err := h.svc.EndRaid(ctx, testGuild, modActor); !errors.Is(err, ErrNoRaid) {
		t.Errorf("ending twice: %v", err)
	}
}

func TestSettingsNormalize(t *testing.T) {
	out, err := SettingsSpec{}.Normalize(nil)
	if err != nil {
		t.Fatal(err)
	}
	var s Settings
	_ = json.Unmarshal(out, &s)
	if s.Mode != ModeEnforce || s.BurstChannels != 3 || s.QuarantineMode != QuarantineTimeout || s.AllowDomains == nil {
		t.Errorf("defaults = %+v", s)
	}

	out, err = SettingsSpec{}.Normalize([]byte(`{"allow_domains": ["https://Example.com/path", "example.com", " "], "deny_domains": ["evil.ru"]}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(out, &s)
	if !slices.Equal(s.AllowDomains, []string{"example.com"}) {
		t.Errorf("allow = %v", s.AllowDomains)
	}

	bad := []string{
		`{"mode": "yolo"}`,
		`{"burst_channels": 1}`,
		`{"quarantine_mode": "role"}`,
		`{"quarantine_hours": 1000}`,
		`{"quiet_days": 3}`,
		`{"allow_domains": ["not a domain"]}`,
		`{"allow_domains": ["a.com"], "deny_domains": ["a.com"]}`,
		`{"unknown": true}`,
		`{"account_age_action": "ban"}`,
	}
	for _, b := range bad {
		if _, err := (SettingsSpec{}).Normalize([]byte(b)); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s: err = %v", b, err)
		}
	}
}

func TestCanVerify(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	s := DefaultSettings()
	s.VerifyMinAccountAgeHours = 48
	young := snowflake.New(now.Add(-24 * time.Hour))
	ok, wait := CanVerify(s, young, now)
	if ok || wait.Sub(now).Round(time.Hour) != 24*time.Hour {
		t.Errorf("young: %v %v", ok, wait)
	}
	if ok, _ := CanVerify(s, snowflake.New(now.Add(-72*time.Hour)), now); !ok {
		t.Error("old account rejected")
	}
}

func TestAudit(t *testing.T) {
	f := Audit(AuditInput{
		EveryonePermissions: []string{PermMentionEveryone, "send_messages"},
		Roles: []AuditRole{
			{ID: 1, Name: "Admin", Permissions: []string{PermAdministrator}, Members: 2},
			{ID: 2, Name: "Helper", Permissions: []string{PermManageMessages}}, // not critical: not reported per role
			{ID: 3, Name: "Mods", Permissions: []string{PermBanMembers, PermKickMembers}},
			{ID: 4, Name: "SomeBot", Permissions: []string{PermAdministrator}, Managed: true},
		},
		Bots: []AuditBot{
			{ID: 10, Name: "SomeBot", Permissions: []string{PermAdministrator}},
			{ID: 11, Name: "Niro", Permissions: []string{PermBanMembers}, Self: true},
		},
		Webhooks:          []AuditWebhook{{ID: 1, ChannelID: 5}, {ID: 2, ChannelID: 5}},
		VerificationLevel: 0,
	})
	var codes []string
	for _, x := range f {
		codes = append(codes, string(x.Severity)+":"+x.Code)
	}
	want := []string{
		"high:everyone_dangerous", "high:role_admin", "high:bot_admin",
		"medium:role_dangerous", "medium:verification_none", "medium:no_moderator_mfa", "info:webhooks",
	}
	if !slices.Equal(codes, want) {
		t.Errorf("findings = %v\nwant       %v", codes, want)
	}
	if f[len(f)-1].Count != 2 || len(f[len(f)-1].ChannelIDs) != 1 {
		t.Errorf("webhooks finding = %+v", f[len(f)-1])
	}

	if got := Audit(AuditInput{EveryonePermissions: []string{PermAdministrator}, VerificationLevel: 2, ModeratorMFA: true}); len(got) != 1 || got[0].Severity != SeverityCritical {
		t.Errorf("@everyone admin = %+v", got)
	}
}

func TestFeedUpdater(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	st := newFakeStore(func() time.Time { return now })
	set := NewDomainSet()
	if _, err := st.ReplaceFeedDomains(context.Background(), "src", []string{"evil.ru"}); err != nil {
		t.Fatal(err)
	}
	f := &FeedUpdater{Store: st, Set: set, Log: discardLogger()}
	f.reload(context.Background())
	if set.Match("x.evil.ru") != "evil.ru" {
		t.Error("stored domains not loaded")
	}
	if _, err := f.update(context.Background(), "http://insecure.example/list"); err == nil {
		t.Error("plain http feed accepted")
	}
}
