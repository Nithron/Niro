// Package welcome greets new members and says goodbye: a channel message,
// an optional DM, and roles given on join. It is platform-agnostic.
//
// It respects Guardian: members the account-age gate removes are never
// greeted; when the verification gate is set up, auto-roles are given when
// a member verifies rather than on join; and during raid mode joins are
// summed up in one message instead of one each, with no DMs or roles.
package welcome

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/roles"
)

// Settings are the per-server welcome settings.
type Settings struct {
	WelcomeEnabled   bool             `json:"welcome_enabled"`
	WelcomeChannelID snowflake.ID     `json:"welcome_channel_id"`
	WelcomeMessage   message.Document `json:"welcome_message"`
	// PingMember lets the welcome message notify the new member (and only them).
	PingMember bool `json:"ping_member"`

	// DMs are off by default (decision 25): unsolicited DMs annoy people.
	DMEnabled bool             `json:"dm_enabled"`
	DMMessage message.Document `json:"dm_message"`

	GoodbyeEnabled   bool             `json:"goodbye_enabled"`
	GoodbyeChannelID snowflake.ID     `json:"goodbye_channel_id"`
	GoodbyeMessage   message.Document `json:"goodbye_message"`

	AutoRoleIDs []snowflake.ID `json:"auto_role_ids"`
}

// MaxAutoRoles limits roles given on join.
const MaxAutoRoles = 10

// DefaultSettings: nothing is posted until a channel is chosen.
func DefaultSettings() Settings {
	return Settings{
		WelcomeMessage: message.Simple("Welcome to **{server}**, {user}! You're member #{memberCount}."),
		PingMember:     true,
		DMMessage:      message.Simple("Welcome to **{server}**! Have a look at the rules and say hi."),
		GoodbyeMessage: message.Simple("**{user.name}** left the server."),
		AutoRoleIDs:    []snowflake.ID{},
	}
}

// ErrInvalidSettings wraps every validation failure.
var ErrInvalidSettings = errors.New("invalid welcome settings")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSettings, fmt.Sprintf(format, a...))
}

// SettingsSpec validates welcome settings for the config service.
type SettingsSpec struct{}

var _ core.SettingsSpec = SettingsSpec{}

func (SettingsSpec) Normalize(raw []byte) ([]byte, error) {
	s := DefaultSettings()
	if len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return nil, invalid("%v", err)
		}
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

func (s *Settings) validate() error {
	if s.WelcomeEnabled && s.WelcomeChannelID == 0 {
		return invalid("choose a channel for the welcome message")
	}
	if s.GoodbyeEnabled && s.GoodbyeChannelID == 0 {
		return invalid("choose a channel for the goodbye message")
	}
	for name, d := range map[string]*message.Document{"welcome": &s.WelcomeMessage, "DM": &s.DMMessage, "goodbye": &s.GoodbyeMessage} {
		if err := d.Validate(message.Budget{}); err != nil {
			return invalid("%s message: %v", name, err)
		}
	}
	ids := []snowflake.ID{}
	for _, id := range s.AutoRoleIDs {
		if id != 0 && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) > MaxAutoRoles {
		return invalid("at most %d auto-roles", MaxAutoRoles)
	}
	s.AutoRoleIDs = ids
	return nil
}

// LoadSettings reads a server's welcome settings.
func LoadSettings(ctx context.Context, cfg *core.ConfigService, guildID snowflake.ID) (Settings, error) {
	raw, err := cfg.Settings(ctx, guildID, core.ModuleWelcome)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	return s, json.Unmarshal(raw, &s)
}

// ---- service -----------------------------------------------------------------

// Member is a member joining or leaving.
type Member struct {
	GuildID snowflake.ID
	UserID  snowflake.ID
	Name    string
	Bot     bool
	At      time.Time
}

// Gate is Guardian's view of a join (see guardian.JoinGate).
type Gate struct {
	Blocked      bool         // the account-age gate removes or quarantines them
	Raid         bool         // raid mode is on
	VerifyRoleID snowflake.ID // the verification gate's role (0 = no gate)
	Unknown      bool         // Guardian's state couldn't be read: no roles or DMs
}

// Platform talks to the chat platform.
type Platform interface {
	// Server returns the server's name and member count.
	Server(ctx context.Context, guildID snowflake.ID) (name string, members int)
	// Post sends a document to a channel; ping is the one user it may notify (0 = nobody).
	Post(ctx context.Context, channelID snowflake.ID, d message.Document, ping snowflake.ID) error
	// DM sends a document to a user.
	DM(ctx context.Context, userID snowflake.ID, d message.Document) error
	// Mention formats a user or channel mention.
	UserMention(id snowflake.ID) string
	ChannelMention(id snowflake.ID) string
	Escape(s string) string
	// Roles and AddRole give auto-roles (with roles.CheckRole).
	Roles(ctx context.Context, guildID snowflake.ID) ([]roles.RoleInfo, error)
	BotTopPosition(ctx context.Context, guildID snowflake.ID) (int, error)
	AddRole(ctx context.Context, guildID, userID, roleID snowflake.ID, reason string) error
}

// Deps are the service's collaborators.
type Deps struct {
	Settings func(ctx context.Context, guildID snowflake.ID) (s Settings, enabled bool, err error)
	Platform Platform
	Gate     func(ctx context.Context, guildID, userID snowflake.ID, at time.Time) Gate
	// RaidSummary posts "N members joined during raid mode".
	RaidSummary func(ctx context.Context, channelID snowflake.ID, guildID snowflake.ID, joins int) error
	Log         *slog.Logger
	// RaidFlush is how long joins during a raid are collected before one
	// summary is posted (default 1 minute).
	RaidFlush time.Duration
}

