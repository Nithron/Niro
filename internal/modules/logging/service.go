package logging

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// SettingsLoader returns a server's logging settings and whether the module is on.
type SettingsLoader func(ctx context.Context, guildID snowflake.ID) (s Settings, enabled bool, err error)

// Deps are the service's collaborators.
type Deps struct {
	Settings SettingsLoader
	Cache    MessageCache
	Sink     Sink
	// Invites and Snapshots find the invite a member used; both optional.
	Invites   InviteSource
	Snapshots InviteSnapshots
	// ModLogChannel is the moderation log channel (0 if none). Niro's own
	// moderation actions already have a case card there, so they are not
	// logged again in that channel (decision 21).
	ModLogChannel func(ctx context.Context, guildID snowflake.ID) snowflake.ID
	// GuardianAlertChannel is where Guardian posts its alerts (0 if none);
	// Guardian entries are not repeated in that channel.
	GuardianAlertChannel func(ctx context.Context, guildID snowflake.ID) snowflake.ID
	Log                  *slog.Logger
	Now                  func() time.Time
	// CacheTTL defaults to MessageCacheTTL.
	CacheTTL time.Duration
}

// Service runs the server log.
type Service struct {
	d Deps
	// niroDeletes remembers messages Niro deleted itself, so their delete
	// log says so (the platform doesn't tell who deleted a message).
	niroDeletes *ttlMap[[2]snowflake.ID, string]
	// automod deduplicates the several actions one AutoMod trigger fires.
	automod *ttlMap[string, struct{}]
}

func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.CacheTTL == 0 {
		d.CacheTTL = MessageCacheTTL
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.ModLogChannel == nil {
		d.ModLogChannel = func(context.Context, snowflake.ID) snowflake.ID { return 0 }
	}
	if d.GuardianAlertChannel == nil {
		d.GuardianAlertChannel = func(context.Context, snowflake.ID) snowflake.ID { return 0 }
	}
	return &Service{
		d:           d,
		niroDeletes: newTTLMap[[2]snowflake.ID, string](d.Now),
		automod:     newTTLMap[string, struct{}](d.Now),
	}
}

const (
	niroDeleteTTL = 2 * time.Minute
	automodTTL    = 10 * time.Second
)

func (s *Service) settings(ctx context.Context, guildID snowflake.ID) (Settings, bool) {
	st, on, err := s.d.Settings(ctx, guildID)
	if err != nil {
		s.d.Log.WarnContext(ctx, "loading logging settings failed", "guild_id", guildID, "err", err)
		return Settings{}, false
	}
	return st, on
}

func (s *Service) post(ctx context.Context, channelID snowflake.ID, e Entry) error {
	if e.At.IsZero() {
		e.At = s.d.Now()
	}
	if err := s.d.Sink.Post(ctx, channelID, e); err != nil {
		return fmt.Errorf("posting %s log: %w", e.Kind, err)
	}
	return nil
}

