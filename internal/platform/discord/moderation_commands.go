package discord

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/i18n"
	"github.com/nithron/nithro/internal/modules/moderation"
)

// ---- command definitions ----------------------------------------------------

func (b *Bot) moderationCommands() []discord.ApplicationCommandCreate {
	d := func(k i18n.Key) (string, map[discord.Locale]string) {
		return b.deps.Catalog.T("en", k, nil), localizations(b.deps.Catalog, k)
	}
	user := func(required bool, k i18n.Key) discord.ApplicationCommandOptionUser {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionUser{Name: "user", Description: desc, DescriptionLocalizations: loc, Required: required}
	}
	str := func(name string, k i18n.Key, required bool, maxLen int) discord.ApplicationCommandOptionString {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionString{Name: name, Description: desc, DescriptionLocalizations: loc, Required: required, MaxLength: &maxLen}
	}
	boolean := func(name string, k i18n.Key) discord.ApplicationCommandOptionBool {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionBool{Name: name, Description: desc, DescriptionLocalizations: loc}
	}
	num := func(name string, k i18n.Key, minV, maxV int) discord.ApplicationCommandOptionInt {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionInt{Name: name, Description: desc, DescriptionLocalizations: loc, Required: true, MinValue: &minV, MaxValue: &maxV}
	}
	reason := str("reason", i18n.ModOptReason, false, moderation.MaxReasonLength)
	silent := boolean("silent", i18n.ModOptSilent)

	cmd := func(name string, k i18n.Key, perm discord.Permissions, opts ...discord.ApplicationCommandOption) discord.SlashCommandCreate {
		desc, loc := d(k)
		return discord.SlashCommandCreate{
			Name: name, Description: desc, DescriptionLocalizations: loc, Options: opts,
			DefaultMemberPermissions: omit.New(&perm),
			Contexts:                 []discord.InteractionContextType{discord.InteractionContextTypeGuild},
			IntegrationTypes:         []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
		}
	}
	choice := func(k i18n.Key, value string) discord.ApplicationCommandOptionChoiceString {
		name, loc := d(k)
		return discord.ApplicationCommandOptionChoiceString{Name: name, NameLocalizations: loc, Value: value}
	}
	deleteMessages := str("delete_messages", i18n.ModOptDeleteMessages, false, 10)
	deleteMessages.MaxLength = nil
	deleteMessages.Choices = []discord.ApplicationCommandOptionChoiceString{
		choice(i18n.ModChoiceDeleteNone, "0"), choice(i18n.ModChoiceDelete1h, "1h"),
		choice(i18n.ModChoiceDelete24h, "24h"), choice(i18n.ModChoiceDelete7d, "7d"),
	}
	sub := func(name string, k i18n.Key, opts ...discord.ApplicationCommandOption) discord.ApplicationCommandOptionSubCommand {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionSubCommand{Name: name, Description: desc, DescriptionLocalizations: loc, Options: opts}
	}
	caseNum := num("number", i18n.ModOptCaseNumber, 1, 1_000_000)

	moderate := discord.PermissionModerateMembers
	return []discord.ApplicationCommandCreate{
		cmd("warn", i18n.CmdWarnDescription, moderate, user(true, i18n.ModOptUser), reason, silent),
		cmd("timeout", i18n.CmdTimeoutDescription, moderate, user(true, i18n.ModOptUser), str("duration", i18n.ModOptDurationTimeout, true, 20), reason, silent),
		cmd("untimeout", i18n.CmdUntimeoutDescription, moderate, user(true, i18n.ModOptUser), reason),
		cmd("kick", i18n.CmdKickDescription, discord.PermissionKickMembers, user(true, i18n.ModOptUser), reason, silent),
		cmd("ban", i18n.CmdBanDescription, discord.PermissionBanMembers, user(true, i18n.ModOptUserOrId), reason,
			str("duration", i18n.ModOptDurationBan, false, 20), deleteMessages, silent),
		cmd("unban", i18n.CmdUnbanDescription, discord.PermissionBanMembers, user(true, i18n.ModOptUserOrId), reason),
		cmd("purge", i18n.CmdPurgeDescription, discord.PermissionManageMessages, num("count", i18n.ModOptCount, 1, moderation.MaxPurge),
			user(false, i18n.ModOptPurgeUser), boolean("bots", i18n.ModOptBots), boolean("links", i18n.ModOptLinks), reason),
		cmd("note", i18n.CmdNoteDescription, moderate, user(true, i18n.ModOptUserOrId), str("text", i18n.ModOptNoteText, true, moderation.MaxReasonLength)),
		cmd("history", i18n.CmdHistoryDescription, moderate, user(true, i18n.ModOptUserOrId)),
		cmd("case", i18n.CmdCaseDescription, moderate,
			sub("view", i18n.CmdCaseViewDescription, caseNum),
			sub("reason", i18n.CmdCaseReasonDescription, caseNum, str("reason", i18n.ModOptNewReason, true, moderation.MaxReasonLength)),
			sub("delete", i18n.CmdCaseDeleteDescription, caseNum),
		),
	}
}

