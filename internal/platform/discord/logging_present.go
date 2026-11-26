package discord

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/logging"
)

// ---- rendering ---------------------------------------------------------------

// Limits that keep a log card under Discord's 4,000-character text budget
// for Components V2 messages.
const (
	logTextLimit    = 1500 // each message text block
	logValueLimit   = 100  // each changed value
	logChangesLimit = 10
	logRolesLimit   = 20
	// logCardLimit is a final cap on the card's main text block.
	logCardLimit = 3600
)

// fitCard caps the card text, closing a code block cut in half.
func fitCard(s string) string {
	if utf8.RuneCountInString(s) <= logCardLimit {
		return s
	}
	s = string([]rune(s)[:logCardLimit]) + "…"
	if strings.Count(s, "```")%2 == 1 {
		s += "\n```"
	}
	return s
}

func logColor(k logging.Kind) int {
	switch k {
	case logging.KindMessageDelete, logging.KindMessageBulkDelete, logging.KindMemberBan, logging.KindChannelDelete,
		logging.KindRoleDelete, logging.KindGuardianAction:
		return colorDanger
	case logging.KindMessageEdit, logging.KindMemberTimeout, logging.KindAutoModAction, logging.KindMemberNick:
		return colorWarning
	case logging.KindMemberLeave, logging.KindMemberKick:
		return colorOrange
	case logging.KindMemberJoin, logging.KindMemberUnban, logging.KindChannelCreate, logging.KindRoleCreate:
		return colorSuccess
	}
	return colorBrand
}

func logEmoji(e *Emojis, k logging.Kind) string {
	switch k.Category() {
	case logging.CategoryModeration, logging.CategoryGuardian, logging.CategoryAutoMod:
		return e.Get(EmojiShield)
	case logging.CategoryMessages:
		if k == logging.KindMessageEdit {
			return e.Get(EmojiInfo)
		}
		return e.Get(EmojiWarn)
	}
	return e.Get(EmojiInfo)
}

// zeroWidth breaks up a code fence inside user text.
var zeroWidth = string(rune(0x200b))

// codeBlock shows user text verbatim (no Markdown, no mentions). Every
// backtick is followed by a zero-width space, so no run of backticks in the
// text can close the block.
func codeBlock(s string, limit int) string {
	return "```\n" + strings.ReplaceAll(clip(s, limit), "`", "`"+zeroWidth) + "\n```"
}

func clip(s string, limit int) string {
	if utf8.RuneCountInString(s) > limit {
		return string([]rune(s)[:limit]) + "…"
	}
	return s
}

func roleMentions(ids []snowflake.ID, limit int) string {
	parts := make([]string, 0, len(ids))
	for i, id := range ids {
		if i == limit {
			parts = append(parts, fmt.Sprintf("+%d", len(ids)-limit))
			break
		}
		parts = append(parts, "<@&"+id.String()+">")
	}
	return strings.Join(parts, " ")
}

func messageURL(guildID, channelID, messageID snowflake.ID) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guildID, channelID, messageID)
}

// inlineCode shows a short value as code, without breaking out of it.
func inlineCode(s string, limit int) string {
	return "`" + strings.ReplaceAll(clip(s, limit), "`", "'") + "`"
}

// knownChangeKeys have a translated label; others show the raw key.
var knownChangeKeys = []string{
	"name", "topic", "nsfw", "rate_limit_per_user", "bitrate", "user_limit", "position", "parent_id", "type",
	"permissions", "permission_overwrites", "overwrite", "allow", "deny", "color", "hoist", "mentionable",
	"icon_hash", "unicode_emoji", "verification_level", "explicit_content_filter", "default_message_notifications",
	"mfa_level", "afk_channel_id", "afk_timeout", "system_channel_id", "rules_channel_id", "public_updates_channel_id",
	"owner_id", "vanity_url_code", "description", "banner_hash", "splash_hash", "preferred_locale",
	"default_auto_archive_duration",
}

func (p presenter) changeLabel(l, key string) string {
	if slices.Contains(knownChangeKeys, key) {
		return p.t(l, i18n.LogChange(key), nil)
	}
	return inlineCode(key, 60)
}

// mentionRE matches a single user, role or channel mention made by the
// adapter (formatChange); only these are shown unescaped.
var mentionRE = regexp.MustCompile(`^<(@&?|#)[0-9]{1,20}>$`)

