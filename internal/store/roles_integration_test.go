package store_test

import (
	"errors"
	"testing"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/roles"
)

func TestRolePanels(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	for _, g := range []core.GuildInfo{{ID: 1, Name: "G", OwnerID: 1}, {ID: 2, Name: "H", OwnerID: 1}} {
		if err := s.UpsertGuild(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	p := roles.Panel{GuildID: 1, Name: "Colours", ChannelID: 50, Message: message.Simple("Pick"), Style: roles.StyleButtons,
		Mode: roles.ModeToggle, Roles: []roles.PanelRole{{RoleID: 10, Label: "Red", Emoji: "🟥"}}, MaxRoles: 1, RequiredRoleID: 16, UpdatedBy: 7}
	created, err := s.CreatePanel(ctx, p)
	if err != nil || created.ID == 0 || created.Roles[0].Emoji != "🟥" || created.Message.Blocks[0].Text != "Pick" || created.RequiredRoleID != 16 {
		t.Fatalf("create = %+v, %v", created, err)
	}
	if err := s.SetPanelMessage(ctx, 1, created.ID, 51, 900); err != nil {
		t.Fatal(err)
	}
	created.Name, created.Mode = "Colour", roles.ModeUnique
	updated, err := s.UpdatePanel(ctx, created)
	if err != nil || updated.Name != "Colour" || updated.MessageID != 900 || updated.ChannelID != 50 {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if _, err := s.GetPanel(ctx, 2, created.ID); !errors.Is(err, roles.ErrNotFound) {
		t.Errorf("other guild = %v", err)
	}
	p.GuildID = 2
	if _, err := s.CreatePanel(ctx, p); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListPanels(ctx, 1); len(list) != 1 {
		t.Errorf("list = %v", list)
	}
	if n, _ := s.CountPanels(ctx, 2); n != 1 {
		t.Errorf("count = %d", n)
	}
	if err := s.DeletePanel(ctx, 1, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPanel(ctx, 1, created.ID); !errors.Is(err, roles.ErrNotFound) {
		t.Errorf("after delete = %v", err)
	}
	p.GuildID = 999
	if _, err := s.CreatePanel(ctx, p); !errors.Is(err, core.ErrUnknownGuild) {
		t.Errorf("unknown guild = %v", err)
	}
}
