package guardian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/moderation"
)

// SettingsLoader returns a server's Guardian settings and whether the module is on.
type SettingsLoader func(ctx context.Context, guildID snowflake.ID) (s Settings, enabled bool, err error)

// Deps are the service's collaborators.
type Deps struct {
	Store      Store
	Platform   Platform
	Alerts     Alerter
	Tracker    Tracker
	Moderation Moderation
	Blocklist  *DomainSet
	Settings   SettingsLoader
	// FallbackAlertChannel is used when Guardian has no alert channel of its
	// own (normally the moderation log channel).
	FallbackAlertChannel func(ctx context.Context, guildID snowflake.ID) snowflake.ID
	// OnEvent, if set, is told about every new event (the server log).
	OnEvent func(ctx context.Context, ev Event)
	Log     *slog.Logger
	Now     func() time.Time
}

// Service runs Guardian.
type Service struct{ d Deps }

func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Blocklist == nil {
		d.Blocklist = NewDomainSet()
	}
	return &Service{d: d}
}

// claimTTL guards against acting twice on one member while a burst is still
// arriving (several copies can be detected at once).
const claimTTL = 30 * time.Second

// HandleMessage checks one message and acts on the verdict. It returns the
// verdict for logging and tests.
func (s *Service) HandleMessage(ctx context.Context, m Message) (Verdict, error) {
	settings, on, err := s.d.Settings(ctx, m.GuildID)
	if err != nil || !on || exempt(m, settings) {
		return Verdict{}, err
	}
	now := m.CreatedAt
	if now.IsZero() {
		now = s.d.Now()
	}

	links := ExtractLinks(m.Content)
	fps := fingerprints(m, links)
	var h history
	if settings.BurstEnabled && len(fps) > 0 {
		window := time.Duration(settings.BurstWindowSeconds) * time.Second
		h.recent, err = s.d.Tracker.RecordMessage(ctx, m.GuildID, m.AuthorID,
			RecentEntry{ChannelID: m.ChannelID, MessageID: m.ID, Fingerprints: fps, At: now}, window)
		if err != nil {
			return Verdict{}, fmt.Errorf("recording message: %w", err)
		}
	}
	if settings.QuietDays > 0 {
		h.quiet, err = s.wasQuiet(ctx, m, settings, now)
		if err != nil {
			s.d.Log.WarnContext(ctx, "quiet-account check failed", "err", err)
		}
	}

	v := analyze(m, settings, links, fps, h, s.d.Blocklist)
	if v.Response == ResponseNone {
		return v, nil
	}
	return v, s.act(ctx, settings, m, v)
}

func (s *Service) wasQuiet(ctx context.Context, m Message, settings Settings, now time.Time) (bool, error) {
	quiet := time.Duration(settings.QuietDays) * 24 * time.Hour
	prev, err := s.d.Tracker.MarkActive(ctx, m.GuildID, m.AuthorID, now, quiet)
	if err != nil {
		return false, err
	}
	if !prev.IsZero() || m.AuthorJoinedAt.IsZero() || now.Sub(m.AuthorJoinedAt) < quiet {
		return false, nil
	}
	// Only trust "no activity seen" once Guardian has watched long enough.
	since, err := s.d.Tracker.WatchingSince(ctx, m.GuildID, now)
	if err != nil {
		return false, err
	}
	return now.Sub(since) >= quiet, nil
}

func (s *Service) act(ctx context.Context, settings Settings, m Message, v Verdict) error {
	monitor := settings.Mode == ModeMonitor
	ev := NewEvent{
		GuildID: m.GuildID, UserID: m.AuthorID, UserName: m.AuthorName, Kind: v.Kind,
		Signals: v.Signals, Score: v.Score, Response: v.Response, Status: StatusInfo,
		Evidence: Evidence{
			Fingerprint: v.Fingerprint, Channels: v.Channels, Messages: len(v.Copies),
			Domains: v.Domains, Monitor: monitor,
		},
	}
	if monitor {
		return s.record(ctx, settings, ev)
	}

	switch v.Response {
	case ResponseAlert:
		return s.record(ctx, settings, ev)
	case ResponseDelete:
		ev.Evidence.Deleted = s.deleteCopies(ctx, v.Copies, &ev.Evidence)
		return s.record(ctx, settings, ev)
	case ResponseQuarantine:
		// Only the first detection in a burst quarantines; the others just
		// delete their own copy.
		ok, err := s.d.Tracker.Claim(ctx, fmt.Sprintf("quarantine:%s:%s", m.GuildID, m.AuthorID), claimTTL)
		if err != nil {
			s.d.Log.WarnContext(ctx, "claiming quarantine lock failed", "err", err)
			ok = true
		}
		if !ok {
			s.deleteCopies(ctx, v.Copies, &ev.Evidence)
			return nil
		}
		ev.Evidence.Deleted = s.deleteCopies(ctx, v.Copies, &ev.Evidence)
		return s.quarantine(ctx, settings, ev, summarize(v.Signals))
	}
	return nil
}

