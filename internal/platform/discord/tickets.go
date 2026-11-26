package discord

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/tickets"
	"github.com/nithron/niro/internal/rpc"
)

// ---- platform ---------------------------------------------------------------

// ticketsPlatform implements tickets.Platform on Discord.
type ticketsPlatform struct {
	client *bot.Client
	p      presenter
	locale func(snowflake.ID) string
}

var _ tickets.Platform = ticketsPlatform{}

// ticketAccess is what openers and staff may do in a ticket channel.
// (Only permissions Niro itself is invited with: it can't grant others.)
const ticketAccess = discord.PermissionViewChannel | discord.PermissionSendMessages | discord.PermissionReadMessageHistory |
	discord.PermissionAttachFiles | discord.PermissionEmbedLinks

// maxStaffAdded caps how many staff members Niro adds to a private thread
// (they are found in the member cache).
const maxStaffAdded = 25

func ticketName(t tickets.Ticket) string { return fmt.Sprintf("ticket-%04d", t.Number) }

func ticketOpenID(panelID int64, categoryID string) string {
	return "/tickets/open/" + strconv.FormatInt(panelID, 10) + "/" + categoryID
}

func (tp ticketsPlatform) panelCard(p tickets.Panel) discord.ContainerComponent {
	buttons := make([]discord.InteractiveComponent, len(p.Categories))
	for i, c := range p.Categories {
		b := discord.NewPrimaryButton(c.Label, ticketOpenID(p.ID, c.ID))
		if c.Emoji != "" {
			b = b.WithEmoji(discord.ComponentEmoji{Name: c.Emoji})
		}
		buttons[i] = b
	}
	return renderDocument(p.Message, discord.NewActionRow(buttons...))
}

func (tp ticketsPlatform) PostPanel(ctx context.Context, p tickets.Panel) (snowflake.ID, error) {
	m, err := tp.client.Rest.CreateMessage(p.ChannelID, documentMessage(tp.panelCard(p), nil), rest.WithCtx(ctx))
	if err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (tp ticketsPlatform) EditPanel(ctx context.Context, p tickets.Panel) error {
	comps := []discord.LayoutComponent{tp.panelCard(p)}
	_, err := tp.client.Rest.UpdateMessage(p.ChannelID, p.MessageID, discord.MessageUpdate{Components: &comps, AllowedMentions: noPings}, rest.WithCtx(ctx))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage, rest.JSONErrorCodeUnknownChannel) {
		return tickets.ErrMessageGone
	}
	return err
}

func (tp ticketsPlatform) DeleteMessage(ctx context.Context, channelID, messageID snowflake.ID) error {
	err := tp.client.Rest.DeleteMessage(channelID, messageID, rest.WithCtx(ctx))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage, rest.JSONErrorCodeUnknownChannel) {
		return nil
	}
	return err
}

func (tp ticketsPlatform) RoleExists(ctx context.Context, guildID, roleID snowflake.ID) bool {
	if roleID == guildID {
		return false
	}
	if _, ok := tp.client.Caches.Role(guildID, roleID); ok {
		return true
	}
	list, err := tp.client.Rest.GetRoles(guildID, rest.WithCtx(ctx))
	return err == nil && slices.ContainsFunc(list, func(r discord.Role) bool { return r.ID == roleID })
}

func (tp ticketsPlatform) OpenThread(ctx context.Context, t tickets.Ticket, channelID snowflake.ID, staff []snowflake.ID) (snowflake.ID, error) {
	no := false
	th, err := tp.client.Rest.CreateThread(channelID, discord.GuildPrivateThreadCreate{
		Name: ticketName(t), AutoArchiveDuration: discord.AutoArchiveDuration1w, Invitable: &no,
	}, rest.WithCtx(ctx), rest.WithReason(fmt.Sprintf("Ticket #%d opened by %s", t.Number, t.OpenerID)))
	if err != nil {
		return 0, err
	}
	if err := tp.client.Rest.AddThreadMember(th.ID(), t.OpenerID, rest.WithCtx(ctx)); err != nil {
		_ = tp.client.Rest.DeleteChannel(th.ID(), rest.WithCtx(ctx))
		return 0, err
	}
	// Private threads have no permission overwrites: add the staff Niro
	// knows (members with Manage Threads see every private thread anyway).
	added := 0
	for m := range tp.client.Caches.Members(t.GuildID) {
		if added == maxStaffAdded {
			break
		}
		if m.User.Bot || m.User.ID == t.OpenerID || !slices.ContainsFunc(m.RoleIDs, func(r snowflake.ID) bool { return slices.Contains(staff, r) }) {
			continue
		}
		if tp.client.Rest.AddThreadMember(th.ID(), m.User.ID, rest.WithCtx(ctx)) == nil {
			added++
		}
	}
	return th.ID(), nil
}

