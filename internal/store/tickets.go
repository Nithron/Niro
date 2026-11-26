package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"

	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/tickets"
	"github.com/nithron/niro/internal/store/dbq"
)

var _ tickets.Store = (*Store)(nil)

// ---- panels --------------------------------------------------------------------

func ticketPanelJSON(p tickets.Panel) (msg, cats []byte) {
	msg, _ = json.Marshal(p.Message)
	if p.Categories == nil {
		p.Categories = []tickets.Category{}
	}
	cats, _ = json.Marshal(p.Categories)
	return msg, cats
}

func (s *Store) CreateTicketPanel(ctx context.Context, p tickets.Panel) (tickets.Panel, error) {
	msg, cats := ticketPanelJSON(p)
	row, err := s.q.InsertTicketPanel(ctx, dbq.InsertTicketPanelParams{
		GuildID: int64(p.GuildID), Name: p.Name, ChannelID: int64(p.ChannelID), Message: msg, Categories: cats, UpdatedBy: optID(p.UpdatedBy),
	})
	if err != nil {
		return tickets.Panel{}, mapFKViolation(err)
	}
	return toTicketPanel(row), nil
}

func (s *Store) UpdateTicketPanel(ctx context.Context, p tickets.Panel) (tickets.Panel, error) {
	msg, cats := ticketPanelJSON(p)
	row, err := s.q.UpdateTicketPanel(ctx, dbq.UpdateTicketPanelParams{
		GuildID: int64(p.GuildID), ID: p.ID, Name: p.Name, ChannelID: int64(p.ChannelID), Message: msg, Categories: cats, UpdatedBy: optID(p.UpdatedBy),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return tickets.Panel{}, tickets.ErrPanelNotFound
	}
	if err != nil {
		return tickets.Panel{}, err
	}
	return toTicketPanel(row), nil
}

func (s *Store) GetTicketPanel(ctx context.Context, guildID snowflake.ID, id int64) (tickets.Panel, error) {
	row, err := s.q.GetTicketPanel(ctx, dbq.GetTicketPanelParams{GuildID: int64(guildID), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return tickets.Panel{}, tickets.ErrPanelNotFound
	}
	if err != nil {
		return tickets.Panel{}, err
	}
	return toTicketPanel(row), nil
}

func (s *Store) ListTicketPanels(ctx context.Context, guildID snowflake.ID) ([]tickets.Panel, error) {
	rows, err := s.q.ListTicketPanels(ctx, int64(guildID))
	if err != nil {
		return nil, err
	}
	out := make([]tickets.Panel, len(rows))
	for i, r := range rows {
		out[i] = toTicketPanel(r)
	}
	return out, nil
}

func (s *Store) CountTicketPanels(ctx context.Context, guildID snowflake.ID) (int, error) {
	n, err := s.q.CountTicketPanels(ctx, int64(guildID))
	return int(n), err
}

func (s *Store) DeleteTicketPanel(ctx context.Context, guildID snowflake.ID, id int64) error {
	return s.q.DeleteTicketPanel(ctx, dbq.DeleteTicketPanelParams{GuildID: int64(guildID), ID: id})
}

func (s *Store) SetTicketPanelMessage(ctx context.Context, guildID snowflake.ID, id int64, channelID, messageID snowflake.ID) error {
	return s.q.SetTicketPanelMessage(ctx, dbq.SetTicketPanelMessageParams{
		GuildID: int64(guildID), ID: id, ChannelID: int64(channelID), MessageID: optID(messageID),
	})
}

func toTicketPanel(r dbq.TicketPanel) tickets.Panel {
	p := tickets.Panel{
		ID: r.ID, GuildID: snowflake.ID(r.GuildID), Name: r.Name, ChannelID: snowflake.ID(r.ChannelID),
		MessageID: snowflake.ID(r.MessageID.Int64), UpdatedBy: snowflake.ID(r.UpdatedBy.Int64), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	_ = json.Unmarshal(r.Message, &p.Message)
	if p.Message.Blocks == nil {
		p.Message = message.Document{Version: message.Version, Blocks: []message.Block{}}
	}
	_ = json.Unmarshal(r.Categories, &p.Categories)
	return p
}

// ---- tickets -------------------------------------------------------------------

func (s *Store) CreateTicket(ctx context.Context, t tickets.NewTicket) (tickets.Ticket, error) {
	var out tickets.Ticket
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		num, err := q.NextTicketNumber(ctx, int64(t.GuildID))
		if err != nil {
			return mapFKViolation(err)
		}
		row, err := q.InsertTicket(ctx, dbq.InsertTicketParams{
			GuildID: int64(t.GuildID), Number: num, PanelID: t.PanelID, CategoryID: t.CategoryID,
			CategoryLabel: t.CategoryLabel, Mode: string(t.Mode), OpenerID: int64(t.OpenerID), OpenerName: t.OpenerName,
		})
		if err != nil {
			return err
		}
		out = toTicket(row, false)
		return nil
	})
	return out, err
}

func (s *Store) DeleteTicket(ctx context.Context, guildID snowflake.ID, id int64) error {
	return s.q.DeleteTicket(ctx, dbq.DeleteTicketParams{GuildID: int64(guildID), ID: id})
}

func (s *Store) SetTicketChannel(ctx context.Context, guildID snowflake.ID, id int64, channelID snowflake.ID) error {
	return s.q.SetTicketChannel(ctx, dbq.SetTicketChannelParams{GuildID: int64(guildID), ID: id, ChannelID: optID(channelID)})
}

func ticketOrNotFound(row dbq.Ticket, has bool, err error) (tickets.Ticket, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return tickets.Ticket{}, tickets.ErrNotFound
	}
	if err != nil {
		return tickets.Ticket{}, err
	}
	return toTicket(row, has), nil
}