// deleteCopies deletes messages, grouped per channel and in parallel (each
// channel has its own rate limit). It returns how many were deleted.
func (s *Service) deleteCopies(ctx context.Context, refs []MessageRef, ev *Evidence) int {
	byChannel := map[snowflake.ID][]snowflake.ID{}
	for _, r := range refs {
		byChannel[r.ChannelID] = append(byChannel[r.ChannelID], r.MessageID)
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		deleted int
	)
	for ch, ids := range byChannel {
		wg.Go(func() {
			err := s.d.Platform.DeleteMessages(ctx, ch, ids, "Guardian: suspected scam or hacked account")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				ev.Problems = appendProblem(ev.Problems, "delete_failed")
				s.d.Log.WarnContext(ctx, "deleting flagged messages failed", "channel_id", ch, "err", err)
				return
			}
			deleted += len(ids)
		})
	}
	wg.Wait()
	return deleted
}

func appendProblem(list []string, p string) []string {
	for _, x := range list {
		if x == p {
			return list
		}
	}
	return append(list, p)
}

// summarize is the case reason for an automatic action.
func summarize(signals []Signal) string {
	parts := make([]string, 0, len(signals))
	for _, sig := range signals {
		p := string(sig.Kind)
		if sig.Detail != "" {
			p += " (" + sig.Detail + ")"
		}
		parts = append(parts, p)
	}
	return "Guardian: " + strings.Join(parts, ", ")
}

// quarantine restricts a member, records the case and the event, DMs them
// recovery steps and alerts moderators.
func (s *Service) quarantine(ctx context.Context, settings Settings, ev NewEvent, reason string) error {
	g, u := ev.GuildID, ev.UserID
	ev.Response = ResponseQuarantine
	member, err := s.d.Platform.Member(ctx, g, u)
	if errors.Is(err, moderation.ErrNotMember) {
		ev.Evidence.Problems = appendProblem(ev.Evidence.Problems, "left_server")
		return s.record(ctx, settings, ev)
	}
	if err != nil {
		return fmt.Errorf("looking up member: %w", err)
	}
	bot, err := s.d.Platform.BotMember(ctx, g)
	if err != nil {
		return fmt.Errorf("looking up bot member: %w", err)
	}
	if ev.UserName == "" {
		ev.UserName = member.Name
	}
	if err := core.CanActOn(bot.Rank, member.Rank, bot.Rank); err != nil {
		ev.Evidence.Problems = appendProblem(ev.Evidence.Problems, problemFor(err))
		return s.record(ctx, settings, ev)
	}

	now := s.d.Now()
	q := Quarantine{GuildID: g, UserID: u, Mode: settings.QuarantineMode, CreatedAt: now}
	hours := time.Duration(settings.QuarantineHours) * time.Hour
	switch settings.QuarantineMode {
	case QuarantineRole:
		q.RoleID = settings.QuarantineRoleID
		for _, r := range member.RoleIDs {
			if r != q.RoleID {
				q.SavedRoleIDs = append(q.SavedRoleIDs, r)
			}
		}
		err = s.d.Platform.SetRoles(ctx, g, u, []snowflake.ID{q.RoleID}, reason)
	default:
		until := now.Add(min(hours, moderation.MaxTimeout))
		q.Until = &until
		err = s.d.Platform.Timeout(ctx, g, u, until, reason)
	}
	if err != nil {
		s.d.Log.WarnContext(ctx, "quarantine action failed", "guild_id", g, "user_id", u, "err", err)
		ev.Evidence.Problems = appendProblem(ev.Evidence.Problems, "restrict_failed")
		return s.record(ctx, settings, ev)
	}

	c, err := s.d.Moderation.Record(ctx, moderation.NewCase{
		GuildID: g, Action: moderation.ActionQuarantine, TargetID: u, TargetName: ev.UserName,
		ModeratorID: bot.ID, ModeratorName: bot.Name, Reason: truncate(reason, moderation.MaxReasonLength),
		Duration: durationIf(settings.QuarantineMode == QuarantineTimeout, hours), ExpiresAt: q.Until,
		Source: moderation.SourceGuardian,
	})
	if err != nil {
		s.d.Log.ErrorContext(ctx, "recording quarantine case failed", "err", err)
	}
	ev.CaseNumber = c.Number
	ev.Status = StatusOpen

	event, err := s.d.Store.CreateEvent(ctx, ev)
	if err != nil {
		return fmt.Errorf("recording event: %w", err)
	}
	q.EventID = event.ID
	if err := s.d.Store.SaveQuarantine(ctx, q); err != nil {
		s.d.Log.ErrorContext(ctx, "saving quarantine failed; restore will need a manual fix", "err", err)
	}
	if settings.DMUser {
		if err := s.d.Alerts.RecoveryDM(ctx, event, s.d.Platform.GuildName(g)); err != nil {
			s.d.Log.InfoContext(ctx, "could not DM quarantined member", "user_id", u, "err", err)
		}
	}
	s.alert(ctx, settings, event)
	return nil
}

