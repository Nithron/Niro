package logging

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

const (
	testGuild    snowflake.ID = 1
	logChannel   snowflake.ID = 900
	modLog       snowflake.ID = 901
	guardAlerts  snowflake.ID = 902
	chatChannel  snowflake.ID = 10
	quietChannel snowflake.ID = 11
	alice        snowflake.ID = 1001
	staffRole    snowflake.ID = 50
)

type posted struct {
	channel snowflake.ID
	entry   Entry
}

type recordingSink struct {
	mu   sync.Mutex
	got  []posted
	fail error
}

func (r *recordingSink) Post(_ context.Context, ch snowflake.ID, e Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	r.got = append(r.got, posted{ch, e})
	return nil
}

type fakeInvites struct {
	list []Invite
	err  error
}

func (f *fakeInvites) ListInvites(context.Context, snowflake.ID) ([]Invite, error) {
	return append([]Invite(nil), f.list...), f.err
}

type harness struct {
	svc     *Service
	sink    *recordingSink
	cache   *MemoryCache
	invites *fakeInvites
	now     time.Time
	on      bool
	st      Settings
}

func newHarness(t *testing.T, settings string) *harness {
	t.Helper()
	raw, err := SettingsSpec{}.Normalize([]byte(settings))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{sink: &recordingSink{}, invites: &fakeInvites{}, now: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC), on: true}
	if err := json.Unmarshal(raw, &h.st); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return h.now }
	h.cache = NewMemoryCache(clock)
	h.svc = NewService(Deps{
		Settings: func(context.Context, snowflake.ID) (Settings, bool, error) { return h.st, h.on, nil },
		Cache:    h.cache, Sink: h.sink, Invites: h.invites, Snapshots: h.cache,
		ModLogChannel:        func(context.Context, snowflake.ID) snowflake.ID { return modLog },
		GuardianAlertChannel: func(context.Context, snowflake.ID) snowflake.ID { return guardAlerts },
		Log:                  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:                  clock,
	})
	return h
}

// everything routes every kind (voice included) to logChannel.
const everything = `{"default_channel_id":"900","events":{"voice_join":true,"voice_leave":true,"voice_move":true}}`

func (h *harness) only(t *testing.T) Entry {
	t.Helper()
	if len(h.sink.got) != 1 {
		t.Fatalf("posted %d entries, want 1: %+v", len(h.sink.got), h.sink.got)
	}
	return h.sink.got[0].entry
}

func (h *harness) none(t *testing.T) {
	t.Helper()
	if len(h.sink.got) != 0 {
		t.Fatalf("posted %d entries, want none: %+v", len(h.sink.got), h.sink.got)
	}
}

func msg(id snowflake.ID, content string) Message {
	return Message{
		ID: id, GuildID: testGuild, ChannelID: chatChannel, AuthorID: alice, AuthorName: "alice",
		Content: content, CreatedAt: time.Date(2026, 11, 26, 11, 0, 0, 0, time.UTC),
	}
}

func TestEditShowsBeforeAndAfter(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	m := msg(5, "helo world")
	if err := h.svc.HandleMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	m.Content, m.EditedAt = "hello world", h.now
	if err := h.svc.HandleMessageEdit(ctx, m); err != nil {
		t.Fatal(err)
	}
	e := h.only(t)
	if e.Kind != KindMessageEdit || e.Before != "helo world" || e.After != "hello world" || !e.Cached || e.UserID != alice {
		t.Errorf("entry = %+v", e)
	}
	// The cache now holds the new text: a second edit diffs against it.
	m.Content, m.EditedAt = "hello, world", h.now.Add(time.Second)
	_ = h.svc.HandleMessageEdit(ctx, m)
	if got := h.sink.got[1].entry; got.Before != "hello world" {
		t.Errorf("second edit before = %q", got.Before)
	}
}

func TestEditsThatAreNotEdits(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	m := msg(5, "see https://example.org")
	_ = h.svc.HandleMessage(ctx, m)
	// A link preview loading sends an update without an edit time.
	_ = h.svc.HandleMessageEdit(ctx, m)
	// An edit time but the same text (e.g. only an embed was removed).
	m.EditedAt = h.now
	_ = h.svc.HandleMessageEdit(ctx, m)
	h.none(t)
	// A real edit, then the same update again (an embed refresh).
	m.Content, m.EditedAt = "see https://example.com", h.now.Add(time.Second)
	_ = h.svc.HandleMessageEdit(ctx, m)
	_ = h.svc.HandleMessageEdit(ctx, m)
	if len(h.sink.got) != 1 {
		t.Errorf("posted %d, want the one real edit", len(h.sink.got))
	}
}

