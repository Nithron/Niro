package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/welcome"
	dc "github.com/nithron/niro/internal/platform/discord"
)

func toAPIWelcome(s welcome.Settings) apigen.WelcomeSettings {
	return apigen.WelcomeSettings{
		WelcomeEnabled: s.WelcomeEnabled, WelcomeChannelId: s.WelcomeChannelID.String(), WelcomeMessage: toAPIDocument(s.WelcomeMessage),
		PingMember: s.PingMember, DmEnabled: s.DMEnabled, DmMessage: toAPIDocument(s.DMMessage),
		GoodbyeEnabled: s.GoodbyeEnabled, GoodbyeChannelId: s.GoodbyeChannelID.String(), GoodbyeMessage: toAPIDocument(s.GoodbyeMessage),
		AutoRoleIds: ids(s.AutoRoleIDs),
	}
}

func fromAPIWelcome(b apigen.WelcomeSettings) (welcome.Settings, error) {
	bad := func(what string) error {
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid "+what+".")
	}
	s := welcome.Settings{WelcomeEnabled: b.WelcomeEnabled, PingMember: b.PingMember, DMEnabled: b.DmEnabled, GoodbyeEnabled: b.GoodbyeEnabled,
		AutoRoleIDs: []snowflake.ID{}}
	var err error
	if s.WelcomeChannelID, err = snowflake.Parse(b.WelcomeChannelId); err != nil {
		return s, bad("welcome channel")
	}
	if s.GoodbyeChannelID, err = snowflake.Parse(b.GoodbyeChannelId); err != nil {
		return s, bad("goodbye channel")
	}
	for _, r := range b.AutoRoleIds {
		id, err := snowflake.Parse(r)
		if err != nil {
			return s, bad("auto-role")
		}
		s.AutoRoleIDs = append(s.AutoRoleIDs, id)
	}
	for _, m := range []struct {
		in  apigen.MessageDocument
		out *message.Document
	}{{b.WelcomeMessage, &s.WelcomeMessage}, {b.DmMessage, &s.DMMessage}, {b.GoodbyeMessage, &s.GoodbyeMessage}} {
		if *m.out, err = fromAPIDocument(m.in, message.Budget{}); err != nil {
			return s, err
		}
	}
	return s, nil
}

func (h handlers) GetWelcomeSettings(ctx context.Context, req apigen.GetWelcomeSettingsRequestObject) (apigen.GetWelcomeSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	s, err := welcome.LoadSettings(ctx, h.config, gid)
	if err != nil {
		return nil, err
	}
	return apigen.GetWelcomeSettings200JSONResponse(toAPIWelcome(s)), nil
}

func (h handlers) UpdateWelcomeSettings(ctx context.Context, req apigen.UpdateWelcomeSettingsRequestObject) (apigen.UpdateWelcomeSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	s, err := fromAPIWelcome(*req.Body)
	if err != nil {
		return nil, err
	}
	if err := h.checkWelcomeRefs(ctx, gid, s); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(s)
	actor := actorID(ctx)
	saved, err := h.config.SetSettings(ctx, gid, core.ModuleWelcome, raw, &actor, core.SourceDashboard)
	if errors.Is(err, core.ErrInvalidSettings) {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, settingsMessage(err))
	}
	if err != nil {
		return nil, err
	}
	var out welcome.Settings
	if err := json.Unmarshal(saved, &out); err != nil {
		return nil, err
	}
	return apigen.UpdateWelcomeSettings200JSONResponse(toAPIWelcome(out)), nil
}

// settingsMessage keeps the detail of a settings validation error.
func settingsMessage(err error) string {
	msg := err.Error()
	for _, p := range []string{core.ErrInvalidSettings.Error() + ": ", welcome.ErrInvalidSettings.Error() + ": "} {
		if i := len(p); len(msg) >= i && msg[:i] == p {
			msg = msg[i:]
		}
	}
	return msg
}

func (h handlers) checkWelcomeRefs(ctx context.Context, gid snowflake.ID, s welcome.Settings) error {
	if s.WelcomeChannelID == 0 && s.GoodbyeChannelID == 0 && len(s.AutoRoleIDs) == 0 {
		return nil
	}
	meta, err := h.guildMeta(ctx, gid)
	if err != nil {
		return err
	}
	bad := func(msg string) error { return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, msg) }
	for _, ch := range []snowflake.ID{s.WelcomeChannelID, s.GoodbyeChannelID} {
		if ch != 0 && !hasTextChannel(meta, ch) {
			return bad("Welcome and goodbye channels must be text or announcement channels in this server.")
		}
	}
	for _, id := range s.AutoRoleIDs {
		var role *dc.MetaRole
		for i := range meta.Roles {
			if meta.Roles[i].ID == id {
				role = &meta.Roles[i]
			}
		}
		switch {
		case role == nil || id == gid:
			return bad("An auto-role doesn't exist in this server.")
		case role.Managed:
			return bad("@" + role.Name + " is managed by a bot or integration and can't be handed out.")
		case role.Dangerous:
			return bad("@" + role.Name + " has dangerous permissions, so Niro won't give it to new members.")
		}
	}
	return nil
}
