package discord

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/tickets"
)

func TestTicketPanelCard(t *testing.T) {
	tp := ticketsPlatform{p: testPresenter(t)}
	p := tickets.Panel{ID: 3, Message: message.Simple("Need help?"), Categories: []tickets.Category{
		{ID: "a1", Label: "Support", Emoji: "🎫"}, {ID: "b2", Label: "Billing"},
	}}
	card := tp.panelCard(p)
	row := card.Components[len(card.Components)-1].(discord.ActionRowComponent)
	first := row.Components[0].(discord.ButtonComponent)
	if len(row.Components) != 2 || first.CustomID != "/tickets/open/3/a1" || first.Emoji.Name != "🎫" {
		t.Errorf("row = %+v", row)
	}
}

func TestTranscriptText(t *testing.T) {
	plain := discord.Message{Content: "hello"}
	v2 := discord.Message{Flags: discord.MessageFlagIsComponentsV2, Components: []discord.LayoutComponent{
		discord.NewContainer(discord.NewTextDisplay("line one"), discord.NewSection(discord.NewTextDisplay("line two"))),
	}}
	if transcriptText(plain) != "hello" || transcriptText(v2) != "line one\nline two" {
		t.Errorf("text = %q / %q", transcriptText(plain), transcriptText(v2))
	}
}

func TestTicketCommandsFitDiscord(t *testing.T) {
	b := &Bot{deps: Deps{Catalog: testPresenter(t).cat}}
	cmds := b.ticketCommands()
	c := cmds[0].(discord.SlashCommandCreate)
	if c.Name != "ticket" || len(c.Options) != 4 || c.DefaultMemberPermissions.Value != nil {
		t.Errorf("command = %+v", c)
	}
	for _, o := range c.Options {
		sub := o.(discord.ApplicationCommandOptionSubCommand)
		if len(sub.Description) == 0 || len(sub.Description) > 100 || strings.Contains(sub.Description, "cmd.") {
			t.Errorf("%s: description %q", sub.Name, sub.Description)
		}
	}
}

func TestTicketActor(t *testing.T) {
	m := &discord.ResolvedMember{Member: discord.Member{User: discord.User{ID: 5, Username: "u"}, RoleIDs: []snowflake.ID{9}},
		Permissions: discord.PermissionManageGuild}
	a := ticketActor(nil, 1, m)
	if !a.Manager || a.ID != 5 || len(a.RoleIDs) != 1 {
		t.Errorf("actor = %+v", a)
	}
}
