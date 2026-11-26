package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/modules/roles"
	dc "github.com/nithron/niro/internal/platform/discord"
)

// RolePanelReader is the read side of the role panel store; changes go
// through the bot (RPC), which posts and edits the Discord messages.
type RolePanelReader interface {
	ListPanels(ctx context.Context, guildID snowflake.ID) ([]roles.Panel, error)
	GetPanel(ctx context.Context, guildID snowflake.ID, id int64) (roles.Panel, error)
}

func toAPIPanel(p roles.Panel) apigen.RolePanel {
	out := apigen.RolePanel{
		Id: ptr(p.ID), Name: p.Name, ChannelId: p.ChannelID.String(), Message: toAPIDocument(p.Message),
		Style: apigen.RolePanelStyle(p.Style), Mode: apigen.RolePanelMode(p.Mode), MaxRoles: p.MaxRoles,
		RequiredRoleId: p.RequiredRoleID.String(), Roles: make([]apigen.RolePanelRole, len(p.Roles)),
	}
	if p.MessageID != 0 {
		out.MessageId = ptr(p.MessageID.String())
	}
	if !p.UpdatedAt.IsZero() {
		out.UpdatedAt = ptr(p.UpdatedAt)
	}
	for i, r := range p.Roles {
		ar := apigen.RolePanelRole{RoleId: r.RoleID.String(), Label: r.Label}
		if r.Emoji != "" {
			ar.Emoji = ptr(r.Emoji)
		}
		if r.Description != "" {
			ar.Description = ptr(r.Description)
		}
		out.Roles[i] = ar
	}
	return out
}

func fromAPIPanel(gid snowflake.ID, a apigen.RolePanel) (roles.Panel, error) {
	bad := func(what string) error {
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid "+what+".")
	}
	p := roles.Panel{GuildID: gid, Name: a.Name, Style: roles.Style(a.Style), Mode: roles.Mode(a.Mode), MaxRoles: a.MaxRoles}
	var err error
	if p.ChannelID, err = snowflake.Parse(a.ChannelId); err != nil {
		return p, bad("channel")
	}
	if p.RequiredRoleID, err = snowflake.Parse(a.RequiredRoleId); err != nil {
		return p, bad("required role")
	}
	for _, r := range a.Roles {
		id, err := snowflake.Parse(r.RoleId)
		if err != nil {
			return p, bad("role")
		}
		p.Roles = append(p.Roles, roles.PanelRole{RoleID: id, Label: r.Label, Emoji: deref(r.Emoji), Description: deref(r.Description)})
	}
	p.Message, err = fromAPIDocument(a.Message, p.Budget())
	if err != nil {
		return p, err
	}
	if err := p.Validate(); err != nil {
		return p, panelError(err)
	}
	return p, nil
}

func panelError(err error) error {
	if errors.Is(err, roles.ErrInvalid) {
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, dc.ValidationMessage(err)+".")
	}
	return err
}

// checkPanelRefs gives quick feedback with the cached server data; the bot
// checks again with live data when it saves.
func (h handlers) checkPanelRefs(ctx context.Context, gid snowflake.ID, p roles.Panel) error {
	meta, err := h.guildMeta(ctx, gid)
	if err != nil {
		return err
	}
	bad := func(msg string) error { return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, msg) }
	if !hasTextChannel(meta, p.ChannelID) {
		return bad("The panel's channel must be a text or announcement channel in this server.")
	}
	find := func(id snowflake.ID) (dc.MetaRole, bool) {
		for _, r := range meta.Roles {
			if r.ID == id {
				return r, true
			}
		}
		return dc.MetaRole{}, false
	}
	for _, pr := range p.Roles {
		r, ok := find(pr.RoleID)
		switch {
		case !ok || r.ID == gid:
			return bad("A role on the panel doesn't exist in this server.")
		case r.Managed:
			return bad("@" + r.Name + " is managed by a bot or integration and can't be handed out.")
		case r.Dangerous:
			return bad("@" + r.Name + " has dangerous permissions (such as Administrator or Manage Server), so Niro won't hand it out.")
		}
	}
	if p.RequiredRoleID != 0 {
		if _, ok := find(p.RequiredRoleID); !ok {
			return bad("The required role doesn't exist in this server.")
		}
	}
	return nil
}

func (h handlers) ListRolePanels(ctx context.Context, req apigen.ListRolePanelsRequestObject) (apigen.ListRolePanelsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	list, err := h.rolePanels.ListPanels(ctx, gid)
	if err != nil {
		return nil, err
	}
	out := apigen.RolePanelList{Items: make([]apigen.RolePanel, len(list))}
	for i, p := range list {
		out.Items[i] = toAPIPanel(p)
	}
	return apigen.ListRolePanels200JSONResponse(out), nil
}

func (h handlers) savePanel(ctx context.Context, rawGuild string, id int64, body *apigen.RolePanel) (roles.Panel, error) {
	gid, err := h.managed(ctx, rawGuild)
	if err != nil {
		return roles.Panel{}, err
	}
	if body == nil {
		return roles.Panel{}, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	p, err := fromAPIPanel(gid, *body)
	if err != nil {
		return roles.Panel{}, err
	}
	if id != 0 {
		if _, err := h.rolePanels.GetPanel(ctx, gid, id); errors.Is(err, roles.ErrNotFound) {
			return roles.Panel{}, httpErr(http.StatusNotFound, apigen.ErrorCodeNotFound, "Role panel not found.")
		} else if err != nil {
			return roles.Panel{}, err
		}
	}
	if err := h.checkPanelRefs(ctx, gid, p); err != nil {
		return roles.Panel{}, err
	}
	p.ID = id
	var saved roles.Panel
	if err := h.rpc.Call(ctx, dc.RPCRolePanelSave, gid, actorID(ctx), p, &saved); err != nil {
		return roles.Panel{}, rpcErr(err)
	}
	return saved, nil
}

func (h handlers) CreateRolePanel(ctx context.Context, req apigen.CreateRolePanelRequestObject) (apigen.CreateRolePanelResponseObject, error) {
	p, err := h.savePanel(ctx, req.GuildId, 0, req.Body)
	if err != nil {
		return nil, err
	}
	return apigen.CreateRolePanel201JSONResponse(toAPIPanel(p)), nil
}

func (h handlers) UpdateRolePanel(ctx context.Context, req apigen.UpdateRolePanelRequestObject) (apigen.UpdateRolePanelResponseObject, error) {
	p, err := h.savePanel(ctx, req.GuildId, req.PanelId, req.Body)
	if err != nil {
		return nil, err
	}
	return apigen.UpdateRolePanel200JSONResponse(toAPIPanel(p)), nil
}

func (h handlers) DeleteRolePanel(ctx context.Context, req apigen.DeleteRolePanelRequestObject) (apigen.DeleteRolePanelResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if err := h.rpc.Call(ctx, dc.RPCRolePanelDelete, gid, actorID(ctx), dc.RolePanelIDParams{ID: req.PanelId}, nil); err != nil {
		return nil, rpcErr(err)
	}
	return apigen.DeleteRolePanel204Response{}, nil
}
