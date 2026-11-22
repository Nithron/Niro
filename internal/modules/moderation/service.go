package moderation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
)

// SettingsLoader returns a server's moderation settings.
type SettingsLoader func(ctx context.Context, guildID snowflake.ID) (Settings, error)

// Service runs moderation actions and manages cases.
type Service struct {
	store    CaseStore
	platform Platform
	notifier Notifier
	settings SettingsLoader
	log      *slog.Logger
	now      func() time.Time
}

func NewService(store CaseStore, platform Platform, notifier Notifier, settings SettingsLoader, log *slog.Logger) *Service {
	return &Service{store: store, platform: platform, notifier: notifier, settings: settings, log: log, now: time.Now}
}

// Actor is who performs an action.
type Actor struct {
	core.Rank
	Name string
}

// Request asks for one moderation action against one user.
type Request struct {
	GuildID   snowflake.ID
	GuildName string
	Action    Action
	Actor     Actor
	TargetID  snowflake.ID
	// TargetName is used when the target is not a member (e.g. ban by ID).
	TargetName string
	Reason     string
	// Duration: required for timeouts; optional for bans (temporary ban).
	Duration time.Duration
	// DeleteMessages: for bans, how much message history to delete (≤ 7 days).
	DeleteMessages time.Duration
	// Notify overrides the server's "DM users" setting when non-nil.
	Notify *bool
	Source Source
}

// Outcome reports what happened besides the case itself.
type Outcome struct {
	Case      Case
	DMSent    bool
	LogPosted bool
}

// Execute validates and performs an action, records the case, notifies the
// user and posts to the moderation log. The case is only recorded if the
// action itself succeeded; DM and log failures are logged, not fatal.
func (s *Service) Execute(ctx context.Context, req Request) (Outcome, error) {
	settings, err := s.settings(ctx, req.GuildID)
	if err != nil {
		return Outcome{}, fmt.Errorf("loading settings: %w", err)
	}
	if err := validate(req, settings); err != nil {
		return Outcome{}, err
	}

	targetName := req.TargetName
	var target *Member
	switch m, err := s.platform.Member(ctx, req.GuildID, req.TargetID); {
	case err == nil:
		target, targetName = &m, m.Name
	case errors.Is(err, ErrNotMember):
		if needsMembership(req.Action) {
			return Outcome{}, ErrNotMember
		}
	default:
		return Outcome{}, fmt.Errorf("looking up member: %w", err)
	}

	if target != nil && checksHierarchy(req.Action) {
		bot, err := s.platform.BotMember(ctx, req.GuildID)
		if err != nil {
			return Outcome{}, fmt.Errorf("looking up bot member: %w", err)
		}
		err = core.CanActOn(req.Actor.Rank, target.Rank, bot.Rank)
		// A warning needs no power over the target, only the moderator's rank.
		if req.Action == ActionWarn && errors.Is(err, core.ErrBotRankTooLow) {
			err = nil
		}
		if err != nil {
			return Outcome{}, err
		}
	}
	if req.Action == ActionUntimeout && (target.TimedOutUntil == nil || !target.TimedOutUntil.After(s.now())) {
		return Outcome{}, ErrNotTimedOut
	}

	notify := settings.DMUsers
	if req.Notify != nil {
		notify = *req.Notify
	}
	notify = notify && target != nil && !target.IsBot && dmsAbout(req.Action)

	now := s.now()
	nc := NewCase{
		GuildID: req.GuildID, Action: req.Action, TargetID: req.TargetID, TargetName: targetName,
		ModeratorID: req.Actor.ID, ModeratorName: req.Actor.Name, Reason: req.Reason,
		Duration: req.Duration, Source: req.Source,
	}
	if nc.Source == "" {
		nc.Source = SourceManual
	}
	if req.Duration > 0 && (req.Action == ActionTimeout || req.Action == ActionBan) {
		exp := now.Add(req.Duration)
		nc.ExpiresAt = &exp
	}
	if req.Action == ActionBan {
		nc.Details.DeleteMessages = int(req.DeleteMessages.Seconds())
		if req.Duration > 0 {
			nc.Details.Temporary = true
			nc.ScheduleUnbanAt = nc.ExpiresAt
		}
	}

	// The user can't receive a DM from a server they were removed from, so
	// kicks and bans notify first. Everything else notifies after.
	preview := previewCase(nc, now)
	dmSent := false
	if notify && (req.Action == ActionKick || req.Action == ActionBan) {
		dmSent = s.notify(ctx, preview, req.GuildName)
	}

	if err := s.perform(ctx, req, auditReason(req)); err != nil {
		return Outcome{}, &ActionError{Action: req.Action, Err: err}
	}

	if notify && !(req.Action == ActionKick || req.Action == ActionBan) {
		dmSent = s.notify(ctx, preview, req.GuildName)
	}
	nc.DMSent = dmSent

	if req.Action == ActionUnban {
		if err := s.store.CancelScheduledUnbans(ctx, req.GuildID, req.TargetID); err != nil {
			s.log.WarnContext(ctx, "cancelling scheduled unbans failed", "err", err)
		}
	}

	c, err := s.store.CreateCase(ctx, nc)
	if err != nil {
		// The action happened; losing the record is bad but must not hide that.
		return Outcome{}, fmt.Errorf("action performed but recording the case failed: %w", err)
	}
	out := Outcome{Case: c, DMSent: dmSent}
	out.LogPosted = s.postLog(ctx, settings, &out.Case)
	return out, nil
}