func (tp ticketsPlatform) OpenChannel(ctx context.Context, t tickets.Ticket, parentID snowflake.ID, staff []snowflake.ID) (snowflake.ID, error) {
	self, ok := tp.client.Caches.SelfUser()
	if !ok {
		return 0, errors.New("bot user not known yet")
	}
	overwrites := []discord.PermissionOverwrite{
		discord.RolePermissionOverwrite{RoleID: t.GuildID, Deny: discord.PermissionViewChannel}, // @everyone
		discord.MemberPermissionOverwrite{UserID: self.ID, Allow: ticketAccess | discord.PermissionManageChannels | discord.PermissionManageMessages},
		discord.MemberPermissionOverwrite{UserID: t.OpenerID, Allow: ticketAccess},
	}
	for _, r := range staff {
		overwrites = append(overwrites, discord.RolePermissionOverwrite{RoleID: r, Allow: ticketAccess})
	}
	ch, err := tp.client.Rest.CreateGuildChannel(t.GuildID, discord.GuildTextChannelCreate{
		Name: ticketName(t), ParentID: parentID, PermissionOverwrites: overwrites,
	}, rest.WithCtx(ctx), rest.WithReason(fmt.Sprintf("Ticket #%d opened by %s", t.Number, t.OpenerID)))
	if err != nil {
		return 0, err
	}
	return ch.ID(), nil
}

func (tp ticketsPlatform) PostOpening(ctx context.Context, t tickets.Ticket, c tickets.Category, answers []tickets.Answer) error {
	l := tp.locale(t.GuildID)
	mentions := "<@" + t.OpenerID.String() + ">"
	for _, r := range c.StaffRoleIDs {
		mentions += " <@&" + r.String() + ">"
	}
	head := discord.NewTextDisplay(mentions + "\n" + tp.p.t(l, i18n.TicketHeader, map[string]any{
		"Number": t.Number, "Category": escapeMarkdown(c.Label), "User": userRef(t.OpenerID, t.OpenerName),
	}))
	var extra []discord.ContainerSubComponent
	if len(answers) > 0 {
		var b strings.Builder
		for _, a := range answers {
			if a.Answer == "" {
				continue
			}
			fmt.Fprintf(&b, "**%s**\n> %s\n", escapeMarkdown(a.Question), strings.ReplaceAll(escapeMarkdown(a.Answer), "\n", "\n> "))
		}
		if b.Len() > 0 {
			extra = append(extra, discord.NewTextDisplay(clip(b.String(), 3000)))
		}
	}
	extra = append(extra, discord.NewActionRow(
		discord.NewSecondaryButton(tp.p.t(l, i18n.TicketButtonClaim, nil), "/tickets/claim/"+strconv.FormatInt(t.ID, 10)),
		discord.NewDangerButton(tp.p.t(l, i18n.TicketButtonClose, nil), "/tickets/close/"+strconv.FormatInt(t.ID, 10)),
	))
	card := renderDocument(c.Opening, extra...)
	card.Components = append([]discord.ContainerSubComponent{head}, card.Components...)
	// Notify the opener and staff (adding them to a private thread); never @everyone.
	allowed := &discord.AllowedMentions{Users: []snowflake.ID{t.OpenerID}, Roles: c.StaffRoleIDs}
	_, err := tp.client.Rest.CreateMessage(t.ChannelID, documentMessage(card, allowed), rest.WithCtx(ctx))
	return err
}

