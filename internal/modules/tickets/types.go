// Package tickets is Niro's support tickets: panels with a button per
// category, private threads or channels per ticket, an optional form,
// claim / close / reopen, and HTML transcripts kept for a limited time.
// It is platform-agnostic; the Discord adapter does the Discord parts.
//
// Privacy: transcripts are the one place Niro stores message text in
// Postgres (decision 25). They are gzip-compressed, expire after the
// server's retention period (90 days by default) and are deleted by a job.
package tickets

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
)

// Mode is where a ticket lives.
type Mode string

const (
	ModeThread  Mode = "thread"  // a private thread in the panel's channel
	ModeChannel Mode = "channel" // a channel only the opener and staff can see
)

// Limits (decision 25 and Discord's).
const (
	MaxPanels        = 10
	MaxCategories    = 5 // one row of buttons
	MaxStaffRoles    = 10
	MaxQuestions     = 5  // Discord: 5 inputs per modal
	MaxQuestionLabel = 45 // Discord: label length in modals
	MaxPlaceholder   = 100
	MaxAnswer        = 1000
	MaxOpenPerMember = 5
	MaxCloseReason   = 500
)

// Question is one field of the form shown when opening a ticket.
type Question struct {
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Long        bool   `json:"long,omitempty"` // a paragraph instead of one line
	Required    bool   `json:"required,omitempty"`
}

// Category is one kind of ticket on a panel (one button).
type Category struct {
	// ID is stable across edits (it is in the button's custom ID).
	ID    string `json:"id"`
	Label string `json:"label"`
	Emoji string `json:"emoji,omitempty"`
	Mode  Mode   `json:"mode"`
	// ParentID is the channel category new ticket channels go in (channel mode; 0 = none).
	ParentID     snowflake.ID     `json:"parent_id,omitempty"`
	StaffRoleIDs []snowflake.ID   `json:"staff_role_ids"`
	Questions    []Question       `json:"questions,omitempty"`
	Opening      message.Document `json:"opening"`
	// MaxOpen is how many open tickets a member may have in this category.
	MaxOpen int `json:"max_open"`
}

