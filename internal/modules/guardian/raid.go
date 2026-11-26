package guardian

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/moderation"
)

// Join is a member joining a server.
type Join struct {
	GuildID  snowflake.ID
	UserID   snowflake.ID
	UserName string
	Bot      bool
	At       time.Time
}

// maxSlowmodeChannels caps how many channels raid mode touches.
const maxSlowmodeChannels = 50

// maxInvitePause is Discord's limit for pausing invites.
const maxInvitePause = 24 * time.Hour

// HandleJoin runs raid detection and the account-age gate for one join.
func (s *Service) HandleJoin(ctx context.Context, j Join) error {
	settings, on, err := s.d.Settings(ctx, j.GuildID)
	if err != nil || !on || j.Bot {
		return err
	}
	at := j.At
	if at.IsZero() {
		at = s.d.Now()
	}

	if settings.RaidEnabled {
		window := time.Duration(settings.RaidWindowSeconds) * time.Second
		n, err := s.d.Tracker.RecordJoin(ctx, j.GuildID, j.UserID, at, window)
		if err != nil {
			return fmt.Errorf("counting joins: %w", err)
		}
		if n >= settings.RaidJoins {
			if _, err := s.startRaid(ctx, settings, j.GuildID, n, nil); err != nil && !errors.Is(err, ErrRaidActive) {
				s.d.Log.ErrorContext(ctx, "starting raid mode failed", "guild_id", j.GuildID, "err", err)
			}
		}
	}

	if settings.MinAccountAgeHours > 0 {
		minAge := time.Duration(settings.MinAccountAgeHours) * time.Hour
		if age := AccountAge(j.UserID, at); age < minAge {
			return s.accountAgeGate(ctx, settings, j, age)
		}
	}
	return nil
}

// JoinGate is how Guardian treats a new member, for other modules (the
// welcome module): whether the account-age gate removes or quarantines
// them, whether raid mode is on, and the verified role of the verification
// gate (0 if it isn't set up).
type JoinGate struct {
	Blocked      bool
	Raid         bool
	VerifyRoleID snowflake.ID
}

// JoinGate reports how Guardian treats userID joining guildID at at.
func (s *Service) JoinGate(ctx context.Context, guildID, userID snowflake.ID, at time.Time) JoinGate {
	settings, on, err := s.d.Settings(ctx, guildID)
	if err != nil || !on {
		return JoinGate{}
	}
	g := JoinGate{VerifyRoleID: settings.VerifyRoleID}
	if settings.MinAccountAgeHours > 0 && settings.Mode == ModeEnforce && settings.AccountAgeAction != AgeAlert {
		g.Blocked = AccountAge(userID, at) < time.Duration(settings.MinAccountAgeHours)*time.Hour
	}
	if _, err := s.d.Store.ActiveRaid(ctx, guildID); err == nil {
		g.Raid = true
	}
	return g
}

func (s *Service) accountAgeGate(ctx context.Context, settings Settings, j Join, age time.Duration) error {
	ev := NewEvent{
		GuildID: j.GuildID, UserID: j.UserID, UserName: j.UserName, Kind: KindAccountAge, Score: 1,
		Signals:  []Signal{{Kind: SignalNewAccount, Detail: moderation.FormatDuration(age.Truncate(time.Minute)), Weight: 1}},
		Response: ResponseAlert, Status: StatusInfo,
		Evidence: Evidence{AccountAge: moderation.FormatDuration(age.Truncate(time.Minute)), Monitor: settings.Mode == ModeMonitor},
	}
	if settings.Mode == ModeMonitor {
		return s.record(ctx, settings, ev)
	}
	reason := fmt.Sprintf("Guardian: account younger than %dh", settings.MinAccountAgeHours)
	switch settings.AccountAgeAction {
	case AgeKick:
		ev.Response = ResponseKick
		if err := s.d.Platform.Kick(ctx, j.GuildID, j.UserID, reason); err != nil {
			ev.Evidence.Problems = appendProblem(ev.Evidence.Problems, "kick_failed")
		} else if bot, err := s.d.Platform.BotMember(ctx, j.GuildID); err == nil {
			c, err := s.d.Moderation.Record(ctx, moderation.NewCase{
				GuildID: j.GuildID, Action: moderation.ActionKick, TargetID: j.UserID, TargetName: j.UserName,
				ModeratorID: bot.ID, ModeratorName: bot.Name, Reason: reason, Source: moderation.SourceGuardian,
			})
			if err == nil {
				ev.CaseNumber = c.Number
			}
		}
		return s.record(ctx, settings, ev)
	case AgeQuarantine:
		return s.quarantine(ctx, settings, ev, reason)
	}
	return s.record(ctx, settings, ev)
}