func (tp ticketsPlatform) Announce(ctx context.Context, t tickets.Ticket, a tickets.Announcement, by tickets.Actor) error {
	l := tp.locale(t.GuildID)
	u := map[string]any{"User": userRef(by.ID, by.Name)}
	var text string
	var row []discord.InteractiveComponent
	id := strconv.FormatInt(t.ID, 10)
	switch a {
	case tickets.AnnounceClaimed:
		text = tp.p.t(l, i18n.TicketAnnounceClaimed, u)
	case tickets.AnnounceReopened:
		text = tp.p.t(l, i18n.TicketAnnounceReopened, u)
	case tickets.AnnounceClosed:
		text = tp.p.t(l, i18n.TicketAnnounceClosed, u)
		if t.CloseReason != "" {
			text += "\n" + tp.p.t(l, i18n.TicketAnnounceReason, map[string]any{"Reason": escapeMarkdown(t.CloseReason)})
		}
		row = []discord.InteractiveComponent{
			discord.NewSecondaryButton(tp.p.t(l, i18n.TicketButtonReopen, nil), "/tickets/reopen/"+id),
			discord.NewDangerButton(tp.p.t(l, i18n.TicketButtonDelete, nil), "/tickets/delete/"+id),
		}
	}
	comps := []discord.ContainerSubComponent{discord.NewTextDisplay(text)}
	if row != nil {
		comps = append(comps, discord.NewActionRow(row...))
	}
	_, err := tp.client.Rest.CreateMessage(t.ChannelID, documentMessage(discord.NewContainer(comps...).WithAccentColor(colorBrand), nil), rest.WithCtx(ctx))
	return err
}

func (tp ticketsPlatform) setThread(ctx context.Context, t tickets.Ticket, archived bool) error {
	_, err := tp.client.Rest.UpdateChannel(t.ChannelID, discord.GuildThreadUpdate{Archived: &archived, Locked: &archived}, rest.WithCtx(ctx))
	return err
}

