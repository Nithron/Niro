// Package logging is Niro's server log: message edits and deletes, member,
// moderation, server, voice, AutoMod and Guardian events, each posted to a
// log channel chosen per category. It is platform-agnostic: the Discord
// adapter turns gateway events into the inputs below and renders Entries.
//
// Privacy (plan §1, §5): message text is never stored in Postgres. Recent
// messages are cached in Valkey with a TTL so that edits and deletes can
// show what changed; servers can turn message content off entirely.
package logging

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// Category groups event kinds; each category has its own log channel.
type Category string

const (
	CategoryMessages   Category = "messages"
	CategoryMembers    Category = "members"
	CategoryModeration Category = "moderation"
	CategoryServer     Category = "server"
	CategoryVoice      Category = "voice"
	CategoryAutoMod    Category = "automod"
	CategoryGuardian   Category = "guardian"
)

// Categories in display order.
var Categories = []Category{
	CategoryMessages, CategoryMembers, CategoryModeration, CategoryServer,
	CategoryVoice, CategoryAutoMod, CategoryGuardian,
}

// Kind is one type of log entry. Kinds are stable: they are stored in
// settings and used in translation keys.
type Kind string

const (
	KindMessageEdit       Kind = "message_edit"
	KindMessageDelete     Kind = "message_delete"
	KindMessageBulkDelete Kind = "message_bulk_delete"

	KindMemberJoin  Kind = "member_join"
	KindMemberLeave Kind = "member_leave"
	KindMemberNick  Kind = "member_nick"
	KindMemberRoles Kind = "member_roles"

	KindMemberBan     Kind = "member_ban"
	KindMemberUnban   Kind = "member_unban"
	KindMemberKick    Kind = "member_kick"
	KindMemberTimeout Kind = "member_timeout"

	KindChannelCreate Kind = "channel_create"
	KindChannelUpdate Kind = "channel_update"
	KindChannelDelete Kind = "channel_delete"
	KindRoleCreate    Kind = "role_create"
	KindRoleUpdate    Kind = "role_update"
	KindRoleDelete    Kind = "role_delete"
	KindGuildUpdate   Kind = "guild_update"

	KindVoiceJoin  Kind = "voice_join"
	KindVoiceLeave Kind = "voice_leave"
	KindVoiceMove  Kind = "voice_move"

	KindAutoModAction  Kind = "automod_action"
	KindGuardianAction Kind = "guardian_action"
)

type kindInfo struct {
	kind      Kind
	category  Category
	defaultOn bool
}

// kinds is the registry of every kind, in display order. Voice is off by
// default because it is noisy.
var kinds = []kindInfo{
	{KindMessageEdit, CategoryMessages, true},
	{KindMessageDelete, CategoryMessages, true},
	{KindMessageBulkDelete, CategoryMessages, true},
	{KindMemberJoin, CategoryMembers, true},
	{KindMemberLeave, CategoryMembers, true},
	{KindMemberNick, CategoryMembers, true},
	{KindMemberRoles, CategoryMembers, true},
	{KindMemberBan, CategoryModeration, true},
	{KindMemberUnban, CategoryModeration, true},
	{KindMemberKick, CategoryModeration, true},
	{KindMemberTimeout, CategoryModeration, true},
	{KindChannelCreate, CategoryServer, true},
	{KindChannelUpdate, CategoryServer, true},
	{KindChannelDelete, CategoryServer, true},
	{KindRoleCreate, CategoryServer, true},
	{KindRoleUpdate, CategoryServer, true},
	{KindRoleDelete, CategoryServer, true},
	{KindGuildUpdate, CategoryServer, true},
	{KindVoiceJoin, CategoryVoice, false},
	{KindVoiceLeave, CategoryVoice, false},
	{KindVoiceMove, CategoryVoice, false},
	{KindAutoModAction, CategoryAutoMod, true},
	{KindGuardianAction, CategoryGuardian, true},
}

// Kinds lists every kind in display order.
var Kinds = func() []Kind {
	out := make([]Kind, len(kinds))
	for i, k := range kinds {
		out[i] = k.kind
	}
	return out
}()