func durationIf(ok bool, d time.Duration) time.Duration {
	if ok {
		return d
	}
	return 0
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func problemFor(err error) string {
	switch {
	case errors.Is(err, core.ErrTargetIsOwner):
		return "target_is_owner"
	case errors.Is(err, core.ErrBotRankTooLow), errors.Is(err, core.ErrActorRankTooLow):
		return "bot_role_too_low"
	case errors.Is(err, core.ErrTargetIsBot), errors.Is(err, core.ErrTargetIsSelf):
		return "target_is_bot"
	}
	return "restrict_failed"
}

// record stores an event that needs no review and alerts about it.
func (s *Service) record(ctx context.Context, settings Settings, ev NewEvent) error {
	event, err := s.d.Store.CreateEvent(ctx, ev)
	if err != nil {
		return fmt.Errorf("recording event: %w", err)
	}
	s.alert(ctx, settings, event)
	return nil
}

func (s *Service) alertChannel(ctx context.Context, settings Settings, guildID snowflake.ID) snowflake.ID {
	if settings.AlertChannelID != 0 {
		return settings.AlertChannelID
	}
	if s.d.FallbackAlertChannel != nil {
		return s.d.FallbackAlertChannel(ctx, guildID)
	}
	return 0
}

// AlertChannel is where a server's Guardian alerts go (0 if nowhere).
func (s *Service) AlertChannel(ctx context.Context, guildID snowflake.ID) snowflake.ID {
	settings, on, err := s.d.Settings(ctx, guildID)
	if err != nil || !on {
		return 0
	}
	return s.alertChannel(ctx, settings, guildID)
}

func (s *Service) alert(ctx context.Context, settings Settings, ev Event) {
	if s.d.OnEvent != nil {
		s.d.OnEvent(ctx, ev)
	}
	ch := s.alertChannel(ctx, settings, ev.GuildID)
	if ch == 0 {
		return
	}
	msgID, err := s.d.Alerts.PostAlert(ctx, ch, ev)
	if err != nil {
		s.d.Log.WarnContext(ctx, "posting guardian alert failed", "guild_id", ev.GuildID, "channel_id", ch, "err", err)
		return
	}
	if err := s.d.Store.SetEventAlert(ctx, ev.GuildID, ev.ID, ch, msgID); err != nil {
		s.d.Log.WarnContext(ctx, "saving alert reference failed", "err", err)
	}
}

func (s *Service) refreshAlert(ctx context.Context, ev Event) {
	if ev.AlertMessageID == 0 {
		return
	}
	if err := s.d.Alerts.EditAlert(ctx, ev.AlertChannelID, ev.AlertMessageID, ev); err != nil {
		s.d.Log.WarnContext(ctx, "updating guardian alert failed", "event_id", ev.ID, "err", err)
	}
}

// Resolve applies a moderator's decision to a quarantine event.
func (s *Service) Resolve(ctx context.Context, guildID snowflake.ID, eventID int64, r Resolution, actor moderation.Actor) (Event, error) {
	ev, err := s.d.Store.GetEvent(ctx, guildID, eventID)
	if err != nil {
		return Event{}, err
	}
	if ev.Status != StatusOpen && ev.Status != StatusKept {
		return ev, ErrAlreadyResolved
	}
	if ev.Response != ResponseQuarantine || ev.UserID == 0 {
		return ev, ErrBadResolution
	}

	var status Status
	switch r {
	case ResolveKeep:
		if ev.Status == StatusKept {
			return ev, ErrAlreadyResolved
		}
		status = StatusKept
	case ResolveRestore:
		if err := s.restore(ctx, ev, actor); err != nil {
			return ev, err
		}
		status = StatusRestored
	case ResolveBan:
		_, err := s.d.Moderation.Execute(ctx, moderation.Request{
			GuildID: guildID, GuildName: s.d.Platform.GuildName(guildID), Action: moderation.ActionBan,
			Actor: actor, TargetID: ev.UserID, TargetName: ev.UserName,
			Reason: fmt.Sprintf("Guardian event #%d confirmed", ev.ID), DeleteMessages: 24 * time.Hour,
			Source: moderation.SourceGuardian,
		})
		if err != nil {
			return ev, err
		}
		if err := s.d.Store.ReleaseQuarantine(ctx, guildID, ev.UserID); err != nil && !errors.Is(err, ErrNotQuarantined) {
			s.d.Log.WarnContext(ctx, "releasing quarantine record failed", "err", err)
		}
		status = StatusBanned
	default:
		return ev, ErrBadResolution
	}

	ev, err = s.d.Store.ResolveEvent(ctx, guildID, eventID, status, actor.ID, actor.Name)
	if err != nil {
		return ev, err
	}
	s.refreshAlert(ctx, ev)
	return ev, nil
}

// restore undoes a quarantine.
func (s *Service) restore(ctx context.Context, ev Event, actor moderation.Actor) error {
	g, u := ev.GuildID, ev.UserID
	q, err := s.d.Store.ActiveQuarantine(ctx, g, u)
	if errors.Is(err, ErrNotQuarantined) {
		// Nothing saved (e.g. lifted by hand): just clear a timeout if any.
		q = Quarantine{GuildID: g, UserID: u, Mode: QuarantineTimeout}
	} else if err != nil {
		return err
	}
	reason := fmt.Sprintf("%s: restored after Guardian event #%d", actor.Name, ev.ID)

	member, err := s.d.Platform.Member(ctx, g, u)
	switch {
	case errors.Is(err, moderation.ErrNotMember):
		// They left; nothing to undo on the platform.
	case err != nil:
		return err
	case q.Mode == QuarantineRole:
		if err := s.d.Platform.SetRoles(ctx, g, u, q.SavedRoleIDs, reason); err != nil {
			return &moderation.ActionError{Action: moderation.ActionUnquarantine, Err: err}
		}
	default:
		if member.TimedOutUntil != nil && member.TimedOutUntil.After(s.d.Now()) {
			if err := s.d.Platform.RemoveTimeout(ctx, g, u, reason); err != nil {
				return &moderation.ActionError{Action: moderation.ActionUnquarantine, Err: err}
			}
		}
	}
	if err := s.d.Store.ReleaseQuarantine(ctx, g, u); err != nil && !errors.Is(err, ErrNotQuarantined) {
		s.d.Log.WarnContext(ctx, "releasing quarantine record failed", "err", err)
	}
	if _, err := s.d.Moderation.Record(ctx, moderation.NewCase{
		GuildID: g, Action: moderation.ActionUnquarantine, TargetID: u, TargetName: ev.UserName,
		ModeratorID: actor.ID, ModeratorName: actor.Name, Source: moderation.SourceGuardian,
		Reason: fmt.Sprintf("False positive (Guardian event #%d)", ev.ID),
	}); err != nil {
		s.d.Log.ErrorContext(ctx, "recording restore case failed", "err", err)
	}
	return nil
}

// GetEvent and ListEvents serve the dashboard and commands.
func (s *Service) GetEvent(ctx context.Context, guildID snowflake.ID, id int64) (Event, error) {
	return s.d.Store.GetEvent(ctx, guildID, id)
}

func (s *Service) ListEvents(ctx context.Context, guildID snowflake.ID, f EventFilter) ([]Event, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 25
	}
	return s.d.Store.ListEvents(ctx, guildID, f)
}

// Retention is how long Guardian events are kept (plan §5).
const Retention = 90 * 24 * time.Hour

// RunMaintenance ends expired raids and deletes old events until ctx is done.
func (s *Service) RunMaintenance(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	lastPurge := time.Time{}
	for {
		s.endDueRaids(ctx)
		if s.d.Now().Sub(lastPurge) > 6*time.Hour {
			if n, err := s.d.Store.PurgeEventsBefore(ctx, s.d.Now().Add(-Retention)); err != nil {
				s.d.Log.WarnContext(ctx, "purging old guardian events failed", "err", err)
			} else if n > 0 {
				s.d.Log.InfoContext(ctx, "purged old guardian events", "count", n)
			}
			lastPurge = s.d.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
