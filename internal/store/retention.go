package store

import (
	"context"
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

// DeleteGuild deletes a server and, through ON DELETE CASCADE, all its data.
// It reports whether the server was known.
func (s *Store) DeleteGuild(ctx context.Context, id snowflake.ID) (bool, error) {
	n, err := s.q.DeleteGuild(ctx, int64(id))
	return n > 0, err
}
