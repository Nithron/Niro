// Package guardian is Niro's protection domain: compromised-account and
// scam-link detection, quarantine with a review flow, anti-raid, the
// verification gate and the permission audit. It is platform-agnostic:
// Discord specifics live behind the Platform and Alerter interfaces.
package guardian

import (
	"context"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/moderation"
)

// SignalKind is one reason a message looks malicious.
type SignalKind string

const (
	SignalBurst         SignalKind = "burst"            // same content in many channels
	SignalBlockedDomain SignalKind = "blocked_domain"   // on the scam blocklist
	SignalDeniedDomain  SignalKind = "denied_domain"    // on the server's deny list
	SignalLookalike     SignalKind = "lookalike_domain" // imitates Discord, Steam…
	SignalMaskedLink    SignalKind = "masked_link"      // link text shows another site
	SignalScamTemplate  SignalKind = "scam_template"    // known scam wording
	SignalQuietAccount  SignalKind = "quiet_account"    // long-silent member posting a link
	SignalNewAccount    SignalKind = "new_account"      // below the minimum account age
	SignalJoinFlood     SignalKind = "join_flood"       // raid detection
	SignalManual        SignalKind = "manual"           // started by a moderator
)

// Signal is a detection with its weight. Weights add up to a score:
// 3+ quarantine, 2 delete the message, 1 alert only.
type Signal struct {
	Kind   SignalKind `json:"kind"`
	Detail string     `json:"detail,omitempty"`
	Weight int        `json:"weight"`
}

// Response is what Guardian does about a detection.
type Response string

const (
	ResponseNone       Response = ""
	ResponseAlert      Response = "alert"      // tell moderators
	ResponseDelete     Response = "delete"     // delete the message(s) and alert
	ResponseQuarantine Response = "quarantine" // delete every copy, restrict the member, alert
	ResponseKick       Response = "kick"       // account-age gate
	ResponseRaidMode   Response = "raid_mode"
)

// Kind groups events for the dashboard.
type Kind string

const (
	KindCompromised Kind = "compromised" // burst-driven
	KindScam        Kind = "scam"        // scam links or texts
	KindRaid        Kind = "raid"
	KindAccountAge  Kind = "account_age"
)

// Status is where an event is in the review flow.
type Status string

const (
	StatusOpen     Status = "open"     // quarantined, waiting for a moderator
	StatusKept     Status = "kept"     // moderator kept the quarantine (still restorable)
	StatusRestored Status = "restored" // false positive: member restored
	StatusBanned   Status = "banned"
	StatusInfo     Status = "info"   // nothing to review (alert, delete, kick, monitor mode)
	StatusActive   Status = "active" // raid mode on
	StatusEnded    Status = "ended"  // raid mode over
)

// Resolution is a moderator's decision on a quarantine.
type Resolution string

const (
	ResolveRestore Resolution = "restore"
	ResolveBan     Resolution = "ban"
	ResolveKeep    Resolution = "keep"
)

// MessageRef points at one message.
type MessageRef struct {
	ChannelID snowflake.ID `json:"channel_id"`
	MessageID snowflake.ID `json:"message_id"`
}

// Evidence is the stored summary of a detection. It never contains message
// text (plan §5): only hashes, channel IDs, counts and domains.
type Evidence struct {
	Fingerprint string         `json:"fingerprint,omitempty"`
	Channels    []snowflake.ID `json:"channels,omitempty"`
	Messages    int            `json:"messages,omitempty"`
	Deleted     int            `json:"deleted,omitempty"`
	Domains     []string       `json:"domains,omitempty"`
	Joins       int            `json:"joins,omitempty"`
	AccountAge  string         `json:"account_age,omitempty"`
	Monitor     bool           `json:"monitor,omitempty"` // monitor mode: no action taken
	// Problems are actions that failed (missing permission, role too high…).
	Problems []string `json:"problems,omitempty"`
}

// Event is one Guardian detection and what happened next.
type Event struct {
	ID             int64
	GuildID        snowflake.ID
	UserID         snowflake.ID // 0 for raids
	UserName       string
	Kind           Kind
	Signals        []Signal
	Score          int
	Response       Response
	Evidence       Evidence
	CaseNumber     int
	Status         Status
	ResolvedBy     snowflake.ID
	ResolvedByName string
	ResolvedAt     *time.Time
	AlertChannelID snowflake.ID
	AlertMessageID snowflake.ID
	CreatedAt      time.Time
}

// NewEvent is an event to record; the store assigns the ID.
type NewEvent struct {
	GuildID    snowflake.ID
	UserID     snowflake.ID
	UserName   string
	Kind       Kind
	Signals    []Signal
	Score      int
	Response   Response
	Evidence   Evidence
	CaseNumber int
	Status     Status
}

