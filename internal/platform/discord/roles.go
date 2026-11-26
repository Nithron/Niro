package discord

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/roles"
	"github.com/nithron/niro/internal/rpc"
)

// ---- platform ---------------------------------------------------------------

// rolesPlatform implements roles.Platform on Discord.
type rolesPlatform struct {
	client *bot.Client
	p      presenter
	locale func(snowflake.ID) string
}

var _ roles.Platform = rolesPlatform{}

func (rp rolesPlatform) Roles(ctx context.Context, guildID snowflake.ID) ([]roles.RoleInfo, error) {
	var list []discord.Role
	for r := range rp.client.Caches.Roles(guildID) {
		list = append(list, r)
	}
	if len(list) == 0 {
		var err error
		if list, err = rp.client.Rest.GetRoles(guildID, rest.WithCtx(ctx)); err != nil {
			return nil, err
		}
	}
	out := make([]roles.RoleInfo, len(list))
	for i, r := range list {
		out[i] = roleInfo(guildID, r)
	}
	return out, nil
}

func roleInfo(guildID snowflake.ID, r discord.Role) roles.RoleInfo {
	return roles.RoleInfo{
		ID: r.ID, Name: r.Name, Position: r.Position, Managed: r.Managed, Everyone: r.ID == guildID,
		Dangerous: dangerousNames(r.Permissions),
	}
}

func (rp rolesPlatform) BotTopPosition(ctx context.Context, guildID snowflake.ID) (int, error) {
	self, ok := rp.client.Caches.SelfUser()
	if !ok {
		return 0, errors.New("bot user not known yet")
	}
	roleIDs, err := rp.MemberRoles(ctx, guildID, self.ID)
	if err != nil {
		return 0, err
	}
	return rankOf(rp.client, guildID, self.ID, roleIDs).TopRolePosition, nil
}

func (rp rolesPlatform) MemberRoles(ctx context.Context, guildID, userID snowflake.ID) ([]snowflake.ID, error) {
	if m, ok := rp.client.Caches.Member(guildID, userID); ok {
		return slices.Clone(m.RoleIDs), nil
	}
	m, err := rp.client.Rest.GetMember(guildID, userID, rest.WithCtx(ctx))
	if err != nil {
		return nil, err
	}
	return m.RoleIDs, nil
}

func (rp rolesPlatform) AddRole(ctx context.Context, guildID, userID, roleID snowflake.ID, reason string) error {
	return rp.client.Rest.AddMemberRole(guildID, userID, roleID, rest.WithCtx(ctx), rest.WithReason(reason))
}

func (rp rolesPlatform) RemoveRole(ctx context.Context, guildID, userID, roleID snowflake.ID, reason string) error {
	return rp.client.Rest.RemoveMemberRole(guildID, userID, roleID, rest.WithCtx(ctx), rest.WithReason(reason))
}

func (rp rolesPlatform) PostPanel(ctx context.Context, p roles.Panel) (snowflake.ID, error) {
	m, err := rp.client.Rest.CreateMessage(p.ChannelID, documentMessage(rp.p.panelCard(rp.locale(p.GuildID), p), nil), rest.WithCtx(ctx))
	if err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (rp rolesPlatform) EditPanel(ctx context.Context, p roles.Panel) error {
	comps := []discord.LayoutComponent{rp.p.panelCard(rp.locale(p.GuildID), p)}
	_, err := rp.client.Rest.UpdateMessage(p.ChannelID, p.MessageID, discord.MessageUpdate{Components: &comps, AllowedMentions: noPings}, rest.WithCtx(ctx))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage, rest.JSONErrorCodeUnknownChannel) {
		return roles.ErrMessageGone
	}
	return err
}

func (rp rolesPlatform) DeleteMessage(ctx context.Context, channelID, messageID snowflake.ID) error {
	err := rp.client.Rest.DeleteMessage(channelID, messageID, rest.WithCtx(ctx))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage, rest.JSONErrorCodeUnknownChannel) {
		return nil
	}
	return err
}

// ---- rendering --------------------------------------------------------------

func panelButtonID(panelID int64, roleID snowflake.ID) string {
	return "/roles/" + strconv.FormatInt(panelID, 10) + "/" + roleID.String()
}

func panelMenuID(panelID int64) string { return "/roles/" + strconv.FormatInt(panelID, 10) }

