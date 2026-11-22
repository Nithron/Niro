package moderation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
)

// ---- fakes ----------------------------------------------------------------

type memStore struct {
	cases     []Case
	scheduled []ScheduledAction
	runAt     map[int64]time.Time
	done      map[int64]string
	cancelled int
}

func newMemStore() *memStore {
	return &memStore{runAt: map[int64]time.Time{}, done: map[int64]string{}}
}

func (m *memStore) CreateCase(_ context.Context, nc NewCase) (Case, error) {
	n := 1
	for _, c := range m.cases {
		if c.GuildID == nc.GuildID {
			n++
		}
	}
	c := previewCase(nc, time.Unix(1000, 0))
	c.Number, c.DMSent = n, nc.DMSent
	m.cases = append(m.cases, c)
	if nc.ScheduleUnbanAt != nil {
		id := int64(len(m.scheduled) + 1)
		m.scheduled = append(m.scheduled, ScheduledAction{ID: id, GuildID: nc.GuildID, Kind: "unban", TargetID: nc.TargetID, CaseNumber: n})
		m.runAt[id] = *nc.ScheduleUnbanAt
	}
	return c, nil
}

func (m *memStore) find(g snowflake.ID, n int) (int, error) {
	i := slices.IndexFunc(m.cases, func(c Case) bool { return c.GuildID == g && c.Number == n && !c.Deleted })
	if i < 0 {
		return 0, ErrCaseNotFound
	}
	return i, nil
}

func (m *memStore) GetCase(_ context.Context, g snowflake.ID, n int) (Case, error) {
	i, err := m.find(g, n)
	if err != nil {
		return Case{}, err
	}
	return m.cases[i], nil
}