func (b *Bot) registerModeration(r handler.Router) {
	for _, a := range []moderation.Action{moderation.ActionWarn, moderation.ActionTimeout, moderation.ActionUntimeout,
		moderation.ActionKick, moderation.ActionUnban, moderation.ActionNote} {
		r.SlashCommand("/"+string(a), b.handleModAction(a))
	}
	r.SlashCommand("/ban", b.handleBan)
	r.ButtonComponent("/mod/ban/{token}/{choice}", b.handleBanConfirm)
	r.SlashCommand("/purge", b.handlePurge)
	r.SlashCommand("/history", b.handleHistory)
	r.SlashCommand("/case/view", b.handleCaseView)
	r.SlashCommand("/case/reason", b.handleCaseReason)
	r.SlashCommand("/case/delete", b.handleCaseDelete)
}

// ---- shared helpers ---------------------------------------------------------

// modContext is what every moderation handler needs about the interaction.
type modContext struct {
	guildID   snowflake.ID
	guildName string
	locale    string
	actor     moderation.Actor
}

func (b *Bot) modCtx(e *handler.CommandEvent) (modContext, bool) {
	gid := e.GuildID()
	m := e.Member()
	if gid == nil || m == nil {
		return modContext{}, false
	}
	mc := modContext{guildID: *gid, locale: e.Locale().Code()}
	if g, ok := e.Guild(); ok {
		mc.guildName = g.Name
	}
	mc.actor = moderation.Actor{
		Rank: rankOf(b.client, *gid, m.User.ID, m.RoleIDs),
		Name: displayName(m.User, m.Nick),
	}
	return mc, true
}

// requireModule replies with a hint when the Moderation module is off.
func (b *Bot) moduleOn(ctx context.Context, e *handler.CommandEvent, mc modContext) (bool, error) {
	on, err := b.deps.Config.IsEnabled(ctx, mc.guildID, core.ModuleModeration)
	if err != nil {
		return false, err
	}
	if !on {
		return false, b.replyNow(e, b.infoCard(mc.locale, EmojiInfo, b.t(discord.Locale(mc.locale), i18n.ModDisabled, nil), colorBrand))
	}
	return true, nil
}

func (b *Bot) infoCard(_ string, emoji, text string, color int) discord.ContainerComponent {
	return discord.NewContainer(discord.NewTextDisplay(strings.TrimSpace(b.emojis.Get(emoji) + " " + text))).WithAccentColor(color)
}

func (b *Bot) replyNow(e *handler.CommandEvent, c discord.ContainerComponent) error {
	return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), Components: []discord.LayoutComponent{c}, AllowedMentions: noPings})
}

