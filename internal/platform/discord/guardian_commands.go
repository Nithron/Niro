package discord

import (
	"context"
	"errors"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/moderation"
)

func (b *Bot) guardianStore() guardian.Store { return b.deps.GuardianStore }

// Guardian exposes the Guardian service (used by the RPC handlers).
func (b *Bot) Guardian() *guardian.Service { return b.guard }

// ---- command definitions ----------------------------------------------------

func (b *Bot) guardianCommands() []discord.ApplicationCommandCreate {
	d := func(k i18n.Key) (string, map[discord.Locale]string) {
		return b.deps.Catalog.T("en", k, nil), localizations(b.deps.Catalog, k)
	}
	sub := func(name string, k i18n.Key, opts ...discord.ApplicationCommandOption) discord.ApplicationCommandOptionSubCommand {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionSubCommand{Name: name, Description: desc, DescriptionLocalizations: loc, Options: opts}
	}
	textChannel := func(k i18n.Key, required bool) discord.ApplicationCommandOptionChannel {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionChannel{
			Name: "channel", Description: desc, DescriptionLocalizations: loc, Required: required,
			ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText, discord.ChannelTypeGuildNews},
		}
	}
	reasonDesc, reasonLoc := d(i18n.ModOptReason)
	maxReason := moderation.MaxReasonLength
	reason := discord.ApplicationCommandOptionString{Name: "reason", Description: reasonDesc, DescriptionLocalizations: reasonLoc, MaxLength: &maxReason}
	cmd := func(name string, k i18n.Key, perm discord.Permissions, opts ...discord.ApplicationCommandOption) discord.SlashCommandCreate {
		desc, loc := d(k)
		return discord.SlashCommandCreate{
			Name: name, Description: desc, DescriptionLocalizations: loc, Options: opts,
			DefaultMemberPermissions: omit.New(&perm),
			Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
			IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
		}
	}
	raidDesc, raidLoc := d(i18n.CmdGuardianRaidDesc)
	return []discord.ApplicationCommandCreate{
		cmd("guardian", i18n.CmdGuardianDescription, discord.PermissionManageGuild,
			sub("audit", i18n.CmdGuardianAuditDesc),
			discord.ApplicationCommandOptionSubCommandGroup{
				Name: "raid", Description: raidDesc, DescriptionLocalizations: raidLoc,
				Options: []discord.ApplicationCommandOptionSubCommand{
					sub("start", i18n.CmdGuardianRaidStartDesc),
					sub("end", i18n.CmdGuardianRaidEndDesc),
					sub("status", i18n.CmdGuardianRaidStatusDesc),
				},
			},
			sub("verify-panel", i18n.CmdGuardianVerifyDesc, textChannel(i18n.CmdGuardianOptChannel, true)),
		),
		cmd("lockdown", i18n.CmdLockdownDescription, discord.PermissionManageChannels,
			sub("channel", i18n.CmdLockdownChannelDesc, textChannel(i18n.LockdownOptChannel, false), reason),
			sub("server", i18n.CmdLockdownServerDesc, reason),
		),
		cmd("unlock", i18n.CmdUnlockDescription, discord.PermissionManageChannels,
			sub("channel", i18n.CmdUnlockChannelDesc, textChannel(i18n.LockdownOptChannel, false)),
			sub("server", i18n.CmdUnlockServerDesc),
		),
	}
}

func (b *Bot) registerGuardian(r handler.Router) {
	r.SlashCommand("/guardian/audit", b.handleAudit)
	r.SlashCommand("/guardian/raid/start", b.handleRaidStart)
	r.SlashCommand("/guardian/raid/end", b.handleRaidEnd)
	r.SlashCommand("/guardian/raid/status", b.handleRaidStatus)
	r.SlashCommand("/guardian/verify-panel", b.handleVerifyPanel)
	r.SlashCommand("/lockdown/channel", b.handleLockdownChannel)
	r.SlashCommand("/lockdown/server", b.handleLockdownServer)
	r.SlashCommand("/unlock/channel", b.handleUnlockChannel)
	r.SlashCommand("/unlock/server", b.handleUnlockServer)
	r.ButtonComponent("/guardian/resolve/{id}/{action}", b.handleResolveButton)
	r.ButtonComponent("/guardian/raid/end", b.handleRaidEndButton)
	r.ButtonComponent("/guardian/verify", b.handleVerifyButton)
}