// Close hides the ticket from the opener and from members added with
// /ticket add; staff keep it, with Reopen and Delete.
func (tp ticketsPlatform) Close(ctx context.Context, t tickets.Ticket) error {
	if t.Mode == tickets.ModeThread {
		// Thread members can still read an archived thread: remove the
		// opener (staff and Manage Threads holders keep access).
		if err := tp.client.Rest.RemoveThreadMember(t.ChannelID, t.OpenerID, rest.WithCtx(ctx)); err != nil {
			return err
		}
		return tp.setThread(ctx, t, true)
	}
	ch, err := tp.client.Rest.GetChannel(t.ChannelID, rest.WithCtx(ctx))
	if err != nil {
		return err
	}
	gc, ok := ch.(discord.GuildChannel)
	if !ok {
		return errors.New("ticket channel is not a server channel")
	}
	self := tp.BotID()
	for _, o := range gc.PermissionOverwrites() {
		if mo, ok := o.(discord.MemberPermissionOverwrite); ok && mo.UserID != self {
			if err := tp.client.Rest.DeletePermissionOverwrite(t.ChannelID, mo.UserID, rest.WithCtx(ctx)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (tp ticketsPlatform) Reopen(ctx context.Context, t tickets.Ticket) error {
	if t.Mode == tickets.ModeThread {
		if err := tp.setThread(ctx, t, false); err != nil {
			return err
		}
		return tp.client.Rest.AddThreadMember(t.ChannelID, t.OpenerID, rest.WithCtx(ctx))
	}
	return tp.allow(ctx, t.ChannelID, t.OpenerID)
}

func (tp ticketsPlatform) BotID() snowflake.ID {
	if u, ok := tp.client.Caches.SelfUser(); ok {
		return u.ID
	}
	return 0
}

func (tp ticketsPlatform) MemberRoles(ctx context.Context, guildID, userID snowflake.ID) ([]snowflake.ID, error) {
	if m, ok := tp.client.Caches.Member(guildID, userID); ok {
		return slices.Clone(m.RoleIDs), nil
	}
	m, err := tp.client.Rest.GetMember(guildID, userID, rest.WithCtx(ctx))
	if err != nil {
		return nil, err
	}
	return m.RoleIDs, nil
}

func (tp ticketsPlatform) allow(ctx context.Context, channelID, userID snowflake.ID) error {
	allow := ticketAccess
	return tp.client.Rest.UpdatePermissionOverwrite(channelID, userID, discord.MemberPermissionOverwriteUpdate{Allow: &allow}, rest.WithCtx(ctx))
}

func (tp ticketsPlatform) Delete(ctx context.Context, t tickets.Ticket) error {
	err := tp.client.Rest.DeleteChannel(t.ChannelID, rest.WithCtx(ctx), rest.WithReason(fmt.Sprintf("Ticket #%d deleted", t.Number)))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownChannel) {
		return nil
	}
	return err
}

func (tp ticketsPlatform) AddMember(ctx context.Context, t tickets.Ticket, userID snowflake.ID) error {
	if t.Mode == tickets.ModeThread {
		return tp.client.Rest.AddThreadMember(t.ChannelID, userID, rest.WithCtx(ctx))
	}
	return tp.allow(ctx, t.ChannelID, userID)
}

func (tp ticketsPlatform) RemoveMember(ctx context.Context, t tickets.Ticket, userID snowflake.ID) error {
	if t.Mode == tickets.ModeThread {
		return tp.client.Rest.RemoveThreadMember(t.ChannelID, userID, rest.WithCtx(ctx))
	}
	return tp.client.Rest.DeletePermissionOverwrite(t.ChannelID, userID, rest.WithCtx(ctx))
}

// Messages pages forward from the channel's creation (its ID is older than
// every message in it).
func (tp ticketsPlatform) Messages(ctx context.Context, channelID snowflake.ID, limit int) ([]tickets.TranscriptMessage, bool, error) {
	var out []tickets.TranscriptMessage
	after := channelID
	for len(out) < limit {
		page, err := tp.client.Rest.GetMessages(channelID, 0, 0, after, 100, rest.WithCtx(ctx))
		if err != nil {
			return out, false, err
		}
		if len(page) == 0 {
			return out, false, nil
		}
		slices.SortFunc(page, func(a, b discord.Message) int { return compareSnowflakes(a.ID, b.ID) })
		for _, m := range page {
			if len(out) == limit {
				return out, true, nil
			}
			tm := tickets.TranscriptMessage{ID: m.ID, AuthorID: m.Author.ID, AuthorName: m.Author.EffectiveName(),
				AuthorBot: m.Author.Bot, Content: transcriptText(m), At: m.CreatedAt}
			for _, a := range m.Attachments {
				tm.Attachments = append(tm.Attachments, a.Filename)
			}
			out = append(out, tm)
		}
		after = page[len(page)-1].ID
		if len(page) < 100 {
			return out, false, nil
		}
	}
	// Exactly at the limit: there are more only if another message exists.
	next, err := tp.client.Rest.GetMessages(channelID, 0, 0, after, 1, rest.WithCtx(ctx))
	return out, err == nil && len(next) > 0, nil
}

func compareSnowflakes(a, b snowflake.ID) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// transcriptText is a message's text; Components V2 messages (Niro's own)
// keep their text in text displays.
func transcriptText(m discord.Message) string {
	if m.Content != "" || m.Flags&discord.MessageFlagIsComponentsV2 == 0 {
		return m.Content
	}
	var parts []string
	var walk func(c discord.Component)
	walk = func(c discord.Component) {
		switch v := c.(type) {
		case discord.TextDisplayComponent:
			parts = append(parts, v.Content)
		case discord.ContainerComponent:
			for _, s := range v.Components {
				walk(s)
			}
		case discord.SectionComponent:
			for _, s := range v.Components {
				walk(s)
			}
		}
	}
	for _, c := range m.Components {
		walk(c)
	}
	return strings.Join(parts, "\n")
}

// maxTranscriptFile stays under Discord's upload limit for every server.
const maxTranscriptFile = 9 << 20

func (tp ticketsPlatform) PostTranscript(ctx context.Context, channelID snowflake.ID, t tickets.Ticket, htmlGzip []byte) error {
	l := tp.locale(t.GuildID)
	html, err := tickets.Transcript{HTMLGzip: htmlGzip}.HTML()
	if err != nil {
		return err
	}
	data := map[string]any{"Number": t.Number, "Category": escapeMarkdown(t.CategoryLabel),
		"User": userRef(t.OpenerID, t.OpenerName), "ClosedBy": userRef(t.ClosedBy, t.ClosedByName)}
	if len(html) > maxTranscriptFile {
		card := discord.NewContainer(discord.NewTextDisplay(tp.p.t(l, i18n.TicketTranscriptBig, data))).WithAccentColor(colorWarning)
		_, err := tp.client.Rest.CreateMessage(channelID, documentMessage(card, nil), rest.WithCtx(ctx))
		return err
	}
	name := fmt.Sprintf("ticket-%04d.html", t.Number)
	card := discord.NewContainer(
		discord.NewTextDisplay(tp.p.t(l, i18n.TicketTranscriptPosted, data)),
		discord.NewFileComponent("attachment://"+name),
	).WithAccentColor(colorBrand)
	m := documentMessage(card, nil)
	m.Files = []*discord.File{discord.NewFile(name, "", bytes.NewReader(html))}
	_, err = tp.client.Rest.CreateMessage(channelID, m, rest.WithCtx(ctx))
	return err
}

func (tp ticketsPlatform) ServerName(guildID snowflake.ID) string {
	if g, ok := tp.client.Caches.Guild(guildID); ok {
		return g.Name
	}
	return ""
}

// ---- interactions -----------------------------------------------------------

func (b *Bot) registerTickets(r handler.Router) {
	r.ButtonComponent("/tickets/open/{panel}/{category}", b.handleTicketOpen)
	r.Modal("/tickets/form/{panel}/{category}", b.handleTicketForm)
	r.ButtonComponent("/tickets/claim/{id}", b.handleTicketClaim)
	r.ButtonComponent("/tickets/close/{id}", b.handleTicketCloseButton)
	r.Modal("/tickets/closing/{id}", b.handleTicketCloseForm)
	r.ButtonComponent("/tickets/reopen/{id}", b.handleTicketReopen)
	r.ButtonComponent("/tickets/delete/{id}", b.handleTicketDelete)
	r.SlashCommand("/ticket/add", b.handleTicketAddCmd)
	r.SlashCommand("/ticket/remove", b.handleTicketRemoveCmd)
	r.SlashCommand("/ticket/close", b.handleTicketCloseCmd)
	r.SlashCommand("/ticket/claim", b.handleTicketClaimCmd)
}

func (b *Bot) ticketCommands() []discord.ApplicationCommandCreate {
	d := func(k i18n.Key) (string, map[discord.Locale]string) {
		return b.deps.Catalog.T("en", k, nil), localizations(b.deps.Catalog, k)
	}
	sub := func(name string, k i18n.Key, opts ...discord.ApplicationCommandOption) discord.ApplicationCommandOptionSubCommand {
		desc, loc := d(k)
		return discord.ApplicationCommandOptionSubCommand{Name: name, Description: desc, DescriptionLocalizations: loc, Options: opts}
	}
	userDesc, userLoc := d(i18n.CmdTicketOptUser)
	user := discord.ApplicationCommandOptionUser{Name: "user", Description: userDesc, DescriptionLocalizations: userLoc, Required: true}
	reasonDesc, reasonLoc := d(i18n.CmdTicketOptReason)
	maxReason := tickets.MaxCloseReason
	reason := discord.ApplicationCommandOptionString{Name: "reason", Description: reasonDesc, DescriptionLocalizations: reasonLoc, MaxLength: &maxReason}
	desc, loc := d(i18n.CmdTicketDescription)
	// Visible to everyone: the opener may close their own ticket; staff
	// checks happen in the tickets service.
	return []discord.ApplicationCommandCreate{discord.SlashCommandCreate{
		Name: "ticket", Description: desc, DescriptionLocalizations: loc,
		Contexts:         []discord.InteractionContextType{discord.InteractionContextTypeGuild},
		IntegrationTypes: []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall},
		Options: []discord.ApplicationCommandOption{
			sub("add", i18n.CmdTicketAddDesc, user),
			sub("remove", i18n.CmdTicketRemoveDesc, user),
			sub("close", i18n.CmdTicketCloseDesc, reason),
			sub("claim", i18n.CmdTicketClaimDesc),
		},
	}}
}

// ticketActor describes the member using an interaction.
func ticketActor(client *bot.Client, gid snowflake.ID, m *discord.ResolvedMember) tickets.Actor {
	a := tickets.Actor{ID: m.User.ID, Name: displayName(m.User, m.Nick), RoleIDs: m.RoleIDs}
	a.Manager = m.Permissions.Has(discord.PermissionAdministrator) || m.Permissions.Has(discord.PermissionManageGuild)
	if client != nil {
		if g, ok := client.Caches.Guild(gid); ok && g.OwnerID == m.User.ID {
			a.Manager = true
		}
	}
	return a
}

func (b *Bot) ticketError(l discord.Locale, err error) (string, int) {
	var le *tickets.LimitError
	switch {
	case errors.As(err, &le):
		chans := make([]string, 0, len(le.Channels))
		for _, c := range le.Channels {
			chans = append(chans, "<#"+c.String()+">")
		}
		return b.t(l, i18n.TicketLimit, map[string]any{"Channels": strings.Join(chans, " ")}), colorWarning
	case errors.Is(err, tickets.ErrModuleOff):
		return b.t(l, i18n.TicketDisabled, nil), colorBrand
	case errors.Is(err, tickets.ErrPanelNotFound):
		return b.t(l, i18n.TicketPanelGone, nil), colorBrand
	case errors.Is(err, tickets.ErrCategoryRemoved):
		return b.t(l, i18n.TicketCategoryRemoved, nil), colorBrand
	case errors.Is(err, tickets.ErrNotStaff):
		return b.t(l, i18n.TicketNotStaff, nil), colorWarning
	case errors.Is(err, tickets.ErrWrongStatus):
		return b.t(l, i18n.TicketWrongStatus, nil), colorWarning
	case errors.Is(err, tickets.ErrNotFound):
		return b.t(l, i18n.TicketNotATicket, nil), colorWarning
	case errors.Is(err, tickets.ErrProtectedMember):
		return b.t(l, i18n.TicketProtected, nil), colorWarning
	case errors.Is(err, tickets.ErrNoTranscript):
		return b.t(l, i18n.TicketNoTranscript, nil), colorWarning
	}
	b.log.Warn("ticket action failed", "err", err)
	var re *rest.Error
	if errors.As(err, &re) && re.Message != "" {
		return b.t(l, i18n.TicketFailed, map[string]any{"Error": escapeMarkdown(re.Message)}), colorDanger
	}
	return b.t(l, i18n.ErrorGeneric, nil), colorDanger
}

func (b *Bot) closedText(l discord.Locale, p tickets.CloseProblems) string {
	lines := []string{b.t(l, i18n.TicketDone, nil)}
	if p.TranscriptFailed {
		lines = append(lines, b.t(l, i18n.TicketTranscriptFailed, nil))
	}
	if p.HideFailed {
		lines = append(lines, b.t(l, i18n.TicketHideFailed, nil))
	}
	return strings.Join(lines, "\n")
}

func (b *Bot) closedReply(l discord.Locale, p tickets.CloseProblems) discord.ContainerComponent {
	if p.TranscriptFailed || p.HideFailed {
		return b.ticketReply(l, b.closedText(l, p), colorWarning, EmojiWarn)
	}
	return b.ticketReply(l, b.closedText(l, p), colorSuccess, EmojiOK)
}

func (b *Bot) ticketReply(l discord.Locale, text string, color int, emoji string) discord.ContainerComponent {
	return b.infoCard(l.Code(), emoji, text, color)
}

func (b *Bot) handleTicketOpen(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	gid, m := e.GuildID(), e.Member()
	panelID, err := strconv.ParseInt(e.Vars["panel"], 10, 64)
	if gid == nil || m == nil || err != nil {
		return errNotInGuild
	}
	cat, err := b.tickets.Form(e.Ctx, *gid, panelID, e.Vars["category"])
	if err != nil {
		text, color := b.ticketError(e.Locale(), err)
		return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), AllowedMentions: noPings,
			Components: []discord.LayoutComponent{b.ticketReply(e.Locale(), text, color, EmojiInfo)}})
	}
	if len(cat.Questions) > 0 {
		comps := make([]discord.LayoutComponent, len(cat.Questions))
		for i, q := range cat.Questions {
			in := discord.NewShortTextInput("q" + strconv.Itoa(i))
			if q.Long {
				in = discord.NewParagraphTextInput("q" + strconv.Itoa(i))
			}
			in = in.WithRequired(q.Required).WithMaxLength(tickets.MaxAnswer)
			if q.Placeholder != "" {
				in = in.WithPlaceholder(q.Placeholder)
			}
			comps[i] = discord.NewLabel(q.Label, in)
		}
		return e.Modal(discord.ModalCreate{
			CustomID:   "/tickets/form/" + e.Vars["panel"] + "/" + e.Vars["category"],
			Title:      clip(b.t(e.Locale(), i18n.TicketFormTitle, map[string]any{"Category": cat.Label}), 45),
			Components: comps,
		})
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	return b.openTicket(e.Ctx, e.UpdateInteractionResponse, e.Locale(), *gid, panelID, e.Vars["category"], ticketActor(b.client, *gid, m), nil)
}