func isMention(v string) bool { return mentionRE.MatchString(v) }

func (p presenter) changeLines(l string, kind logging.Kind, changes []logging.Change) []string {
	none := p.t(l, i18n.LogNone, nil)
	val := func(v string) string {
		switch {
		case v == "":
			return none
		case isMention(v):
			return v
		}
		return escapeMarkdown(clip(v, logValueLimit))
	}
	var lines []string
	for i, c := range changes {
		if i == logChangesLimit {
			lines = append(lines, p.t(l, i18n.LogMoreChanges, map[string]any{"Count": len(changes) - i}))
			break
		}
		label := p.changeLabel(l, c.Key)
		switch {
		case kind == logging.KindChannelCreate || kind == logging.KindRoleCreate || c.Key == "overwrite":
			lines = append(lines, fmt.Sprintf("• **%s:** %s", label, val(c.New)))
		case kind == logging.KindChannelDelete || kind == logging.KindRoleDelete:
			lines = append(lines, fmt.Sprintf("• **%s:** %s", label, val(c.Old)))
		default:
			lines = append(lines, fmt.Sprintf("• **%s:** %s → %s", label, val(c.Old), val(c.New)))
		}
	}
	return lines
}

// logCard renders one log entry. guildName and channelName are used in the
// bulk-delete file, which is nil for other kinds.
func (p presenter) logCard(l string, e logging.Entry, guildName, channelName string) (discord.ContainerComponent, *discord.File) {
	lines := []string{"### " + strings.TrimSpace(logEmoji(p.emojis, e.Kind)+" "+p.t(l, i18n.LogTitle(string(e.Kind)), nil))}
	add := func(k i18n.Key, data map[string]any) { lines = append(lines, p.t(l, k, data)) }
	user := func(k i18n.Key) {
		if e.UserID != 0 {
			add(k, map[string]any{"User": userRef(e.UserID, e.UserName)})
		}
	}
	channel := func(id snowflake.ID) {
		if id != 0 {
			add(i18n.LogChannel, map[string]any{"Channel": "<#" + id.String() + ">"})
		}
	}
	by := func() {
		switch {
		case e.ByNiro && e.ByNiroSource != "":
			add(i18n.LogByNiro, map[string]any{"Source": p.t(l, i18n.LogNiroSource(e.ByNiroSource), nil)})
		case e.ByNiro:
			add(i18n.LogBy, map[string]any{"Actor": "Niro"})
		case e.ActorID != 0:
			add(i18n.LogBy, map[string]any{"Actor": userRef(e.ActorID, e.ActorName)})
		}
		if e.Reason != "" {
			add(i18n.LogReason, map[string]any{"Reason": escapeMarkdown(clip(e.Reason, 500))})
		}
	}
	text := func(head i18n.Key, s string) {
		add(head, nil)
		if s == "" {
			add(i18n.LogEmpty, nil)
			return
		}
		lines = append(lines, codeBlock(s, logTextLimit))
	}
	files := func() {
		if len(e.Attachments) > 0 {
			names := make([]string, len(e.Attachments))
			for i, a := range e.Attachments {
				names[i] = inlineCode(a, 80)
			}
			add(i18n.LogFiles, map[string]any{"Files": strings.Join(names, ", ")})
		}
	}
	var file *discord.File

	switch e.Kind {
	case logging.KindMessageEdit:
		user(i18n.LogAuthor)
		channel(e.ChannelID)
		add(i18n.LogJump, map[string]any{"URL": messageURL(e.GuildID, e.ChannelID, e.MessageID)})
		switch {
		case e.ContentHidden:
			add(i18n.LogContentHidden, nil)
		case e.Cached:
			text(i18n.LogBefore, e.Before)
			text(i18n.LogAfter, e.After)
		default:
			text(i18n.LogAfter, e.After)
			add(i18n.LogNotCached, nil)
		}
		files()
	case logging.KindMessageDelete:
		user(i18n.LogAuthor)
		channel(e.ChannelID)
		by()
		switch {
		case !e.Cached:
			add(i18n.LogNotCached, nil)
		case e.ContentHidden:
			add(i18n.LogContentHidden, nil)
		default:
			text(i18n.LogContent, e.Before)
		}
		files()
	case logging.KindMessageBulkDelete:
		channel(e.ChannelID)
		by()
		add(i18n.LogBulkCount, map[string]any{"Count": e.Count, "Cached": len(e.Messages)})
		file = p.bulkFile(l, e, guildName, channelName)
	case logging.KindMemberJoin:
		user(i18n.LogMember)
		if !e.AccountCreated.IsZero() {
			add(i18n.LogAccountCreated, map[string]any{"Created": ts(e.AccountCreated, "R")})
			if e.At.Sub(e.AccountCreated) < 7*24*time.Hour {
				add(i18n.LogNewAccount, nil)
			}
		}
		if e.UserBot {
			add(i18n.LogBotAccount, nil)
		}
		if e.Invite != nil {
			inviter := "—"
			if e.Invite.InviterID != 0 {
				inviter = userRef(e.Invite.InviterID, e.Invite.InviterName)
			}
			add(i18n.LogInvite, map[string]any{"Code": strings.ReplaceAll(e.Invite.Code, "`", ""), "Inviter": inviter, "Uses": e.Invite.Uses})
		} else {
			add(i18n.LogInviteUnknown, nil)
		}
	case logging.KindMemberLeave:
		user(i18n.LogMember)
		if !e.JoinedAt.IsZero() {
			add(i18n.LogJoined, map[string]any{"Joined": ts(e.JoinedAt, "R")})
		}
		if len(e.RoleIDs) > 0 {
			add(i18n.LogRoles, map[string]any{"Roles": roleMentions(e.RoleIDs, logRolesLimit)})
		}
		if e.UserBot {
			add(i18n.LogBotAccount, nil)
		}
	case logging.KindMemberNick:
		user(i18n.LogMember)
		none := p.t(l, i18n.LogNone, nil)
		for _, c := range e.Changes {
			old, nw := none, none
			if c.Old != "" {
				old = escapeMarkdown(c.Old)
			}
			if c.New != "" {
				nw = escapeMarkdown(c.New)
			}
			add(i18n.LogNick, map[string]any{"Old": old, "New": nw})
		}
		by()
	case logging.KindMemberRoles:
		user(i18n.LogMember)
		for _, c := range e.Changes {
			switch c.Key {
			case logging.ChangeRolesAdded:
				add(i18n.LogRolesAdded, map[string]any{"Roles": c.New})
			case logging.ChangeRolesRemoved:
				add(i18n.LogRolesRemoved, map[string]any{"Roles": c.New})
			}
		}
		by()
	case logging.KindMemberBan, logging.KindMemberUnban, logging.KindMemberKick:
		user(i18n.LogMember)
		by()
	case logging.KindMemberTimeout:
		user(i18n.LogMember)
		if e.TimeoutUntil.IsZero() {
			add(i18n.LogTimeoutRemoved, nil)
		} else {
			add(i18n.LogTimeoutUntil, map[string]any{"Until": ts(e.TimeoutUntil, "f") + " (" + ts(e.TimeoutUntil, "R") + ")"})
		}
		by()
	case logging.KindChannelCreate, logging.KindChannelUpdate, logging.KindChannelDelete:
		ref := "<#" + e.TargetID.String() + ">"
		if e.Kind == logging.KindChannelDelete {
			ref = "#" + escapeMarkdown(e.TargetName)
		} else if e.TargetName != "" {
			ref += " (" + escapeMarkdown(e.TargetName) + ")"
		}
		add(i18n.LogChannel, map[string]any{"Channel": ref})
		p.addChanges(l, e, &lines)
		by()
	case logging.KindRoleCreate, logging.KindRoleUpdate, logging.KindRoleDelete:
		ref := "<@&" + e.TargetID.String() + ">"
		if e.Kind == logging.KindRoleDelete {
			ref = "@" + escapeMarkdown(e.TargetName)
		} else if e.TargetName != "" {
			ref += " (" + escapeMarkdown(e.TargetName) + ")"
		}
		add(i18n.LogRole, map[string]any{"Role": ref})
		p.addChanges(l, e, &lines)
		by()
	case logging.KindGuildUpdate:
		p.addChanges(l, e, &lines)
		by()
	case logging.KindVoiceJoin:
		user(i18n.LogMember)
		channel(e.ChannelID)
	case logging.KindVoiceLeave:
		user(i18n.LogMember)
		channel(e.OldChannelID)
	case logging.KindVoiceMove:
		user(i18n.LogMember)
		add(i18n.LogVoiceMove, map[string]any{"From": "<#" + e.OldChannelID.String() + ">", "To": "<#" + e.ChannelID.String() + ">"})
	case logging.KindAutoModAction:
		user(i18n.LogMember)
		channel(e.ChannelID)
		if e.RuleName != "" {
			add(i18n.LogRule, map[string]any{"Rule": escapeMarkdown(e.RuleName)})
		}
		if e.Keyword != "" {
			add(i18n.LogKeyword, map[string]any{"Keyword": strings.ReplaceAll(clip(e.Keyword, 100), "`", "'")})
		}
		if e.Action != "" {
			add(i18n.LogAction, map[string]any{"Action": p.t(l, i18n.LogAutoModAction(e.Action), nil)})
		}
		switch {
		case e.ContentHidden:
			add(i18n.LogContentHidden, nil)
		case e.Before != "":
			text(i18n.LogContent, e.Before)
		}
	case logging.KindGuardianAction:
		user(i18n.LogMember)
		add(i18n.LogDetection, map[string]any{
			"Title":  p.t(l, i18n.GuardianTitle(e.RuleName), nil),
			"Action": p.t(l, i18n.GuardianAction(e.Action), nil),
		})
		if len(e.Signals) > 0 {
			sig := make([]string, len(e.Signals))
			for i, s := range e.Signals {
				sig[i] = p.t(l, i18n.GuardianSignal(s), nil)
			}
			add(i18n.LogSignals, map[string]any{"Signals": strings.Join(sig, " · ")})
		}
		if len(e.Channels) > 0 {
			add(i18n.LogChannels, map[string]any{"Channels": channelMentions(e.Channels, 10)})
		}
		if e.Monitor {
			add(i18n.LogMonitor, nil)
		}
	}

	footer := []string{ts(e.At, "f")}
	if e.CaseNumber > 0 {
		footer = append(footer, p.t(l, i18n.LogCase, map[string]any{"Number": e.CaseNumber}))
	}
	if e.UserID != 0 {
		footer = append(footer, "ID "+e.UserID.String())
	}
	comps := []discord.ContainerSubComponent{discord.NewTextDisplay(fitCard(strings.Join(lines, "\n")))}
	if file != nil {
		comps = append(comps, discord.NewFileComponent("attachment://"+file.Name))
	}
	comps = append(comps, discord.NewSmallSeparator(), discord.NewTextDisplay("-# "+strings.Join(footer, " · ")))
	return discord.NewContainer(comps...).WithAccentColor(logColor(e.Kind)), file
}

