package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/bot/handlers"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/logging"
)

// The server log (M3). Gateway events become logging inputs; the logging
// service filters, routes and posts them through logSink.

// parentOf is a thread's parent channel (0 for other channels), so ignore
// lists cover threads.
func (b *Bot) parentOf(channelID snowflake.ID) snowflake.ID {
	if th, ok := b.client.Caches.GuildThread(channelID); ok && th.ParentID() != nil {
		return *th.ParentID()
	}
	return 0
}

func (b *Bot) logRun(what string, fn func(ctx context.Context) error) {
	go b.safely(what, func(ctx context.Context) {
		if err := fn(ctx); err != nil {
			b.log.Warn(what+" failed", "err", err)
		}
	})
}

func (b *Bot) logMessage(guildID snowflake.ID, m discord.Message) (logging.Message, bool) {
	if m.Type.System() {
		return logging.Message{}, false
	}
	lm := logging.Message{
		ID: m.ID, GuildID: guildID, ChannelID: m.ChannelID, ParentID: b.parentOf(m.ChannelID),
		AuthorID: m.Author.ID, AuthorName: m.Author.EffectiveName(),
		AuthorBot: m.Author.Bot || m.Author.System || m.WebhookID != nil,
		FromNiro:  m.Author.ID == b.selfID(), Content: m.Content, CreatedAt: m.CreatedAt,
	}
	if m.EditedTimestamp != nil {
		lm.EditedAt = *m.EditedTimestamp
	}
	for _, a := range m.Attachments {
		lm.Attachments = append(lm.Attachments, a.Filename)
	}
	switch {
	case m.Member != nil:
		lm.AuthorRoleIDs = slices.Clone(m.Member.RoleIDs)
		if m.Member.Nick != nil && *m.Member.Nick != "" {
			lm.AuthorName = *m.Member.Nick
		}
	default:
		if mem, ok := b.client.Caches.Member(guildID, m.Author.ID); ok {
			lm.AuthorRoleIDs = slices.Clone(mem.RoleIDs)
		}
	}
	return lm, true
}

func (b *Bot) onLogMessageCreate(e *events.GuildMessageCreate) {
	if m, ok := b.logMessage(e.GuildID, e.Message); ok {
		b.logRun("caching message", func(ctx context.Context) error { return b.logs.HandleMessage(ctx, m) })
	}
}

func (b *Bot) onLogMessageUpdate(e *events.GuildMessageUpdate) {
	if m, ok := b.logMessage(e.GuildID, e.Message); ok {
		b.logRun("logging message edit", func(ctx context.Context) error { return b.logs.HandleMessageEdit(ctx, m) })
	}
}

func (b *Bot) onLogMessageDelete(e *events.GuildMessageDelete) {
	guildID, channelID, messageID, parent := e.GuildID, e.ChannelID, e.MessageID, b.parentOf(e.ChannelID)
	b.logRun("logging message delete", func(ctx context.Context) error {
		return b.logs.HandleMessageDelete(ctx, guildID, channelID, parent, messageID)
	})
}

// gatewayHandlers are disgo's handlers with MESSAGE_DELETE_BULK replaced:
// disgo turns a bulk delete into one delete event per message, but the log
// shows a bulk delete as one entry with a file.
func (b *Bot) gatewayHandlers() map[gateway.EventType]bot.GatewayEventHandler {
	hs := handlers.GetGatewayHandlers()
	hs[gateway.EventTypeMessageDeleteBulk] = bot.NewGatewayEventHandler(gateway.EventTypeMessageDeleteBulk,
		func(_ *bot.Client, _ int, _ int, ev gateway.EventMessageDeleteBulk) {
			if ev.GuildID == nil || len(ev.IDs) == 0 {
				return
			}
			guildID, channelID, ids, parent := *ev.GuildID, ev.ChannelID, slices.Clone(ev.IDs), b.parentOf(ev.ChannelID)
			b.logRun("logging bulk delete", func(ctx context.Context) error {
				return b.logs.HandleBulkDelete(ctx, guildID, channelID, parent, ids)
			})
		})
	return hs
}