func (b *Bot) handleTicketForm(e *handler.ModalEvent) error {
	gid, m := e.GuildID(), e.Member()
	panelID, err := strconv.ParseInt(e.Vars["panel"], 10, 64)
	if gid == nil || m == nil || err != nil {
		return errNotInGuild
	}
	var answers []string
	for i := range tickets.MaxQuestions {
		v, ok := e.Data.OptText("q" + strconv.Itoa(i))
		if !ok {
			break
		}
		answers = append(answers, v)
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	return b.openTicket(e.Ctx, e.UpdateInteractionResponse, e.Locale(), *gid, panelID, e.Vars["category"], ticketActor(b.client, *gid, m), answers)
}

func (b *Bot) openTicket(ctx context.Context, update func(discord.MessageUpdate, ...rest.RequestOpt) (*discord.Message, error),
	l discord.Locale, gid snowflake.ID, panelID int64, category string, who tickets.Actor, answers []string) error {
	t, err := b.tickets.Open(ctx, gid, panelID, category, who, answers)
	if err != nil {
		text, color := b.ticketError(l, err)
		return replyDeferred(update, b.ticketReply(l, text, color, EmojiInfo))
	}
	return replyDeferred(update, b.ticketReply(l, b.t(l, i18n.TicketOpened, map[string]any{"Channel": "<#" + t.ChannelID.String() + ">"}), colorSuccess, EmojiOK))
}

func ticketID(e *handler.ComponentEvent) (snowflake.ID, int64, *discord.ResolvedMember, bool) {
	gid, m := e.GuildID(), e.Member()
	id, err := strconv.ParseInt(e.Vars["id"], 10, 64)
	if gid == nil || m == nil || err != nil {
		return 0, 0, nil, false
	}
	return *gid, id, m, true
}

// ticketAction runs fn with a deferred ephemeral reply.
func (b *Bot) ticketAction(e *handler.ComponentEvent, fn func(gid snowflake.ID, id int64, a tickets.Actor) error) error {
	gid, id, m, ok := ticketID(e)
	if !ok {
		return errNotInGuild
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	if err := fn(gid, id, ticketActor(b.client, gid, m)); err != nil {
		text, color := b.ticketError(e.Locale(), err)
		return replyDeferred(e.UpdateInteractionResponse, b.ticketReply(e.Locale(), text, color, EmojiInfo))
	}
	return replyDeferred(e.UpdateInteractionResponse, b.ticketReply(e.Locale(), b.t(e.Locale(), i18n.TicketDone, nil), colorSuccess, EmojiOK))
}

func (b *Bot) handleTicketClaim(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	return b.ticketAction(e, func(gid snowflake.ID, id int64, a tickets.Actor) error {
		_, err := b.tickets.Claim(e.Ctx, gid, id, a)
		return err
	})
}

func (b *Bot) handleTicketCloseButton(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	gid, id, m, ok := ticketID(e)
	if !ok {
		return errNotInGuild
	}
	t, err := b.deps.Tickets.GetTicket(e.Ctx, gid, id)
	if err == nil && m.User.ID != t.OpenerID && !b.tickets.IsStaff(e.Ctx, t, ticketActor(b.client, gid, m)) {
		err = tickets.ErrNotStaff
	}
	if err != nil {
		text, color := b.ticketError(e.Locale(), err)
		return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), AllowedMentions: noPings,
			Components: []discord.LayoutComponent{b.ticketReply(e.Locale(), text, color, EmojiInfo)}})
	}
	maxReason := tickets.MaxCloseReason
	return e.Modal(discord.ModalCreate{
		CustomID: "/tickets/closing/" + e.Vars["id"],
		Title:    truncateRunes(b.t(e.Locale(), i18n.TicketCloseTitle, map[string]any{"Number": t.Number}), 45),
		Components: []discord.LayoutComponent{discord.NewLabel(b.t(e.Locale(), i18n.TicketCloseReason, nil),
			discord.NewParagraphTextInput("reason").WithRequired(false).WithMaxLength(maxReason))},
	})
}