// replyDeferred edits the deferred ephemeral response.
func replyDeferred(update func(discord.MessageUpdate, ...rest.RequestOpt) (*discord.Message, error), c discord.ContainerComponent) error {
	comps := []discord.LayoutComponent{c}
	flags := discord.MessageFlagIsComponentsV2
	_, err := update(discord.MessageUpdate{Components: &comps, Flags: &flags, AllowedMentions: noPings})
	return err
}

// errorText turns a domain error into a translated message. Unknown errors
// are logged and shown generically.
func (b *Bot) errorText(locale string, err error, target string) string {
	l := discord.Locale(locale)
	u := map[string]any{"User": target}
	var ae *moderation.ActionError
	switch {
	case errors.Is(err, core.ErrTargetIsSelf):
		return b.t(l, i18n.ModErrSelf, nil)
	case errors.Is(err, core.ErrTargetIsOwner):
		return b.t(l, i18n.ModErrOwner, nil)
	case errors.Is(err, core.ErrTargetIsBot):
		return b.t(l, i18n.ModErrTargetBot, nil)
	case errors.Is(err, core.ErrActorRankTooLow):
		return b.t(l, i18n.ModErrActorRank, u)
	case errors.Is(err, core.ErrBotRankTooLow):
		return b.t(l, i18n.ModErrBotRank, u)
	case errors.Is(err, moderation.ErrNotMember):
		return b.t(l, i18n.ModErrNotMember, u)
	case errors.Is(err, moderation.ErrNotBanned):
		return b.t(l, i18n.ModErrNotBanned, u)
	case errors.Is(err, moderation.ErrNotTimedOut):
		return b.t(l, i18n.ModErrNotTimedOut, u)
	case errors.Is(err, moderation.ErrReasonRequired):
		return b.t(l, i18n.ModErrReasonRequired, nil)
	case errors.Is(err, moderation.ErrReasonTooLong):
		return b.t(l, i18n.ModErrReasonTooLong, nil)
	case errors.Is(err, moderation.ErrBadDuration):
		return b.t(l, i18n.ModErrBadDuration, nil)
	case errors.Is(err, moderation.ErrBadPurge):
		return b.t(l, i18n.ModErrPurgeCount, nil)
	case errors.As(err, &ae):
		var re *rest.Error
		msg := ae.Err.Error()
		if errors.As(ae.Err, &re) && re.Message != "" {
			msg = re.Message
		}
		return b.t(l, i18n.ModErrActionFailed, map[string]any{"Error": msg})
	}
	b.log.Error("moderation command failed", "err", err)
	return b.t(l, i18n.ErrorGeneric, nil)
}

func (b *Bot) errorCard(mc modContext, err error, target string) discord.ContainerComponent {
	return b.infoCard(mc.locale, EmojiError, b.errorText(mc.locale, err, target), colorDanger)
}

// resultCard summarises a finished action for the moderator.
func (b *Bot) resultCard(mc modContext, out moderation.Outcome) discord.ContainerComponent {
	l := discord.Locale(mc.locale)
	c := out.Case
	who := "**" + escapeMarkdown(c.TargetName) + "**"
	if c.TargetName == "" {
		who = fmt.Sprintf("<@%s>", c.TargetID)
	}
	data := map[string]any{"User": who, "Duration": moderation.FormatDuration(c.Duration), "Count": c.Details.Deleted}
	var key i18n.Key
	switch c.Action {
	case moderation.ActionWarn:
		key = i18n.ModReplyWarn
	case moderation.ActionTimeout:
		key = i18n.ModReplyTimeout
	case moderation.ActionUntimeout:
		key = i18n.ModReplyUntimeout
	case moderation.ActionKick:
		key = i18n.ModReplyKick
	case moderation.ActionBan:
		key = i18n.ModReplyBan
		if c.Details.Temporary {
			key = i18n.ModReplyBanTemp
		}
	case moderation.ActionUnban:
		key = i18n.ModReplyUnban
	case moderation.ActionNote:
		key = i18n.ModReplyNote
	case moderation.ActionPurge:
		key = i18n.ModReplyPurge
	}
	lines := []string{fmt.Sprintf("**%s** · %s", b.t(l, i18n.ModReplyCase, map[string]any{"Number": c.Number}), b.t(l, key, data))}
	if notifiable(c.Action) && c.TargetID != 0 {
		if out.DMSent {
			lines = append(lines, b.t(l, i18n.ModReplyDmSent, nil))
		} else if c.Action != moderation.ActionNote {
			lines = append(lines, b.t(l, i18n.ModReplyDmFailed, nil))
		}
	}
	if out.LogPosted {
		lines = append(lines, b.t(l, i18n.ModReplyLogged, map[string]any{"Channel": fmt.Sprintf("<#%s>", c.LogChannelID)}))
	} else {
		lines = append(lines, "-# "+b.t(l, i18n.ModReplyNoLog, nil))
	}
	return discord.NewContainer(discord.NewTextDisplay(b.emojis.Get(EmojiOK) + " " + strings.Join(lines, "\n"))).WithAccentColor(colorSuccess)
}