// panelControls are the panel's buttons (rows of 5) or its select menu.
func (p presenter) panelControls(l string, panel roles.Panel) []discord.ContainerSubComponent {
	if panel.Style == roles.StyleSelect {
		opts := make([]discord.StringSelectMenuOption, len(panel.Roles))
		for i, r := range panel.Roles {
			o := discord.NewStringSelectMenuOption(r.Label, r.RoleID.String())
			o.Description = r.Description
			if r.Emoji != "" {
				o.Emoji = &discord.ComponentEmoji{Name: r.Emoji}
			}
			opts[i] = o
		}
		menu := discord.NewStringSelectMenu(panelMenuID(panel.ID), p.t(l, i18n.RolesSelectPlaceholder, nil), opts...).
			WithMinValues(0).WithMaxValues(panel.MaxPicks())
		return []discord.ContainerSubComponent{discord.NewActionRow(menu)}
	}
	var rows []discord.ContainerSubComponent
	for start := 0; start < len(panel.Roles); start += 5 {
		end := min(start+5, len(panel.Roles))
		buttons := make([]discord.InteractiveComponent, 0, end-start)
		for _, r := range panel.Roles[start:end] {
			b := discord.NewSecondaryButton(r.Label, panelButtonID(panel.ID, r.RoleID))
			if r.Emoji != "" {
				b = b.WithEmoji(discord.ComponentEmoji{Name: r.Emoji})
			}
			buttons = append(buttons, b)
		}
		rows = append(rows, discord.NewActionRow(buttons...))
	}
	return rows
}

func (p presenter) panelCard(l string, panel roles.Panel) discord.ContainerComponent {
	return renderDocument(panel.Message, p.panelControls(l, panel)...)
}

func roleList(ids []snowflake.ID) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "<@&" + id.String() + ">"
	}
	return strings.Join(parts, ", ")
}

// pickReply describes what a pick did (or why it didn't).
func (b *Bot) pickReply(l discord.Locale, res roles.Result, err error) discord.ContainerComponent {
	card := func(emoji, text string, color int) discord.ContainerComponent {
		return b.infoCard(l.Code(), emoji, text, color)
	}
	var tooNew *roles.TooNewError
	switch {
	case errors.As(err, &tooNew):
		return card(EmojiWarn, b.t(l, i18n.RolesTooNew, map[string]any{"When": ts(tooNew.When, "R")}), colorWarning)
	case errors.Is(err, roles.ErrModuleOff):
		return card(EmojiInfo, b.t(l, i18n.RolesDisabled, nil), colorBrand)
	case errors.Is(err, roles.ErrNotFound):
		return card(EmojiInfo, b.t(l, i18n.RolesPanelGone, nil), colorBrand)
	case errors.Is(err, roles.ErrNotOnPanel):
		return card(EmojiWarn, b.t(l, i18n.RolesNotOnPanel, nil), colorWarning)
	case errors.Is(err, roles.ErrLimitReached), errors.Is(err, roles.ErrTooManyPicked):
		return card(EmojiWarn, b.t(l, i18n.RolesTooMany, nil), colorWarning)
	case err != nil:
		return card(EmojiError, b.errorText(l.Code(), err, ""), colorDanger)
	}
	var lines []string
	if len(res.Added) > 0 {
		lines = append(lines, b.t(l, i18n.RolesAdded, map[string]any{"Roles": roleList(res.Added)}))
	}
	if len(res.Removed) > 0 {
		lines = append(lines, b.t(l, i18n.RolesRemoved, map[string]any{"Roles": roleList(res.Removed)}))
	}
	if len(res.Failed) > 0 {
		ids := make([]snowflake.ID, len(res.Failed))
		for i, f := range res.Failed {
			ids[i] = f.RoleID
		}
		lines = append(lines, b.t(l, i18n.RolesFailed, map[string]any{"Roles": roleList(ids)}))
	}
	if len(lines) == 0 {
		return card(EmojiInfo, b.t(l, i18n.RolesNothing, nil), colorBrand)
	}
	color := colorSuccess
	if len(res.Failed) > 0 {
		color = colorWarning
	}
	return card(EmojiOK, strings.Join(lines, "\n"), color)
}

// ---- interactions -----------------------------------------------------------

func (b *Bot) registerRoles(r handler.Router) {
	r.ButtonComponent("/roles/{panel}/{role}", b.handleRoleButton)
	r.SelectMenuComponent("/roles/{panel}", b.handleRoleMenu)
}

