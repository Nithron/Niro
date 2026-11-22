package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"

	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/store/dbq"
)

var _ guardian.Store = (*Store)(nil)

func (s *Store) CreateEvent(ctx context.Context, e guardian.NewEvent) (guardian.Event, error) {
	signals, _ := json.Marshal(nonNil(e.Signals))
	evidence, _ := json.Marshal(e.Evidence)
	row, err := s.q.InsertGuardianEvent(ctx, dbq.InsertGuardianEventParams{
		GuildID: int64(e.GuildID), UserID: optID(e.UserID), UserName: e.UserName, Kind: string(e.Kind),
		Signals: signals, Score: int32(e.Score), Response: string(e.Response), Evidence: evidence,
		CaseNumber: optInt(e.CaseNumber), Status: string(e.Status),
	})
	if err != nil {
		return guardian.Event{}, mapFKViolation(err)
	}
	return toEvent(row), nil
}

func nonNil(s []guardian.Signal) []guardian.Signal {
	if s == nil {
		return []guardian.Signal{}
	}
	return s
}

func (s *Store) GetEvent(ctx context.Context, guildID snowflake.ID, id int64) (guardian.Event, error) {
	row, err := s.q.GetGuardianEvent(ctx, dbq.GetGuardianEventParams{GuildID: int64(guildID), ID: id})
	return eventOrNotFound(row, err, guardian.ErrEventNotFound)
}

func (s *Store) ListEvents(ctx context.Context, guildID snowflake.ID, f guardian.EventFilter) ([]guardian.Event, error) {
	rows, err := s.q.ListGuardianEvents(ctx, dbq.ListGuardianEventsParams{
		GuildID: int64(guildID), Status: optText(string(f.Status)), UserID: optID(f.UserID),
		Before: optInt8(f.Before), Lim: int32(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]guardian.Event, len(rows))
	for i, r := range rows {
		out[i] = toEvent(r)
	}
	return out, nil
}

func (s *Store) SetEventAlert(ctx context.Context, guildID snowflake.ID, id int64, channelID, messageID snowflake.ID) error {
	return s.q.SetGuardianEventAlert(ctx, dbq.SetGuardianEventAlertParams{
		GuildID: int64(guildID), ID: id, AlertChannelID: optID(channelID), AlertMessageID: optID(messageID),
	})
}

func (s *Store) ResolveEvent(ctx context.Context, guildID snowflake.ID, id int64, status guardian.Status, by snowflake.ID, byName string) (guardian.Event, error) {
	row, err := s.q.ResolveGuardianEvent(ctx, dbq.ResolveGuardianEventParams{
		GuildID: int64(guildID), ID: id, Status: string(status), ResolvedBy: optID(by), ResolvedByName: byName,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Either it does not exist or it was already resolved.
		if _, gerr := s.GetEvent(ctx, guildID, id); gerr != nil {
			return guardian.Event{}, gerr
		}
		return guardian.Event{}, guardian.ErrAlreadyResolved
	}
	return eventOrNotFound(row, err, guardian.ErrEventNotFound)
}

func (s *Store) SetEventStatus(ctx context.Context, guildID snowflake.ID, id int64, status guardian.Status, by snowflake.ID, byName string) (guardian.Event, error) {
	row, err := s.q.SetGuardianEventStatus(ctx, dbq.SetGuardianEventStatusParams{
		GuildID: int64(guildID), ID: id, Status: string(status), ResolvedBy: optID(by), ResolvedByName: byName,
	})
	return eventOrNotFound(row, err, guardian.ErrEventNotFound)
}

func (s *Store) PurgeEventsBefore(ctx context.Context, t time.Time) (int64, error) {
	return s.q.PurgeGuardianEvents(ctx, t)
}

func eventOrNotFound(row dbq.GuardianEvent, err error, notFound error) (guardian.Event, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return guardian.Event{}, notFound
	}
	if err != nil {
		return guardian.Event{}, err
	}
	return toEvent(row), nil
}

func toEvent(r dbq.GuardianEvent) guardian.Event {
	e := guardian.Event{
		ID: r.ID, GuildID: snowflake.ID(r.GuildID), UserID: snowflake.ID(r.UserID.Int64), UserName: r.UserName,
		Kind: guardian.Kind(r.Kind), Score: int(r.Score), Response: guardian.Response(r.Response),
		CaseNumber: int(r.CaseNumber.Int32), Status: guardian.Status(r.Status),
		ResolvedBy: snowflake.ID(r.ResolvedBy.Int64), ResolvedByName: r.ResolvedByName,
		AlertChannelID: snowflake.ID(r.AlertChannelID.Int64), AlertMessageID: snowflake.ID(r.AlertMessageID.Int64),
		CreatedAt: r.CreatedAt,
	}
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time
		e.ResolvedAt = &t
	}
	_ = json.Unmarshal(r.Signals, &e.Signals)
	_ = json.Unmarshal(r.Evidence, &e.Evidence)
	return e
}