func targetLabel(u discord.User) string { return "**" + escapeMarkdown(u.EffectiveName()) + "**" }

func optSilent(data discord.SlashCommandInteractionData) *bool {
	if s, ok := data.OptBool("silent"); ok && s {
		notify := false
		return &notify
	}
	return nil
}

// ---- action commands --------------------------------------------------------

// handleModAction handles warn, timeout, untimeout, kick, unban and note.
func (b *Bot) handleModAction(action moderation.Action) handler.SlashCommandHandler {
	return func(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
		mc, ok := b.modCtx(e)
		if !ok {
			return errNotInGuild
		}
		if on, err := b.moduleOn(e.Ctx, e, mc); !on || err != nil {
			return err
		}
		target := data.User("user")
		req := moderation.Request{
			GuildID: mc.guildID, GuildName: mc.guildName, Action: action, Actor: mc.actor,
			TargetID: target.ID, TargetName: target.Username, Notify: optSilent(data),
		}
		req.Reason, _ = data.OptString("reason")
		if action == moderation.ActionNote {
			req.Reason, _ = data.OptString("text")
		}
		if action == moderation.ActionTimeout {
			d, err := moderation.ParseDuration(data.String("duration"))
			if err != nil {
				return b.replyNow(e, b.errorCard(mc, moderation.ErrBadDuration, ""))
			}
			req.Duration = d
		}
		if err := e.DeferCreateMessage(true); err != nil {
			return err
		}
		out, err := b.mod.Execute(e.Ctx, req)
		if err != nil {
			return replyDeferred(e.UpdateInteractionResponse, b.errorCard(mc, err, targetLabel(target)))
		}
		return replyDeferred(e.UpdateInteractionResponse, b.resultCard(mc, out))
	}
}

// pendingBans holds ban requests waiting for the moderator to confirm.
type pendingBans struct {
	mu sync.Mutex
	m  map[string]pendingBan
}

type pendingBan struct {
	req     moderation.Request
	mc      modContext
	target  string
	expires time.Time
}

func (p *pendingBans) put(pb pendingBan) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]pendingBan{}
	}
	now := time.Now()
	for k, v := range p.m { // drop expired entries
		if now.After(v.expires) {
			delete(p.m, k)
		}
	}
	token := strconv.FormatInt(now.UnixNano(), 36)
	p.m[token] = pb
	return token
}

func (p *pendingBans) take(token string) (pendingBan, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pb, ok := p.m[token]
	delete(p.m, token)
	if !ok || time.Now().After(pb.expires) {
		return pendingBan{}, false
	}
	return pb, true
}

