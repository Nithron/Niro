package roles

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
)

const (
	guild snowflake.ID = 1
	red   snowflake.ID = 10
	blue  snowflake.ID = 11
	green snowflake.ID = 12
	admin snowflake.ID = 13
	bots  snowflake.ID = 14
	high  snowflake.ID = 15
	vip   snowflake.ID = 16
	user  snowflake.ID = 100
)

func panel(mode Mode, ids ...snowflake.ID) Panel {
	p := Panel{GuildID: guild, Name: "Colours", ChannelID: 50, Style: StyleButtons, Mode: mode, Message: message.Simple("Pick a colour")}
	for _, id := range ids {
		p.Roles = append(p.Roles, PanelRole{RoleID: id, Label: id.String()})
	}
	return p
}

func TestValidate(t *testing.T) {
	ok := panel(ModeToggle, red, blue)
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	many := panel(ModeToggle)
	for i := range 26 {
		many.Roles = append(many.Roles, PanelRole{RoleID: snowflake.ID(1000 + i), Label: "x"})
	}
	bad := map[string]func(p *Panel){
		"no name":          func(p *Panel) { p.Name = " " },
		"no channel":       func(p *Panel) { p.ChannelID = 0 },
		"bad style":        func(p *Panel) { p.Style = "slider" },
		"bad mode":         func(p *Panel) { p.Mode = "random" },
		"no roles":         func(p *Panel) { p.Roles = nil },
		"duplicate":        func(p *Panel) { p.Roles = append(p.Roles, PanelRole{RoleID: red, Label: "again"}) },
		"no label":         func(p *Panel) { p.Roles[0].Label = "" },
		"select no label":  func(p *Panel) { p.Style, p.Roles[0].Label, p.Roles[0].Emoji = StyleSelect, "", "🎨" },
		"text emoji":       func(p *Panel) { p.Roles[0].Emoji = "<:x:1>" },
		"limit too high":   func(p *Panel) { p.MaxRoles = 3 },
		"required on list": func(p *Panel) { p.RequiredRoleID = red },
		"too many":         func(p *Panel) { *p = many },
	}
	for name, edit := range bad {
		p := panel(ModeToggle, red, blue)
		edit(&p)
		if err := p.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// An emoji alone is enough for a button; the menu-only description is dropped.
	p := panel(ModeUnique, red)
	p.Roles[0] = PanelRole{RoleID: red, Emoji: "🟥", Description: "x"}
	p.MaxRoles = 1
	if err := p.Validate(); err != nil || p.Roles[0].Description != "" || p.MaxRoles != 0 {
		t.Errorf("emoji-only button: %v %+v", err, p)
	}
	// 25 buttons fit next to a short message.
	full := panel(ModeToggle)
	for i := range 25 {
		full.Roles = append(full.Roles, PanelRole{RoleID: snowflake.ID(1000 + i), Label: "x"})
	}
	if err := full.Validate(); err != nil {
		t.Errorf("25 buttons: %v", err)
	}
}

func TestCheckRole(t *testing.T) {
	roles := []RoleInfo{
		{ID: red, Name: "Red", Position: 2}, {ID: admin, Name: "Admins", Position: 3, Dangerous: []string{"administrator"}},
		{ID: bots, Name: "Bots", Position: 1, Managed: true}, {ID: high, Name: "High", Position: 9}, {ID: guild, Everyone: true},
	}
	for id, want := range map[snowflake.ID]error{red: nil, admin: ErrRoleDangerous, bots: ErrRoleManaged, high: ErrRoleTooHigh, guild: ErrRoleManaged, 999: ErrRoleMissing} {
		err := CheckRole(id, roles, 5)
		if (want == nil) != (err == nil) || (want != nil && !errors.Is(err, want)) {
			t.Errorf("role %s: %v, want %v", id, err, want)
		}
	}
}

func TestPress(t *testing.T) {
	cases := []struct {
		name   string
		panel  Panel
		member []snowflake.ID
		press  snowflake.ID
		add    []snowflake.ID
		remove []snowflake.ID
		err    error
	}{
		{"toggle add", panel(ModeToggle, red, blue), nil, red, []snowflake.ID{red}, nil, nil},
		{"toggle remove", panel(ModeToggle, red, blue), []snowflake.ID{red}, red, nil, []snowflake.ID{red}, nil},
		{"unique swaps", panel(ModeUnique, red, blue, green), []snowflake.ID{red, vip}, blue, []snowflake.ID{blue}, []snowflake.ID{red}, nil},
		{"unique remove", panel(ModeUnique, red, blue), []snowflake.ID{blue}, blue, nil, []snowflake.ID{blue}, nil},
		{"verify adds", panel(ModeVerify, red), nil, red, []snowflake.ID{red}, nil, nil},
		{"verify never removes", panel(ModeVerify, red), []snowflake.ID{red}, red, nil, nil, nil},
		{"not on panel", panel(ModeToggle, red), nil, blue, nil, nil, ErrNotOnPanel},
	}
	for _, c := range cases {
		got, err := c.panel.Press(c.member, c.press)
		if !errors.Is(err, c.err) || !slices.Equal(got.Add, c.add) || !slices.Equal(got.Remove, c.remove) {
			t.Errorf("%s: %+v, %v", c.name, got, err)
		}
	}
	limited := panel(ModeToggle, red, blue, green)
	limited.MaxRoles = 2
	if _, err := limited.Press([]snowflake.ID{red, blue}, green); !errors.Is(err, ErrLimitReached) {
		t.Errorf("limit: %v", err)
	}
	if c, err := limited.Press([]snowflake.ID{red, blue}, red); err != nil || len(c.Remove) != 1 {
		t.Errorf("removing at the limit: %+v %v", c, err)
	}
	gated := panel(ModeToggle, red)
	gated.RequiredRoleID = vip
	if _, err := gated.Press(nil, red); !errors.Is(err, ErrNeedsRole) {
		t.Errorf("required role: %v", err)
	}
}

func TestChoose(t *testing.T) {
	p := panel(ModeToggle, red, blue, green)
	p.Style = StyleSelect
	c, err := p.Choose([]snowflake.ID{red, vip}, []snowflake.ID{blue, green})
	if err != nil || !slices.Equal(c.Add, []snowflake.ID{blue, green}) || !slices.Equal(c.Remove, []snowflake.ID{red}) {
		t.Errorf("toggle menu: %+v %v", c, err)
	}
	if _, err := p.Choose(nil, []snowflake.ID{vip}); !errors.Is(err, ErrNotOnPanel) {
		t.Errorf("forged option: %v", err)
	}
	u := panel(ModeUnique, red, blue)
	if _, err := u.Choose(nil, []snowflake.ID{red, blue}); !errors.Is(err, ErrTooManyPicked) || u.MaxPicks() != 1 {
		t.Errorf("unique menu: %v", err)
	}
	v := panel(ModeVerify, red, blue)
	if c, _ := v.Choose([]snowflake.ID{red}, nil); !c.Empty() {
		t.Errorf("verify menu removed roles: %+v", c)
	}
}

// ---- service ---------------------------------------------------------------

type memStore struct {
	mu     sync.Mutex
	panels map[int64]Panel
	next   int64
}

func (m *memStore) CreatePanel(_ context.Context, p Panel) (Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	p.ID = m.next
	m.panels[p.ID] = p
	return p, nil
}

func (m *memStore) UpdatePanel(_ context.Context, p Panel) (Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.panels[p.ID]; !ok {
		return Panel{}, ErrNotFound
	}
	m.panels[p.ID] = p
	return p, nil
}

func (m *memStore) GetPanel(_ context.Context, g snowflake.ID, id int64) (Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.panels[id]
	if !ok || p.GuildID != g {
		return Panel{}, ErrNotFound
	}
	return p, nil
}

func (m *memStore) ListPanels(_ context.Context, g snowflake.ID) ([]Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Panel
	for _, p := range m.panels {
		if p.GuildID == g {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memStore) CountPanels(ctx context.Context, g snowflake.ID) (int, error) {
	l, _ := m.ListPanels(ctx, g)
	return len(l), nil
}

func (m *memStore) DeletePanel(_ context.Context, g snowflake.ID, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.panels, id)
	return nil
}

func (m *memStore) SetPanelMessage(_ context.Context, g snowflake.ID, id int64, ch, msg snowflake.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.panels[id]
	p.ChannelID, p.MessageID = ch, msg
	m.panels[id] = p
	return nil
}

type fakePlatform struct {
	roles    []RoleInfo
	top      int
	member   []snowflake.ID
	posted   int
	edited   int
	deleted  []snowflake.ID
	gone     bool
	nextMsg  snowflake.ID
	addedLog []snowflake.ID
}

func (f *fakePlatform) Roles(context.Context, snowflake.ID) ([]RoleInfo, error)   { return f.roles, nil }
func (f *fakePlatform) BotTopPosition(context.Context, snowflake.ID) (int, error) { return f.top, nil }
func (f *fakePlatform) MemberRoles(context.Context, snowflake.ID, snowflake.ID) ([]snowflake.ID, error) {
	return slices.Clone(f.member), nil
}

func (f *fakePlatform) AddRole(_ context.Context, _, _, r snowflake.ID, _ string) error {
	f.member = append(f.member, r)
	f.addedLog = append(f.addedLog, r)
	return nil
}

func (f *fakePlatform) RemoveRole(_ context.Context, _, _, r snowflake.ID, _ string) error {
	f.member = slices.DeleteFunc(f.member, func(x snowflake.ID) bool { return x == r })
	return nil
}

func (f *fakePlatform) PostPanel(context.Context, Panel) (snowflake.ID, error) {
	f.posted++
	f.nextMsg++
	return 500 + f.nextMsg, nil
}

func (f *fakePlatform) EditPanel(context.Context, Panel) error {
	if f.gone {
		return ErrMessageGone
	}
	f.edited++
	return nil
}

func (f *fakePlatform) DeleteMessage(_ context.Context, _, m snowflake.ID) error {
	f.deleted = append(f.deleted, m)
	return nil
}

func newService(t *testing.T) (*Service, *memStore, *fakePlatform) {
	t.Helper()
	st := &memStore{panels: map[int64]Panel{}}
	pf := &fakePlatform{top: 5, roles: []RoleInfo{
		{ID: red, Name: "Red", Position: 1}, {ID: blue, Name: "Blue", Position: 2}, {ID: green, Name: "Green", Position: 3},
		{ID: admin, Name: "Admins", Position: 4, Dangerous: []string{"administrator"}}, {ID: vip, Name: "VIP", Position: 4},
	}}
	return NewService(Deps{Store: st, Platform: pf, Enabled: func(context.Context, snowflake.ID) (bool, error) { return true, nil }}), st, pf
}

func TestSavePostsThenEdits(t *testing.T) {
	ctx := context.Background()
	svc, st, pf := newService(t)
	p, err := svc.Save(ctx, panel(ModeToggle, red, blue))
	if err != nil || p.ID == 0 || p.MessageID == 0 || pf.posted != 1 {
		t.Fatalf("create: %+v %v (posted %d)", p, err, pf.posted)
	}
	p.Name = "Colours v2"
	if _, err := svc.Save(ctx, p); err != nil || pf.edited != 1 || pf.posted != 1 {
		t.Fatalf("edit: %v (edited %d posted %d)", err, pf.edited, pf.posted)
	}
	// The message was deleted in Discord: saving posts it again.
	pf.gone = true
	again, _ := svc.Save(ctx, p)
	if pf.posted != 2 || again.MessageID == p.MessageID {
		t.Errorf("repost: %+v (posted %d)", again, pf.posted)
	}
	// Moving the panel deletes the old message and posts in the new channel.
	pf.gone = false
	again.ChannelID = 51
	moved, _ := svc.Save(ctx, again)
	if !slices.Contains(pf.deleted, again.MessageID) || moved.MessageID == again.MessageID || st.panels[moved.ID].ChannelID != 51 {
		t.Errorf("move: %+v deleted %v", moved, pf.deleted)
	}
	if err := svc.Delete(ctx, guild, moved.ID); err != nil || len(st.panels) != 0 {
		t.Errorf("delete: %v", err)
	}
}

func TestSaveRefusesUnsafeRoles(t *testing.T) {
	svc, _, _ := newService(t)
	for _, id := range []snowflake.ID{admin, 999} {
		if _, err := svc.Save(context.Background(), panel(ModeToggle, red, id)); err == nil {
			t.Errorf("role %s accepted", id)
		}
	}
	tooHigh := panel(ModeToggle, red)
	svc.d.Platform.(*fakePlatform).top = 1
	var re *RoleError
	if _, err := svc.Save(context.Background(), tooHigh); !errors.As(err, &re) || !errors.Is(err, ErrRoleTooHigh) {
		t.Errorf("role above Niro: %v", err)
	}
}

func TestPanelLimit(t *testing.T) {
	svc, _, _ := newService(t)
	for range MaxPanels {
		if _, err := svc.Save(context.Background(), panel(ModeToggle, red)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Save(context.Background(), panel(ModeToggle, red)); !errors.Is(err, ErrTooManyPanel) {
		t.Errorf("26th panel: %v", err)
	}
}

func TestPicksRecheckRoles(t *testing.T) {
	ctx := context.Background()
	svc, _, pf := newService(t)
	p, _ := svc.Save(ctx, panel(ModeToggle, red, blue))
	res, err := svc.PressButton(ctx, guild, p.ID, user, red)
	if err != nil || !slices.Equal(res.Added, []snowflake.ID{red}) {
		t.Fatalf("press: %+v %v", res, err)
	}
	// Someone gave Blue the Administrator permission after the panel was saved.
	pf.roles[1].Dangerous = []string{"administrator"}
	res, err = svc.PressButton(ctx, guild, p.ID, user, blue)
	if err != nil || len(res.Added) != 0 || len(res.Failed) != 1 || !errors.Is(res.Failed[0], ErrRoleDangerous) {
		t.Errorf("dangerous now: %+v %v", res, err)
	}
	if slices.Contains(pf.member, blue) {
		t.Error("dangerous role was given")
	}
	res, err = svc.ChooseMenu(ctx, guild, p.ID, user, nil)
	if err != nil || !slices.Equal(res.Removed, []snowflake.ID{red}) {
		t.Errorf("menu clear: %+v %v", res, err)
	}
}

func TestVerifyGateAndModuleOff(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newService(t)
	p, _ := svc.Save(ctx, panel(ModeVerify, red))
	when := time.Now().Add(time.Hour)
	svc.d.VerifyGate = func(context.Context, snowflake.ID, snowflake.ID) (bool, time.Time) { return false, when }
	var tn *TooNewError
	if _, err := svc.PressButton(ctx, guild, p.ID, user, red); !errors.As(err, &tn) || !tn.When.Equal(when) {
		t.Errorf("gate: %v", err)
	}
	if _, err := svc.PressButton(ctx, 999, p.ID, user, red); !errors.Is(err, ErrNotFound) {
		t.Errorf("another server's panel: %v", err)
	}
	svc.d.Enabled = func(context.Context, snowflake.ID) (bool, error) { return false, nil }
	if _, err := svc.PressButton(ctx, guild, p.ID, user, red); !errors.Is(err, ErrModuleOff) {
		t.Errorf("module off: %v", err)
	}
}
