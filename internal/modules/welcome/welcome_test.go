package welcome

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/roles"
)

const (
	guild   snowflake.ID = 1
	hello   snowflake.ID = 20
	bye     snowflake.ID = 21
	member  snowflake.ID = 100
	newbie  snowflake.ID = 10
	artist  snowflake.ID = 11
	admins  snowflake.ID = 12
	verifed snowflake.ID = 13
)

type post struct {
	channel snowflake.ID
	text    string
	ping    snowflake.ID
}

type fakePlatform struct {
	mu    sync.Mutex
	posts []post
	dms   []string
	added []snowflake.ID
	dmErr error
}

func (f *fakePlatform) Server(context.Context, snowflake.ID) (string, int) {
	return "Test *Server*", 42
}
func (f *fakePlatform) Post(_ context.Context, ch snowflake.ID, d message.Document, ping snowflake.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts = append(f.posts, post{ch, d.Blocks[0].Text, ping})
	return nil
}

func (f *fakePlatform) DM(_ context.Context, _ snowflake.ID, d message.Document) error {
	f.dms = append(f.dms, d.Blocks[0].Text)
	return f.dmErr
}
func (f *fakePlatform) UserMention(id snowflake.ID) string    { return "<@" + id.String() + ">" }
func (f *fakePlatform) ChannelMention(id snowflake.ID) string { return "<#" + id.String() + ">" }
func (f *fakePlatform) Escape(s string) string                { return strings.ReplaceAll(s, "*", "\\*") }
func (f *fakePlatform) Roles(context.Context, snowflake.ID) ([]roles.RoleInfo, error) {
	return []roles.RoleInfo{{ID: newbie, Position: 1}, {ID: artist, Position: 2}, {ID: admins, Position: 3, Dangerous: []string{"administrator"}}}, nil
}
func (f *fakePlatform) BotTopPosition(context.Context, snowflake.ID) (int, error) { return 5, nil }
func (f *fakePlatform) AddRole(_ context.Context, _, _, r snowflake.ID, _ string) error {
	f.added = append(f.added, r)
	return nil
}

