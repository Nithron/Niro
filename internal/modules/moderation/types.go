// Package moderation is Niro's moderation domain: cases, the moderation
// actions (warn, timeout, kick, ban…), message purges, and scheduled actions
// such as lifting temporary bans. It is platform-agnostic: Discord specifics
// live behind the Platform and Notifier interfaces.
package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
)

// Action is a moderation action recorded as a case.
type Action string

const (
	ActionWarn      Action = "warn"
	ActionTimeout   Action = "timeout"
	ActionUntimeout Action = "untimeout"
	ActionKick      Action = "kick"
	ActionBan       Action = "ban"
	ActionUnban     Action = "unban"
	ActionNote      Action = "note"
	ActionPurge     Action = "purge"
	// Guardian records these; moderators cannot run them as commands.
	ActionQuarantine   Action = "quarantine"
	ActionUnquarantine Action = "unquarantine"
)

// Actions lists every action (for validation and filters).
var Actions = []Action{ActionWarn, ActionTimeout, ActionUntimeout, ActionKick, ActionBan, ActionUnban, ActionNote, ActionPurge,
	ActionQuarantine, ActionUnquarantine}

func (a Action) Valid() bool {
	for _, x := range Actions {
		if a == x {
			return true
		}
	}
	return false
}

// Source records what triggered a case.
type Source string

const (
	SourceManual    Source = "manual"
	SourceAutoMod   Source = "automod"
	SourceGuardian  Source = "guardian"
	SourceDashboard Source = "dashboard"
	SourceSystem    Source = "system"
)

// Limits.
const (
	MaxReasonLength = 512
	MinTimeout      = time.Minute
	MaxTimeout      = 28 * 24 * time.Hour // Discord's limit
	MinTempBan      = time.Minute
	MaxTempBan      = 365 * 24 * time.Hour
	MaxBanDeleteAge = 7 * 24 * time.Hour // Discord's limit for deleting a banned user's messages
	MaxPurge        = 100
)

// Details holds action-specific extras, stored as JSON.
type Details struct {
	ChannelID      snowflake.ID `json:"channel_id,omitempty"`      // purge
	Deleted        int          `json:"deleted,omitempty"`         // purge
	DeleteMessages int          `json:"delete_messages,omitempty"` // ban: seconds of messages deleted
	Temporary      bool         `json:"temporary,omitempty"`       // ban with an expiry
}

// Case is one recorded moderation action.
type Case struct {
	GuildID       snowflake.ID
	Number        int
	Action        Action
	TargetID      snowflake.ID // 0 for purges without a user filter
	TargetName    string
	ModeratorID   snowflake.ID
	ModeratorName string
	Reason        string
	Duration      time.Duration // timeouts and temporary bans
	ExpiresAt     *time.Time
	Source        Source
	Details       Details
	DMSent        bool
	LogChannelID  snowflake.ID
	LogMessageID  snowflake.ID
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Deleted       bool
}

// NewCase is a case to record; the store assigns the number.
type NewCase struct {
	GuildID       snowflake.ID
	Action        Action
	TargetID      snowflake.ID
	TargetName    string
	ModeratorID   snowflake.ID
	ModeratorName string
	Reason        string
	Duration      time.Duration
	ExpiresAt     *time.Time
	Source        Source
	Details       Details
	DMSent        bool
	// ScheduleUnbanAt, if set, schedules lifting a temporary ban in the same
	// transaction as the case.
	ScheduleUnbanAt *time.Time
}

// CaseFilter selects cases for listing. Zero values mean "any".
type CaseFilter struct {
	TargetID    snowflake.ID
	ModeratorID snowflake.ID
	Action      Action
	Before      int // only cases numbered below this (cursor pagination)
	Limit       int
}

// ScheduledAction is a claimed piece of deferred work.
type ScheduledAction struct {
	ID         int64
	GuildID    snowflake.ID
	Kind       string
	TargetID   snowflake.ID
	CaseNumber int
	Attempts   int
}