// EventFilter selects events for listing. Zero values mean "any".
type EventFilter struct {
	Status Status
	UserID snowflake.ID
	Before int64 // events with a lower ID (cursor pagination)
	Limit  int
}

// Quarantine is a member's active restriction, kept so it can be undone.
type Quarantine struct {
	GuildID      snowflake.ID
	UserID       snowflake.ID
	EventID      int64
	Mode         QuarantineMode
	SavedRoleIDs []snowflake.ID // role mode: roles to give back
	RoleID       snowflake.ID   // role mode: the quarantine role applied
	Until        *time.Time     // timeout mode
	CreatedAt    time.Time
}

// Raid is a raid-mode period and the state to restore when it ends.
type Raid struct {
	ID          int64
	GuildID     snowflake.ID
	EventID     int64
	StartedAt   time.Time
	EndsAt      time.Time
	EndedAt     *time.Time
	EndedBy     snowflake.ID
	Manual      bool
	Joins       int
	State       RaidState
	StartedBy   snowflake.ID
	StartedName string
}

// RaidState records what raid mode changed.
type RaidState struct {
	InvitesPaused    bool                 `json:"invites_paused,omitempty"`
	PrevVerification *int                 `json:"prev_verification,omitempty"`
	SetVerification  int                  `json:"set_verification,omitempty"`
	PrevSlowmodes    map[snowflake.ID]int `json:"prev_slowmodes,omitempty"`
	SetSlowmode      int                  `json:"set_slowmode,omitempty"`
	Problems         []string             `json:"problems,omitempty"`
}

// LockState is a channel's @everyone overwrite before a lockdown.
type LockState struct {
	GuildID      snowflake.ID
	ChannelID    snowflake.ID
	HadOverwrite bool
	Allow        int64 // raw platform permission bits
	Deny         int64
	By           snowflake.ID
	CreatedAt    time.Time
}

// Store persists Guardian data.
type Store interface {
	CreateEvent(ctx context.Context, e NewEvent) (Event, error)
	GetEvent(ctx context.Context, guildID snowflake.ID, id int64) (Event, error)
	ListEvents(ctx context.Context, guildID snowflake.ID, f EventFilter) ([]Event, error)
	SetEventAlert(ctx context.Context, guildID snowflake.ID, id int64, channelID, messageID snowflake.ID) error
	// ResolveEvent moves an open or kept event to status; ErrAlreadyResolved otherwise.
	ResolveEvent(ctx context.Context, guildID snowflake.ID, id int64, status Status, by snowflake.ID, byName string) (Event, error)
	// SetEventStatus changes the status unconditionally (raid start/end).
	SetEventStatus(ctx context.Context, guildID snowflake.ID, id int64, status Status, by snowflake.ID, byName string) (Event, error)
	PurgeEventsBefore(ctx context.Context, t time.Time) (int64, error)

	SaveQuarantine(ctx context.Context, q Quarantine) error
	// ActiveQuarantine returns ErrNotQuarantined if there is none.
	ActiveQuarantine(ctx context.Context, guildID, userID snowflake.ID) (Quarantine, error)
	ReleaseQuarantine(ctx context.Context, guildID, userID snowflake.ID) error

	// StartRaid records a raid; started is false (and the active raid is
	// returned) if one is already running.
	StartRaid(ctx context.Context, r Raid) (raid Raid, started bool, err error)
	UpdateRaid(ctx context.Context, guildID snowflake.ID, id int64, eventID int64, state RaidState) error
	// ActiveRaid returns ErrNoRaid if raid mode is off.
	ActiveRaid(ctx context.Context, guildID snowflake.ID) (Raid, error)
	// EndRaid marks the active raid ended; ErrNoRaid if none (or another process won).
	EndRaid(ctx context.Context, guildID snowflake.ID, by snowflake.ID) (Raid, error)
	DueRaids(ctx context.Context, now time.Time) ([]Raid, error)

	SaveLockdowns(ctx context.Context, states []LockState) error
	Lockdowns(ctx context.Context, guildID snowflake.ID) ([]LockState, error)
	DeleteLockdowns(ctx context.Context, guildID snowflake.ID, channelIDs []snowflake.ID) error

	ReplaceFeedDomains(ctx context.Context, source string, domains []string) (total int, err error)
	ScamDomains(ctx context.Context) ([]string, error)
	FeedUpdatedAt(ctx context.Context, source string) (time.Time, error)
}