func TestEditOfUncachedMessage(t *testing.T) {
	h := newHarness(t, everything)
	m := msg(5, "new text")
	m.EditedAt = h.now
	_ = h.svc.HandleMessageEdit(context.Background(), m)
	e := h.only(t)
	if e.Cached || e.Before != "" || e.After != "new text" {
		t.Errorf("entry = %+v", e)
	}
}

func TestDeleteShowsCachedTextAndForgetsIt(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	m := msg(5, "secret plans")
	m.Attachments = []string{"plan.png"}
	_ = h.svc.HandleMessage(ctx, m)
	if err := h.svc.HandleMessageDelete(ctx, testGuild, chatChannel, 0, 5); err != nil {
		t.Fatal(err)
	}
	e := h.only(t)
	if e.Kind != KindMessageDelete || e.Before != "secret plans" || !e.Cached || e.UserID != alice ||
		!slices.Equal(e.Attachments, []string{"plan.png"}) || e.ByNiro {
		t.Errorf("entry = %+v", e)
	}
	if h.cache.Len() != 0 {
		t.Error("deleted message is still cached")
	}
}

func TestDeleteOfUncachedMessage(t *testing.T) {
	h := newHarness(t, everything)
	_ = h.svc.HandleMessageDelete(context.Background(), testGuild, chatChannel, 0, 5)
	if e := h.only(t); e.Cached || e.UserID != 0 || e.MessageID != 5 {
		t.Errorf("entry = %+v", e)
	}
}

func TestDeleteByNiro(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	h.svc.MarkNiroDeletes(chatChannel, []snowflake.ID{5}, "guardian")
	_ = h.svc.HandleMessageDelete(ctx, testGuild, chatChannel, 0, 5)
	if e := h.only(t); !e.ByNiro || e.ByNiroSource != "guardian" {
		t.Errorf("entry = %+v", e)
	}
	// The mark expires.
	h.now = h.now.Add(niroDeleteTTL + time.Second)
	_ = h.svc.HandleMessageDelete(ctx, testGuild, chatChannel, 0, 5)
	if h.sink.got[1].entry.ByNiro {
		t.Error("old mark still applies")
	}
}

func TestBulkDeleteIsOneEntry(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	for _, id := range []snowflake.ID{7, 5, 6} {
		_ = h.svc.HandleMessage(ctx, msg(id, "spam"))
	}
	h.svc.MarkNiroDeletes(chatChannel, []snowflake.ID{5, 6, 7, 8}, "purge")
	if err := h.svc.HandleBulkDelete(ctx, testGuild, chatChannel, 0, []snowflake.ID{5, 6, 7, 8}); err != nil {
		t.Fatal(err)
	}
	e := h.only(t)
	if e.Kind != KindMessageBulkDelete || e.Count != 4 || len(e.Messages) != 3 || !e.ByNiro || e.ByNiroSource != "purge" {
		t.Fatalf("entry = %+v", e)
	}
	if e.Messages[0].ID != 5 || e.Messages[2].ID != 7 {
		t.Errorf("messages not oldest first: %v", e.Messages)
	}
	if h.cache.Len() != 0 {
		t.Error("bulk-deleted messages are still cached")
	}
}

func TestNoMessageContent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, `{"default_channel_id":"900","no_message_content":true}`)
	m := msg(5, "private")
	_ = h.svc.HandleMessage(ctx, m)
	if c, ok, _ := h.cache.Get(ctx, chatChannel, 5); !ok || c.Content != "" || c.AuthorID != alice {
		t.Fatalf("cached %+v (found %v): text must not be cached, the author must", c, ok)
	}
	m.Content, m.EditedAt = "still private", h.now
	_ = h.svc.HandleMessageEdit(ctx, m)
	_ = h.svc.HandleMessageDelete(ctx, testGuild, chatChannel, 0, 5)
	_ = h.svc.HandleAutoMod(ctx, AutoModAction{GuildID: testGuild, RuleID: 3, UserID: alice, Content: "bad words"})
	if len(h.sink.got) != 3 {
		t.Fatalf("posted %d, want edit, delete and automod", len(h.sink.got))
	}
	for _, p := range h.sink.got {
		if p.entry.Before != "" || p.entry.After != "" || !p.entry.ContentHidden {
			t.Errorf("%s shows content: %+v", p.entry.Kind, p.entry)
		}
	}
}