// ---- quarantines -------------------------------------------------------------

func (s *Store) SaveQuarantine(ctx context.Context, q guardian.Quarantine) error {
	roles := make([]int64, len(q.SavedRoleIDs))
	for i, r := range q.SavedRoleIDs {
		roles[i] = int64(r)
	}
	err := s.q.UpsertQuarantine(ctx, dbq.UpsertQuarantineParams{
		GuildID: int64(q.GuildID), UserID: int64(q.UserID), EventID: optInt8(q.EventID), Mode: string(q.Mode),
		SavedRoleIds: roles, RoleID: optID(q.RoleID), Until: optTime(q.Until), CreatedAt: q.CreatedAt,
	})
	return mapFKViolation(err)
}

func (s *Store) ActiveQuarantine(ctx context.Context, guildID, userID snowflake.ID) (guardian.Quarantine, error) {
	r, err := s.q.GetQuarantine(ctx, dbq.GetQuarantineParams{GuildID: int64(guildID), UserID: int64(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return guardian.Quarantine{}, guardian.ErrNotQuarantined
	}
	if err != nil {
		return guardian.Quarantine{}, err
	}
	q := guardian.Quarantine{
		GuildID: snowflake.ID(r.GuildID), UserID: snowflake.ID(r.UserID), EventID: r.EventID.Int64,
		Mode: guardian.QuarantineMode(r.Mode), RoleID: snowflake.ID(r.RoleID.Int64), CreatedAt: r.CreatedAt,
	}
	for _, id := range r.SavedRoleIds {
		q.SavedRoleIDs = append(q.SavedRoleIDs, snowflake.ID(id))
	}
	if r.Until.Valid {
		t := r.Until.Time
		q.Until = &t
	}
	return q, nil
}

func (s *Store) ReleaseQuarantine(ctx context.Context, guildID, userID snowflake.ID) error {
	n, err := s.q.DeleteQuarantine(ctx, dbq.DeleteQuarantineParams{GuildID: int64(guildID), UserID: int64(userID)})
	if err == nil && n == 0 {
		return guardian.ErrNotQuarantined
	}
	return err
}

// ---- raids -------------------------------------------------------------------

func (s *Store) StartRaid(ctx context.Context, r guardian.Raid) (guardian.Raid, bool, error) {
	row, err := s.q.InsertRaid(ctx, dbq.InsertRaidParams{
		GuildID: int64(r.GuildID), StartedAt: r.StartedAt, EndsAt: r.EndsAt, Manual: r.Manual,
		StartedBy: optID(r.StartedBy), StartedName: r.StartedName, Joins: int32(r.Joins),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING: a raid is already running.
		active, err := s.ActiveRaid(ctx, r.GuildID)
		return active, false, err
	}
	if err != nil {
		return guardian.Raid{}, false, mapFKViolation(err)
	}
	return toRaid(row), true, nil
}

func (s *Store) UpdateRaid(ctx context.Context, guildID snowflake.ID, id, eventID int64, st guardian.RaidState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.q.UpdateRaid(ctx, dbq.UpdateRaidParams{GuildID: int64(guildID), ID: id, EventID: optInt8(eventID), State: b})
}

func (s *Store) ActiveRaid(ctx context.Context, guildID snowflake.ID) (guardian.Raid, error) {
	row, err := s.q.GetActiveRaid(ctx, int64(guildID))
	if errors.Is(err, pgx.ErrNoRows) {
		return guardian.Raid{}, guardian.ErrNoRaid
	}
	if err != nil {
		return guardian.Raid{}, err
	}
	return toRaid(row), nil
}

func (s *Store) EndRaid(ctx context.Context, guildID, by snowflake.ID) (guardian.Raid, error) {
	row, err := s.q.EndActiveRaid(ctx, dbq.EndActiveRaidParams{GuildID: int64(guildID), EndedBy: optID(by)})
	if errors.Is(err, pgx.ErrNoRows) {
		return guardian.Raid{}, guardian.ErrNoRaid
	}
	if err != nil {
		return guardian.Raid{}, err
	}
	return toRaid(row), nil
}

func (s *Store) DueRaids(ctx context.Context, now time.Time) ([]guardian.Raid, error) {
	rows, err := s.q.DueRaids(ctx, now)
	if err != nil {
		return nil, err
	}
	out := make([]guardian.Raid, len(rows))
	for i, r := range rows {
		out[i] = toRaid(r)
	}
	return out, nil
}

func toRaid(r dbq.GuardianRaid) guardian.Raid {
	out := guardian.Raid{
		ID: r.ID, GuildID: snowflake.ID(r.GuildID), EventID: r.EventID.Int64, StartedAt: r.StartedAt, EndsAt: r.EndsAt,
		EndedBy: snowflake.ID(r.EndedBy.Int64), Manual: r.Manual, Joins: int(r.Joins),
		StartedBy: snowflake.ID(r.StartedBy.Int64), StartedName: r.StartedName,
	}
	if r.EndedAt.Valid {
		t := r.EndedAt.Time
		out.EndedAt = &t
	}
	_ = json.Unmarshal(r.State, &out.State)
	return out
}

// ---- lockdowns -----------------------------------------------------------------

func (s *Store) SaveLockdowns(ctx context.Context, states []guardian.LockState) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, l := range states {
			if err := q.UpsertLockdown(ctx, dbq.UpsertLockdownParams{
				GuildID: int64(l.GuildID), ChannelID: int64(l.ChannelID), HadOverwrite: l.HadOverwrite,
				AllowBits: l.Allow, DenyBits: l.Deny, LockedBy: optID(l.By), CreatedAt: l.CreatedAt,
			}); err != nil {
				return mapFKViolation(err)
			}
		}
		return nil
	})
}