func (m *memStore) ListCases(_ context.Context, g snowflake.ID, f CaseFilter) ([]Case, error) {
	var out []Case
	for i := len(m.cases) - 1; i >= 0; i-- {
		c := m.cases[i]
		if c.GuildID == g && !c.Deleted && (f.TargetID == 0 || c.TargetID == f.TargetID) && (f.Action == "" || c.Action == f.Action) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memStore) CountCases(ctx context.Context, g snowflake.ID, f CaseFilter) (int, error) {
	c, _ := m.ListCases(ctx, g, f)
	return len(c), nil
}

func (m *memStore) UpdateCaseReason(_ context.Context, g snowflake.ID, n int, reason string) (Case, error) {
	i, err := m.find(g, n)
	if err != nil {
		return Case{}, err
	}
	m.cases[i].Reason = reason
	return m.cases[i], nil
}

func (m *memStore) DeleteCase(_ context.Context, g snowflake.ID, n int, _ snowflake.ID) (Case, error) {
	i, err := m.find(g, n)
	if err != nil {
		return Case{}, err
	}
	m.cases[i].Deleted = true
	return m.cases[i], nil
}

func (m *memStore) SetCaseLogMessage(_ context.Context, g snowflake.ID, n int, ch, msg snowflake.ID) error {
	i, err := m.find(g, n)
	if err == nil {
		m.cases[i].LogChannelID, m.cases[i].LogMessageID = ch, msg
	}
	return err
}

func (m *memStore) CancelScheduledUnbans(context.Context, snowflake.ID, snowflake.ID) error {
	m.cancelled++
	return nil
}

func (m *memStore) ClaimDueActions(context.Context, int) ([]ScheduledAction, error) {
	var out []ScheduledAction
	for i, a := range m.scheduled {
		if _, done := m.done[a.ID]; !done && !m.runAt[a.ID].After(time.Unix(1_000_000, 0)) {
			m.scheduled[i].Attempts++
			out = append(out, m.scheduled[i])
		}
	}
	return out, nil
}

func (m *memStore) CompleteScheduledAction(_ context.Context, id int64, note string) error {
	m.done[id] = note
	return nil
}

func (m *memStore) RetryScheduledAction(context.Context, int64, string, time.Duration) error {
	return nil
}

type fakePlatform struct {
	members  map[snowflake.ID]Member
	bot      Member
	banned   map[snowflake.ID]bool
	calls    []string
	fail     error
	messages []Message
	deleted  []snowflake.ID
}

func (p *fakePlatform) Member(_ context.Context, _, u snowflake.ID) (Member, error) {
	m, ok := p.members[u]
	if !ok {
		return Member{}, ErrNotMember
	}
	return m, nil
}
func (p *fakePlatform) BotMember(context.Context, snowflake.ID) (Member, error) { return p.bot, nil }
func (p *fakePlatform) record(s string) error {
	p.calls = append(p.calls, s)
	return p.fail
}
func (p *fakePlatform) Timeout(_ context.Context, _, _ snowflake.ID, _ time.Time, _ string) error {
	return p.record("timeout")
}
func (p *fakePlatform) RemoveTimeout(context.Context, snowflake.ID, snowflake.ID, string) error {
	return p.record("untimeout")
}
func (p *fakePlatform) Kick(context.Context, snowflake.ID, snowflake.ID, string) error {
	return p.record("kick")
}
func (p *fakePlatform) Ban(_ context.Context, _, u snowflake.ID, _ time.Duration, _ string) error {
	p.banned[u] = true
	return p.record("ban")
}
func (p *fakePlatform) Unban(_ context.Context, _, u snowflake.ID, _ string) error {
	if !p.banned[u] {
		return ErrNotBanned
	}
	delete(p.banned, u)
	return p.record("unban")
}
func (p *fakePlatform) RecentMessages(_ context.Context, _, before snowflake.ID, limit int) ([]Message, error) {
	start := 0
	if before != 0 {
		start = slices.IndexFunc(p.messages, func(m Message) bool { return m.ID == before }) + 1
	}
	end := min(start+limit, len(p.messages))
	return p.messages[start:end], nil
}
func (p *fakePlatform) DeleteMessages(_ context.Context, _ snowflake.ID, ids []snowflake.ID, _ string) error {
	p.deleted = append(p.deleted, ids...)
	return p.record("delete")
}

type fakeNotifier struct {
	p       *fakePlatform // to record ordering relative to actions
	dmFail  error
	logged  []Case
	edited  []Case
	nextMsg snowflake.ID
}

func (n *fakeNotifier) NotifyUser(_ context.Context, c Case, _ string) error {
	n.p.calls = append(n.p.calls, "dm")
	return n.dmFail
}
func (n *fakeNotifier) PostLog(_ context.Context, _ snowflake.ID, c Case) (snowflake.ID, error) {
	n.logged = append(n.logged, c)
	n.nextMsg++
	return 9000 + n.nextMsg, nil
}
func (n *fakeNotifier) EditLog(_ context.Context, _, _ snowflake.ID, c Case) error {
	n.edited = append(n.edited, c)
	return nil
}

// ---- harness --------------------------------------------------------------

const (
	g        snowflake.ID = 1
	ownerID  snowflake.ID = 10
	modID    snowflake.ID = 20
	memberID snowflake.ID = 30
	adminID  snowflake.ID = 40
	botID    snowflake.ID = 99
	strayID  snowflake.ID = 50 // not a member
)

type env struct {
	svc      *Service
	store    *memStore
	plat     *fakePlatform
	notif    *fakeNotifier
	settings Settings
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{store: newMemStore(), settings: Settings{DMUsers: true, LogChannelID: 777}}
	e.plat = &fakePlatform{
		members: map[snowflake.ID]Member{
			ownerID:  {Rank: core.Rank{ID: ownerID, IsOwner: true}, Name: "owner"},
			modID:    {Rank: core.Rank{ID: modID, TopRolePosition: 5}, Name: "mod"},
			memberID: {Rank: core.Rank{ID: memberID, TopRolePosition: 1}, Name: "member"},
			adminID:  {Rank: core.Rank{ID: adminID, TopRolePosition: 9}, Name: "admin"},
		},
		bot:    Member{Rank: core.Rank{ID: botID, TopRolePosition: 8}, Name: "Niro"},
		banned: map[snowflake.ID]bool{},
	}
	e.notif = &fakeNotifier{p: e.plat}
	e.svc = NewService(e.store, e.plat, e.notif,
		func(context.Context, snowflake.ID) (Settings, error) { return e.settings, nil },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.svc.now = func() time.Time { return time.Unix(1000, 0) }
	return e
}

func mod() Actor { return Actor{Rank: core.Rank{ID: modID, TopRolePosition: 5}, Name: "mod"} }

func req(a Action, target snowflake.ID) Request {
	return Request{GuildID: g, GuildName: "Test", Action: a, Actor: mod(), TargetID: target, Reason: "spam"}
}

// ---- tests ----------------------------------------------------------------

func TestWarnRecordsCaseDMsAndLogs(t *testing.T) {
	e := newEnv(t)
	out, err := e.svc.Execute(context.Background(), req(ActionWarn, memberID))
	if err != nil {
		t.Fatal(err)
	}
	c := out.Case
	if c.Number != 1 || c.Action != ActionWarn || c.TargetName != "member" || c.ModeratorName != "mod" || !c.DMSent {
		t.Fatalf("case = %+v", c)
	}
	if !out.LogPosted || c.LogMessageID == 0 || len(e.notif.logged) != 1 {
		t.Fatalf("log not posted: %+v", out)
	}
	if !slices.Equal(e.plat.calls, []string{"dm"}) {
		t.Fatalf("calls = %v", e.plat.calls)
	}
	out2, _ := e.svc.Execute(context.Background(), req(ActionWarn, memberID))
	if out2.Case.Number != 2 {
		t.Fatalf("second case number = %d, want 2", out2.Case.Number)
	}
}

func TestKickAndBanNotifyBeforeActing(t *testing.T) {
	for _, a := range []Action{ActionKick, ActionBan} {
		e := newEnv(t)
		if _, err := e.svc.Execute(context.Background(), req(a, memberID)); err != nil {
			t.Fatal(err)
		}
		if len(e.plat.calls) != 2 || e.plat.calls[0] != "dm" || e.plat.calls[1] != string(a) {
			t.Fatalf("%s: calls = %v, want DM first", a, e.plat.calls)
		}
	}
}

func TestTimeoutNotifiesAfterActing(t *testing.T) {
	e := newEnv(t)
	r := req(ActionTimeout, memberID)
	r.Duration = time.Hour
	out, err := e.svc.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(e.plat.calls, []string{"timeout", "dm"}) {
		t.Fatalf("calls = %v", e.plat.calls)
	}
	if out.Case.ExpiresAt == nil || !out.Case.ExpiresAt.Equal(time.Unix(1000, 0).Add(time.Hour)) {
		t.Fatalf("expires = %v", out.Case.ExpiresAt)
	}
}

func TestHierarchyAndValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	timeout := func(target snowflake.ID, d time.Duration) Request {
		r := req(ActionTimeout, target)
		r.Duration = d
		return r
	}
	cases := []struct {
		name string
		r    Request
		want error
	}{
		{"self", req(ActionKick, modID), core.ErrTargetIsSelf},
		{"owner", req(ActionBan, ownerID), core.ErrTargetIsOwner},
		{"higher role", req(ActionKick, adminID), core.ErrActorRankTooLow},
		{"kick non-member", req(ActionKick, strayID), ErrNotMember},
		{"timeout too short", timeout(memberID, time.Second), ErrBadDuration},
		{"timeout too long", timeout(memberID, 29*24*time.Hour), ErrBadDuration},
		{"note needs reason", Request{GuildID: g, Action: ActionNote, Actor: mod(), TargetID: memberID}, ErrReasonRequired},
		{"reason too long", Request{GuildID: g, Action: ActionWarn, Actor: mod(), TargetID: memberID, Reason: strings.Repeat("x", 513)}, ErrReasonTooLong},
		{"purge via Execute", req(ActionPurge, memberID), ErrBadAction},
		{"untimeout not timed out", req(ActionUntimeout, memberID), ErrNotTimedOut},
		{"unban not banned", req(ActionUnban, strayID), ErrNotBanned},
	}
	for _, c := range cases {
		_, err := e.svc.Execute(ctx, c.r)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if len(e.store.cases) != 0 {
		t.Fatalf("failed actions must not create cases: %+v", e.store.cases)
	}
}

func TestWarnIgnoresBotRank(t *testing.T) {
	e := newEnv(t)
	e.plat.bot.TopRolePosition = 0 // bot below everyone
	if _, err := e.svc.Execute(context.Background(), req(ActionWarn, memberID)); err != nil {
		t.Fatalf("warn should not need the bot's rank: %v", err)
	}
	if _, err := e.svc.Execute(context.Background(), req(ActionKick, memberID)); !errors.Is(err, core.ErrBotRankTooLow) {
		t.Fatalf("kick with low bot rank: %v", err)
	}
}

func TestBanByIDOfNonMember(t *testing.T) {
	e := newEnv(t)
	r := req(ActionBan, strayID)
	r.TargetName = "stray#0"
	out, err := e.svc.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Case.TargetName != "stray#0" || out.DMSent {
		t.Fatalf("ban of non-member: %+v (should not DM)", out)
	}
}

func TestRequireReasonSetting(t *testing.T) {
	e := newEnv(t)
	e.settings.RequireReason = true
	r := req(ActionWarn, memberID)
	r.Reason = "  "
	if _, err := e.svc.Execute(context.Background(), r); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestNotifyOverrideAndDMFailure(t *testing.T) {
	e := newEnv(t)
	off := false
	r := req(ActionWarn, memberID)
	r.Notify = &off
	out, _ := e.svc.Execute(context.Background(), r)
	if out.DMSent || slices.Contains(e.plat.calls, "dm") {
		t.Fatal("silent action sent a DM")
	}
	e.notif.dmFail = errors.New("cannot send messages to this user")
	out, err := e.svc.Execute(context.Background(), req(ActionWarn, memberID))
	if err != nil || out.DMSent {
		t.Fatalf("DM failure must not fail the action: out=%+v err=%v", out, err)
	}
}

func TestPlatformFailureCreatesNoCase(t *testing.T) {
	e := newEnv(t)
	e.plat.fail = errors.New("missing permissions")
	_, err := e.svc.Execute(context.Background(), req(ActionKick, memberID))
	var ae *ActionError
	if !errors.As(err, &ae) || ae.Action != ActionKick {
		t.Fatalf("err = %v, want ActionError", err)
	}
	if len(e.store.cases) != 0 {
		t.Fatal("case recorded for failed action")
	}
}

func TestNoLogChannel(t *testing.T) {
	e := newEnv(t)
	e.settings.LogChannelID = 0
	out, _ := e.svc.Execute(context.Background(), req(ActionWarn, memberID))
	if out.LogPosted || len(e.notif.logged) != 0 {
		t.Fatal("posted to log without a log channel")
	}
}

func TestTempBanIsLiftedByScheduler(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := req(ActionBan, memberID)
	r.Duration = 24 * time.Hour
	out, err := e.svc.Execute(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Case.Details.Temporary || len(e.store.scheduled) != 1 {
		t.Fatalf("temp ban not scheduled: %+v", out.Case)
	}
	bot := Actor{Rank: core.Rank{ID: botID}, Name: "Niro"}
	n, err := e.svc.RunDue(ctx, bot)
	if err != nil || n != 1 {
		t.Fatalf("RunDue = %d, %v", n, err)
	}
	if e.plat.banned[memberID] {
		t.Fatal("user still banned after expiry")
	}
	last := e.store.cases[len(e.store.cases)-1]
	if last.Action != ActionUnban || last.Source != SourceSystem || !strings.Contains(last.Reason, "case #1") {
		t.Fatalf("unban case = %+v", last)
	}
	if _, done := e.store.done[1]; !done {
		t.Fatal("scheduled action not completed")
	}
}

func TestScheduledUnbanAfterManualUnbanIsNoop(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := req(ActionBan, strayID)
	r.Duration = time.Hour
	_, _ = e.svc.Execute(ctx, r)
	if _, err := e.svc.Execute(ctx, req(ActionUnban, strayID)); err != nil {
		t.Fatal(err)
	}
	if e.store.cancelled != 1 {
		t.Fatal("manual unban should cancel the scheduled unban")
	}
	// Even if the action was already claimed, running it is harmless.
	_, _ = e.svc.RunDue(ctx, Actor{Name: "Niro"})
	if note := e.store.done[1]; note != "already unbanned" {
		t.Fatalf("scheduled action note = %q", note)
	}
	if got := len(e.store.cases); got != 2 {
		t.Fatalf("cases = %d, want 2 (ban + manual unban)", got)
	}
}

func TestUpdateReasonAndDeleteRefreshLog(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, _ = e.svc.Execute(ctx, req(ActionWarn, memberID))
	c, err := e.svc.UpdateReason(ctx, g, 1, "  new reason ")
	if err != nil || c.Reason != "new reason" {
		t.Fatalf("UpdateReason = %+v, %v", c, err)
	}
	if _, err := e.svc.DeleteCase(ctx, g, 1, modID); err != nil {
		t.Fatal(err)
	}
	if len(e.notif.edited) != 2 || !e.notif.edited[1].Deleted {
		t.Fatalf("log edits = %+v", e.notif.edited)
	}
	if _, err := e.svc.GetCase(ctx, g, 1); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("deleted case still visible: %v", err)
	}
}

func TestListCasesFilters(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, _ = e.svc.Execute(ctx, req(ActionWarn, memberID))
	_, _ = e.svc.Execute(ctx, req(ActionNote, memberID))
	_, _ = e.svc.Execute(ctx, req(ActionWarn, ownerID+1000)) // non-member warn fails
	cases, total, err := e.svc.ListCases(ctx, g, CaseFilter{TargetID: memberID})
	if err != nil || total != 2 || cases[0].Action != ActionNote {
		t.Fatalf("list = %+v total=%d err=%v", cases, total, err)
	}
	_, total, _ = e.svc.ListCases(ctx, g, CaseFilter{Action: ActionWarn})
	if total != 1 {
		t.Fatalf("warn filter total = %d", total)
	}
}

func TestPurgeFilters(t *testing.T) {
	now := time.Unix(1000, 0)
	e := newEnv(t)
	e.svc.now = func() time.Time { return now.Add(15 * 24 * time.Hour) }
	recent := now.Add(14 * 24 * time.Hour) // a day before "now"
	e.plat.messages = []Message{
		{ID: 10, AuthorID: memberID, Content: "hi", CreatedAt: recent},
		{ID: 9, AuthorID: 77, AuthorBot: true, Content: "beep", CreatedAt: recent},
		{ID: 8, AuthorID: memberID, Content: "see https://scam.example", CreatedAt: recent},
		{ID: 7, AuthorID: memberID, Content: "pinned", Pinned: true, CreatedAt: recent},
		{ID: 6, AuthorID: memberID, Content: "discord.gg/abc", CreatedAt: recent},
		{ID: 5, AuthorID: memberID, Content: "ancient", CreatedAt: now}, // too old to bulk delete
	}
	ctx := context.Background()
	run := func(count int, f PurgeFilter) []snowflake.ID {
		e.plat.deleted = nil
		out, err := e.svc.Purge(ctx, PurgeRequest{GuildID: g, ChannelID: 555, Actor: mod(), Count: count, Filter: f})
		if err != nil {
			t.Fatal(err)
		}
		if out.Case.Action != ActionPurge || out.Case.Details.Deleted != len(e.plat.deleted) || out.Case.Details.ChannelID != 555 {
			t.Fatalf("purge case = %+v", out.Case)
		}
		return e.plat.deleted
	}
	if got := run(100, PurgeFilter{}); !slices.Equal(got, []snowflake.ID{10, 9, 8, 6}) {
		t.Errorf("no filter: %v (pinned and old messages must be skipped)", got)
	}
	if got := run(2, PurgeFilter{}); !slices.Equal(got, []snowflake.ID{10, 9}) {
		t.Errorf("count 2: %v", got)
	}
	if got := run(100, PurgeFilter{BotsOnly: true}); !slices.Equal(got, []snowflake.ID{9}) {
		t.Errorf("bots: %v", got)
	}
	if got := run(100, PurgeFilter{LinksOnly: true}); !slices.Equal(got, []snowflake.ID{8, 6}) {
		t.Errorf("links: %v", got)
	}
	if got := run(100, PurgeFilter{UserID: memberID, LinksOnly: true}); !slices.Equal(got, []snowflake.ID{8, 6}) {
		t.Errorf("user+links: %v", got)
	}
	if _, err := e.svc.Purge(ctx, PurgeRequest{GuildID: g, Count: 0}); !errors.Is(err, ErrBadPurge) {
		t.Errorf("count 0: %v", err)
	}
}

func TestSettingsSpec(t *testing.T) {
	spec := SettingsSpec{}
	def, err := spec.Normalize(nil)
	if err != nil || string(def) != `{"log_channel_id":"0","dm_users":true,"require_reason":false}` {
		t.Fatalf("defaults = %s, %v", def, err)
	}
	if _, err := spec.Normalize([]byte(`{"log_channel_id":"123","unknown":1}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	got, err := spec.Normalize([]byte(`{"log_channel_id":"123","dm_users":false}`))
	if err != nil || !strings.Contains(string(got), `"123"`) || !strings.Contains(string(got), `"dm_users":false`) {
		t.Fatalf("normalize = %s, %v", got, err)
	}
}