func (b *Bot) onLogMemberJoin(e *events.GuildMemberJoin) {
	u := e.Member.User
	j := logging.MemberJoin{
		GuildID: e.GuildID, UserID: u.ID, UserName: displayName(u, e.Member.Nick), Bot: u.Bot,
		AccountCreated: u.CreatedAt(), JoinedAt: time.Now(),
	}
	if e.Member.JoinedAt != nil {
		j.JoinedAt = *e.Member.JoinedAt
	}
	b.logRun("logging join", func(ctx context.Context) error { return b.logs.HandleMemberJoin(ctx, j) })
}

func (b *Bot) onLogMemberLeave(e *events.GuildMemberLeave) {
	l := logging.MemberLeave{GuildID: e.GuildID, UserID: e.User.ID, UserName: e.User.EffectiveName(), Bot: e.User.Bot}
	if e.Member.User.ID != 0 { // the member was cached
		l.UserName = displayName(e.User, e.Member.Nick)
		l.RoleIDs = slices.Clone(e.Member.RoleIDs)
		if e.Member.JoinedAt != nil {
			l.JoinedAt = *e.Member.JoinedAt
		}
	}
	b.logRun("logging leave", func(ctx context.Context) error { return b.logs.HandleMemberLeave(ctx, l) })
}

func (b *Bot) onLogVoice(guildID snowflake.ID, member discord.Member, from, to *snowflake.ID) {
	v := logging.VoiceChange{
		GuildID: guildID, UserID: member.User.ID, UserName: displayName(member.User, member.Nick),
		Bot: member.User.Bot, RoleIDs: slices.Clone(member.RoleIDs),
	}
	if from != nil {
		v.From = *from
	}
	if to != nil {
		v.To = *to
	}
	b.logRun("logging voice", func(ctx context.Context) error { return b.logs.HandleVoice(ctx, v) })
}

var automodActions = map[discord.AutoModerationActionType]string{
	discord.AutoModerationActionTypeBlockMessage:           "block_message",
	discord.AutoModerationActionTypeSendAlertMessage:       "send_alert",
	discord.AutoModerationActionTypeTimeout:                "timeout",
	discord.AutoModerationActionTypeBlockMemberInteraction: "block_interaction",
}

func (b *Bot) onLogAutoMod(e *events.AutoModerationActionExecution) {
	a := logging.AutoModAction{
		GuildID: e.GuildID, RuleID: e.RuleID, UserID: e.UserID, Action: automodActions[e.Action.Type],
		Content: e.Content,
	}
	if e.ChannelID != nil {
		a.ChannelID = *e.ChannelID
	}
	if e.MatchedKeywords != nil {
		a.Keyword = *e.MatchedKeywords
	}
	if a.Keyword == "" && e.MatchedContent != nil {
		a.Keyword = *e.MatchedContent
	}
	if m, ok := b.client.Caches.Member(e.GuildID, e.UserID); ok {
		a.UserName = displayName(m.User, m.Nick)
	}
	b.logRun("logging automod", func(ctx context.Context) error {
		a.RuleName = b.ruleNames.name(ctx, b.client, a.GuildID, a.RuleID)
		return b.logs.HandleAutoMod(ctx, a)
	})
}

// ruleNameCache remembers AutoMod rule names for a few minutes, so a burst
// of triggers costs one REST call.
type ruleNameCache struct {
	mu    sync.Mutex
	names map[snowflake.ID]ruleName
}

type ruleName struct {
	name string
	at   time.Time
}

func (c *ruleNameCache) name(ctx context.Context, client *bot.Client, guildID, ruleID snowflake.ID) string {
	c.mu.Lock()
	if n, ok := c.names[ruleID]; ok && time.Since(n.at) < 10*time.Minute {
		c.mu.Unlock()
		return n.name
	}
	c.mu.Unlock()
	rule, err := client.Rest.GetAutoModerationRule(guildID, ruleID, rest.WithCtx(ctx))
	if err != nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.names == nil || len(c.names) > 10000 {
		c.names = map[snowflake.ID]ruleName{}
	}
	c.names[ruleID] = ruleName{name: rule.Name, at: time.Now()}
	return rule.Name
}

func (b *Bot) refreshInvites(guildID snowflake.ID) {
	b.logRun("refreshing invites", func(ctx context.Context) error { return b.logs.RefreshInvites(ctx, guildID) })
}

