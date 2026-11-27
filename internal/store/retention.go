package store

import (
	"context"
	"fmt"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nithron/niro/internal/retention"
)

var _ retention.Store = (*Store)(nil)

func tstz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func (s *Store) PurgeLeftGuilds(ctx context.Context, before time.Time) (int64, error) {
	return s.q.PurgeLeftGuilds(ctx, tstz(before))
}

func (s *Store) PurgeDoneScheduledActions(ctx context.Context, before time.Time) (int64, error) {
	return s.q.PurgeDoneScheduledActions(ctx, tstz(before))
}

func (s *Store) PurgeEndedRaids(ctx context.Context, before time.Time) (int64, error) {
	return s.q.PurgeEndedRaids(ctx, tstz(before))
}

func (s *Store) PurgeConfigAudit(ctx context.Context, before time.Time) (int64, error) {
	return s.q.PurgeConfigAudit(ctx, before)
}

// ReconcileGuilds marks as left every active server not in present (the
// servers the gateway reported). It refuses, and marks nothing, when that
// would be more than maxShare of the active servers (and more than 5): an
// incomplete list must never start the 30-day purge for healthy servers.
func (s *Store) ReconcileGuilds(ctx context.Context, present []snowflake.ID, maxShare float64) (int64, error) {
	ids := make([]int64, len(present))
	for i, id := range present {
		ids[i] = int64(id)
	}
	missing, err := s.q.CountMissingGuilds(ctx, ids)
	if err != nil || missing == 0 {
		return 0, err
	}
	active, err := s.q.CountActiveGuilds(ctx)
	if err != nil {
		return 0, err
	}
	if missing > 5 && float64(missing) > maxShare*float64(active) {
		return 0, fmt.Errorf("%d of %d active servers were not reported by the gateway; not marking them as left", missing, active)
	}
	return s.q.MarkMissingGuildsLeft(ctx, ids)
}

// DeleteGuild deletes a server and, through ON DELETE CASCADE, all its data.
// It reports whether the server was known.
func (s *Store) DeleteGuild(ctx context.Context, id snowflake.ID) (bool, error) {
	n, err := s.q.DeleteGuild(ctx, int64(id))
	return n > 0, err
}