// Panel is a ticket panel.
type Panel struct {
	ID         int64
	GuildID    snowflake.ID
	Name       string
	ChannelID  snowflake.ID
	MessageID  snowflake.ID
	Message    message.Document
	Categories []Category
	UpdatedBy  snowflake.ID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Category returns the category with id.
func (p Panel) Category(id string) (Category, bool) {
	i := slices.IndexFunc(p.Categories, func(c Category) bool { return c.ID == id })
	if i < 0 {
		return Category{}, false
	}
	return p.Categories[i], true
}

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid ticket panel")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// openingBudget: the ticket's first message carries a row with Claim and Close.
var openingBudget = message.Budget{Components: 3}

// Validate checks and normalizes the panel; categories without an ID get one.
func (p *Panel) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 100 {
		return invalid("the name must be 1 to 100 characters")
	}
	if p.ChannelID == 0 {
		return invalid("choose a channel")
	}
	if len(p.Categories) == 0 || len(p.Categories) > MaxCategories {
		return invalid("a panel has 1 to %d categories", MaxCategories)
	}
	ids := map[string]bool{}
	for i := range p.Categories {
		c := &p.Categories[i]
		n := i + 1
		c.Label = strings.TrimSpace(c.Label)
		if c.ID == "" {
			c.ID = newCategoryID()
		}
		switch {
		case ids[c.ID] || len(c.ID) > 16:
			return invalid("category %d: bad ID", n)
		case c.Label == "" || utf8.RuneCountInString(c.Label) > 80:
			return invalid("category %d: the label must be 1 to 80 characters", n)
		case c.Emoji != "" && !message.IsEmoji(c.Emoji):
			return invalid("category %d: emoji must be a single Unicode emoji", n)
		case c.Mode != ModeThread && c.Mode != ModeChannel:
			return invalid("category %d: mode must be thread or channel", n)
		case len(c.StaffRoleIDs) == 0 || len(c.StaffRoleIDs) > MaxStaffRoles:
			return invalid("category %d: choose 1 to %d staff roles", n, MaxStaffRoles)
		case len(c.Questions) > MaxQuestions:
			return invalid("category %d: at most %d questions", n, MaxQuestions)
		}
		ids[c.ID] = true
		if c.Mode == ModeThread {
			c.ParentID = 0
		}
		if c.MaxOpen == 0 {
			c.MaxOpen = 1
		}
		if c.MaxOpen < 1 || c.MaxOpen > MaxOpenPerMember {
			return invalid("category %d: open tickets per member must be 1 to %d", n, MaxOpenPerMember)
		}
		for j := range c.Questions {
			q := &c.Questions[j]
			q.Label, q.Placeholder = strings.TrimSpace(q.Label), strings.TrimSpace(q.Placeholder)
			if q.Label == "" || utf8.RuneCountInString(q.Label) > MaxQuestionLabel {
				return invalid("category %d, question %d: the question must be 1 to %d characters", n, j+1, MaxQuestionLabel)
			}
			if utf8.RuneCountInString(q.Placeholder) > MaxPlaceholder {
				return invalid("category %d, question %d: the hint is too long", n, j+1)
			}
		}
		if err := c.Opening.Validate(openingBudget); err != nil {
			return invalid("category %d: opening message: %v", n, err)
		}
	}
	// The panel message carries one row with a button per category.
	if err := p.Message.Validate(message.Budget{Components: 1 + len(p.Categories)}); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

func newCategoryID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Status is where a ticket is in its lifecycle.
type Status string

const (
	StatusOpen    Status = "open"
	StatusClaimed Status = "claimed"
	StatusClosed  Status = "closed"
)

// Ticket is one ticket.
type Ticket struct {
	ID            int64
	GuildID       snowflake.ID
	Number        int
	PanelID       int64
	CategoryID    string
	CategoryLabel string
	Mode          Mode
	ChannelID     snowflake.ID
	OpenerID      snowflake.ID
	OpenerName    string
	ClaimedBy     snowflake.ID
	ClaimedByName string
	Status        Status
	CloseReason   string
	ClosedBy      snowflake.ID
	ClosedByName  string
	ClosedAt      *time.Time
	CreatedAt     time.Time
	// HasTranscript: a transcript is stored (and not expired).
	HasTranscript bool
}

// NewTicket is a ticket to record; the store assigns ID and Number.
type NewTicket struct {
	GuildID       snowflake.ID
	PanelID       int64
	CategoryID    string
	CategoryLabel string
	Mode          Mode
	OpenerID      snowflake.ID
	OpenerName    string
}

// Filter selects tickets for listing. Zero values mean "any".
type Filter struct {
	Status Status
	Before int // ticket number (cursor pagination)
	Limit  int
}

// Transcript is a closed ticket's conversation, as gzip-compressed HTML.
type Transcript struct {
	TicketID  int64
	GuildID   snowflake.ID
	HTMLGzip  []byte
	Messages  int
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Store persists panels, tickets and transcripts.
type Store interface {
	CreateTicketPanel(ctx context.Context, p Panel) (Panel, error)
	UpdateTicketPanel(ctx context.Context, p Panel) (Panel, error)
	GetTicketPanel(ctx context.Context, guildID snowflake.ID, id int64) (Panel, error)
	ListTicketPanels(ctx context.Context, guildID snowflake.ID) ([]Panel, error)
	CountTicketPanels(ctx context.Context, guildID snowflake.ID) (int, error)
	DeleteTicketPanel(ctx context.Context, guildID snowflake.ID, id int64) error
	SetTicketPanelMessage(ctx context.Context, guildID snowflake.ID, id int64, channelID, messageID snowflake.ID) error

	CreateTicket(ctx context.Context, t NewTicket) (Ticket, error)
	DeleteTicket(ctx context.Context, guildID snowflake.ID, id int64) error
	SetTicketChannel(ctx context.Context, guildID snowflake.ID, id int64, channelID snowflake.ID) error
	GetTicket(ctx context.Context, guildID snowflake.ID, id int64) (Ticket, error)
	GetTicketByNumber(ctx context.Context, guildID snowflake.ID, number int) (Ticket, error)
	GetTicketByChannel(ctx context.Context, guildID, channelID snowflake.ID) (Ticket, error)
	ListTickets(ctx context.Context, guildID snowflake.ID, f Filter) ([]Ticket, error)
	CountOpenTickets(ctx context.Context, guildID, openerID snowflake.ID, panelID int64, categoryID string) (int, error)
	// ClaimTicket claims an open ticket; ErrWrongStatus if it isn't open.
	ClaimTicket(ctx context.Context, guildID snowflake.ID, id int64, by snowflake.ID, byName string) (Ticket, error)
	// CloseTicket closes an open or claimed ticket; ErrWrongStatus otherwise.
	CloseTicket(ctx context.Context, guildID snowflake.ID, id int64, by snowflake.ID, byName, reason string) (Ticket, error)
	// ReopenTicket reopens a closed ticket; ErrWrongStatus otherwise.
	ReopenTicket(ctx context.Context, guildID snowflake.ID, id int64) (Ticket, error)

	SaveTranscript(ctx context.Context, t Transcript) error
	GetTranscript(ctx context.Context, guildID snowflake.ID, ticketID int64) (Transcript, error)
	PurgeTranscripts(ctx context.Context, before time.Time) (int64, error)
}

var (
	ErrNotFound        = errors.New("ticket not found")
	ErrPanelNotFound   = errors.New("ticket panel not found")
	ErrTooManyPanels   = errors.New("this server has the maximum number of ticket panels")
	ErrWrongStatus     = errors.New("the ticket is not in a state that allows this")
	ErrLimit           = errors.New("you already have the most open tickets this category allows")
	ErrNotStaff        = errors.New("only staff can do that")
	ErrModuleOff       = errors.New("the Tickets module is turned off")
	ErrMessageGone     = errors.New("the panel's message was deleted")
	ErrNoTranscript    = errors.New("no transcript (it expired or the ticket was never closed)")
	ErrBadAnswers      = errors.New("the form answers don't match the questions")
	ErrCategoryRemoved = errors.New("that category was removed from the panel")
)

// ---- settings ------------------------------------------------------------------

// Settings are the per-server ticket settings.
type Settings struct {
	// RetentionDays is how long transcripts are kept (decision 25: 90).
	RetentionDays int `json:"retention_days"`
	// TranscriptChannelID gets a copy of every transcript (0 = none).
	TranscriptChannelID snowflake.ID `json:"transcript_channel_id"`
}

func DefaultSettings() Settings { return Settings{RetentionDays: 90} }

// ErrInvalidSettings wraps settings validation failures.
var ErrInvalidSettings = errors.New("invalid ticket settings")

// SettingsSpec validates ticket settings for the config service.
type SettingsSpec struct{}

var _ core.SettingsSpec = SettingsSpec{}

func (SettingsSpec) Normalize(raw []byte) ([]byte, error) {
	s := DefaultSettings()
	if len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSettings, err)
		}
	}
	if s.RetentionDays < 1 || s.RetentionDays > 365 {
		return nil, fmt.Errorf("%w: transcripts are kept 1 to 365 days", ErrInvalidSettings)
	}
	return json.Marshal(s)
}

// LoadSettings reads a server's ticket settings.
func LoadSettings(ctx context.Context, cfg *core.ConfigService, guildID snowflake.ID) (Settings, error) {
	raw, err := cfg.Settings(ctx, guildID, core.ModuleTickets)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	return s, json.Unmarshal(raw, &s)
}