// Category returns the kind's category ("" for an unknown kind).
func (k Kind) Category() Category {
	for _, x := range kinds {
		if x.kind == k {
			return x.category
		}
	}
	return ""
}

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k.Category() != "" }

// Message is a message as seen by the platform (create or edit).
type Message struct {
	ID        snowflake.ID
	GuildID   snowflake.ID
	ChannelID snowflake.ID
	// ParentID is the parent channel of a thread (0 otherwise); ignore lists
	// apply to a thread through its parent.
	ParentID      snowflake.ID
	AuthorID      snowflake.ID
	AuthorName    string
	AuthorBot     bool // bots and webhooks
	AuthorRoleIDs []snowflake.ID
	// FromNiro is true for Niro's own messages, which are never logged.
	FromNiro    bool
	Content     string
	Attachments []string // file names
	CreatedAt   time.Time
	EditedAt    time.Time // zero if never edited
}

// CachedMessage is what the message cache keeps (Valkey, with a TTL).
// Content is empty when the server turned message content off.
type CachedMessage struct {
	ID            snowflake.ID   `json:"id"`
	ChannelID     snowflake.ID   `json:"c"`
	AuthorID      snowflake.ID   `json:"a"`
	AuthorName    string         `json:"n,omitempty"`
	AuthorBot     bool           `json:"b,omitempty"`
	AuthorRoleIDs []snowflake.ID `json:"r,omitempty"`
	Content       string         `json:"t,omitempty"`
	Attachments   []string       `json:"f,omitempty"`
	CreatedAt     time.Time      `json:"at"`
	EditedAt      time.Time      `json:"e,omitzero"`
}

// MaxCachedContent caps the cached text (Discord's longest message).
const MaxCachedContent = 4000

// MessageCacheTTL is how long message text is kept for edit and delete
// logs: the approved 7-day retention (plan §5).
const MessageCacheTTL = 7 * 24 * time.Hour

// MessageCache keeps recent messages (Valkey in production).
type MessageCache interface {
	Put(ctx context.Context, m CachedMessage, ttl time.Duration) error
	Get(ctx context.Context, channelID, messageID snowflake.ID) (CachedMessage, bool, error)
	// Take returns and removes the messages that are cached; missing ones
	// are simply absent from the result.
	Take(ctx context.Context, channelID snowflake.ID, messageIDs []snowflake.ID) ([]CachedMessage, error)
}

// Invite is a server invite and how often it was used.
type Invite struct {
	Code        string       `json:"code"`
	InviterID   snowflake.ID `json:"inviter_id,omitempty"`
	InviterName string       `json:"inviter_name,omitempty"`
	Uses        int          `json:"uses"`
	MaxUses     int          `json:"max_uses,omitempty"` // 0 = unlimited
}

// InviteSource lists a server's invites (needs Manage Server on Discord).
type InviteSource interface {
	ListInvites(ctx context.Context, guildID snowflake.ID) ([]Invite, error)
}

// InviteSnapshots stores the last known invite list per server.
type InviteSnapshots interface {
	// SwapInvites stores cur and returns the previous snapshot (found is
	// false if there was none).
	SwapInvites(ctx context.Context, guildID snowflake.ID, cur []Invite) (prev []Invite, found bool, err error)
}

// MemberJoin is a member joining.
type MemberJoin struct {
	GuildID        snowflake.ID
	UserID         snowflake.ID
	UserName       string
	Bot            bool
	AccountCreated time.Time
	JoinedAt       time.Time
}

// MemberLeave is a member leaving (or being kicked or banned).
type MemberLeave struct {
	GuildID  snowflake.ID
	UserID   snowflake.ID
	UserName string
	Bot      bool
	JoinedAt time.Time // zero if unknown
	RoleIDs  []snowflake.ID
}

// AuditAction is a platform audit-log action that Niro logs.
type AuditAction string

const (
	AuditMemberBan        AuditAction = "member_ban"
	AuditMemberUnban      AuditAction = "member_unban"
	AuditMemberKick       AuditAction = "member_kick"
	AuditMemberUpdate     AuditAction = "member_update" // nickname, timeout
	AuditMemberRoleUpdate AuditAction = "member_role_update"
	AuditChannelCreate    AuditAction = "channel_create"
	AuditChannelUpdate    AuditAction = "channel_update" // also overwrites and positions
	AuditChannelDelete    AuditAction = "channel_delete"
	AuditRoleCreate       AuditAction = "role_create"
	AuditRoleUpdate       AuditAction = "role_update" // also positions
	AuditRoleDelete       AuditAction = "role_delete"
	AuditGuildUpdate      AuditAction = "guild_update"
)