// Service runs the welcome module.
type Service struct {
	d     Deps
	mu    sync.Mutex
	raids map[snowflake.ID]*raidBatch
}

type raidBatch struct {
	joins   int
	channel snowflake.ID
}

func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.RaidFlush == 0 {
		d.RaidFlush = time.Minute
	}
	if d.Gate == nil {
		d.Gate = func(context.Context, snowflake.ID, snowflake.ID, time.Time) Gate { return Gate{} }
	}
	return &Service{d: d, raids: map[snowflake.ID]*raidBatch{}}
}

func (s *Service) vars(ctx context.Context, m Member, channelID snowflake.ID) message.Vars {
	name, count := s.d.Platform.Server(ctx, m.GuildID)
	v := message.Vars{
		UserMention: s.d.Platform.UserMention(m.UserID), UserName: m.Name, UserID: m.UserID.String(),
		Server: name, MemberCount: count,
	}
	if channelID != 0 {
		v.Channel = s.d.Platform.ChannelMention(channelID)
	}
	return v
}

// HandleJoin greets a new member and gives auto-roles.
func (s *Service) HandleJoin(ctx context.Context, m Member) error {
	st, on, err := s.d.Settings(ctx, m.GuildID)
	if err != nil || !on || m.Bot {
		return err
	}
	g := s.d.Gate(ctx, m.GuildID, m.UserID, m.At)
	if g.Blocked {
		return nil // Guardian removes or quarantines them: no welcome
	}
	if g.Raid {
		if st.WelcomeEnabled {
			s.countRaidJoin(m.GuildID, st.WelcomeChannelID)
		}
		return nil // no DMs or roles for accounts joining in a raid
	}
	var errs []error
	if st.WelcomeEnabled {
		ping := snowflake.ID(0)
		if st.PingMember {
			ping = m.UserID
		}
		d := st.WelcomeMessage.Render(s.vars(ctx, m, st.WelcomeChannelID), s.d.Platform.Escape)
		if err := s.d.Platform.Post(ctx, st.WelcomeChannelID, d, ping); err != nil {
			errs = append(errs, fmt.Errorf("posting welcome: %w", err))
		}
	}
	if st.DMEnabled && !g.Unknown {
		d := st.DMMessage.Render(s.vars(ctx, m, st.WelcomeChannelID), s.d.Platform.Escape)
		if err := s.d.Platform.DM(ctx, m.UserID, d); err != nil {
			s.d.Log.DebugContext(ctx, "welcome DM failed (DMs closed?)", "guild_id", m.GuildID, "err", err)
		}
	}
	if g.VerifyRoleID == 0 && !g.Unknown {
		if err := s.giveAutoRoles(ctx, st, m.GuildID, m.UserID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// HandleVerified gives auto-roles when a member passes Guardian's
// verification gate (instead of on join).
func (s *Service) HandleVerified(ctx context.Context, guildID, userID snowflake.ID) error {
	st, on, err := s.d.Settings(ctx, guildID)
	if err != nil || !on {
		return err
	}
	return s.giveAutoRoles(ctx, st, guildID, userID)
}

func (s *Service) giveAutoRoles(ctx context.Context, st Settings, guildID, userID snowflake.ID) error {
	if len(st.AutoRoleIDs) == 0 {
		return nil
	}
	list, err := s.d.Platform.Roles(ctx, guildID)
	if err != nil {
		return err
	}
	top, err := s.d.Platform.BotTopPosition(ctx, guildID)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range st.AutoRoleIDs {
		if err := roles.CheckRole(id, list, top); err != nil {
			s.d.Log.WarnContext(ctx, "auto-role skipped", "guild_id", guildID, "err", err)
			continue
		}
		if err := s.d.Platform.AddRole(ctx, guildID, userID, id, "Welcome: auto-role"); err != nil {
			errs = append(errs, fmt.Errorf("giving auto-role %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// HandleLeave says goodbye.
func (s *Service) HandleLeave(ctx context.Context, m Member) error {
	st, on, err := s.d.Settings(ctx, m.GuildID)
	if err != nil || !on || m.Bot || !st.GoodbyeEnabled {
		return err
	}
	if g := s.d.Gate(ctx, m.GuildID, m.UserID, m.At); g.Raid || g.Blocked {
		return nil // raiders and accounts the age gate removed get no goodbye
	}
	d := st.GoodbyeMessage.Render(s.vars(ctx, m, st.GoodbyeChannelID), s.d.Platform.Escape)
	return s.d.Platform.Post(ctx, st.GoodbyeChannelID, d, 0)
}

// countRaidJoin adds a join to the guild's raid batch; the first one starts
// the timer that posts the summary.
func (s *Service) countRaidJoin(guildID, channelID snowflake.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.raids[guildID]; ok {
		b.joins++
		return
	}
	s.raids[guildID] = &raidBatch{joins: 1, channel: channelID}
	time.AfterFunc(s.d.RaidFlush, func() { s.flushRaid(guildID) })
}

func (s *Service) flushRaid(guildID snowflake.ID) {
	s.mu.Lock()
	b := s.raids[guildID]
	delete(s.raids, guildID)
	s.mu.Unlock()
	if b == nil || s.d.RaidSummary == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.d.RaidSummary(ctx, b.channel, guildID, b.joins); err != nil {
		s.d.Log.Warn("posting raid join summary failed", "guild_id", guildID, "err", err)
	}
}