// CaseStore persists cases and scheduled actions.
type CaseStore interface {
	CreateCase(ctx context.Context, c NewCase) (Case, error)
	GetCase(ctx context.Context, guildID snowflake.ID, number int) (Case, error)
	ListCases(ctx context.Context, guildID snowflake.ID, f CaseFilter) ([]Case, error)
	CountCases(ctx context.Context, guildID snowflake.ID, f CaseFilter) (int, error)
	UpdateCaseReason(ctx context.Context, guildID snowflake.ID, number int, reason string) (Case, error)
	DeleteCase(ctx context.Context, guildID snowflake.ID, number int, by snowflake.ID) (Case, error)
	SetCaseLogMessage(ctx context.Context, guildID snowflake.ID, number int, channelID, messageID snowflake.ID) error
	CancelScheduledUnbans(ctx context.Context, guildID, targetID snowflake.ID) error
	ClaimDueActions(ctx context.Context, limit int) ([]ScheduledAction, error)
	CompleteScheduledAction(ctx context.Context, id int64, note string) error
	RetryScheduledAction(ctx context.Context, id int64, errText string, after time.Duration) error
}

// Member is what the domain needs to know about a server member.
type Member struct {
	core.Rank
	Name          string
	IsBot         bool
	TimedOutUntil *time.Time
}

// Message is a message considered for a purge.
type Message struct {
	ID        snowflake.ID
	AuthorID  snowflake.ID
	AuthorBot bool
	Content   string
	HasEmbeds bool
	Pinned    bool
	CreatedAt time.Time
}

// Platform performs actions on the chat platform.
type Platform interface {
	// Member returns ErrNotMember if the user is not in the guild.
	Member(ctx context.Context, guildID, userID snowflake.ID) (Member, error)
	BotMember(ctx context.Context, guildID snowflake.ID) (Member, error)
	Timeout(ctx context.Context, guildID, userID snowflake.ID, until time.Time, reason string) error
	RemoveTimeout(ctx context.Context, guildID, userID snowflake.ID, reason string) error
	Kick(ctx context.Context, guildID, userID snowflake.ID, reason string) error
	Ban(ctx context.Context, guildID, userID snowflake.ID, deleteMessages time.Duration, reason string) error
	// Unban returns ErrNotBanned if the user is not banned.
	Unban(ctx context.Context, guildID, userID snowflake.ID, reason string) error
	// RecentMessages returns up to limit messages before `before` (0 = newest), newest first.
	RecentMessages(ctx context.Context, channelID, before snowflake.ID, limit int) ([]Message, error)
	DeleteMessages(ctx context.Context, channelID snowflake.ID, ids []snowflake.ID, reason string) error
}

// Notifier tells people about cases: the affected user (DM) and the
// server's moderation log channel.
type Notifier interface {
	NotifyUser(ctx context.Context, c Case, guildName string) error
	PostLog(ctx context.Context, channelID snowflake.ID, c Case) (snowflake.ID, error)
	EditLog(ctx context.Context, channelID, messageID snowflake.ID, c Case) error
}

var (
	ErrNotMember      = errors.New("user is not a member of this server")
	ErrNotBanned      = errors.New("user is not banned")
	ErrCaseNotFound   = errors.New("case not found")
	ErrReasonRequired = errors.New("a reason is required")
	ErrReasonTooLong  = errors.New("reason is too long")
	ErrBadDuration    = errors.New("duration out of range")
	ErrBadAction      = errors.New("action not allowed here")
	ErrNotTimedOut    = errors.New("user is not timed out")
	ErrBadPurge       = errors.New("purge count must be between 1 and 100")
)

// ActionError wraps a platform failure while performing an action.
type ActionError struct {
	Action Action
	Err    error
}

func (e *ActionError) Error() string { return "performing " + string(e.Action) + ": " + e.Err.Error() }
func (e *ActionError) Unwrap() error { return e.Err }

// detailsJSON is used by stores.
func (d Details) JSON() []byte {
	b, _ := json.Marshal(d)
	return b
}