// guardianOn replies with a hint when the Guardian module is off.
func (b *Bot) guardianOn(e *handler.CommandEvent, mc modContext) (bool, error) {
	on, err := b.deps.Config.IsEnabled(e.Ctx, mc.guildID, core.ModuleGuardian)
	if err != nil {
		return false, err
	}
	if !on {
		return false, b.replyNow(e, b.infoCard(mc.locale, EmojiInfo, b.t(discord.Locale(mc.locale), i18n.GuardianDisabled, nil), barNone))
	}
	return true, nil
}

// ---- gateway events ---------------------------------------------------------

// guardianTimeout bounds the work for one message or join.
const guardianTimeout = 15 * time.Second

// moderatorPerms exempt a member from message checks.
const moderatorPerms = discord.PermissionAdministrator | discord.PermissionManageGuild |
	discord.PermissionManageMessages | discord.PermissionModerateMembers

func (b *Bot) onGuildMessage(e *events.GuildMessageCreate) {
	m := e.Message
	if m.Author.Bot || m.Author.System || m.WebhookID != nil || m.Type.System() {
		return
	}
	msg := guardian.Message{
		GuildID: e.GuildID, ChannelID: e.ChannelID, ID: m.ID, AuthorID: m.Author.ID,
		AuthorName: m.Author.EffectiveName(), Content: m.Content, CreatedAt: m.CreatedAt,
	}
	for _, a := range m.Attachments {
		msg.Attachments = append(msg.Attachments, a.Filename+":"+strconv.Itoa(a.Size))
	}
	if m.Member != nil {
		msg.AuthorRoleIDs = m.Member.RoleIDs
		if m.Member.JoinedAt != nil {
			msg.AuthorJoinedAt = *m.Member.JoinedAt
		}
		member := *m.Member
		member.GuildID, member.User = e.GuildID, m.Author
		perms := b.client.Caches.MemberPermissions(member)
		msg.AuthorModerator = perms.Has(discord.PermissionAdministrator) || perms&moderatorPerms != 0
	}
	if g, ok := b.client.Caches.Guild(e.GuildID); ok && g.OwnerID == m.Author.ID {
		msg.AuthorModerator = true
	}
	go b.safely("guardian message", func(ctx context.Context) {
		start := time.Now()
		v, err := b.guard.HandleMessage(ctx, msg)
		if err != nil {
			b.log.Error("guardian message check failed", "guild_id", msg.GuildID, "err", err)
			return
		}
		if v.Response != guardian.ResponseNone {
			b.metrics.guardianActions.WithLabelValues(string(v.Response)).Inc()
			b.log.Info("guardian acted", "guild_id", msg.GuildID, "user_id", msg.AuthorID,
				"response", v.Response, "score", v.Score, "took", time.Since(start))
		}
	})
}

func (b *Bot) onMemberJoin(e *events.GuildMemberJoin) {
	j := guardian.Join{GuildID: e.GuildID, UserID: e.Member.User.ID, UserName: e.Member.User.EffectiveName(), Bot: e.Member.User.Bot}
	if e.Member.JoinedAt != nil {
		j.At = *e.Member.JoinedAt
	}
	w := welcomeMember(e)
	go b.safely("guardian join", func(ctx context.Context) {
		if err := b.guard.HandleJoin(ctx, j); err != nil {
			b.log.Error("guardian join check failed", "guild_id", j.GuildID, "err", err)
		}
		// Welcome runs after Guardian, so a join that starts raid mode (or
		// is removed by the age gate) is seen as such.
		if b.welcome != nil {
			if err := b.welcome.HandleJoin(ctx, w); err != nil {
				b.log.Warn("welcome failed", "guild_id", j.GuildID, "err", err)
			}
		}
	})
}

// safely runs fn with a timeout and turns a panic into a log line, so one
// bad event never takes the bot down.
func (b *Bot) safely(what string, fn func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("panic in "+what, "panic", r, "stack", string(debug.Stack()))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), guardianTimeout)
	defer cancel()
	fn(ctx)
}

// ---- commands ---------------------------------------------------------------

func (b *Bot) handleAudit(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	if on, err := b.guardianOn(e, mc); !on || err != nil {
		return err
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	res, err := b.runAudit(e.Ctx, mc.guildID)
	if err != nil {
		return err
	}
	return replyDeferred(e.UpdateInteractionResponse, b.presenter().auditCard(mc.locale, mc.guildName, res.Findings, res.WebhooksChecked))
}

func (b *Bot) handleRaidStart(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	l := discord.Locale(mc.locale)
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	r, err := b.guard.StartRaid(e.Ctx, mc.guildID, mc.actor)
	switch {
	case errors.Is(err, guardian.ErrModuleOff):
		return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.GuardianDisabled, nil), barNone))
	case errors.Is(err, guardian.ErrRaidActive):
		return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.GuardianRaidAlready, map[string]any{"Ends": ts(r.EndsAt, "R")}), barNone))
	case err != nil:
		return err
	}
	text := b.t(l, i18n.GuardianRaidStarted, map[string]any{"Ends": ts(r.EndsAt, "R")})
	if len(r.State.Problems) > 0 {
		text += "\n" + b.t(l, i18n.GuardianRaidProblems, nil)
		for _, p := range r.State.Problems {
			text += "\n- " + b.t(l, i18n.GuardianProblem(p), nil)
		}
	}
	return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiWarn, text, barEarlier))
}

