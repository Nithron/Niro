// Package retention deletes data Niro no longer needs, on the schedule in
// plan §5 and decision 28. Message text is never stored in Postgres outside
// ticket transcripts, which the tickets module expires itself.
package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Policy: how long each kind of data is kept.
const (
	// LeftGuildGrace: a server Niro left is deleted after this, with all its
	// data. Re-adding Niro within it restores everything.
	LeftGuildGrace = 30 * 24 * time.Hour
	// DoneScheduledActions: finished timed actions (temp-ban expiries).
	DoneScheduledActions = 30 * 24 * time.Hour
	// EndedRaids: raid-mode periods after they end.
	EndedRaids = 90 * 24 * time.Hour
	// ConfigAudit: who changed which setting.
	ConfigAudit = 365 * 24 * time.Hour
)

// Store deletes rows older than a cutoff and reports how many went.
type Store interface {
	PurgeLeftGuilds(ctx context.Context, before time.Time) (int64, error)
	PurgeDoneScheduledActions(ctx context.Context, before time.Time) (int64, error)
	PurgeEndedRaids(ctx context.Context, before time.Time) (int64, error)
	PurgeConfigAudit(ctx context.Context, before time.Time) (int64, error)
}

// Runner applies the policy periodically.
type Runner struct {
	Store Store
	Log   *slog.Logger
	// Now defaults to time.Now.
	Now func() time.Time
	// Done, if set, is told how each run went (job metrics).
	Done func(err error)
	// Delay holds back the first run. After a restart, servers that re-added
	// Niro while it was down are only marked as present again once the
	// gateway has delivered them; purging before that could delete them.
	Delay time.Duration
}

// RunOnce applies every rule once. A failing rule doesn't stop the others.
func (r Runner) RunOnce(ctx context.Context) error {
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	rules := []struct {
		name  string
		keep  time.Duration
		purge func(context.Context, time.Time) (int64, error)
	}{
		{"left servers", LeftGuildGrace, r.Store.PurgeLeftGuilds},
		{"done scheduled actions", DoneScheduledActions, r.Store.PurgeDoneScheduledActions},
		{"ended raids", EndedRaids, r.Store.PurgeEndedRaids},
		{"config audit", ConfigAudit, r.Store.PurgeConfigAudit},
	}
	var errs []error
	for _, rule := range rules {
		n, err := rule.purge(ctx, now.Add(-rule.keep))
		if err != nil {
			errs = append(errs, fmt.Errorf("purging %s: %w", rule.name, err))
			continue
		}
		if n > 0 {
			r.Log.InfoContext(ctx, "retention purge", "data", rule.name, "rows", n)
		}
	}
	return errors.Join(errs...)
}

// Run applies the policy now and then every interval until ctx ends.
func (r Runner) Run(ctx context.Context, every time.Duration) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(r.Delay):
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		err := r.RunOnce(ctx)
		if ctx.Err() != nil {
			return // shutting down: not a failed run
		}
		if err != nil {
			r.Log.ErrorContext(ctx, "retention run failed", "err", err)
		}
		if r.Done != nil {
			r.Done(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