// RecentEntry is a recent message kept briefly for burst detection and for
// deleting every copy. Fingerprints are hashes, never text.
type RecentEntry struct {
	ChannelID    snowflake.ID `json:"c"`
	MessageID    snowflake.ID `json:"m"`
	Fingerprints []string     `json:"f"`
	At           time.Time    `json:"t"`
}

// Tracker holds short-lived counters (Valkey in production).
type Tracker interface {
	// RecordMessage stores e and returns the user's entries from the last
	// window, including e.
	RecordMessage(ctx context.Context, guildID, userID snowflake.ID, e RecentEntry, window time.Duration) ([]RecentEntry, error)
	// MarkActive records activity at `at` and returns the previous activity
	// time (zero if none within ttl).
	MarkActive(ctx context.Context, guildID, userID snowflake.ID, at time.Time, ttl time.Duration) (time.Time, error)
	// WatchingSince returns when Guardian first saw activity in the guild
	// (setting it to now on first call).
	WatchingSince(ctx context.Context, guildID snowflake.ID, now time.Time) (time.Time, error)
	// RecordJoin counts joins in the last window, including this one.
	RecordJoin(ctx context.Context, guildID, userID snowflake.ID, at time.Time, window time.Duration) (int, error)
	// Claim takes a short lock; false if someone else holds it.
	Claim(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

// Member is what Guardian needs to know about a member.
type Member struct {
	core.Rank
	Name          string
	IsBot         bool
	RoleIDs       []snowflake.ID
	TimedOutUntil *time.Time
}

// Channel is a text channel (for raid-mode slowmode).
type Channel struct {
	ID       snowflake.ID
	Slowmode int
}

// Platform performs actions on the chat platform.
type Platform interface {
	GuildName(guildID snowflake.ID) string
	// Member returns moderation.ErrNotMember if the user is not in the guild.
	Member(ctx context.Context, guildID, userID snowflake.ID) (Member, error)
	BotMember(ctx context.Context, guildID snowflake.ID) (Member, error)
	DeleteMessages(ctx context.Context, channelID snowflake.ID, ids []snowflake.ID, reason string) error
	Timeout(ctx context.Context, guildID, userID snowflake.ID, until time.Time, reason string) error
	RemoveTimeout(ctx context.Context, guildID, userID snowflake.ID, reason string) error
	// SetRoles replaces the member's roles (the platform keeps roles it
	// cannot remove, such as integration roles).
	SetRoles(ctx context.Context, guildID, userID snowflake.ID, roleIDs []snowflake.ID, reason string) error
	Kick(ctx context.Context, guildID, userID snowflake.ID, reason string) error
	// PauseInvites disables new joins via invites until `until`; a zero
	// time resumes them.
	PauseInvites(ctx context.Context, guildID snowflake.ID, until time.Time) error
	VerificationLevel(ctx context.Context, guildID snowflake.ID) (int, error)
	SetVerificationLevel(ctx context.Context, guildID snowflake.ID, level int, reason string) error
	TextChannels(ctx context.Context, guildID snowflake.ID) ([]Channel, error)
	SetSlowmode(ctx context.Context, channelID snowflake.ID, seconds int, reason string) error
}

// Alerter tells people about events.
type Alerter interface {
	// RecoveryDM sends a quarantined member steps to secure their account.
	RecoveryDM(ctx context.Context, ev Event, guildName string) error
	PostAlert(ctx context.Context, channelID snowflake.ID, ev Event) (snowflake.ID, error)
	EditAlert(ctx context.Context, channelID, messageID snowflake.ID, ev Event) error
}

// Moderation is the part of the moderation service Guardian uses.
type Moderation interface {
	Record(ctx context.Context, nc moderation.NewCase) (moderation.Case, error)
	Execute(ctx context.Context, req moderation.Request) (moderation.Outcome, error)
}

var (
	ErrEventNotFound   = errors.New("guardian event not found")
	ErrAlreadyResolved = errors.New("this event was already handled")
	ErrNotQuarantined  = errors.New("member is not quarantined")
	ErrNoRaid          = errors.New("raid mode is not active")
	ErrRaidActive      = errors.New("raid mode is already active")
	ErrBadResolution   = errors.New("this event cannot be resolved that way")
	ErrModuleOff       = errors.New("the Guardian module is turned off")
)

// Verification levels (Discord's numbering; other platforms map onto it).
const VerificationHigh = 3

// Problems lists every problem code Guardian records in Evidence.Problems
// (each has a translation).
var Problems = []string{
	"left_server", "bot_role_too_low", "target_is_owner", "target_is_bot", "restrict_failed", "delete_failed",
	"kick_failed", "pause_invites_failed", "verification_failed", "slowmode_failed",
}
