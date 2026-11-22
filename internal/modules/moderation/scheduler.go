package moderation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const maxScheduledAttempts = 5

// RunScheduler executes due scheduled actions (currently: lifting temporary
// bans) until ctx is done. bot identifies the bot as the case moderator.
func (s *Service) RunScheduler(ctx context.Context, bot Actor, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n, err := s.RunDue(ctx, bot); err != nil {
			s.log.WarnContext(ctx, "running scheduled actions failed", "err", err)
		} else if n > 0 {
			s.log.InfoContext(ctx, "ran scheduled actions", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunDue claims and runs due actions once; it returns how many it handled.
func (s *Service) RunDue(ctx context.Context, bot Actor) (int, error) {
	due, err := s.store.ClaimDueActions(ctx, 25)
	if err != nil {
		return 0, err
	}
	for _, a := range due {
		s.runOne(ctx, bot, a)
	}
	return len(due), nil
}

func (s *Service) runOne(ctx context.Context, bot Actor, a ScheduledAction) {
	if a.Kind != "unban" {
		_ = s.store.CompleteScheduledAction(ctx, a.ID, "unknown kind")
		return
	}
	reason := "Temporary ban expired"
	if a.CaseNumber > 0 {
		reason = fmt.Sprintf("Temporary ban expired (case #%d)", a.CaseNumber)
	}
	_, err := s.Execute(ctx, Request{
		GuildID: a.GuildID, Action: ActionUnban, Actor: bot, TargetID: a.TargetID,
		Reason: reason, Source: SourceSystem, Notify: new(bool),
	})
	switch {
	case err == nil:
		_ = s.store.CompleteScheduledAction(ctx, a.ID, "")
	case errors.Is(err, ErrNotBanned):
		// Someone already unbanned them by hand: nothing to do.
		_ = s.store.CompleteScheduledAction(ctx, a.ID, "already unbanned")
	case a.Attempts >= maxScheduledAttempts:
		s.log.ErrorContext(ctx, "giving up on scheduled unban", "guild_id", a.GuildID, "user_id", a.TargetID, "err", err)
		_ = s.store.CompleteScheduledAction(ctx, a.ID, "gave up: "+err.Error())
	default:
		s.log.WarnContext(ctx, "scheduled unban failed, will retry", "guild_id", a.GuildID, "user_id", a.TargetID, "attempt", a.Attempts, "err", err)
		_ = s.store.RetryScheduledAction(ctx, a.ID, err.Error(), time.Duration(a.Attempts)*5*time.Minute)
	}
}