func (b *Bot) handleRaidEnd(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	l := discord.Locale(mc.locale)
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	if _, err := b.guard.EndRaid(e.Ctx, mc.guildID, mc.actor); errors.Is(err, guardian.ErrNoRaid) {
		return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.GuardianRaidNone, nil), barNone))
	} else if err != nil {
		return err
	}
	return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiOK, b.t(l, i18n.GuardianRaidEnded, nil), barPass))
}

func (b *Bot) handleRaidStatus(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	l := discord.Locale(mc.locale)
	r, err := b.guard.ActiveRaid(e.Ctx, mc.guildID)
	if errors.Is(err, guardian.ErrNoRaid) {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiOK, b.t(l, i18n.GuardianRaidNone, nil), barPass))
	}
	if err != nil {
		return err
	}
	return b.replyNow(e, b.infoCard(mc.locale, EmojiWarn,
		b.t(l, i18n.GuardianRaidStatus, map[string]any{"Since": ts(r.StartedAt, "R"), "Ends": ts(r.EndsAt, "R")}), barEarlier))
}

func (b *Bot) handleVerifyPanel(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	if on, err := b.guardianOn(e, mc); !on || err != nil {
		return err
	}
	l := discord.Locale(mc.locale)
	s, err := guardian.LoadSettings(e.Ctx, b.deps.Config, mc.guildID)
	if err != nil {
		return err
	}
	if s.VerifyRoleID == 0 {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.GuardianVerifyNotSetUp, nil), barNone))
	}
	ch := data.Channel("channel")
	gl := b.guildLocale(mc.guildID)
	card := discord.NewContainer(
		discord.NewTextDisplay("### "+b.deps.Catalog.T(gl, i18n.GuardianVerifyPanelTitle, nil)+"\n"+b.deps.Catalog.T(gl, i18n.GuardianVerifyPanelBody, nil)),
		discord.NewActionRow(discord.NewSuccessButton(b.deps.Catalog.T(gl, i18n.GuardianButtonVerify, nil), "/guardian/verify")),
	)
	if _, err := b.client.Rest.CreateMessage(ch.ID, discord.MessageCreate{
		Flags: discord.MessageFlagIsComponentsV2, Components: []discord.LayoutComponent{card}, AllowedMentions: noPings,
	}, rest.WithCtx(e.Ctx)); err != nil {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiError, b.errorText(mc.locale, &moderation.ActionError{Action: "verify", Err: err}, ""), barFailed))
	}
	return b.replyNow(e, b.infoCard(mc.locale, EmojiOK, b.t(l, i18n.GuardianVerifyPosted, map[string]any{"Channel": "<#" + ch.ID.String() + ">"}), barPass))
}

// ---- buttons ----------------------------------------------------------------

func (b *Bot) ephemeralError(e *handler.ComponentEvent, text string) error {
	return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), AllowedMentions: noPings, Components: []discord.LayoutComponent{
		b.infoCard(e.Locale().Code(), EmojiError, text, barFailed)}})
}

func (b *Bot) componentActor(e *handler.ComponentEvent) (moderation.Actor, discord.Permissions, bool) {
	m := e.Member()
	gid := e.GuildID()
	if m == nil || gid == nil {
		return moderation.Actor{}, 0, false
	}
	return moderation.Actor{Rank: rankOf(b.client, *gid, m.User.ID, m.RoleIDs), Name: displayName(m.User, m.Nick)}, m.Permissions, true
}

