// Package store is Nithro's PostgreSQL persistence layer. Queries live in
// queries/*.sql and are compiled to type-safe Go by sqlc (package dbq);
// migrations live in migrations/*.sql and are embedded in the binary.
package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/store/dbq"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store implements core.ConfigStore and core.GuildDirectory.
type Store struct {
	pool *pgxpool.Pool
	q    *dbq.Queries
}

var (
	_ core.ConfigStore    = (*Store)(nil)
	_ core.GuildDirectory = (*Store)(nil)
)

// Connect opens a pool and verifies the connection, retrying for up to
// timeout so containers that start together don't race.
func Connect(ctx context.Context, url string, timeout time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		err = pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("connecting to postgres: %w", err)
		}
		time.Sleep(time.Second)
	}
}

// Migrate applies all pending migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]*goose.MigrationResult, error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return p.Up(ctx)
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, q: dbq.New(pool)} }

// Ping is used by readiness checks.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) UpsertGuild(ctx context.Context, g core.GuildInfo) error {
	locale := g.Locale
	if locale == "" {
		locale = "en-US"
	}
	return s.q.UpsertGuild(ctx, dbq.UpsertGuildParams{
		ID:      int64(g.ID),
		Name:    g.Name,
		Icon:    pgtype.Text{String: g.Icon, Valid: g.Icon != ""},
		OwnerID: int64(g.OwnerID),
		Locale:  locale,
	})
}

func (s *Store) MarkGuildLeft(ctx context.Context, id snowflake.ID) error {
	return s.q.MarkGuildLeft(ctx, int64(id))
}

func (s *Store) ActiveGuilds(ctx context.Context, ids []snowflake.ID) (map[snowflake.ID]bool, error) {
	out := make(map[snowflake.ID]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	raw := make([]int64, len(ids))
	for i, id := range ids {
		raw[i] = int64(id)
	}
	rows, err := s.q.FilterActiveGuildIDs(ctx, raw)
	if err != nil {
		return nil, err
	}
	for _, id := range rows {
		out[snowflake.ID(id)] = true
	}
	return out, nil
}

func (s *Store) IsGuildActive(ctx context.Context, id snowflake.ID) (bool, error) {
	return s.q.IsGuildActive(ctx, int64(id))
}

func (s *Store) ListModuleStates(ctx context.Context, guildID snowflake.ID) ([]core.StoredModuleState, error) {
	rows, err := s.q.ListModuleStates(ctx, int64(guildID))
	if err != nil {
		return nil, err
	}
	out := make([]core.StoredModuleState, len(rows))
	for i, r := range rows {
		out[i] = core.StoredModuleState{Module: core.ModuleID(r.Module), Enabled: r.Enabled, Config: r.Config, Version: r.Version, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

// SetModuleEnabled updates the module row and writes the audit entry in one
// transaction. The audit entry records the previous value (null if the
// module was still on its default).
func (s *Store) SetModuleEnabled(ctx context.Context, guildID snowflake.ID, module core.ModuleID, enabled bool, actorID *snowflake.ID, source core.ChangeSource) (core.StoredModuleState, error) {
	var result core.StoredModuleState
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)

		var previous *bool
		prev, err := q.GetModuleEnabledForUpdate(ctx, dbq.GetModuleEnabledForUpdateParams{GuildID: int64(guildID), Module: string(module)})
		switch {
		case err == nil:
			previous = &prev
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}

		actor := pgtype.Int8{}
		if actorID != nil {
			actor = pgtype.Int8{Int64: int64(*actorID), Valid: true}
		}
		row, err := q.UpsertModuleEnabled(ctx, dbq.UpsertModuleEnabledParams{
			GuildID: int64(guildID), Module: string(module), Enabled: enabled, UpdatedBy: actor,
		})
		if err != nil {
			return mapFKViolation(err)
		}
		change, err := json.Marshal(map[string]any{"enabled": map[string]any{"from": previous, "to": enabled}})
		if err != nil {
			return err
		}
		if err := q.InsertConfigAudit(ctx, dbq.InsertConfigAuditParams{
			GuildID: int64(guildID), Module: string(module), ActorID: actor, Source: string(source), Change: change,
		}); err != nil {
			return err
		}
		result = core.StoredModuleState{Module: core.ModuleID(row.Module), Enabled: row.Enabled, Config: row.Config, Version: row.Version, UpdatedAt: row.UpdatedAt}
		return nil
	})
	return result, err
}

// SetModuleSettings saves a module's settings and writes the audit entry
// (old and new settings) in one transaction.
func (s *Store) SetModuleSettings(ctx context.Context, guildID snowflake.ID, module core.ModuleID, config []byte, defaultEnabled bool, actorID *snowflake.ID, source core.ChangeSource) (core.StoredModuleState, error) {
	var result core.StoredModuleState
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var previous json.RawMessage
		prev, err := q.GetModuleConfigForUpdate(ctx, dbq.GetModuleConfigForUpdateParams{GuildID: int64(guildID), Module: string(module)})
		switch {
		case err == nil:
			previous = prev
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		actor := pgtype.Int8{}
		if actorID != nil {
			actor = pgtype.Int8{Int64: int64(*actorID), Valid: true}
		}
		row, err := q.UpsertModuleConfig(ctx, dbq.UpsertModuleConfigParams{
			GuildID: int64(guildID), Module: string(module), Enabled: defaultEnabled, Config: config, UpdatedBy: actor,
		})
		if err != nil {
			return mapFKViolation(err)
		}
		change, err := json.Marshal(map[string]any{"settings": map[string]any{"from": previous, "to": json.RawMessage(config)}})
		if err != nil {
			return err
		}
		if err := q.InsertConfigAudit(ctx, dbq.InsertConfigAuditParams{
			GuildID: int64(guildID), Module: string(module), ActorID: actor, Source: string(source), Change: change,
		}); err != nil {
			return err
		}
		result = core.StoredModuleState{Module: core.ModuleID(row.Module), Enabled: row.Enabled, Config: row.Config, Version: row.Version, UpdatedAt: row.UpdatedAt}
		return nil
	})
	return result, err
}

// AuditEntry is one config_audit row.
type AuditEntry struct {
	Module    string
	ActorID   *snowflake.ID
	Source    string
	Change    json.RawMessage
	CreatedAt time.Time
}

// ListConfigAudit returns the latest audit entries for a guild, newest first.
func (s *Store) ListConfigAudit(ctx context.Context, guildID snowflake.ID, limit int32) ([]AuditEntry, error) {
	rows, err := s.q.ListConfigAudit(ctx, dbq.ListConfigAuditParams{GuildID: int64(guildID), Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]AuditEntry, len(rows))
	for i, r := range rows {
		e := AuditEntry{Module: r.Module, Source: r.Source, Change: r.Change, CreatedAt: r.CreatedAt}
		if r.ActorID.Valid {
			id := snowflake.ID(r.ActorID.Int64)
			e.ActorID = &id
		}
		out[i] = e
	}
	return out, nil
}

func mapFKViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return core.ErrUnknownGuild
	}
	return err
}