func (p presenter) addChanges(l string, e logging.Entry, lines *[]string) {
	if cl := p.changeLines(l, e.Kind, e.Changes); len(cl) > 0 {
		*lines = append(*lines, p.t(l, i18n.LogChanges, nil))
		*lines = append(*lines, cl...)
	}
}

func (p presenter) bulkFile(l string, e logging.Entry, guildName, channelName string) *discord.File {
	header := []string{p.t(l, i18n.LogBulkHeader, map[string]any{
		"Count": e.Count, "Channel": channelName, "Server": guildName, "Time": e.At.UTC().Format(time.DateTime) + " UTC",
	})}
	if e.ByNiro && e.ByNiroSource != "" {
		header = append(header, p.t(l, i18n.LogBulkHeaderBy, map[string]any{"Source": p.t(l, i18n.LogNiroSource(e.ByNiroSource), nil)}))
	}
	ids := make([]snowflake.ID, 0, len(e.Messages))
	for _, m := range e.Messages {
		ids = append(ids, m.ID)
	}
	var buf bytes.Buffer
	_ = logging.WriteTranscript(&buf, logging.TranscriptLabels{
		Header: header, NotCached: p.t(l, i18n.LogBulkNotCached, nil), Files: p.t(l, i18n.LogBulkFiles, nil),
		NoContent: p.t(l, i18n.LogBulkNoContent, nil), EmptyTexts: p.t(l, i18n.LogBulkEmpty, nil),
	}, e.Messages, ids, e.ContentHidden)
	if missing := e.Count - len(e.Messages); missing > 0 {
		fmt.Fprintf(&buf, "\n+%d %s\n", missing, p.t(l, i18n.LogBulkNotCached, nil))
	}
	name := fmt.Sprintf("%s-%s.txt", p.t(l, i18n.LogBulkFilename, nil), e.At.UTC().Format("20060102-150405"))
	return discord.NewFile(name, "", &buf)
}