// logGuardianEvent hands a Guardian event to the server log.
func (b *Bot) logGuardianEvent(ctx context.Context, ev guardian.Event) {
	g := logging.GuardianAction{
		GuildID: ev.GuildID, EventID: ev.ID, Kind: string(ev.Kind), Response: string(ev.Response),
		UserID: ev.UserID, UserName: ev.UserName, Channels: ev.Evidence.Channels,
		Monitor: ev.Evidence.Monitor, CaseNumber: ev.CaseNumber,
	}
	for _, s := range ev.Signals {
		g.Signals = append(g.Signals, string(s.Kind))
	}
	if err := b.logs.HandleGuardian(ctx, g); err != nil {
		b.log.Warn("logging guardian event failed", "err", err)
	}
}

// ---- audit log -----------------------------------------------------------------

var auditActions = map[discord.AuditLogEvent]logging.AuditAction{
	discord.AuditLogEventMemberBanAdd:           logging.AuditMemberBan,
	discord.AuditLogEventMemberBanRemove:        logging.AuditMemberUnban,
	discord.AuditLogEventMemberKick:             logging.AuditMemberKick,
	discord.AuditLogEventMemberUpdate:           logging.AuditMemberUpdate,
	discord.AuditLogEventMemberRoleUpdate:       logging.AuditMemberRoleUpdate,
	discord.AuditLogEventChannelCreate:          logging.AuditChannelCreate,
	discord.AuditLogEventChannelUpdate:          logging.AuditChannelUpdate,
	discord.AuditLogEventChannelDelete:          logging.AuditChannelDelete,
	discord.AuditLogEventChannelOverwriteCreate: logging.AuditChannelUpdate,
	discord.AuditLogEventChannelOverwriteUpdate: logging.AuditChannelUpdate,
	discord.AuditLogEventChannelOverwriteDelete: logging.AuditChannelUpdate,
	discord.AuditLogEventChannelPositionUpdate:  logging.AuditChannelUpdate,
	discord.AuditLogEventRoleCreate:             logging.AuditRoleCreate,
	discord.AuditLogEventRoleUpdate:             logging.AuditRoleUpdate,
	discord.AuditLogEventRoleDelete:             logging.AuditRoleDelete,
	discord.AuditLogEventRolePositionUpdate:     logging.AuditRoleUpdate,
	discord.AuditLogEventGuildUpdate:            logging.AuditGuildUpdate,
}

func (b *Bot) onLogAudit(e *events.GuildAuditLogEntryCreate) {
	a, ok := b.auditEntry(e.GuildID, e.AuditLogEntry)
	if !ok {
		return
	}
	b.logRun("logging audit entry", func(ctx context.Context) error { return b.logs.HandleAudit(ctx, a) })
}

func (b *Bot) auditEntry(guildID snowflake.ID, en discord.AuditLogEntry) (logging.AuditEntry, bool) {
	action, ok := auditActions[en.ActionType]
	if !ok {
		return logging.AuditEntry{}, false
	}
	a := logging.AuditEntry{
		GuildID: guildID, Action: action, ActorID: en.UserID, ByNiro: en.UserID == b.selfID(), At: en.ID.Time(),
	}
	if en.Reason != nil {
		a.Reason = *en.Reason
	}
	if en.TargetID != nil {
		a.TargetID = *en.TargetID
	}
	if m, ok := b.client.Caches.Member(guildID, en.UserID); ok {
		a.ActorName = displayName(m.User, m.Nick)
	}
	switch en.ActionType {
	case discord.AuditLogEventChannelOverwriteCreate, discord.AuditLogEventChannelOverwriteUpdate,
		discord.AuditLogEventChannelOverwriteDelete:
		if o := en.Options; o != nil && o.ID != nil {
			who := "<@&" + *o.ID + ">"
			if o.Type != nil && *o.Type == "1" {
				who = "<@" + *o.ID + ">"
			}
			a.Changes = append(a.Changes, logging.Change{Key: "overwrite", New: who})
		}
	}
	for _, c := range en.Changes {
		if ch, ok := formatChange(c); ok {
			a.Changes = append(a.Changes, ch)
		}
	}
	a.TargetName = b.auditTargetName(guildID, action, a)
	return a, true
}

