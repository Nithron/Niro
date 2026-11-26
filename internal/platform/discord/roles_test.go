package discord

import (
	"errors"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/roles"
)

func testPanel(style roles.Style, n int) roles.Panel {
	p := roles.Panel{ID: 7, GuildID: 1, Name: "P", ChannelID: 2, Style: style, Mode: roles.ModeToggle, Message: message.Simple("Pick")}
	for i := range n {
		p.Roles = append(p.Roles, roles.PanelRole{RoleID: snowflake.ID(100 + i), Label: "R", Emoji: "🎨", Description: "d"})
	}
	return p
}

func TestPanelButtonsInRowsOfFive(t *testing.T) {
	p := testPresenter(t)
	controls := p.panelControls("en", testPanel(roles.StyleButtons, 7))
	if len(controls) != 2 {
		t.Fatalf("rows = %d", len(controls))
	}
	row := controls[0].(discord.ActionRowComponent)
	btn := row.Components[0].(discord.ButtonComponent)
	if len(row.Components) != 5 || btn.CustomID != "/roles/7/100" || btn.Emoji == nil || btn.Emoji.Name != "🎨" {
		t.Errorf("first row = %+v", row)
	}
	if n := len(controls[1].(discord.ActionRowComponent).Components); n != 2 {
		t.Errorf("second row has %d buttons", n)
	}
}

func TestPanelMenu(t *testing.T) {
	p := testPresenter(t)
	panel := testPanel(roles.StyleSelect, 3)
	panel.MaxRoles = 2
	menu := p.panelControls("en", panel)[0].(discord.ActionRowComponent).Components[0].(discord.StringSelectMenuComponent)
	if menu.CustomID != "/roles/7" || *menu.MinValues != 0 || menu.MaxValues != 2 || len(menu.Options) != 3 ||
		menu.Options[0].Value != "100" || menu.Options[0].Description != "d" || menu.Placeholder != "Pick your roles" {
		t.Errorf("menu = %+v", menu)
	}
}

func TestPickReply(t *testing.T) {
	b := &Bot{deps: Deps{Catalog: testPresenter(t).cat}, emojis: testPresenter(t).emojis}
	text := func(c discord.ContainerComponent) string { s, _ := cardParts(c); return s }
	got := text(b.pickReply(discord.LocaleEnglishUS, roles.Result{Added: []snowflake.ID{1}, Removed: []snowflake.ID{2},
		Failed: []*roles.RoleError{{RoleID: 3, Err: roles.ErrRoleDangerous}}}, nil))
	for _, want := range []string{"**Added:** <@&1>", "**Removed:** <@&2>", "can't hand out <@&3>"} {
		if !strings.Contains(got, want) {
			t.Errorf("reply missing %q:\n%s", want, got)
		}
	}
	for err, want := range map[error]string{
		nil: "Nothing changed", roles.ErrModuleOff: "turned off", roles.ErrNotFound: "deleted",
		&roles.TooNewError{}: "too new", roles.ErrLimitReached: "more roles than",
	} {
		if got := text(b.pickReply(discord.LocaleEnglishUS, roles.Result{}, err)); !strings.Contains(got, want) {
			t.Errorf("%v: %q", err, got)
		}
	}
	if !isPickError(&roles.TooNewError{}) || isPickError(errors.New("boom")) {
		t.Error("isPickError")
	}
}