func TestIgnoredMessagesAreNotCached(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, `{"default_channel_id":"900","ignore_channel_ids":["11"],"ignore_role_ids":["50"]}`)
	quiet := msg(5, "in an ignored channel")
	quiet.ChannelID = quietChannel
	staff := msg(6, "from staff")
	staff.AuthorRoleIDs = []snowflake.ID{staffRole}
	bot := msg(7, "from a bot")
	bot.AuthorBot = true
	niro := msg(8, "Niro's own log")
	niro.FromNiro = true
	for _, m := range []Message{quiet, staff, bot, niro} {
		_ = h.svc.HandleMessage(ctx, m)
		m.EditedAt, m.Content = h.now, "edited"
		_ = h.svc.HandleMessageEdit(ctx, m)
	}
	if h.cache.Len() != 0 {
		t.Errorf("%d ignored messages cached", h.cache.Len())
	}
	_ = h.svc.HandleMessageDelete(ctx, testGuild, quietChannel, 0, 5)
	h.none(t)
}

func TestNothingIsCachedWhenMessagesAreNotLogged(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, `{"channels":{"members":"900"}}`)
	_ = h.svc.HandleMessage(ctx, msg(5, "hi"))
	if h.cache.Len() != 0 {
		t.Error("message cached although message logs are off")
	}
	h = newHarness(t, everything)
	h.on = false
	_ = h.svc.HandleMessage(ctx, msg(5, "hi"))
	if h.cache.Len() != 0 {
		t.Error("message cached although the module is off")
	}
}

func TestJoinWithInvite(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	h.invites.list = []Invite{{Code: "abc", Uses: 3, InviterID: 7}, {Code: "xyz", Uses: 1}}
	if err := h.svc.RefreshInvites(ctx, testGuild); err != nil {
		t.Fatal(err)
	}
	h.invites.list = []Invite{{Code: "abc", Uses: 4, InviterID: 7}, {Code: "xyz", Uses: 1}}
	created := h.now.Add(-48 * time.Hour)
	_ = h.svc.HandleMemberJoin(ctx, MemberJoin{GuildID: testGuild, UserID: alice, UserName: "alice", AccountCreated: created, JoinedAt: h.now})
	e := h.only(t)
	if e.Kind != KindMemberJoin || e.Invite == nil || e.Invite.Code != "abc" || !e.AccountCreated.Equal(created) {
		t.Fatalf("entry = %+v", e)
	}
	// Without Manage Server the invite is simply unknown.
	h.invites.err = errors.New("missing permissions")
	_ = h.svc.HandleMemberJoin(ctx, MemberJoin{GuildID: testGuild, UserID: 2, JoinedAt: h.now})
	if h.sink.got[1].entry.Invite != nil {
		t.Error("invite guessed without an invite list")
	}
}

func TestFirstJoinWithoutSnapshotHasNoInvite(t *testing.T) {
	h := newHarness(t, everything)
	h.invites.list = []Invite{{Code: "abc", Uses: 4}}
	_ = h.svc.HandleMemberJoin(context.Background(), MemberJoin{GuildID: testGuild, UserID: alice, JoinedAt: h.now})
	if h.only(t).Invite != nil {
		t.Error("invite named without a previous snapshot")
	}
}

func TestLeave(t *testing.T) {
	h := newHarness(t, everything)
	joined := h.now.Add(-time.Hour)
	_ = h.svc.HandleMemberLeave(context.Background(), MemberLeave{GuildID: testGuild, UserID: alice, UserName: "alice", JoinedAt: joined, RoleIDs: []snowflake.ID{staffRole}})
	if e := h.only(t); e.Kind != KindMemberLeave || !e.JoinedAt.Equal(joined) || len(e.RoleIDs) != 1 {
		t.Errorf("entry = %+v", e)
	}
}