func (b *Bot) handleRoleButton(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	gid, m := e.GuildID(), e.Member()
	panelID, err1 := strconv.ParseInt(e.Vars["panel"], 10, 64)
	roleID, err2 := snowflake.Parse(e.Vars["role"])
	if gid == nil || m == nil || err1 != nil || err2 != nil {
		return errNotInGuild
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	res, err := b.roles.PressButton(e.Ctx, *gid, panelID, m.User.ID, roleID)
	return b.finishPick(e, res, err)
}

func (b *Bot) handleRoleMenu(data discord.SelectMenuInteractionData, e *handler.ComponentEvent) error {
	gid, m := e.GuildID(), e.Member()
	panelID, err := strconv.ParseInt(e.Vars["panel"], 10, 64)
	sel, ok := data.(discord.StringSelectMenuInteractionData)
	if gid == nil || m == nil || err != nil || !ok {
		return errNotInGuild
	}
	chosen := make([]snowflake.ID, 0, len(sel.Values))
	for _, v := range sel.Values {
		id, err := snowflake.Parse(v)
		if err != nil {
			return errNotInGuild
		}
		chosen = append(chosen, id)
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	res, err := b.roles.ChooseMenu(e.Ctx, *gid, panelID, m.User.ID, chosen)
	return b.finishPick(e, res, err)
}

func (b *Bot) finishPick(e *handler.ComponentEvent, res roles.Result, err error) error {
	if errors.Is(err, roles.ErrNeedsRole) {
		if p, gerr := b.deps.RolePanels.GetPanel(e.Ctx, *e.GuildID(), mustPanelID(e)); gerr == nil {
			return replyDeferred(e.UpdateInteractionResponse, b.infoCard(e.Locale().Code(), EmojiWarn,
				b.t(e.Locale(), i18n.RolesNeedsRole, map[string]any{"Role": "<@&" + p.RequiredRoleID.String() + ">"}), colorWarning))
		}
	}
	if err != nil && !isPickError(err) {
		b.log.Warn("role panel pick failed", "guild_id", e.GuildID(), "err", err)
	}
	return replyDeferred(e.UpdateInteractionResponse, b.pickReply(e.Locale(), res, err))
}

func mustPanelID(e *handler.ComponentEvent) int64 {
	id, _ := strconv.ParseInt(e.Vars["panel"], 10, 64)
	return id
}

func isPickError(err error) bool {
	for _, known := range []error{roles.ErrModuleOff, roles.ErrNotFound, roles.ErrNotOnPanel, roles.ErrLimitReached,
		roles.ErrTooManyPicked, roles.ErrNeedsRole, roles.ErrTooNew} {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}

// verifyGate applies Guardian's minimum account age to verify-mode panels.
func (b *Bot) verifyGate(ctx context.Context, guildID, userID snowflake.ID) (bool, time.Time) {
	on, err := b.deps.Config.IsEnabled(ctx, guildID, core.ModuleGuardian)
	if err != nil || !on {
		return true, time.Time{}
	}
	s, err := guardian.LoadSettings(ctx, b.deps.Config, guildID)
	if err != nil {
		return true, time.Time{}
	}
	return guardian.CanVerify(s, userID, time.Now())
}

// ---- RPC --------------------------------------------------------------------

// Role panel RPC methods (shared with the API).
const (
	RPCRolePanelSave   = "roles.panel.save"
	RPCRolePanelDelete = "roles.panel.delete"
)

type RolePanelIDParams struct {
	ID int64 `json:"id"`
}

func (b *Bot) registerRolesRPC(srv *rpc.Server) {
	srv.Handle(RPCRolePanelSave, func(ctx context.Context, r rpc.Request) (any, error) {
		var p roles.Panel
		if err := r.Bind(&p); err != nil {
			return nil, err
		}
		p.GuildID, p.UpdatedBy = r.GuildID, r.ActorID
		saved, err := b.roles.Save(ctx, p)
		if err != nil {
			return nil, rolesRPCError(err)
		}
		return saved, nil
	})
	srv.Handle(RPCRolePanelDelete, func(ctx context.Context, r rpc.Request) (any, error) {
		var p RolePanelIDParams
		if err := r.Bind(&p); err != nil {
			return nil, err
		}
		return nil, rolesRPCError(b.roles.Delete(ctx, r.GuildID, p.ID))
	})
}

func rolesRPCError(err error) error {
	var re *roles.RoleError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, roles.ErrNotFound):
		return rpc.Errorf(rpc.CodeNotFound, "role panel not found")
	case errors.As(err, &re):
		return rpc.Errorf(rpc.CodeInvalid, "%s", re.Error())
	case errors.Is(err, roles.ErrInvalid):
		msg := err.Error()
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		return rpc.Errorf(rpc.CodeInvalid, "%s", msg)
	case errors.Is(err, roles.ErrTooManyPanel):
		return rpc.Errorf(rpc.CodeInvalid, "a server can have at most %d role panels", roles.MaxPanels)
	}
	return discordRPCError(err)
}