func (s *Store) GetTicket(ctx context.Context, guildID snowflake.ID, id int64) (tickets.Ticket, error) {
	r, err := s.q.GetTicket(ctx, dbq.GetTicketParams{GuildID: int64(guildID), ID: id})
	return ticketOrNotFound(r.Ticket, r.HasTranscript, err)
}

func (s *Store) GetTicketByNumber(ctx context.Context, guildID snowflake.ID, number int) (tickets.Ticket, error) {
	r, err := s.q.GetTicketByNumber(ctx, dbq.GetTicketByNumberParams{GuildID: int64(guildID), Number: int32(number)})
	return ticketOrNotFound(r.Ticket, r.HasTranscript, err)
}

func (s *Store) GetTicketByChannel(ctx context.Context, guildID, channelID snowflake.ID) (tickets.Ticket, error) {
	r, err := s.q.GetTicketByChannel(ctx, dbq.GetTicketByChannelParams{GuildID: int64(guildID), ChannelID: optID(channelID)})
	return ticketOrNotFound(r.Ticket, r.HasTranscript, err)
}

func (s *Store) ListTickets(ctx context.Context, guildID snowflake.ID, f tickets.Filter) ([]tickets.Ticket, error) {
	rows, err := s.q.ListTickets(ctx, dbq.ListTicketsParams{
		GuildID: int64(guildID), Status: optText(string(f.Status)), Before: optInt(f.Before), Lim: int32(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]tickets.Ticket, len(rows))
	for i, r := range rows {
		out[i] = toTicket(r.Ticket, r.HasTranscript)
	}
	return out, nil
}

func (s *Store) CountOpenTickets(ctx context.Context, guildID, openerID snowflake.ID, panelID int64, categoryID string) (int, error) {
	n, err := s.q.CountOpenTickets(ctx, dbq.CountOpenTicketsParams{
		GuildID: int64(guildID), OpenerID: int64(openerID), PanelID: panelID, CategoryID: categoryID,
	})
	return int(n), err
}

// statusChange maps "no row updated" to not found or wrong status.
func (s *Store) statusChange(ctx context.Context, guildID snowflake.ID, id int64, row dbq.Ticket, err error) (tickets.Ticket, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		if _, gerr := s.GetTicket(ctx, guildID, id); gerr != nil {
			return tickets.Ticket{}, gerr
		}
		return tickets.Ticket{}, tickets.ErrWrongStatus
	}
	if err != nil {
		return tickets.Ticket{}, err
	}
	return toTicket(row, false), nil
}