func (b *Bot) handleBan(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	if on, err := b.moduleOn(e.Ctx, e, mc); !on || err != nil {
		return err
	}
	target := data.User("user")
	req := moderation.Request{
		GuildID: mc.guildID, GuildName: mc.guildName, Action: moderation.ActionBan, Actor: mc.actor,
		TargetID: target.ID, TargetName: target.Username, Notify: optSilent(data),
	}
	req.Reason, _ = data.OptString("reason")
	if s, ok := data.OptString("duration"); ok && strings.TrimSpace(s) != "" {
		d, err := moderation.ParseDuration(s)
		if err != nil {
			return b.replyNow(e, b.errorCard(mc, moderation.ErrBadDuration, ""))
		}
		req.Duration = d
	}
	if s, ok := data.OptString("delete_messages"); ok && s != "0" {
		req.DeleteMessages, _ = moderation.ParseDuration(s)
	}

	l := discord.Locale(mc.locale)
	label := targetLabel(target)
	token := b.pending.put(pendingBan{req: req, mc: mc, target: label, expires: time.Now().Add(2 * time.Minute)})
	body := b.t(l, i18n.ModConfirmBanBody, nil)
	if req.Duration > 0 {
		body = b.t(l, i18n.ModConfirmBanTempBody, map[string]any{"Duration": moderation.FormatDuration(req.Duration)})
	}
	card := discord.NewContainer(
		discord.NewTextDisplay("### "+b.t(l, i18n.ModConfirmBanTitle, map[string]any{"User": label})+"\n"+body),
		discord.NewActionRow(
			discord.NewDangerButton(b.t(l, i18n.ModConfirmBanButton, nil), "/mod/ban/"+token+"/yes"),
			discord.NewSecondaryButton(b.t(l, i18n.ModConfirmCancelButton, nil), "/mod/ban/"+token+"/no"),
		),
	).WithAccentColor(colorDanger)
	return b.replyNow(e, card)
}

func (b *Bot) handleBanConfirm(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	l := e.Locale()
	done := func(c discord.ContainerComponent) error {
		comps := []discord.LayoutComponent{c}
		return e.UpdateMessage(discord.MessageUpdate{Components: &comps, AllowedMentions: noPings})
	}
	pb, ok := b.pending.take(e.Vars["token"])
	if !ok {
		return done(b.infoCard(l.Code(), EmojiInfo, b.t(l, i18n.ModConfirmExpired, nil), colorBrand))
	}
	if e.User().ID != pb.req.Actor.ID {
		b.pending.put(pb) // keep it for the right moderator
		return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), Components: []discord.LayoutComponent{
			b.infoCard(l.Code(), EmojiError, b.t(l, i18n.ModConfirmNotYours, nil), colorDanger)}})
	}
	if e.Vars["choice"] != "yes" {
		return done(b.infoCard(l.Code(), EmojiInfo, b.t(l, i18n.ModConfirmCancelled, nil), colorBrand))
	}
	if err := e.DeferUpdateMessage(); err != nil {
		return err
	}
	out, err := b.mod.Execute(e.Ctx, pb.req)
	if err != nil {
		return replyDeferred(e.UpdateInteractionResponse, b.errorCard(pb.mc, err, pb.target))
	}
	return replyDeferred(e.UpdateInteractionResponse, b.resultCard(pb.mc, out))
}

func (b *Bot) handlePurge(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	if on, err := b.moduleOn(e.Ctx, e, mc); !on || err != nil {
		return err
	}
	req := moderation.PurgeRequest{GuildID: mc.guildID, ChannelID: e.Channel().ID(), Actor: mc.actor, Count: data.Int("count")}
	if u, ok := data.OptUser("user"); ok {
		req.Filter.UserID = u.ID
	}
	req.Filter.BotsOnly, _ = data.OptBool("bots")
	req.Filter.LinksOnly, _ = data.OptBool("links")
	req.Reason, _ = data.OptString("reason")
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	out, err := b.mod.Purge(e.Ctx, req)
	if err != nil {
		return replyDeferred(e.UpdateInteractionResponse, b.errorCard(mc, err, ""))
	}
	return replyDeferred(e.UpdateInteractionResponse, b.resultCard(mc, out))
}