func TestAuditEntries(t *testing.T) {
	until := time.Date(2026, 12, 3, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		entry AuditEntry
		want  []Kind
	}{
		{"ban", AuditEntry{Action: AuditMemberBan, TargetID: alice, Reason: "spam"}, []Kind{KindMemberBan}},
		{"unban", AuditEntry{Action: AuditMemberUnban, TargetID: alice}, []Kind{KindMemberUnban}},
		{"kick", AuditEntry{Action: AuditMemberKick, TargetID: alice}, []Kind{KindMemberKick}},
		{"nick and timeout", AuditEntry{Action: AuditMemberUpdate, TargetID: alice, Changes: []Change{
			{Key: ChangeNick, Old: "al", New: "alice"}, {Key: ChangeTimeoutUntil, New: until.Format(time.RFC3339)}, {Key: "deaf", New: "true"},
		}}, []Kind{KindMemberNick, KindMemberTimeout}},
		{"other member update", AuditEntry{Action: AuditMemberUpdate, Changes: []Change{{Key: "mute", New: "true"}}}, nil},
		{"roles", AuditEntry{Action: AuditMemberRoleUpdate, Changes: []Change{{Key: ChangeRolesAdded, New: "<@&50>"}}}, []Kind{KindMemberRoles}},
		{"channel create", AuditEntry{Action: AuditChannelCreate, TargetID: 12, TargetName: "general"}, []Kind{KindChannelCreate}},
		{"channel update", AuditEntry{Action: AuditChannelUpdate, Changes: []Change{{Key: "position", Old: "1", New: "3"}}}, []Kind{KindChannelUpdate}},
		{"channel delete", AuditEntry{Action: AuditChannelDelete}, []Kind{KindChannelDelete}},
		{"role create", AuditEntry{Action: AuditRoleCreate}, []Kind{KindRoleCreate}},
		{"role update", AuditEntry{Action: AuditRoleUpdate}, []Kind{KindRoleUpdate}},
		{"role delete", AuditEntry{Action: AuditRoleDelete}, []Kind{KindRoleDelete}},
		{"server", AuditEntry{Action: AuditGuildUpdate, Changes: []Change{{Key: "verification_level", Old: "None", New: "High"}}}, []Kind{KindGuildUpdate}},
		{"unknown", AuditEntry{Action: "webhook_create"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, everything)
			c.entry.GuildID, c.entry.ActorID, c.entry.ActorName = testGuild, 77, "mod"
			if err := h.svc.HandleAudit(context.Background(), c.entry); err != nil {
				t.Fatal(err)
			}
			var got []Kind
			for _, p := range h.sink.got {
				got = append(got, p.entry.Kind)
				if p.entry.ActorID != 77 || p.channel != logChannel {
					t.Errorf("entry = %+v to %s", p.entry, p.channel)
				}
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("kinds = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAuditDetails(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	until := time.Date(2026, 12, 3, 12, 0, 0, 0, time.UTC)
	_ = h.svc.HandleAudit(ctx, AuditEntry{GuildID: testGuild, Action: AuditMemberUpdate, TargetID: alice, TargetName: "alice",
		Reason: "calm down", Changes: []Change{{Key: ChangeTimeoutUntil, Old: "", New: until.Format(time.RFC3339)}}})
	_ = h.svc.HandleAudit(ctx, AuditEntry{GuildID: testGuild, Action: AuditMemberUpdate, TargetID: alice,
		Changes: []Change{{Key: ChangeTimeoutUntil, Old: until.Format(time.RFC3339), New: ""}}})
	set, removed := h.sink.got[0].entry, h.sink.got[1].entry
	if set.UserID != alice || set.Reason != "calm down" || !set.TimeoutUntil.Equal(until) {
		t.Errorf("timeout = %+v", set)
	}
	if !removed.TimeoutUntil.IsZero() {
		t.Errorf("removed timeout has an end: %+v", removed)
	}
}

func TestNiroActionsAndTheModerationLog(t *testing.T) {
	ctx := context.Background()
	ban := AuditEntry{GuildID: testGuild, Action: AuditMemberBan, TargetID: alice, ByNiro: true}
	role := AuditEntry{GuildID: testGuild, Action: AuditMemberRoleUpdate, TargetID: alice, ByNiro: true}

	// A separate log channel gets everything, Niro's own actions included.
	h := newHarness(t, everything)
	_ = h.svc.HandleAudit(ctx, ban)
	if e := h.only(t); !e.ByNiro {
		t.Errorf("entry = %+v", e)
	}

	// Logging moderation into the moderation log skips Niro's actions (their
	// case card is there), but not other people's, nor non-moderation kinds.
	h = newHarness(t, `{"default_channel_id":"901"}`)
	_ = h.svc.HandleAudit(ctx, ban)
	h.none(t)
	human := ban
	human.ByNiro = false
	_ = h.svc.HandleAudit(ctx, human)
	_ = h.svc.HandleAudit(ctx, role)
	if len(h.sink.got) != 2 {
		t.Errorf("posted %d, want the human ban and Niro's role change", len(h.sink.got))
	}
}

func TestVoice(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything+"")
	h.st.IgnoreChannelIDs = []snowflake.ID{quietChannel}
	steps := []VoiceChange{
		{From: 0, To: 20},            // join
		{From: 20, To: 21},           // move
		{From: 21, To: 0},            // leave
		{From: 20, To: 20},           // mute: no channel change
		{From: 0, To: quietChannel},  // join an ignored channel
		{From: quietChannel, To: 20}, // move out of it: logged
	}
	for _, v := range steps {
		v.GuildID, v.UserID = testGuild, alice
		_ = h.svc.HandleVoice(ctx, v)
	}
	var got []Kind
	for _, p := range h.sink.got {
		got = append(got, p.entry.Kind)
	}
	want := []Kind{KindVoiceJoin, KindVoiceMove, KindVoiceLeave, KindVoiceMove}
	if !slices.Equal(got, want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
	if m := h.sink.got[1].entry; m.OldChannelID != 20 || m.ChannelID != 21 {
		t.Errorf("move = %+v", m)
	}
}

func TestVoiceOffByDefault(t *testing.T) {
	h := newHarness(t, `{"default_channel_id":"900"}`)
	_ = h.svc.HandleVoice(context.Background(), VoiceChange{GuildID: testGuild, UserID: alice, To: 20})
	h.none(t)
}

func TestAutoModOncePerTrigger(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	a := AutoModAction{GuildID: testGuild, RuleID: 3, RuleName: "Invites", UserID: alice, ChannelID: chatChannel,
		Keyword: "discord.gg/*", Content: "join discord.gg/x"}
	for _, action := range []string{"block_message", "send_alert", "timeout"} {
		a.Action = action
		_ = h.svc.HandleAutoMod(ctx, a)
	}
	e := h.only(t)
	if e.RuleName != "Invites" || e.Action != "block_message" || e.Before != "join discord.gg/x" {
		t.Errorf("entry = %+v", e)
	}
	h.now = h.now.Add(automodTTL + time.Second)
	_ = h.svc.HandleAutoMod(ctx, a)
	if len(h.sink.got) != 2 {
		t.Error("a later trigger was swallowed")
	}
}

func TestGuardian(t *testing.T) {
	ctx := context.Background()
	g := GuardianAction{GuildID: testGuild, EventID: 4, Kind: "scam", Response: "quarantine", UserID: alice, Signals: []string{"lookalike_domain"}}
	h := newHarness(t, everything)
	_ = h.svc.HandleGuardian(ctx, g)
	if e := h.only(t); e.EventID != 4 || e.Action != "quarantine" || !e.ByNiro {
		t.Errorf("entry = %+v", e)
	}
	// Not repeated in the channel that already has the alert.
	h = newHarness(t, `{"channels":{"guardian":"902"}}`)
	_ = h.svc.HandleGuardian(ctx, g)
	h.none(t)
}

func TestModuleOff(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, everything)
	h.on = false
	_ = h.svc.HandleMemberJoin(ctx, MemberJoin{GuildID: testGuild, UserID: alice})
	_ = h.svc.HandleAudit(ctx, AuditEntry{GuildID: testGuild, Action: AuditMemberBan})
	_ = h.svc.HandleGuardian(ctx, GuardianAction{GuildID: testGuild})
	h.none(t)
}

func TestSinkErrorsAreReturned(t *testing.T) {
	h := newHarness(t, everything)
	h.sink.fail = errors.New("missing access")
	if err := h.svc.HandleMemberLeave(context.Background(), MemberLeave{GuildID: testGuild, UserID: alice}); err == nil {
		t.Error("sink error swallowed")
	}
}

func TestEveryKindHasACategory(t *testing.T) {
	seen := map[Kind]bool{}
	for _, k := range Kinds {
		if seen[k] {
			t.Errorf("%s listed twice", k)
		}
		seen[k] = true
		if !slices.Contains(Categories, k.Category()) {
			t.Errorf("%s has category %q", k, k.Category())
		}
	}
}