func cached(m Message, hideContent bool) CachedMessage {
	c := CachedMessage{
		ID: m.ID, ChannelID: m.ChannelID, AuthorID: m.AuthorID, AuthorName: m.AuthorName,
		AuthorBot: m.AuthorBot, AuthorRoleIDs: m.AuthorRoleIDs, Attachments: m.Attachments,
		CreatedAt: m.CreatedAt, EditedAt: m.EditedAt,
	}
	if !hideContent {
		c.Content = truncate(m.Content, MaxCachedContent)
	}
	return c
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// HandleMessage caches a new message so a later edit or delete can show it.
func (s *Service) HandleMessage(ctx context.Context, m Message) error {
	st, on := s.settings(ctx, m.GuildID)
	if !on || m.FromNiro || !st.cachesMessages() || st.ignored(m.ChannelID, m.ParentID, m.AuthorRoleIDs, m.AuthorBot) {
		return nil
	}
	return s.d.Cache.Put(ctx, cached(m, st.NoMessageContent), s.d.CacheTTL)
}

// HandleMessageEdit logs an edit with the text before and after.
func (s *Service) HandleMessageEdit(ctx context.Context, m Message) error {
	st, on := s.settings(ctx, m.GuildID)
	if !on || m.FromNiro || !st.cachesMessages() || st.ignored(m.ChannelID, m.ParentID, m.AuthorRoleIDs, m.AuthorBot) {
		return nil
	}
	old, found, err := s.d.Cache.Get(ctx, m.ChannelID, m.ID)
	if err != nil {
		return fmt.Errorf("reading message cache: %w", err)
	}
	if err := s.d.Cache.Put(ctx, cached(m, st.NoMessageContent), s.d.CacheTTL); err != nil {
		return fmt.Errorf("writing message cache: %w", err)
	}
	dest := st.Destination(KindMessageEdit)
	if dest == 0 || m.EditedAt.IsZero() {
		return nil // not an edit by the author (e.g. a link preview loaded)
	}
	if found {
		if !m.EditedAt.After(old.EditedAt) {
			return nil
		}
		if !st.NoMessageContent && old.Content == truncate(m.Content, MaxCachedContent) &&
			slices.Equal(old.Attachments, m.Attachments) {
			return nil
		}
	}
	e := Entry{
		Kind: KindMessageEdit, GuildID: m.GuildID, At: m.EditedAt,
		UserID: m.AuthorID, UserName: m.AuthorName, UserBot: m.AuthorBot,
		ChannelID: m.ChannelID, MessageID: m.ID, Cached: found,
		ContentHidden: st.NoMessageContent, Attachments: m.Attachments,
	}
	if !st.NoMessageContent {
		e.After = m.Content
		if found {
			e.Before = old.Content
		}
	}
	return s.post(ctx, dest, e)
}

// MarkNiroDeletes records that Niro is about to delete these messages
// itself (source: "guardian", "purge"…), so their delete logs say so.
func (s *Service) MarkNiroDeletes(channelID snowflake.ID, messageIDs []snowflake.ID, source string) {
	for _, id := range messageIDs {
		s.niroDeletes.Set([2]snowflake.ID{channelID, id}, source, niroDeleteTTL)
	}
}

func (s *Service) niroSource(channelID snowflake.ID, ids []snowflake.ID) (string, bool) {
	for _, id := range ids {
		if src, ok := s.niroDeletes.Get([2]snowflake.ID{channelID, id}); ok {
			return src, true
		}
	}
	return "", false
}

// HandleMessageDelete logs a deleted message with its cached text.
// parentID is the parent channel when channelID is a thread.
func (s *Service) HandleMessageDelete(ctx context.Context, guildID, channelID, parentID, messageID snowflake.ID) error {
	st, on := s.settings(ctx, guildID)
	if !on || !st.cachesMessages() {
		return nil
	}
	// Always take the message out of the cache: deleted text must not linger.
	msgs, err := s.d.Cache.Take(ctx, channelID, []snowflake.ID{messageID})
	if err != nil {
		return fmt.Errorf("reading message cache: %w", err)
	}
	dest := st.Destination(KindMessageDelete)
	if dest == 0 || st.ignored(channelID, parentID, nil, false) {
		return nil
	}
	e := Entry{
		Kind: KindMessageDelete, GuildID: guildID, ChannelID: channelID, MessageID: messageID,
		ContentHidden: st.NoMessageContent,
	}
	if len(msgs) == 1 {
		m := msgs[0]
		if st.ignored(channelID, parentID, m.AuthorRoleIDs, m.AuthorBot) {
			return nil
		}
		e.Cached = true
		e.UserID, e.UserName, e.UserBot = m.AuthorID, m.AuthorName, m.AuthorBot
		e.Attachments = m.Attachments
		if !st.NoMessageContent {
			e.Before = m.Content
		}
	}
	if src, ok := s.niroSource(channelID, []snowflake.ID{messageID}); ok {
		e.ByNiro, e.ByNiroSource = true, src
	}
	return s.post(ctx, dest, e)
}

// HandleBulkDelete logs a bulk delete as one entry; the adapter attaches
// the cached messages as a file.
func (s *Service) HandleBulkDelete(ctx context.Context, guildID, channelID, parentID snowflake.ID, messageIDs []snowflake.ID) error {
	st, on := s.settings(ctx, guildID)
	if !on || !st.cachesMessages() || len(messageIDs) == 0 {
		return nil
	}
	msgs, err := s.d.Cache.Take(ctx, channelID, messageIDs)
	if err != nil {
		return fmt.Errorf("reading message cache: %w", err)
	}
	dest := st.Destination(KindMessageBulkDelete)
	if dest == 0 || st.ignored(channelID, parentID, nil, false) {
		return nil
	}
	slices.SortFunc(msgs, func(a, b CachedMessage) int { return compareIDs(a.ID, b.ID) })
	if st.NoMessageContent {
		for i := range msgs {
			msgs[i].Content = ""
		}
	}
	e := Entry{
		Kind: KindMessageBulkDelete, GuildID: guildID, ChannelID: channelID,
		Messages: msgs, Count: len(messageIDs), ContentHidden: st.NoMessageContent,
	}
	if src, ok := s.niroSource(channelID, messageIDs); ok {
		e.ByNiro, e.ByNiroSource = true, src
	}
	return s.post(ctx, dest, e)
}

func compareIDs(a, b snowflake.ID) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// HandleMemberJoin logs a join with the account's age and the invite used.
func (s *Service) HandleMemberJoin(ctx context.Context, j MemberJoin) error {
	st, on := s.settings(ctx, j.GuildID)
	if !on {
		return nil
	}
	dest := st.Destination(KindMemberJoin)
	if dest == 0 {
		return nil
	}
	e := Entry{
		Kind: KindMemberJoin, GuildID: j.GuildID, At: j.JoinedAt,
		UserID: j.UserID, UserName: j.UserName, UserBot: j.Bot,
		AccountCreated: j.AccountCreated, JoinedAt: j.JoinedAt,
	}
	if inv, ok := s.usedInvite(ctx, j.GuildID); ok {
		e.Invite = &inv
	}
	return s.post(ctx, dest, e)
}

// usedInvite compares the invite list with the last snapshot.
func (s *Service) usedInvite(ctx context.Context, guildID snowflake.ID) (Invite, bool) {
	if s.d.Invites == nil || s.d.Snapshots == nil {
		return Invite{}, false
	}
	cur, err := s.d.Invites.ListInvites(ctx, guildID)
	if err != nil {
		s.d.Log.DebugContext(ctx, "listing invites failed", "guild_id", guildID, "err", err)
		return Invite{}, false
	}
	prev, found, err := s.d.Snapshots.SwapInvites(ctx, guildID, cur)
	if err != nil || !found {
		return Invite{}, false
	}
	return DiffInvites(prev, cur)
}

// RefreshInvites updates the invite snapshot (on start-up and when invites
// are created or deleted), so the next join can be matched to its invite.
func (s *Service) RefreshInvites(ctx context.Context, guildID snowflake.ID) error {
	st, on := s.settings(ctx, guildID)
	if !on || st.Destination(KindMemberJoin) == 0 || s.d.Invites == nil || s.d.Snapshots == nil {
		return nil
	}
	cur, err := s.d.Invites.ListInvites(ctx, guildID)
	if err != nil {
		return err
	}
	_, _, err = s.d.Snapshots.SwapInvites(ctx, guildID, cur)
	return err
}

// HandleMemberLeave logs a member leaving. Kicks and bans are logged
// separately, from the audit log.
func (s *Service) HandleMemberLeave(ctx context.Context, l MemberLeave) error {
	st, on := s.settings(ctx, l.GuildID)
	if !on {
		return nil
	}
	dest := st.Destination(KindMemberLeave)
	if dest == 0 {
		return nil
	}
	return s.post(ctx, dest, Entry{
		Kind: KindMemberLeave, GuildID: l.GuildID,
		UserID: l.UserID, UserName: l.UserName, UserBot: l.Bot,
		JoinedAt: l.JoinedAt, RoleIDs: l.RoleIDs,
	})
}

// HandleAudit logs moderation actions, member updates and server changes
// from a new audit-log entry. One entry can produce two log entries (a
// nickname change and a timeout in the same update).
func (s *Service) HandleAudit(ctx context.Context, a AuditEntry) error {
	st, on := s.settings(ctx, a.GuildID)
	if !on {
		return nil
	}
	var errs []error
	for _, e := range auditEntries(a) {
		dest := st.Destination(e.Kind)
		if dest == 0 {
			continue
		}
		if a.ByNiro && e.Kind.Category() == CategoryModeration && dest == s.d.ModLogChannel(ctx, a.GuildID) {
			continue // the case card is already in this channel
		}
		if err := s.post(ctx, dest, e); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func auditEntries(a AuditEntry) []Entry {
	base := Entry{
		GuildID: a.GuildID, At: a.At, ActorID: a.ActorID, ActorName: a.ActorName,
		ByNiro: a.ByNiro, Reason: a.Reason,
	}
	member := func(k Kind) Entry {
		e := base
		e.Kind, e.UserID, e.UserName = k, a.TargetID, a.TargetName
		return e
	}
	target := func(k Kind) Entry {
		e := base
		e.Kind, e.TargetID, e.TargetName, e.Changes = k, a.TargetID, a.TargetName, a.Changes
		return e
	}
	switch a.Action {
	case AuditMemberBan:
		return []Entry{member(KindMemberBan)}
	case AuditMemberUnban:
		return []Entry{member(KindMemberUnban)}
	case AuditMemberKick:
		return []Entry{member(KindMemberKick)}
	case AuditMemberUpdate:
		var out []Entry
		for _, c := range a.Changes {
			switch c.Key {
			case ChangeNick:
				e := member(KindMemberNick)
				e.Changes = []Change{c}
				out = append(out, e)
			case ChangeTimeoutUntil:
				e := member(KindMemberTimeout)
				if t, err := time.Parse(time.RFC3339, c.New); err == nil {
					e.TimeoutUntil = t
				}
				out = append(out, e)
			}
		}
		return out
	case AuditMemberRoleUpdate:
		e := member(KindMemberRoles)
		e.Changes = a.Changes
		return []Entry{e}
	case AuditChannelCreate:
		return []Entry{target(KindChannelCreate)}
	case AuditChannelUpdate:
		return []Entry{target(KindChannelUpdate)}
	case AuditChannelDelete:
		return []Entry{target(KindChannelDelete)}
	case AuditRoleCreate:
		return []Entry{target(KindRoleCreate)}
	case AuditRoleUpdate:
		return []Entry{target(KindRoleUpdate)}
	case AuditRoleDelete:
		return []Entry{target(KindRoleDelete)}
	case AuditGuildUpdate:
		return []Entry{target(KindGuildUpdate)}
	}
	return nil
}

// HandleVoice logs voice joins, leaves and moves.
func (s *Service) HandleVoice(ctx context.Context, v VoiceChange) error {
	if v.From == v.To {
		return nil // mute, deafen, stream… not a channel change
	}
	st, on := s.settings(ctx, v.GuildID)
	if !on {
		return nil
	}
	kind := KindVoiceMove
	switch {
	case v.From == 0:
		kind = KindVoiceJoin
	case v.To == 0:
		kind = KindVoiceLeave
	}
	dest := st.Destination(kind)
	if dest == 0 || st.ignored(0, 0, v.RoleIDs, v.Bot) {
		return nil
	}
	fromIgnored := v.From == 0 || st.ignored(v.From, 0, nil, false)
	toIgnored := v.To == 0 || st.ignored(v.To, 0, nil, false)
	if fromIgnored && toIgnored {
		return nil
	}
	return s.post(ctx, dest, Entry{
		Kind: kind, GuildID: v.GuildID, UserID: v.UserID, UserName: v.UserName, UserBot: v.Bot,
		ChannelID: v.To, OldChannelID: v.From,
	})
}

// HandleAutoMod logs an AutoMod rule firing. One trigger fires one event per
// action (block, alert, timeout); only the first is logged.
func (s *Service) HandleAutoMod(ctx context.Context, a AutoModAction) error {
	key := fmt.Sprintf("%s:%s:%s:%s:%s", a.GuildID, a.RuleID, a.UserID, a.ChannelID, a.Content)
	if _, dup := s.automod.Get(key); dup {
		return nil
	}
	s.automod.Set(key, struct{}{}, automodTTL)
	st, on := s.settings(ctx, a.GuildID)
	if !on {
		return nil
	}
	dest := st.Destination(KindAutoModAction)
	if dest == 0 {
		return nil
	}
	e := Entry{
		Kind: KindAutoModAction, GuildID: a.GuildID, UserID: a.UserID, UserName: a.UserName,
		ChannelID: a.ChannelID, RuleName: a.RuleName, Keyword: a.Keyword, Action: a.Action,
		ContentHidden: st.NoMessageContent,
	}
	if !st.NoMessageContent {
		e.Before = a.Content
	}
	return s.post(ctx, dest, e)
}

// HandleGuardian logs a Guardian detection, unless the log channel is the
// Guardian alert channel (the alert is already there).
func (s *Service) HandleGuardian(ctx context.Context, g GuardianAction) error {
	st, on := s.settings(ctx, g.GuildID)
	if !on {
		return nil
	}
	dest := st.Destination(KindGuardianAction)
	if dest == 0 || dest == s.d.GuardianAlertChannel(ctx, g.GuildID) {
		return nil
	}
	return s.post(ctx, dest, Entry{
		Kind: KindGuardianAction, GuildID: g.GuildID, UserID: g.UserID, UserName: g.UserName,
		ByNiro: true, ByNiroSource: "guardian", Action: g.Response, RuleName: g.Kind,
		Signals: g.Signals, Channels: g.Channels, EventID: g.EventID, CaseNumber: g.CaseNumber,
		Monitor: g.Monitor,
	})
}

// ttlMap is a small concurrent map whose entries expire.
type ttlMap[K comparable, V any] struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[K]ttlItem[V]
	sweep time.Time
}

type ttlItem[V any] struct {
	v   V
	exp time.Time
}

func newTTLMap[K comparable, V any](now func() time.Time) *ttlMap[K, V] {
	return &ttlMap[K, V]{now: now, items: map[K]ttlItem[V]{}}
}

func (m *ttlMap[K, V]) Set(k K, v V, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if now.After(m.sweep) {
		for key, it := range m.items {
			if now.After(it.exp) {
				delete(m.items, key)
			}
		}
		m.sweep = now.Add(time.Minute)
	}
	m.items[k] = ttlItem[V]{v: v, exp: now.Add(ttl)}
}

func (m *ttlMap[K, V]) Get(k K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[k]
	if !ok || m.now().After(it.exp) {
		var zero V
		return zero, false
	}
	return it.v, true
}