// ---- case commands ----------------------------------------------------------

func (b *Bot) handleHistory(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	if on, err := b.moduleOn(e.Ctx, e, mc); !on || err != nil {
		return err
	}
	l := discord.Locale(mc.locale)
	target := data.User("user")
	cases, total, err := b.mod.ListCases(e.Ctx, mc.guildID, moderation.CaseFilter{TargetID: target.ID, Limit: 10})
	if err != nil {
		return err
	}
	label := targetLabel(target)
	if total == 0 {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.ModHistoryEmpty, map[string]any{"User": label}), colorBrand))
	}
	var lines []string
	for _, c := range cases {
		reason := c.Reason
		if r := []rune(reason); len(r) > 80 {
			reason = string(r[:80]) + "…"
		}
		line := fmt.Sprintf("`#%d` %s **%s** · %s", c.Number, actionEmoji(b.emojis, c.Action), b.t(l, i18n.ModAction(string(c.Action)), nil), ts(c.CreatedAt, "R"))
		if reason != "" {
			line += "\n-# " + escapeMarkdown(reason)
		}
		lines = append(lines, line)
	}
	container := discord.NewContainer(
		discord.NewTextDisplay("### "+b.t(l, i18n.ModHistoryTitle, map[string]any{"User": label})),
		discord.NewTextDisplay(strings.Join(lines, "\n")),
	).WithAccentColor(colorBrand)
	if total > len(cases) {
		container = container.AddComponents(discord.NewTextDisplay("-# " + b.t(l, i18n.ModHistoryMore, map[string]any{"Shown": len(cases), "Total": total})))
	}
	url := b.deps.DashboardURL.JoinPath("guilds", mc.guildID.String(), "cases")
	q := url.Query()
	q.Set("user", target.ID.String())
	url.RawQuery = q.Encode()
	container = withLinks(container, link{b.t(l, i18n.ModHistoryViewAll, nil), url.String()})
	return b.replyNow(e, container)
}

func (b *Bot) handleCaseView(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	c, err := b.mod.GetCase(e.Ctx, mc.guildID, data.Int("number"))
	if errors.Is(err, moderation.ErrCaseNotFound) {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiError, b.t(discord.Locale(mc.locale), i18n.ModErrCaseNotFound, map[string]any{"Number": data.Int("number")}), colorDanger))
	}
	if err != nil {
		return err
	}
	return b.replyNow(e, b.presenter().caseCard(mc.locale, c))
}

func (b *Bot) handleCaseReason(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	l := discord.Locale(mc.locale)
	n := data.Int("number")
	_, err := b.mod.UpdateReason(e.Ctx, mc.guildID, n, data.String("reason"))
	switch {
	case errors.Is(err, moderation.ErrCaseNotFound):
		return b.replyNow(e, b.infoCard(mc.locale, EmojiError, b.t(l, i18n.ModErrCaseNotFound, map[string]any{"Number": n}), colorDanger))
	case err != nil:
		return b.replyNow(e, b.errorCard(mc, err, ""))
	}
	return b.replyNow(e, b.infoCard(mc.locale, EmojiOK, b.t(l, i18n.ModReplyReasonUpdated, map[string]any{"Number": n}), colorSuccess))
}

func (b *Bot) handleCaseDelete(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	l := discord.Locale(mc.locale)
	n := data.Int("number")
	_, err := b.mod.DeleteCase(e.Ctx, mc.guildID, n, mc.actor.ID)
	if errors.Is(err, moderation.ErrCaseNotFound) {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiError, b.t(l, i18n.ModErrCaseNotFound, map[string]any{"Number": n}), colorDanger))
	}
	if err != nil {
		return err
	}
	return b.replyNow(e, b.infoCard(mc.locale, EmojiOK, b.t(l, i18n.ModReplyCaseDeleted, map[string]any{"Number": n}), colorSuccess))
}