func (b *Bot) auditTargetName(guildID snowflake.ID, action logging.AuditAction, a logging.AuditEntry) string {
	for _, c := range a.Changes {
		if c.Key == "name" {
			if c.New != "" {
				return c.New
			}
			return c.Old
		}
	}
	switch action {
	case logging.AuditMemberBan, logging.AuditMemberUnban, logging.AuditMemberKick, logging.AuditMemberUpdate,
		logging.AuditMemberRoleUpdate:
		if m, ok := b.client.Caches.Member(guildID, a.TargetID); ok {
			return displayName(m.User, m.Nick)
		}
	case logging.AuditChannelUpdate:
		if ch, ok := b.client.Caches.Channel(a.TargetID); ok {
			return ch.Name()
		}
	case logging.AuditRoleUpdate:
		if r, ok := b.client.Caches.Role(guildID, a.TargetID); ok {
			return r.Name
		}
	}
	return ""
}

// skippedChangeKeys are noise in a log (internal IDs, flags, nested lists).
var skippedChangeKeys = []string{"id", "guild_id", "flags", "available_tags", "permission_overwrites", "tags", "icon"}

var channelIDKeys = []string{"parent_id", "afk_channel_id", "system_channel_id", "rules_channel_id",
	"public_updates_channel_id", "safety_alerts_channel_id", "widget_channel_id"}

var verificationLevels = []string{"None", "Low", "Medium", "High", "Highest"}

var channelTypes = map[int]string{0: "Text", 2: "Voice", 4: "Category", 5: "Announcement", 13: "Stage", 15: "Forum", 16: "Media"}

// formatChange turns an audit-log change into display-ready old and new
// values (the platform's field names are kept as keys).
func formatChange(c discord.AuditLogChange) (logging.Change, bool) {
	key := string(c.Key)
	if slices.Contains(skippedChangeKeys, key) {
		return logging.Change{}, false
	}
	if key == logging.ChangeRolesAdded || key == logging.ChangeRolesRemoved {
		var roles []struct {
			ID   snowflake.ID `json:"id"`
			Name string       `json:"name"`
		}
		_ = json.Unmarshal(c.NewValue, &roles)
		ids := make([]snowflake.ID, len(roles))
		for i, r := range roles {
			ids[i] = r.ID
		}
		return logging.Change{Key: key, New: roleMentions(ids, logRolesLimit)}, len(ids) > 0
	}
	return logging.Change{Key: key, Old: formatValue(key, c.OldValue), New: formatValue(key, c.NewValue)}, true
}

func formatValue(key string, raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	switch x := v.(type) {
	case bool:
		if x {
			return "✓"
		}
		return "✗"
	case float64:
		n := int64(x)
		switch {
		case key == "color":
			return fmt.Sprintf("#%06X", n)
		case key == "verification_level" && n >= 0 && int(n) < len(verificationLevels):
			return verificationLevels[n]
		case key == "type":
			if t, ok := channelTypes[int(n)]; ok {
				return t
			}
		}
		return strconv.FormatInt(n, 10)
	case string:
		switch {
		case key == "permissions" || key == "allow" || key == "deny":
			n, err := strconv.ParseInt(x, 10, 64)
			if err != nil {
				return x
			}
			return discord.Permissions(n).String()
		case slices.Contains(channelIDKeys, key):
			return "<#" + x + ">"
		case key == "owner_id":
			return "<@" + x + ">"
		case strings.HasSuffix(key, "_hash"):
			return "✓"
		}
		return x
	}
	return "" // arrays and objects are not shown
}

// markDeletes returns a hook that tells the server log Niro itself is
// deleting messages (source: "guardian", "purge").
func (b *Bot) markDeletes(source string) func(snowflake.ID, []snowflake.ID) {
	return func(channelID snowflake.ID, ids []snowflake.ID) {
		if b.logs != nil {
			b.logs.MarkNiroDeletes(channelID, ids, source)
		}
	}
}

// invitePlatform implements logging.InviteSource (needs Manage Server).
type invitePlatform struct{ client *bot.Client }

func (p invitePlatform) ListInvites(ctx context.Context, guildID snowflake.ID) ([]logging.Invite, error) {
	invs, err := p.client.Rest.GetGuildInvites(guildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]logging.Invite, len(invs))
	for i, inv := range invs {
		out[i] = logging.Invite{Code: inv.Code, Uses: inv.Uses, MaxUses: inv.MaxUses}
		if inv.Inviter != nil {
			out[i].InviterID, out[i].InviterName = inv.Inviter.ID, inv.Inviter.EffectiveName()
		}
	}
	return out, nil
}
