package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/logging"
	dc "github.com/nithron/niro/internal/platform/discord"
)

func toAPILoggingSettings(s logging.Settings) apigen.LoggingSettings {
	events := make(map[string]bool, len(s.Events))
	for k, v := range s.Events {
		events[string(k)] = v
	}
	c := s.Channels
	return apigen.LoggingSettings{
		DefaultChannelId: s.DefaultChannelID.String(),
		Channels: apigen.LoggingChannels{
			Messages: c.Messages.String(), Members: c.Members.String(), Moderation: c.Moderation.String(),
			Server: c.Server.String(), Voice: c.Voice.String(), Automod: c.AutoMod.String(), Guardian: c.Guardian.String(),
		},
		Events: events, IgnoreChannelIds: ids(s.IgnoreChannelIDs), IgnoreRoleIds: ids(s.IgnoreRoleIDs),
		IgnoreBots: s.IgnoreBots, NoMessageContent: s.NoMessageContent,
	}
}

func fromAPILoggingSettings(b apigen.LoggingSettings) (logging.Settings, error) {
	bad := func(what string) error {
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid "+what+".")
	}
	s := logging.Settings{
		Events: make(map[logging.Kind]bool, len(b.Events)), IgnoreBots: b.IgnoreBots, NoMessageContent: b.NoMessageContent,
		IgnoreChannelIDs: []snowflake.ID{}, IgnoreRoleIDs: []snowflake.ID{},
	}
	for k, v := range b.Events {
		s.Events[logging.Kind(k)] = v
	}
	var err error
	parse := func(raw string, dst *snowflake.ID) {
		if err == nil {
			*dst, err = snowflake.Parse(raw)
		}
	}
	c := b.Channels
	parse(b.DefaultChannelId, &s.DefaultChannelID)
	parse(c.Messages, &s.Channels.Messages)
	parse(c.Members, &s.Channels.Members)
	parse(c.Moderation, &s.Channels.Moderation)
	parse(c.Server, &s.Channels.Server)
	parse(c.Voice, &s.Channels.Voice)
	parse(c.Automod, &s.Channels.AutoMod)
	parse(c.Guardian, &s.Channels.Guardian)
	if err != nil {
		return s, bad("log channel")
	}
	for _, raw := range b.IgnoreChannelIds {
		id, err := snowflake.Parse(raw)
		if err != nil {
			return s, bad("ignored channel")
		}
		s.IgnoreChannelIDs = append(s.IgnoreChannelIDs, id)
	}
	for _, raw := range b.IgnoreRoleIds {
		id, err := snowflake.Parse(raw)
		if err != nil {
			return s, bad("ignored role")
		}
		s.IgnoreRoleIDs = append(s.IgnoreRoleIDs, id)
	}
	return s, nil
}

func (h handlers) GetLoggingSettings(ctx context.Context, req apigen.GetLoggingSettingsRequestObject) (apigen.GetLoggingSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	s, err := logging.LoadSettings(ctx, h.config, gid)
	if err != nil {
		return nil, err
	}
	return apigen.GetLoggingSettings200JSONResponse(toAPILoggingSettings(s)), nil
}

func (h handlers) UpdateLoggingSettings(ctx context.Context, req apigen.UpdateLoggingSettingsRequestObject) (apigen.UpdateLoggingSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	s, err := fromAPILoggingSettings(*req.Body)
	if err != nil {
		return nil, err
	}
	if err := h.checkLoggingRefs(ctx, gid, s); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(s)
	actor := actorID(ctx)
	saved, err := h.config.SetSettings(ctx, gid, core.ModuleLogging, raw, &actor, core.SourceDashboard)
	if errors.Is(err, core.ErrInvalidSettings) {
		msg := err.Error()
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, msg)
	}
	if err != nil {
		return nil, err
	}
	var out logging.Settings
	if err := json.Unmarshal(saved, &out); err != nil {
		return nil, err
	}
	return apigen.UpdateLoggingSettings200JSONResponse(toAPILoggingSettings(out)), nil
}

// checkLoggingRefs makes sure log channels are text channels of the server
// and ignored channels and roles belong to it.
func (h handlers) checkLoggingRefs(ctx context.Context, gid snowflake.ID, s logging.Settings) error {
	logChannels := s.Channels.All()
	if s.DefaultChannelID != 0 && !slices.Contains(logChannels, s.DefaultChannelID) {
		logChannels = append(logChannels, s.DefaultChannelID)
	}
	if len(logChannels) == 0 && len(s.IgnoreChannelIDs) == 0 && len(s.IgnoreRoleIDs) == 0 {
		return nil
	}
	meta, err := h.guildMeta(ctx, gid)
	if err != nil {
		return err
	}
	bad := func(msg string) error { return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, msg) }
	for _, id := range logChannels {
		if !hasTextChannel(meta, id) {
			return bad("Log channels must be text or announcement channels in this server.")
		}
	}
	for _, id := range s.IgnoreChannelIDs {
		if !slices.ContainsFunc(meta.Channels, func(c dc.MetaChannel) bool { return c.ID == id }) {
			return bad("An ignored channel does not exist in this server.")
		}
	}
	for _, id := range s.IgnoreRoleIDs {
		if !slices.ContainsFunc(meta.Roles, func(r dc.MetaRole) bool { return r.ID == id }) {
			return bad("An ignored role does not exist in this server.")
		}
	}
	return nil
}
