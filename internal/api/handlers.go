package api

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core"
	dc "github.com/nithron/niro/internal/platform/discord"
)

const userGuildsTTL = 60 * time.Second

// handlers implements the generated StrictServerInterface.
type handlers struct{ *Server }

var _ apigen.StrictServerInterface = handlers{}

var (
	errForbidden     = errors.New("forbidden")
	errBotNotInGuild = errors.New("bot not in guild")
)

func apiErr(code apigen.ErrorCode, msg string) apigen.Error {
	return apigen.Error{Code: code, Message: msg}
}

func (h handlers) GetMe(ctx context.Context, _ apigen.GetMeRequestObject) (apigen.GetMeResponseObject, error) {
	u := sessionFrom(ctx).User
	resp := apigen.GetMe200JSONResponse{Id: u.ID.String(), Username: u.Username, AvatarUrl: u.AvatarURL}
	if u.GlobalName != "" {
		resp.GlobalName = &u.GlobalName
	}
	return resp, nil
}

func (h handlers) ListGuilds(ctx context.Context, _ apigen.ListGuildsRequestObject) (apigen.ListGuildsResponseObject, error) {
	guilds, err := h.manageableGuilds(ctx, sessionFrom(ctx))
	if err != nil {
		return nil, err
	}
	ids := make([]snowflake.ID, len(guilds))
	for i, g := range guilds {
		ids[i] = g.ID
	}
	active, err := h.guilds.ActiveGuilds(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("checking bot presence: %w", err)
	}
	out := make(apigen.ListGuilds200JSONResponse, 0, len(guilds))
	for _, g := range guilds {
		out = append(out, toSummary(g, active[g.ID]))
	}
	slices.SortStableFunc(out, func(a, b apigen.GuildSummary) int {
		if a.BotPresent != b.BotPresent {
			if a.BotPresent {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

func (h handlers) GetGuild(ctx context.Context, req apigen.GetGuildRequestObject) (apigen.GetGuildResponseObject, error) {
	g, present, err := h.authorizeGuild(ctx, req.GuildId, false)
	if errors.Is(err, errForbidden) {
		return apigen.GetGuild403JSONResponse{ForbiddenJSONResponse: apigen.ForbiddenJSONResponse(apiErr(apigen.ErrorCodeForbidden, "You can't manage this server."))}, nil
	}
	if err != nil {
		return nil, err
	}
	return apigen.GetGuild200JSONResponse(toSummary(g, present)), nil
}

func (h handlers) ListGuildModules(ctx context.Context, req apigen.ListGuildModulesRequestObject) (apigen.ListGuildModulesResponseObject, error) {
	g, _, err := h.authorizeGuild(ctx, req.GuildId, true)
	switch {
	case errors.Is(err, errForbidden):
		return apigen.ListGuildModules403JSONResponse{ForbiddenJSONResponse: apigen.ForbiddenJSONResponse(apiErr(apigen.ErrorCodeForbidden, "You can't manage this server."))}, nil
	case errors.Is(err, errBotNotInGuild):
		return apigen.ListGuildModules409JSONResponse{BotNotInGuildJSONResponse: apigen.BotNotInGuildJSONResponse(apiErr(apigen.ErrorCodeBotNotInGuild, "Add Niro to this server first."))}, nil
	case err != nil:
		return nil, err
	}
	states, err := h.config.ModuleStates(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	out := make(apigen.ListGuildModules200JSONResponse, len(states))
	for i, st := range states {
		out[i] = toModuleState(st)
	}
	return out, nil
}

func (h handlers) UpdateGuildModule(ctx context.Context, req apigen.UpdateGuildModuleRequestObject) (apigen.UpdateGuildModuleResponseObject, error) {
	if req.Body == nil {
		return apigen.UpdateGuildModule400JSONResponse{BadRequestJSONResponse: apigen.BadRequestJSONResponse(apiErr(apigen.ErrorCodeBadRequest, "Missing request body."))}, nil
	}
	g, _, err := h.authorizeGuild(ctx, req.GuildId, true)
	switch {
	case errors.Is(err, errForbidden):
		return apigen.UpdateGuildModule403JSONResponse{ForbiddenJSONResponse: apigen.ForbiddenJSONResponse(apiErr(apigen.ErrorCodeForbidden, "You can't manage this server."))}, nil
	case errors.Is(err, errBotNotInGuild):
		return apigen.UpdateGuildModule409JSONResponse{BotNotInGuildJSONResponse: apigen.BotNotInGuildJSONResponse(apiErr(apigen.ErrorCodeBotNotInGuild, "Add Niro to this server first."))}, nil
	case err != nil:
		return nil, err
	}

	actor := sessionFrom(ctx).User.ID
	st, err := h.config.SetEnabled(ctx, g.ID, core.ModuleID(req.ModuleId), req.Body.Enabled, &actor, core.SourceDashboard)
	switch {
	case errors.Is(err, core.ErrUnknownModule):
		return apigen.UpdateGuildModule404JSONResponse{NotFoundJSONResponse: apigen.NotFoundJSONResponse(apiErr(apigen.ErrorCodeNotFound, "Unknown module."))}, nil
	case errors.Is(err, core.ErrModuleAlwaysOn):
		return apigen.UpdateGuildModule400JSONResponse{BadRequestJSONResponse: apigen.BadRequestJSONResponse(apiErr(apigen.ErrorCodeModuleAlwaysOn, "This module is always on."))}, nil
	case errors.Is(err, core.ErrUnknownGuild):
		return apigen.UpdateGuildModule409JSONResponse{BotNotInGuildJSONResponse: apigen.BotNotInGuildJSONResponse(apiErr(apigen.ErrorCodeBotNotInGuild, "Add Niro to this server first."))}, nil
	case err != nil:
		return nil, err
	}
	h.log.InfoContext(ctx, "module changed from dashboard", "guild_id", g.ID, "module", req.ModuleId, "enabled", st.Enabled, "user_id", actor)
	return apigen.UpdateGuildModule200JSONResponse(toModuleState(st)), nil
}

func (h handlers) GetInvite(_ context.Context, req apigen.GetInviteRequestObject) (apigen.GetInviteResponseObject, error) {
	var guildID snowflake.ID
	if req.Params.GuildId != nil {
		id, err := snowflake.Parse(*req.Params.GuildId)
		if err != nil {
			return nil, err
		}
		guildID = id
	}
	return apigen.GetInvite200JSONResponse{Url: dc.InviteURL(h.cfg.ClientID, guildID)}, nil
}

// manageableGuilds returns the servers the user may configure, from a
// 60-second per-user cache to stay well inside Discord's rate limits.
func (h handlers) manageableGuilds(ctx context.Context, sess *Session) ([]UserGuild, error) {
	key := "niro:user-guilds:" + sess.User.ID.String()
	var all []UserGuild
	found, err := h.kv.GetJSON(ctx, key, &all)
	if err != nil {
		h.log.WarnContext(ctx, "reading user guild cache failed", "err", err)
	}
	if !found {
		if all, err = h.oauth.CurrentUserGuilds(ctx, sess.AccessToken); err != nil {
			return nil, fmt.Errorf("fetching the user's servers from Discord: %w", err)
		}
		if err := h.kv.SetJSON(ctx, key, all, userGuildsTTL); err != nil {
			h.log.WarnContext(ctx, "writing user guild cache failed", "err", err)
		}
	}
	out := all[:0:0]
	for _, g := range all {
		if dc.CanManageGuild(g.Owner, g.Permissions) {
			out = append(out, g)
		}
	}
	return out, nil
}

// authorizeGuild checks the user may manage the server and, if requireBot,
// that Niro is in it. It returns errForbidden for servers the user cannot
// manage or is not in, so the API never reveals whether a server exists.
func (h handlers) authorizeGuild(ctx context.Context, rawID string, requireBot bool) (UserGuild, bool, error) {
	id, err := snowflake.Parse(rawID)
	if err != nil {
		return UserGuild{}, false, errForbidden
	}
	guilds, err := h.manageableGuilds(ctx, sessionFrom(ctx))
	if err != nil {
		return UserGuild{}, false, err
	}
	idx := slices.IndexFunc(guilds, func(g UserGuild) bool { return g.ID == id })
	if idx < 0 {
		return UserGuild{}, false, errForbidden
	}
	present, err := h.guilds.IsGuildActive(ctx, id)
	if err != nil {
		return UserGuild{}, false, err
	}
	if requireBot && !present {
		return guilds[idx], false, errBotNotInGuild
	}
	return guilds[idx], present, nil
}

func toSummary(g UserGuild, present bool) apigen.GuildSummary {
	s := apigen.GuildSummary{Id: g.ID.String(), Name: g.Name, Owner: g.Owner, BotPresent: present}
	if g.IconURL != "" {
		icon := g.IconURL
		s.IconUrl = &icon
	}
	return s
}

func toModuleState(st core.ModuleState) apigen.ModuleState {
	m := apigen.ModuleState{Id: string(st.Module.ID), Enabled: st.Enabled, AlwaysOn: st.Module.AlwaysOn, Version: st.Version}
	if !st.UpdatedAt.IsZero() {
		t := st.UpdatedAt
		m.UpdatedAt = &t
	}
	return m
}
