package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/store/dbq"
)

var _ moderation.CaseStore = (*Store)(nil)

// CreateCase assigns the next case number and inserts the case (plus any
// scheduled unban) in one transaction.
func (s *Store) CreateCase(ctx context.Context, c moderation.NewCase) (moderation.Case, error) {
	var out moderation.Case
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		num, err := q.NextCaseNumber(ctx, int64(c.GuildID))
		if err != nil {
			return mapFKViolation(err)
		}
		row, err := q.InsertCase(ctx, dbq.InsertCaseParams{
			GuildID: int64(c.GuildID), CaseNumber: num, Action: string(c.Action),
			TargetID: optID(c.TargetID), TargetName: c.TargetName,
			ModeratorID: int64(c.ModeratorID), ModeratorName: c.ModeratorName, Reason: c.Reason,
			DurationSeconds: optSeconds(c.Duration), ExpiresAt: optTime(c.ExpiresAt),
			Source: string(c.Source), Details: c.Details.JSON(), DmSent: c.DMSent,
		})
		if err != nil {
			return err
		}
		if c.ScheduleUnbanAt != nil {
			if err := q.InsertScheduledAction(ctx, dbq.InsertScheduledActionParams{
				GuildID: int64(c.GuildID), Kind: "unban", TargetID: int64(c.TargetID),
				CaseNumber: pgtype.Int4{Int32: num, Valid: true}, RunAt: *c.ScheduleUnbanAt,
			}); err != nil {
				return err
			}
		}
		out = toCase(row)
		return nil
	})
	return out, err
}

func (s *Store) GetCase(ctx context.Context, guildID snowflake.ID, number int) (moderation.Case, error) {
	row, err := s.q.GetCase(ctx, dbq.GetCaseParams{GuildID: int64(guildID), CaseNumber: int32(number)})
	return caseOrNotFound(row, err)
}

func (s *Store) ListCases(ctx context.Context, guildID snowflake.ID, f moderation.CaseFilter) ([]moderation.Case, error) {
	rows, err := s.q.ListCases(ctx, dbq.ListCasesParams{
		GuildID: int64(guildID), TargetID: optID(f.TargetID), ModeratorID: optID(f.ModeratorID),
		Action: optText(string(f.Action)), Before: optInt(f.Before), Lim: int32(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]moderation.Case, len(rows))
	for i, r := range rows {
		out[i] = toCase(r)
	}
	return out, nil
}

func (s *Store) CountCases(ctx context.Context, guildID snowflake.ID, f moderation.CaseFilter) (int, error) {
	n, err := s.q.CountCases(ctx, dbq.CountCasesParams{
		GuildID: int64(guildID), TargetID: optID(f.TargetID), ModeratorID: optID(f.ModeratorID), Action: optText(string(f.Action)),
	})
	return int(n), err
}

func (s *Store) UpdateCaseReason(ctx context.Context, guildID snowflake.ID, number int, reason string) (moderation.Case, error) {
	row, err := s.q.UpdateCaseReason(ctx, dbq.UpdateCaseReasonParams{GuildID: int64(guildID), CaseNumber: int32(number), Reason: reason})
	return caseOrNotFound(row, err)
}

func (s *Store) DeleteCase(ctx context.Context, guildID snowflake.ID, number int, by snowflake.ID) (moderation.Case, error) {
	row, err := s.q.SoftDeleteCase(ctx, dbq.SoftDeleteCaseParams{GuildID: int64(guildID), CaseNumber: int32(number), DeletedBy: optID(by)})
	return caseOrNotFound(row, err)
}

func (s *Store) SetCaseLogMessage(ctx context.Context, guildID snowflake.ID, number int, channelID, messageID snowflake.ID) error {
	return s.q.SetCaseLogMessage(ctx, dbq.SetCaseLogMessageParams{
		GuildID: int64(guildID), CaseNumber: int32(number), LogChannelID: optID(channelID), LogMessageID: optID(messageID),
	})
}

func (s *Store) CancelScheduledUnbans(ctx context.Context, guildID, targetID snowflake.ID) error {
	return s.q.CancelScheduledActions(ctx, dbq.CancelScheduledActionsParams{GuildID: int64(guildID), Kind: "unban", TargetID: int64(targetID)})
}

func (s *Store) ClaimDueActions(ctx context.Context, limit int) ([]moderation.ScheduledAction, error) {
	rows, err := s.q.ClaimDueActions(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]moderation.ScheduledAction, len(rows))
	for i, r := range rows {
		out[i] = moderation.ScheduledAction{
			ID: r.ID, GuildID: snowflake.ID(r.GuildID), Kind: r.Kind, TargetID: snowflake.ID(r.TargetID),
			CaseNumber: int(r.CaseNumber.Int32), Attempts: int(r.Attempts),
		}
	}
	return out, nil
}

func (s *Store) CompleteScheduledAction(ctx context.Context, id int64, note string) error {
	return s.q.CompleteScheduledAction(ctx, dbq.CompleteScheduledActionParams{ID: id, LastError: optText(note)})
}

func (s *Store) RetryScheduledAction(ctx context.Context, id int64, errText string, after time.Duration) error {
	return s.q.FailScheduledAction(ctx, dbq.FailScheduledActionParams{ID: id, LastError: optText(errText), Column3: int32(max(after.Minutes(), 1))})
}

func caseOrNotFound(row dbq.ModCase, err error) (moderation.Case, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return moderation.Case{}, moderation.ErrCaseNotFound
	}
	if err != nil {
		return moderation.Case{}, err
	}
	return toCase(row), nil
}

func toCase(r dbq.ModCase) moderation.Case {
	c := moderation.Case{
		GuildID: snowflake.ID(r.GuildID), Number: int(r.CaseNumber), Action: moderation.Action(r.Action),
		TargetID: snowflake.ID(r.TargetID.Int64), TargetName: r.TargetName,
		ModeratorID: snowflake.ID(r.ModeratorID), ModeratorName: r.ModeratorName, Reason: r.Reason,
		Duration: time.Duration(r.DurationSeconds.Int32) * time.Second, Source: moderation.Source(r.Source),
		DMSent: r.DmSent, LogChannelID: snowflake.ID(r.LogChannelID.Int64), LogMessageID: snowflake.ID(r.LogMessageID.Int64),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.DeletedAt.Valid,
	}
	if r.ExpiresAt.Valid {
		t := r.ExpiresAt.Time
		c.ExpiresAt = &t
	}
	_ = json.Unmarshal(r.Details, &c.Details)
	return c
}

func optID(id snowflake.ID) pgtype.Int8 { return pgtype.Int8{Int64: int64(id), Valid: id != 0} }
func optText(s string) pgtype.Text      { return pgtype.Text{String: s, Valid: s != ""} }
func optInt(n int) pgtype.Int4          { return pgtype.Int4{Int32: int32(n), Valid: n != 0} }
func optSeconds(d time.Duration) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(d.Seconds()), Valid: d > 0}
}
func optTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