func settings(t *testing.T, raw string) Settings {
	t.Helper()
	out, err := SettingsSpec{}.Normalize([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	var s Settings
	_ = json.Unmarshal(out, &s)
	return s
}

func newService(t *testing.T, st Settings, gate Gate) (*Service, *fakePlatform, chan int) {
	t.Helper()
	pf := &fakePlatform{}
	summaries := make(chan int, 10)
	svc := NewService(Deps{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Settings: func(context.Context, snowflake.ID) (Settings, bool, error) { return st, true, nil },
		Platform: pf,
		Gate:     func(context.Context, snowflake.ID, snowflake.ID, time.Time) Gate { return gate },
		RaidSummary: func(_ context.Context, _ snowflake.ID, _ snowflake.ID, n int) error {
			summaries <- n
			return nil
		},
		RaidFlush: 50 * time.Millisecond,
	})
	return svc, pf, summaries
}

const everything = `{"welcome_enabled":true,"welcome_channel_id":"20","dm_enabled":true,"goodbye_enabled":true,"goodbye_channel_id":"21","auto_role_ids":["10","11","12"]}`

func TestSettings(t *testing.T) {
	d := settings(t, "")
	if d.WelcomeEnabled || d.DMEnabled || d.GoodbyeEnabled || !d.PingMember || len(d.WelcomeMessage.Blocks) != 1 {
		t.Errorf("defaults = %+v", d)
	}
	for name, raw := range map[string]string{
		"welcome without channel": `{"welcome_enabled":true}`,
		"goodbye without channel": `{"goodbye_enabled":true}`,
		"empty message":           `{"welcome_message":{"version":1,"blocks":[]}}`,
		"too many roles":          `{"auto_role_ids":["1","2","3","4","5","6","7","8","9","10","11"]}`,
		"unknown field":           `{"wave":true}`,
	} {
		if _, err := (SettingsSpec{}).Normalize([]byte(raw)); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestJoinWelcomesAndGivesRoles(t *testing.T) {
	svc, pf, _ := newService(t, settings(t, everything), Gate{})
	if err := svc.HandleJoin(context.Background(), Member{GuildID: guild, UserID: member, Name: "a*b"}); err != nil {
		t.Fatal(err)
	}
	if len(pf.posts) != 1 || pf.posts[0].channel != hello || pf.posts[0].ping != member ||
		pf.posts[0].text != "Welcome to **Test \\*Server\\***, <@100>! You're member #42." {
		t.Errorf("posts = %+v", pf.posts)
	}
	if len(pf.dms) != 1 || !strings.Contains(pf.dms[0], "Test \\*Server\\*") {
		t.Errorf("dms = %v", pf.dms)
	}
	// The dangerous role is skipped.
	if !slices.Equal(pf.added, []snowflake.ID{newbie, artist}) {
		t.Errorf("roles = %v", pf.added)
	}
}

func TestPingCanBeTurnedOff(t *testing.T) {
	st := settings(t, `{"welcome_enabled":true,"welcome_channel_id":"20","ping_member":false}`)
	svc, pf, _ := newService(t, st, Gate{})
	_ = svc.HandleJoin(context.Background(), Member{GuildID: guild, UserID: member})
	if pf.posts[0].ping != 0 {
		t.Error("pinged with ping_member off")
	}
}

func TestGuardianGates(t *testing.T) {
	ctx := context.Background()
	st := settings(t, everything)

	svc, pf, _ := newService(t, st, Gate{Blocked: true})
	_ = svc.HandleJoin(ctx, Member{GuildID: guild, UserID: member})
	if len(pf.posts)+len(pf.dms)+len(pf.added) != 0 {
		t.Error("a member removed by the age gate was welcomed")
	}

	// With the verification gate, roles come on verify, not on join.
	svc, pf, _ = newService(t, st, Gate{VerifyRoleID: verifed})
	_ = svc.HandleJoin(ctx, Member{GuildID: guild, UserID: member})
	if len(pf.posts) != 1 || len(pf.added) != 0 {
		t.Errorf("gate: posts %d roles %v", len(pf.posts), pf.added)
	}
	_ = svc.HandleVerified(ctx, guild, member)
	if len(pf.added) != 2 {
		t.Errorf("roles on verify = %v", pf.added)
	}

	_ = svc.HandleJoin(ctx, Member{GuildID: guild, UserID: 5, Bot: true})
	if len(pf.posts) != 1 {
		t.Error("a bot was welcomed")
	}
}

func TestRaidJoinsAreSummed(t *testing.T) {
	ctx := context.Background()
	svc, pf, summaries := newService(t, settings(t, everything), Gate{Raid: true})
	for i := range 12 {
		_ = svc.HandleJoin(ctx, Member{GuildID: guild, UserID: snowflake.ID(1000 + i)})
		_ = svc.HandleLeave(ctx, Member{GuildID: guild, UserID: snowflake.ID(1000 + i)})
	}
	if len(pf.posts)+len(pf.dms)+len(pf.added) != 0 {
		t.Errorf("raid joins were welcomed one by one: %+v %v %v", pf.posts, pf.dms, pf.added)
	}
	select {
	case n := <-summaries:
		if n != 12 {
			t.Errorf("summary counted %d joins", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no raid summary")
	}
	select {
	case n := <-summaries:
		t.Errorf("a second summary (%d)", n)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestGoodbyeNeverPings(t *testing.T) {
	svc, pf, _ := newService(t, settings(t, everything), Gate{})
	_ = svc.HandleLeave(context.Background(), Member{GuildID: guild, UserID: member, Name: "ana"})
	if len(pf.posts) != 1 || pf.posts[0].channel != bye || pf.posts[0].ping != 0 || pf.posts[0].text != "**ana** left the server." {
		t.Errorf("goodbye = %+v", pf.posts)
	}
}

func TestDMFailureIsNotAnError(t *testing.T) {
	svc, pf, _ := newService(t, settings(t, everything), Gate{})
	pf.dmErr = errors.New("cannot send messages to this user")
	if err := svc.HandleJoin(context.Background(), Member{GuildID: guild, UserID: member}); err != nil {
		t.Errorf("closed DMs failed the join: %v", err)
	}
}