// ---- sending -----------------------------------------------------------------

// The backlog per log channel is bounded; beyond it entries are dropped (and
// counted) rather than piling up during a flood.
const (
	logQueueSize = 200
	logQueueIdle = time.Minute
)

// sendQueue sends messages to each channel in order, one at a time. disgo's
// REST client waits for Discord's rate limits, so a burst queues up here
// instead of failing.
type sendQueue struct {
	mu     sync.Mutex
	queues map[snowflake.ID]chan queuedLog
	send   func(channelID snowflake.ID, m queuedLog)
	size   int
	idle   time.Duration
}

// queuedLog is a rendered log message and its kind (for metrics).
type queuedLog struct {
	msg  discord.MessageCreate
	kind logging.Kind
}

func newSendQueue(send func(snowflake.ID, queuedLog)) *sendQueue {
	return &sendQueue{queues: map[snowflake.ID]chan queuedLog{}, send: send, size: logQueueSize, idle: logQueueIdle}
}

// enqueue returns false if the channel's queue is full. It holds the lock
// while queueing so a worker that is shutting down never strands a message.
func (q *sendQueue) enqueue(channelID snowflake.ID, m queuedLog) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	c, ok := q.queues[channelID]
	if !ok {
		c = make(chan queuedLog, q.size)
		q.queues[channelID] = c
		go q.run(channelID, c)
	}
	select {
	case c <- m:
		return true
	default:
		return false
	}
}