func (s *Store) ClaimTicket(ctx context.Context, guildID snowflake.ID, id int64, by snowflake.ID, byName string) (tickets.Ticket, error) {
	row, err := s.q.ClaimTicket(ctx, dbq.ClaimTicketParams{GuildID: int64(guildID), ID: id, ClaimedBy: optID(by), ClaimedByName: byName})
	return s.statusChange(ctx, guildID, id, row, err)
}

func (s *Store) CloseTicket(ctx context.Context, guildID snowflake.ID, id int64, by snowflake.ID, byName, reason string) (tickets.Ticket, error) {
	row, err := s.q.CloseTicket(ctx, dbq.CloseTicketParams{GuildID: int64(guildID), ID: id, ClosedBy: optID(by), ClosedByName: byName, CloseReason: reason})
	return s.statusChange(ctx, guildID, id, row, err)
}

func (s *Store) ReopenTicket(ctx context.Context, guildID snowflake.ID, id int64) (tickets.Ticket, error) {
	row, err := s.q.ReopenTicket(ctx, dbq.ReopenTicketParams{GuildID: int64(guildID), ID: id})
	return s.statusChange(ctx, guildID, id, row, err)
}

func toTicket(r dbq.Ticket, hasTranscript bool) tickets.Ticket {
	t := tickets.Ticket{
		ID: r.ID, GuildID: snowflake.ID(r.GuildID), Number: int(r.Number), PanelID: r.PanelID, CategoryID: r.CategoryID,
		CategoryLabel: r.CategoryLabel, Mode: tickets.Mode(r.Mode), ChannelID: snowflake.ID(r.ChannelID.Int64),
		OpenerID: snowflake.ID(r.OpenerID), OpenerName: r.OpenerName, ClaimedBy: snowflake.ID(r.ClaimedBy.Int64),
		ClaimedByName: r.ClaimedByName, Status: tickets.Status(r.Status), CloseReason: r.CloseReason,
		ClosedBy: snowflake.ID(r.ClosedBy.Int64), ClosedByName: r.ClosedByName, CreatedAt: r.CreatedAt, HasTranscript: hasTranscript,
	}
	if r.ClosedAt.Valid {
		at := r.ClosedAt.Time
		t.ClosedAt = &at
	}
	return t
}

// ---- transcripts ---------------------------------------------------------------

func (s *Store) SaveTranscript(ctx context.Context, t tickets.Transcript) error {
	return s.q.UpsertTranscript(ctx, dbq.UpsertTranscriptParams{
		TicketID: t.TicketID, GuildID: int64(t.GuildID), HtmlGzip: t.HTMLGzip, MessageCount: int32(t.Messages), ExpiresAt: t.ExpiresAt,
	})
}

func (s *Store) GetTranscript(ctx context.Context, guildID snowflake.ID, ticketID int64) (tickets.Transcript, error) {
	r, err := s.q.GetTranscript(ctx, dbq.GetTranscriptParams{GuildID: int64(guildID), TicketID: ticketID})
	if errors.Is(err, pgx.ErrNoRows) {
		return tickets.Transcript{}, tickets.ErrNoTranscript
	}
	if err != nil {
		return tickets.Transcript{}, err
	}
	return tickets.Transcript{TicketID: r.TicketID, GuildID: snowflake.ID(r.GuildID), HTMLGzip: r.HtmlGzip,
		Messages: int(r.MessageCount), ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt}, nil
}

func (s *Store) PurgeTranscripts(ctx context.Context, before time.Time) (int64, error) {
	return s.q.PurgeTranscripts(ctx, before)
}