func (b *Bot) handleTicketCloseForm(e *handler.ModalEvent) error {
	gid, m := e.GuildID(), e.Member()
	id, err := strconv.ParseInt(e.Vars["id"], 10, 64)
	if gid == nil || m == nil || err != nil {
		return errNotInGuild
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	reason, _ := e.Data.OptText("reason")
	_, probs, err := b.tickets.Close(e.Ctx, *gid, id, ticketActor(b.client, *gid, m), reason)
	if err != nil {
		text, color := b.ticketError(e.Locale(), err)
		return replyDeferred(e.UpdateInteractionResponse, b.ticketReply(e.Locale(), text, color, EmojiInfo))
	}
	return replyDeferred(e.UpdateInteractionResponse, b.closedReply(e.Locale(), probs))
}

func (b *Bot) handleTicketReopen(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	return b.ticketAction(e, func(gid snowflake.ID, id int64, a tickets.Actor) error {
		_, err := b.tickets.Reopen(e.Ctx, gid, id, a)
		return err
	})
}

func (b *Bot) handleTicketDelete(_ discord.ButtonInteractionData, e *handler.ComponentEvent) error {
	return b.ticketAction(e, func(gid snowflake.ID, id int64, a tickets.Actor) error {
		return b.tickets.Delete(e.Ctx, gid, id, a)
	})
}

// ---- slash commands -----------------------------------------------------------

func (b *Bot) ticketCmd(e *handler.CommandEvent, fn func(gid snowflake.ID, t tickets.Ticket, a tickets.Actor) (string, error)) error {
	gid, m := e.GuildID(), e.Member()
	if gid == nil || m == nil {
		return errNotInGuild
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	l := e.Locale()
	t, err := b.tickets.ByChannel(e.Ctx, *gid, e.Channel().ID())
	var text string
	if err == nil {
		text, err = fn(*gid, t, ticketActor(b.client, *gid, m))
	}
	if err != nil {
		msg, color := b.ticketError(l, err)
		return replyDeferred(e.UpdateInteractionResponse, b.ticketReply(l, msg, color, EmojiInfo))
	}
	if text == "" {
		text = b.t(l, i18n.TicketDone, nil)
	}
	return replyDeferred(e.UpdateInteractionResponse, b.ticketReply(l, text, colorSuccess, EmojiOK))
}

func (b *Bot) handleTicketAddCmd(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	u := data.User("user")
	return b.ticketCmd(e, func(gid snowflake.ID, t tickets.Ticket, a tickets.Actor) (string, error) {
		_, err := b.tickets.AddMember(e.Ctx, gid, t.ChannelID, a, u.ID)
		return b.t(e.Locale(), i18n.TicketMemberAdded, map[string]any{"User": "<@" + u.ID.String() + ">"}), err
	})
}

func (b *Bot) handleTicketRemoveCmd(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	u := data.User("user")
	return b.ticketCmd(e, func(gid snowflake.ID, t tickets.Ticket, a tickets.Actor) (string, error) {
		_, err := b.tickets.RemoveMember(e.Ctx, gid, t.ChannelID, a, u.ID)
		return b.t(e.Locale(), i18n.TicketMemberRemoved, map[string]any{"User": "<@" + u.ID.String() + ">"}), err
	})
}

func (b *Bot) handleTicketCloseCmd(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	reason, _ := data.OptString("reason")
	return b.ticketCmd(e, func(gid snowflake.ID, t tickets.Ticket, a tickets.Actor) (string, error) {
		_, probs, err := b.tickets.Close(e.Ctx, gid, t.ID, a, reason)
		return b.closedText(e.Locale(), probs), err
	})
}

func (b *Bot) handleTicketClaimCmd(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	return b.ticketCmd(e, func(gid snowflake.ID, t tickets.Ticket, a tickets.Actor) (string, error) {
		_, err := b.tickets.Claim(e.Ctx, gid, t.ID, a)
		return "", err
	})
}

// ---- RPC --------------------------------------------------------------------

// Ticket panel RPC methods (shared with the API).
const (
	RPCTicketPanelSave   = "tickets.panel.save"
	RPCTicketPanelDelete = "tickets.panel.delete"
)

func (b *Bot) registerTicketsRPC(srv *rpc.Server) {
	srv.Handle(RPCTicketPanelSave, func(ctx context.Context, r rpc.Request) (any, error) {
		var p tickets.Panel
		if err := r.Bind(&p); err != nil {
			return nil, err
		}
		p.GuildID, p.UpdatedBy = r.GuildID, r.ActorID
		saved, err := b.tickets.SavePanel(ctx, p)
		if err != nil {
			return nil, ticketsRPCError(err)
		}
		return saved, nil
	})
	srv.Handle(RPCTicketPanelDelete, func(ctx context.Context, r rpc.Request) (any, error) {
		var p RolePanelIDParams
		if err := r.Bind(&p); err != nil {
			return nil, err
		}
		return nil, ticketsRPCError(b.tickets.DeletePanel(ctx, r.GuildID, p.ID))
	})
}

func ticketsRPCError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, tickets.ErrPanelNotFound):
		return rpc.Errorf(rpc.CodeNotFound, "ticket panel not found")
	case errors.Is(err, tickets.ErrInvalid):
		return rpc.Errorf(rpc.CodeInvalid, "%s", ValidationMessage(err))
	case errors.Is(err, tickets.ErrTooManyPanels):
		return rpc.Errorf(rpc.CodeInvalid, "a server can have at most %d ticket panels", tickets.MaxPanels)
	}
	return discordRPCError(err)
}
