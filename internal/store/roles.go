package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"

	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/roles"
	"github.com/nithron/niro/internal/store/dbq"
)

var _ roles.Store = (*Store)(nil)

func panelJSON(p roles.Panel) (msg, list []byte) {
	msg, _ = json.Marshal(p.Message)
	if p.Roles == nil {
		p.Roles = []roles.PanelRole{}
	}
	list, _ = json.Marshal(p.Roles)
	return msg, list
}

func (s *Store) CreatePanel(ctx context.Context, p roles.Panel) (roles.Panel, error) {
	msg, list := panelJSON(p)
	row, err := s.q.InsertRolePanel(ctx, dbq.InsertRolePanelParams{
		GuildID: int64(p.GuildID), Name: p.Name, ChannelID: int64(p.ChannelID), Message: msg, Style: string(p.Style),
		Mode: string(p.Mode), Roles: list, MaxRoles: int32(p.MaxRoles), RequiredRoleID: optID(p.RequiredRoleID), UpdatedBy: optID(p.UpdatedBy),
	})
	if err != nil {
		return roles.Panel{}, mapFKViolation(err)
	}
	return toPanel(row), nil
}

func (s *Store) UpdatePanel(ctx context.Context, p roles.Panel) (roles.Panel, error) {
	msg, list := panelJSON(p)
	row, err := s.q.UpdateRolePanel(ctx, dbq.UpdateRolePanelParams{
		GuildID: int64(p.GuildID), ID: p.ID, Name: p.Name, ChannelID: int64(p.ChannelID), Message: msg, Style: string(p.Style),
		Mode: string(p.Mode), Roles: list, MaxRoles: int32(p.MaxRoles), RequiredRoleID: optID(p.RequiredRoleID), UpdatedBy: optID(p.UpdatedBy),
	})
	return panelOrNotFound(row, err)
}

func (s *Store) GetPanel(ctx context.Context, guildID snowflake.ID, id int64) (roles.Panel, error) {
	row, err := s.q.GetRolePanel(ctx, dbq.GetRolePanelParams{GuildID: int64(guildID), ID: id})
	return panelOrNotFound(row, err)
}

func (s *Store) ListPanels(ctx context.Context, guildID snowflake.ID) ([]roles.Panel, error) {
	rows, err := s.q.ListRolePanels(ctx, int64(guildID))
	if err != nil {
		return nil, err
	}
	out := make([]roles.Panel, len(rows))
	for i, r := range rows {
		out[i] = toPanel(r)
	}
	return out, nil
}

func (s *Store) CountPanels(ctx context.Context, guildID snowflake.ID) (int, error) {
	n, err := s.q.CountRolePanels(ctx, int64(guildID))
	return int(n), err
}

func (s *Store) DeletePanel(ctx context.Context, guildID snowflake.ID, id int64) error {
	return s.q.DeleteRolePanel(ctx, dbq.DeleteRolePanelParams{GuildID: int64(guildID), ID: id})
}

func (s *Store) SetPanelMessage(ctx context.Context, guildID snowflake.ID, id int64, channelID, messageID snowflake.ID) error {
	return s.q.SetRolePanelMessage(ctx, dbq.SetRolePanelMessageParams{
		GuildID: int64(guildID), ID: id, ChannelID: int64(channelID), MessageID: optID(messageID),
	})
}

func panelOrNotFound(row dbq.RolePanel, err error) (roles.Panel, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return roles.Panel{}, roles.ErrNotFound
	}
	if err != nil {
		return roles.Panel{}, err
	}
	return toPanel(row), nil
}

func toPanel(r dbq.RolePanel) roles.Panel {
	p := roles.Panel{
		ID: r.ID, GuildID: snowflake.ID(r.GuildID), Name: r.Name, ChannelID: snowflake.ID(r.ChannelID),
		MessageID: snowflake.ID(r.MessageID.Int64), Style: roles.Style(r.Style), Mode: roles.Mode(r.Mode),
		MaxRoles: int(r.MaxRoles), RequiredRoleID: snowflake.ID(r.RequiredRoleID.Int64), UpdatedBy: snowflake.ID(r.UpdatedBy.Int64),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	_ = json.Unmarshal(r.Message, &p.Message)
	if p.Message.Blocks == nil {
		p.Message = message.Document{Version: message.Version, Blocks: []message.Block{}}
	}
	_ = json.Unmarshal(r.Roles, &p.Roles)
	return p
}