func (b *Bot) handleResolveButton(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	l := e.Locale()
	actor, perms, ok := b.componentActor(e)
	if !ok {
		return errNotInGuild
	}
	id, err := strconv.ParseInt(e.Vars["id"], 10, 64)
	if err != nil {
		return b.ephemeralError(e, b.t(l, i18n.GuardianResolveNotFound, nil))
	}
	r := guardian.Resolution(e.Vars["action"])
	need, needName := discord.PermissionModerateMembers, "Timeout Members"
	if r == guardian.ResolveBan {
		need, needName = discord.PermissionBanMembers, "Ban Members"
	}
	if !perms.Has(need) && !perms.Has(discord.PermissionAdministrator) {
		return b.ephemeralError(e, b.t(l, i18n.GuardianResolveNoPerm, map[string]any{"Permission": needName}))
	}
	if err := e.DeferUpdateMessage(); err != nil {
		return err
	}
	ev, err := b.guard.Resolve(e.Ctx, *e.GuildID(), id, r, actor)
	if err != nil {
		var text string
		switch {
		case errors.Is(err, guardian.ErrAlreadyResolved):
			text = b.t(l, i18n.GuardianResolveAlready, nil)
		case errors.Is(err, guardian.ErrEventNotFound):
			text = b.t(l, i18n.GuardianResolveNotFound, nil)
		case errors.Is(err, guardian.ErrBadResolution):
			text = b.t(l, i18n.GuardianResolveBadAction, nil)
		default:
			text = b.errorText(l.Code(), err, userRef(ev.UserID, ev.UserName))
		}
		if ev.ID != 0 { // show the current state either way
			_ = replyDeferred(e.UpdateInteractionResponse, b.presenter().guardianCard(b.guildLocale(ev.GuildID), ev))
		}
		_, ferr := e.CreateFollowupMessage(discord.MessageCreate{Flags: ephemeralV2(), AllowedMentions: noPings,
			Components: []discord.LayoutComponent{b.infoCard(l.Code(), EmojiError, text, barFailed)}})
		return ferr
	}
	return replyDeferred(e.UpdateInteractionResponse, b.presenter().guardianCard(b.guildLocale(ev.GuildID), ev))
}

func (b *Bot) handleRaidEndButton(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	l := e.Locale()
	actor, perms, ok := b.componentActor(e)
	if !ok {
		return errNotInGuild
	}
	if !CanManageGuild(false, perms) {
		return b.ephemeralError(e, b.t(l, i18n.GuardianResolveNoPerm, map[string]any{"Permission": "Manage Server"}))
	}
	if err := e.DeferUpdateMessage(); err != nil {
		return err
	}
	r, err := b.guard.EndRaid(e.Ctx, *e.GuildID(), actor)
	if errors.Is(err, guardian.ErrNoRaid) {
		_, ferr := e.CreateFollowupMessage(discord.MessageCreate{Flags: ephemeralV2(), AllowedMentions: noPings,
			Components: []discord.LayoutComponent{b.infoCard(l.Code(), EmojiInfo, b.t(l, i18n.GuardianRaidNone, nil), barNone)}})
		return ferr
	}
	if err != nil {
		return err
	}
	if ev, err := b.guard.GetEvent(e.Ctx, *e.GuildID(), r.EventID); err == nil {
		return replyDeferred(e.UpdateInteractionResponse, b.presenter().guardianCard(b.guildLocale(ev.GuildID), ev))
	}
	return nil
}

func (b *Bot) handleVerifyButton(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	l := e.Locale()
	m := e.Member()
	gid := e.GuildID()
	if m == nil || gid == nil {
		return errNotInGuild
	}
	reply := func(emoji, text string, color int) error {
		return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), AllowedMentions: noPings,
			Components: []discord.LayoutComponent{b.infoCard(l.Code(), emoji, text, color)}})
	}
	s, err := guardian.LoadSettings(e.Ctx, b.deps.Config, *gid)
	if err != nil {
		return err
	}
	if s.VerifyRoleID == 0 {
		return reply(EmojiInfo, b.t(l, i18n.GuardianVerifyNotSetUp, nil), barNone)
	}
	for _, r := range m.RoleIDs {
		if r == s.VerifyRoleID {
			return reply(EmojiOK, b.t(l, i18n.GuardianVerifyAlready, nil), barPass)
		}
	}
	if ok, when := guardian.CanVerify(s, m.User.ID, time.Now()); !ok {
		return reply(EmojiWarn, b.t(l, i18n.GuardianVerifyTooNew, map[string]any{"When": ts(when, "R")}), barEarlier)
	}
	if err := b.client.Rest.AddMemberRole(*gid, m.User.ID, s.VerifyRoleID, rest.WithCtx(e.Ctx), rest.WithReason("Guardian verification gate")); err != nil {
		b.log.Warn("verification role failed", "guild_id", *gid, "err", err)
		return reply(EmojiError, b.t(l, i18n.GuardianVerifyFailed, nil), barFailed)
	}
	guildID, userID := *gid, m.User.ID
	b.logRun("welcome auto-roles", func(ctx context.Context) error { return b.welcome.HandleVerified(ctx, guildID, userID) })
	return reply(EmojiOK, b.t(l, i18n.GuardianVerifyOK, nil), barPass)
}