// StartRaid turns raid mode on by hand.
func (s *Service) StartRaid(ctx context.Context, guildID snowflake.ID, actor moderation.Actor) (Raid, error) {
	settings, on, err := s.d.Settings(ctx, guildID)
	if err != nil {
		return Raid{}, err
	}
	if !on {
		return Raid{}, ErrModuleOff
	}
	return s.startRaid(ctx, settings, guildID, 0, &actor)
}

func (s *Service) startRaid(ctx context.Context, settings Settings, guildID snowflake.ID, joins int, by *moderation.Actor) (Raid, error) {
	now := s.d.Now()
	manual := by != nil
	if settings.Mode == ModeMonitor && !manual {
		// Alert once per raid window instead of acting.
		if ok, err := s.d.Tracker.Claim(ctx, "raid-alert:"+guildID.String(), 10*time.Minute); err != nil || !ok {
			return Raid{}, ErrRaidActive
		}
		return Raid{}, s.record(ctx, settings, NewEvent{
			GuildID: guildID, Kind: KindRaid, Response: ResponseRaidMode, Status: StatusInfo, Score: 3,
			Signals:  []Signal{{Kind: SignalJoinFlood, Detail: fmt.Sprintf("%d/%ds", joins, settings.RaidWindowSeconds), Weight: 3}},
			Evidence: Evidence{Joins: joins, Monitor: true},
		})
	}

	r := Raid{
		GuildID: guildID, StartedAt: now, Manual: manual, Joins: joins,
		EndsAt: now.Add(time.Duration(settings.RaidDurationMinutes) * time.Minute),
	}
	if by != nil {
		r.StartedBy, r.StartedName = by.ID, by.Name
	}
	r, started, err := s.d.Store.StartRaid(ctx, r)
	if err != nil {
		return Raid{}, err
	}
	if !started {
		return r, ErrRaidActive
	}

	reason := "Guardian: raid mode"
	st := RaidState{}
	if settings.RaidPauseInvites {
		until := r.EndsAt
		if until.Sub(now) > maxInvitePause {
			until = now.Add(maxInvitePause)
		}
		if err := s.d.Platform.PauseInvites(ctx, guildID, until); err != nil {
			st.Problems = appendProblem(st.Problems, "pause_invites_failed")
			s.d.Log.WarnContext(ctx, "pausing invites failed", "guild_id", guildID, "err", err)
		} else {
			st.InvitesPaused = true
		}
	}
	if settings.RaidRaiseVerification {
		if lvl, err := s.d.Platform.VerificationLevel(ctx, guildID); err != nil {
			st.Problems = appendProblem(st.Problems, "verification_failed")
		} else if lvl < VerificationHigh {
			if err := s.d.Platform.SetVerificationLevel(ctx, guildID, VerificationHigh, reason); err != nil {
				st.Problems = appendProblem(st.Problems, "verification_failed")
				s.d.Log.WarnContext(ctx, "raising verification level failed", "guild_id", guildID, "err", err)
			} else {
				st.PrevVerification, st.SetVerification = &lvl, VerificationHigh
			}
		}
	}
	if settings.RaidSlowmodeSeconds > 0 {
		s.applySlowmode(ctx, guildID, settings.RaidSlowmodeSeconds, reason, &st)
	}

	signal := Signal{Kind: SignalJoinFlood, Detail: fmt.Sprintf("%d/%ds", joins, settings.RaidWindowSeconds), Weight: 3}
	if manual {
		signal = Signal{Kind: SignalManual, Detail: by.Name, Weight: 3}
	}
	ev, err := s.d.Store.CreateEvent(ctx, NewEvent{
		GuildID: guildID, Kind: KindRaid, Response: ResponseRaidMode, Status: StatusActive, Score: 3,
		Signals: []Signal{signal}, Evidence: Evidence{Joins: joins, Problems: st.Problems},
	})
	if err != nil {
		s.d.Log.ErrorContext(ctx, "recording raid event failed", "err", err)
	}
	r.EventID, r.State = ev.ID, st
	if err := s.d.Store.UpdateRaid(ctx, guildID, r.ID, ev.ID, st); err != nil {
		s.d.Log.ErrorContext(ctx, "saving raid state failed; it may not be fully undone", "err", err)
	}
	s.d.Log.InfoContext(ctx, "raid mode on", "guild_id", guildID, "joins", joins, "manual", manual)
	if ev.ID != 0 {
		s.alert(ctx, settings, ev)
	}
	return r, nil
}