func (s *Service) perform(ctx context.Context, req Request, reason string) error {
	p, g, u := s.platform, req.GuildID, req.TargetID
	switch req.Action {
	case ActionWarn, ActionNote:
		return nil // record only
	case ActionTimeout:
		return p.Timeout(ctx, g, u, s.now().Add(req.Duration), reason)
	case ActionUntimeout:
		return p.RemoveTimeout(ctx, g, u, reason)
	case ActionKick:
		return p.Kick(ctx, g, u, reason)
	case ActionBan:
		return p.Ban(ctx, g, u, req.DeleteMessages, reason)
	case ActionUnban:
		return p.Unban(ctx, g, u, reason)
	}
	return ErrBadAction
}

func (s *Service) notify(ctx context.Context, c Case, guildName string) bool {
	if err := s.notifier.NotifyUser(ctx, c, guildName); err != nil {
		// Common and expected: the user has DMs from server members turned off.
		s.log.InfoContext(ctx, "could not DM user about case", "guild_id", c.GuildID, "user_id", c.TargetID, "err", err)
		return false
	}
	return true
}

func (s *Service) postLog(ctx context.Context, settings Settings, c *Case) bool {
	if settings.LogChannelID == 0 {
		return false
	}
	msgID, err := s.notifier.PostLog(ctx, settings.LogChannelID, *c)
	if err != nil {
		s.log.WarnContext(ctx, "posting to the moderation log failed", "guild_id", c.GuildID, "channel_id", settings.LogChannelID, "err", err)
		return false
	}
	c.LogChannelID, c.LogMessageID = settings.LogChannelID, msgID
	if err := s.store.SetCaseLogMessage(ctx, c.GuildID, c.Number, settings.LogChannelID, msgID); err != nil {
		s.log.WarnContext(ctx, "saving the log message reference failed", "err", err)
	}
	return true
}

// UpdateReason changes a case's reason and refreshes its log message.
func (s *Service) UpdateReason(ctx context.Context, guildID snowflake.ID, number int, reason string) (Case, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > MaxReasonLength {
		return Case{}, ErrReasonTooLong
	}
	c, err := s.store.UpdateCaseReason(ctx, guildID, number, reason)
	if err != nil {
		return Case{}, err
	}
	s.refreshLog(ctx, c)
	return c, nil
}

// DeleteCase hides a case from the history (kept for the audit trail) and
// marks its log message as deleted.
func (s *Service) DeleteCase(ctx context.Context, guildID snowflake.ID, number int, by snowflake.ID) (Case, error) {
	c, err := s.store.DeleteCase(ctx, guildID, number, by)
	if err != nil {
		return Case{}, err
	}
	s.refreshLog(ctx, c)
	return c, nil
}

func (s *Service) refreshLog(ctx context.Context, c Case) {
	if c.LogMessageID == 0 {
		return
	}
	if err := s.notifier.EditLog(ctx, c.LogChannelID, c.LogMessageID, c); err != nil {
		s.log.WarnContext(ctx, "updating the log message failed", "guild_id", c.GuildID, "case", c.Number, "err", err)
	}
}

func (s *Service) GetCase(ctx context.Context, guildID snowflake.ID, number int) (Case, error) {
	return s.store.GetCase(ctx, guildID, number)
}

// ListCases returns matching cases (newest first) and the total count.
func (s *Service) ListCases(ctx context.Context, guildID snowflake.ID, f CaseFilter) ([]Case, int, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 25
	}
	cases, err := s.store.ListCases(ctx, guildID, f)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountCases(ctx, guildID, f)
	return cases, total, err
}

func validate(req Request, settings Settings) error {
	if !req.Action.Valid() || req.Action == ActionPurge {
		return ErrBadAction
	}
	reason := strings.TrimSpace(req.Reason)
	if utf8.RuneCountInString(reason) > MaxReasonLength {
		return ErrReasonTooLong
	}
	if reason == "" && (settings.RequireReason || req.Action == ActionNote) {
		return ErrReasonRequired
	}
	switch req.Action {
	case ActionTimeout:
		if req.Duration < MinTimeout || req.Duration > MaxTimeout {
			return ErrBadDuration
		}
	case ActionBan:
		if req.Duration != 0 && (req.Duration < MinTempBan || req.Duration > MaxTempBan) {
			return ErrBadDuration
		}
		if req.DeleteMessages < 0 || req.DeleteMessages > MaxBanDeleteAge {
			return ErrBadDuration
		}
	}
	return nil
}

func needsMembership(a Action) bool {
	return a == ActionWarn || a == ActionTimeout || a == ActionUntimeout || a == ActionKick
}

func checksHierarchy(a Action) bool {
	return a == ActionWarn || a == ActionTimeout || a == ActionUntimeout || a == ActionKick || a == ActionBan
}

func dmsAbout(a Action) bool {
	return a == ActionWarn || a == ActionTimeout || a == ActionKick || a == ActionBan
}

// auditReason is what appears in Discord's audit log.
func auditReason(req Request) string {
	r := req.Actor.Name + ": " + strings.TrimSpace(req.Reason)
	if strings.TrimSpace(req.Reason) == "" {
		r = req.Actor.Name + " (no reason given)"
	}
	if len(r) > 512 {
		r = r[:512]
	}
	return r
}

// previewCase is the case as the DM sees it (before the number is assigned).
func previewCase(nc NewCase, now time.Time) Case {
	return Case{
		GuildID: nc.GuildID, Action: nc.Action, TargetID: nc.TargetID, TargetName: nc.TargetName,
		ModeratorID: nc.ModeratorID, ModeratorName: nc.ModeratorName, Reason: nc.Reason,
		Duration: nc.Duration, ExpiresAt: nc.ExpiresAt, Source: nc.Source, Details: nc.Details, CreatedAt: now,
	}
}