// Change keys with special meaning (the platform's own field names pass
// through for everything else).
const (
	ChangeNick         = "nick"
	ChangeTimeoutUntil = "communication_disabled_until"
	ChangeRolesAdded   = "$add"
	ChangeRolesRemoved = "$remove"
)

// Change is one changed field. Old and New are display-ready values
// (the adapter formats permissions, mentions, times…); empty means none.
type Change struct {
	Key string
	Old string
	New string
}

// AuditEntry is a new audit-log entry: who did what, to whom, and why.
type AuditEntry struct {
	GuildID    snowflake.ID
	Action     AuditAction
	ActorID    snowflake.ID
	ActorName  string
	ByNiro     bool // Niro itself did it
	TargetID   snowflake.ID
	TargetName string
	Reason     string
	Changes    []Change
	At         time.Time
}

// VoiceChange is a member joining, leaving or moving between voice channels.
type VoiceChange struct {
	GuildID  snowflake.ID
	UserID   snowflake.ID
	UserName string
	Bot      bool
	RoleIDs  []snowflake.ID
	From     snowflake.ID // 0 = joined
	To       snowflake.ID // 0 = left
}

// AutoModAction is one action a native AutoMod rule took.
type AutoModAction struct {
	GuildID   snowflake.ID
	RuleID    snowflake.ID
	RuleName  string
	UserID    snowflake.ID
	UserName  string
	ChannelID snowflake.ID
	Action    string // block_message, send_alert, timeout, block_interaction
	Keyword   string // matched keyword or pattern
	Content   string // the matched message text (hidden if content is off)
}

// GuardianAction is a Guardian detection, as reported by the Guardian module.
type GuardianAction struct {
	GuildID    snowflake.ID
	EventID    int64
	Kind       string // guardian.Kind
	Response   string // guardian.Response
	UserID     snowflake.ID
	UserName   string
	Signals    []string // guardian.SignalKind values
	Channels   []snowflake.ID
	Monitor    bool
	CaseNumber int
}

// Entry is one log entry to post. Which fields are set depends on Kind.
type Entry struct {
	Kind    Kind
	GuildID snowflake.ID
	At      time.Time

	// Who did it (0 = unknown). ByNiro: Niro itself.
	ActorID   snowflake.ID
	ActorName string
	ByNiro    bool
	// ByNiroSource says which Niro feature acted ("guardian", "purge"…).
	ByNiroSource string
	Reason       string

	// The member concerned.
	UserID   snowflake.ID
	UserName string
	UserBot  bool

	// The channel concerned (message, voice or changed channel); for voice
	// moves OldChannelID is where they came from.
	ChannelID    snowflake.ID
	OldChannelID snowflake.ID
	// The role, channel or server changed (server category).
	TargetID   snowflake.ID
	TargetName string
	Changes    []Change

	// Messages: the message, and text before and after an edit.
	MessageID snowflake.ID
	Cached    bool // the message was in the cache (Before is known)
	Before    string
	After     string
	// ContentHidden: the server turned message content off.
	ContentHidden bool
	Attachments   []string
	// Bulk deletes: the cached messages (oldest first) and how many were
	// deleted in total.
	Messages []CachedMessage
	Count    int

	// Joins and leaves.
	AccountCreated time.Time
	JoinedAt       time.Time
	RoleIDs        []snowflake.ID
	Invite         *Invite

	// Timeouts: until when (zero = timeout removed).
	TimeoutUntil time.Time

	// AutoMod and Guardian.
	RuleName   string
	Keyword    string
	Action     string
	Signals    []string
	Channels   []snowflake.ID
	EventID    int64
	CaseNumber int
	Monitor    bool
}

// Sink posts entries to a log channel.
type Sink interface {
	Post(ctx context.Context, channelID snowflake.ID, e Entry) error
}