func (q *sendQueue) run(channelID snowflake.ID, c chan queuedLog) {
	timer := time.NewTimer(q.idle)
	defer timer.Stop()
	for {
		select {
		case m := <-c:
			q.send(channelID, m)
			timer.Reset(q.idle)
		case <-timer.C:
			q.mu.Lock()
			if len(c) == 0 {
				delete(q.queues, channelID)
				q.mu.Unlock()
				return
			}
			q.mu.Unlock()
			timer.Reset(q.idle)
		}
	}
}

// logSink implements logging.Sink: it renders entries and queues them.
type logSink struct {
	client  *bot.Client
	p       presenter
	locale  func(snowflake.ID) string
	metrics *metrics
	log     *slog.Logger
	queue   *sendQueue
	// warned limits "cannot post" warnings to one per channel per hour.
	warned sync.Map
}

var _ logging.Sink = (*logSink)(nil)

func newLogSink(client *bot.Client, p presenter, locale func(snowflake.ID) string, m *metrics, log *slog.Logger) *logSink {
	s := &logSink{client: client, p: p, locale: locale, metrics: m, log: log}
	s.queue = newSendQueue(s.deliver)
	return s
}

func (s *logSink) Post(_ context.Context, channelID snowflake.ID, e logging.Entry) error {
	guildName, channelName := "", e.ChannelID.String()
	if g, ok := s.client.Caches.Guild(e.GuildID); ok {
		guildName = g.Name
	}
	if ch, ok := s.client.Caches.Channel(e.ChannelID); ok {
		channelName = ch.Name()
	}
	card, file := s.p.logCard(s.locale(e.GuildID), e, guildName, channelName)
	m := discord.MessageCreate{
		Flags: discord.MessageFlagIsComponentsV2, Components: []discord.LayoutComponent{card}, AllowedMentions: noPings,
	}
	if file != nil {
		m.Files = []*discord.File{file}
	}
	if !s.queue.enqueue(channelID, queuedLog{msg: m, kind: e.Kind}) {
		s.metrics.logEntries.WithLabelValues(string(e.Kind), "dropped").Inc()
		return fmt.Errorf("log queue for channel %s is full", channelID)
	}
	s.metrics.logEntries.WithLabelValues(string(e.Kind), "queued").Inc()
	return nil
}

func (s *logSink) deliver(channelID snowflake.ID, q queuedLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.client.Rest.CreateMessage(channelID, q.msg, rest.WithCtx(ctx)); err != nil {
		s.metrics.logEntries.WithLabelValues(string(q.kind), "failed").Inc()
		if last, ok := s.warned.Load(channelID); ok && time.Since(last.(time.Time)) < time.Hour {
			return
		}
		s.warned.Store(channelID, time.Now())
		s.log.Warn("posting to the log channel failed (check Niro can view it, send messages and attach files)",
			"channel_id", channelID, "err", err)
	}
}