func (s *Service) applySlowmode(ctx context.Context, guildID snowflake.ID, seconds int, reason string, st *RaidState) {
	chans, err := s.d.Platform.TextChannels(ctx, guildID)
	if err != nil {
		st.Problems = appendProblem(st.Problems, "slowmode_failed")
		return
	}
	st.PrevSlowmodes, st.SetSlowmode = map[snowflake.ID]int{}, seconds
	for i, ch := range chans {
		if i >= maxSlowmodeChannels {
			break
		}
		if ch.Slowmode >= seconds {
			continue
		}
		if err := s.d.Platform.SetSlowmode(ctx, ch.ID, seconds, reason); err != nil {
			st.Problems = appendProblem(st.Problems, "slowmode_failed")
			continue
		}
		st.PrevSlowmodes[ch.ID] = ch.Slowmode
	}
}

// EndRaid turns raid mode off and undoes what it changed. by is 0 when it
// ends on its own.
func (s *Service) EndRaid(ctx context.Context, guildID snowflake.ID, by moderation.Actor) (Raid, error) {
	r, err := s.d.Store.EndRaid(ctx, guildID, by.ID)
	if err != nil {
		return Raid{}, err
	}
	st := r.State
	reason := "Guardian: raid mode ended"
	if st.InvitesPaused {
		if err := s.d.Platform.PauseInvites(ctx, guildID, time.Time{}); err != nil {
			s.d.Log.WarnContext(ctx, "resuming invites failed", "guild_id", guildID, "err", err)
		}
	}
	if st.PrevVerification != nil {
		// Only undo our own change: leave it if an admin changed it since.
		if lvl, err := s.d.Platform.VerificationLevel(ctx, guildID); err == nil && lvl == st.SetVerification {
			if err := s.d.Platform.SetVerificationLevel(ctx, guildID, *st.PrevVerification, reason); err != nil {
				s.d.Log.WarnContext(ctx, "restoring verification level failed", "guild_id", guildID, "err", err)
			}
		}
	}
	if len(st.PrevSlowmodes) > 0 {
		current := map[snowflake.ID]int{}
		if chans, err := s.d.Platform.TextChannels(ctx, guildID); err == nil {
			for _, c := range chans {
				current[c.ID] = c.Slowmode
			}
		}
		for ch, prev := range st.PrevSlowmodes {
			if cur, ok := current[ch]; ok && cur == st.SetSlowmode {
				if err := s.d.Platform.SetSlowmode(ctx, ch, prev, reason); err != nil {
					s.d.Log.WarnContext(ctx, "restoring slowmode failed", "channel_id", ch, "err", err)
				}
			}
		}
	}
	if r.EventID != 0 {
		if ev, err := s.d.Store.SetEventStatus(ctx, guildID, r.EventID, StatusEnded, by.ID, by.Name); err == nil {
			s.refreshAlert(ctx, ev)
		}
	}
	s.d.Log.InfoContext(ctx, "raid mode off", "guild_id", guildID, "by", by.ID)
	return r, nil
}

// ActiveRaid returns the running raid, or ErrNoRaid.
func (s *Service) ActiveRaid(ctx context.Context, guildID snowflake.ID) (Raid, error) {
	return s.d.Store.ActiveRaid(ctx, guildID)
}

func (s *Service) endDueRaids(ctx context.Context) {
	due, err := s.d.Store.DueRaids(ctx, s.d.Now())
	if err != nil {
		s.d.Log.WarnContext(ctx, "listing due raids failed", "err", err)
		return
	}
	for _, r := range due {
		if _, err := s.EndRaid(ctx, r.GuildID, moderation.Actor{Name: "Guardian"}); err != nil && !errors.Is(err, ErrNoRaid) {
			s.d.Log.WarnContext(ctx, "ending raid failed", "guild_id", r.GuildID, "err", err)
		}
	}
}