func (s *Store) Lockdowns(ctx context.Context, guildID snowflake.ID) ([]guardian.LockState, error) {
	rows, err := s.q.ListLockdowns(ctx, int64(guildID))
	if err != nil {
		return nil, err
	}
	out := make([]guardian.LockState, len(rows))
	for i, r := range rows {
		out[i] = guardian.LockState{
			GuildID: snowflake.ID(r.GuildID), ChannelID: snowflake.ID(r.ChannelID), HadOverwrite: r.HadOverwrite,
			Allow: r.AllowBits, Deny: r.DenyBits, By: snowflake.ID(r.LockedBy.Int64), CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

func (s *Store) DeleteLockdowns(ctx context.Context, guildID snowflake.ID, channelIDs []snowflake.ID) error {
	ids := make([]int64, len(channelIDs))
	for i, c := range channelIDs {
		ids[i] = int64(c)
	}
	return s.q.DeleteLockdowns(ctx, dbq.DeleteLockdownsParams{GuildID: int64(guildID), ChannelIds: ids})
}

// ---- scam-domain feeds ---------------------------------------------------------

func (s *Store) ReplaceFeedDomains(ctx context.Context, source string, domains []string) (int, error) {
	var n int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteFeedDomains(ctx, source); err != nil {
			return err
		}
		rows := make([]dbq.InsertFeedDomainsParams, len(domains))
		for i, d := range domains {
			rows[i] = dbq.InsertFeedDomainsParams{Domain: d, Source: source}
		}
		var err error
		if n, err = q.InsertFeedDomains(ctx, rows); err != nil {
			return err
		}
		return q.UpsertFeed(ctx, dbq.UpsertFeedParams{Source: source, Domains: int32(n), UpdatedAt: time.Now()})
	})
	return int(n), err
}

func (s *Store) ScamDomains(ctx context.Context) ([]string, error) { return s.q.ListScamDomains(ctx) }

// FeedUpdatedAt returns the zero time for a feed never fetched.
func (s *Store) FeedUpdatedAt(ctx context.Context, source string) (time.Time, error) {
	t, err := s.q.GetFeedUpdatedAt(ctx, source)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return t, err
}
