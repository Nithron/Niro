package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/tickets"
	dc "github.com/nithron/niro/internal/platform/discord"
)

// TicketReader is the read side of the ticket store; changes go through the
// bot (RPC).
type TicketReader interface {
	ListTicketPanels(ctx context.Context, guildID snowflake.ID) ([]tickets.Panel, error)
	GetTicketPanel(ctx context.Context, guildID snowflake.ID, id int64) (tickets.Panel, error)
	ListTickets(ctx context.Context, guildID snowflake.ID, f tickets.Filter) ([]tickets.Ticket, error)
	GetTicketByNumber(ctx context.Context, guildID snowflake.ID, number int) (tickets.Ticket, error)
	GetTranscript(ctx context.Context, guildID snowflake.ID, ticketID int64) (tickets.Transcript, error)
}

// transcriptCSP: a transcript page runs no scripts, loads nothing and is
// its own unique origin (sandbox); only inline styles are allowed.
const transcriptCSP = "sandbox; default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// ---- settings ------------------------------------------------------------------

func (h handlers) GetTicketSettings(ctx context.Context, req apigen.GetTicketSettingsRequestObject) (apigen.GetTicketSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	s, err := tickets.LoadSettings(ctx, h.config, gid)
	if err != nil {
		return nil, err
	}
	return apigen.GetTicketSettings200JSONResponse{RetentionDays: s.RetentionDays, TranscriptChannelId: s.TranscriptChannelID.String()}, nil
}

func (h handlers) UpdateTicketSettings(ctx context.Context, req apigen.UpdateTicketSettingsRequestObject) (apigen.UpdateTicketSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	ch, err := snowflake.Parse(req.Body.TranscriptChannelId)
	if err != nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid transcript channel.")
	}
	if ch != 0 {
		meta, err := h.guildMeta(ctx, gid)
		if err != nil {
			return nil, err
		}
		if !hasTextChannel(meta, ch) {
			return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "The transcript channel must be a text or announcement channel in this server.")
		}
	}
	raw, _ := json.Marshal(tickets.Settings{RetentionDays: req.Body.RetentionDays, TranscriptChannelID: ch})
	actor := actorID(ctx)
	saved, err := h.config.SetSettings(ctx, gid, core.ModuleTickets, raw, &actor, core.SourceDashboard)
	if errors.Is(err, core.ErrInvalidSettings) {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Transcripts are kept 1 to 365 days.")
	}
	if err != nil {
		return nil, err
	}
	var out tickets.Settings
	if err := json.Unmarshal(saved, &out); err != nil {
		return nil, err
	}
	return apigen.UpdateTicketSettings200JSONResponse{RetentionDays: out.RetentionDays, TranscriptChannelId: out.TranscriptChannelID.String()}, nil
}

// ---- panels --------------------------------------------------------------------

func toAPITicketPanel(p tickets.Panel) apigen.TicketPanel {
	out := apigen.TicketPanel{Id: ptr(p.ID), Name: p.Name, ChannelId: p.ChannelID.String(), Message: toAPIDocument(p.Message),
		Categories: make([]apigen.TicketCategory, len(p.Categories))}
	if p.MessageID != 0 {
		out.MessageId = ptr(p.MessageID.String())
	}
	if !p.UpdatedAt.IsZero() {
		out.UpdatedAt = ptr(p.UpdatedAt)
	}
	for i, c := range p.Categories {
		ac := apigen.TicketCategory{Id: ptr(c.ID), Label: c.Label, Mode: apigen.TicketCategoryMode(c.Mode), ParentId: c.ParentID.String(),
			StaffRoleIds: ids(c.StaffRoleIDs), Opening: toAPIDocument(c.Opening), MaxOpen: c.MaxOpen, Questions: []apigen.TicketQuestion{}}
		if c.Emoji != "" {
			ac.Emoji = ptr(c.Emoji)
		}
		for _, q := range c.Questions {
			aq := apigen.TicketQuestion{Label: q.Label, Long: q.Long, Required: q.Required}
			if q.Placeholder != "" {
				aq.Placeholder = ptr(q.Placeholder)
			}
			ac.Questions = append(ac.Questions, aq)
		}
		out.Categories[i] = ac
	}
	return out
}

func fromAPITicketPanel(gid snowflake.ID, a apigen.TicketPanel) (tickets.Panel, error) {
	bad := func(what string) error {
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid "+what+".")
	}
	p := tickets.Panel{GuildID: gid, Name: a.Name}
	var err error
	if p.ChannelID, err = snowflake.Parse(a.ChannelId); err != nil {
		return p, bad("channel")
	}
	for _, ac := range a.Categories {
		c := tickets.Category{ID: deref(ac.Id), Label: ac.Label, Emoji: deref(ac.Emoji), Mode: tickets.Mode(ac.Mode), MaxOpen: ac.MaxOpen}
		if c.ParentID, err = snowflake.Parse(ac.ParentId); err != nil {
			return p, bad("channel category")
		}
		for _, r := range ac.StaffRoleIds {
			id, err := snowflake.Parse(r)
			if err != nil {
				return p, bad("staff role")
			}
			c.StaffRoleIDs = append(c.StaffRoleIDs, id)
		}
		for _, q := range ac.Questions {
			c.Questions = append(c.Questions, tickets.Question{Label: q.Label, Placeholder: deref(q.Placeholder), Long: q.Long, Required: q.Required})
		}
		if c.Opening, err = fromAPIDocument(ac.Opening, message.Budget{Components: 3}); err != nil {
			return p, err
		}
		p.Categories = append(p.Categories, c)
	}
	if p.Message, err = fromAPIDocument(a.Message, message.Budget{Components: 1 + len(p.Categories)}); err != nil {
		return p, err
	}
	if err := p.Validate(); err != nil {
		if errors.Is(err, tickets.ErrInvalid) {
			return p, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, dc.ValidationMessage(err)+".")
		}
		return p, err
	}
	return p, nil
}

func (h handlers) checkTicketPanelRefs(ctx context.Context, gid snowflake.ID, p tickets.Panel) error {
	meta, err := h.guildMeta(ctx, gid)
	if err != nil {
		return err
	}
	bad := func(msg string) error { return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, msg) }
	if !hasTextChannel(meta, p.ChannelID) {
		return bad("The panel's channel must be a text or announcement channel in this server.")
	}
	announcement := slices.ContainsFunc(meta.Channels, func(m dc.MetaChannel) bool { return m.ID == p.ChannelID && m.Type == "announcement" })
	for _, c := range p.Categories {
		if announcement && c.Mode == tickets.ModeThread {
			return bad("\"" + c.Label + "\" opens private threads, which announcement channels can't have. Post the panel in a text channel or use private channels.")
		}
		for _, r := range c.StaffRoleIDs {
			if r == gid || !slices.ContainsFunc(meta.Roles, func(m dc.MetaRole) bool { return m.ID == r }) {
				return bad("A staff role of \"" + c.Label + "\" doesn't exist in this server.")
			}
		}
		if c.ParentID != 0 && !slices.ContainsFunc(meta.Channels, func(m dc.MetaChannel) bool { return m.ID == c.ParentID && m.Type == "category" }) {
			return bad("The channel category of \"" + c.Label + "\" doesn't exist in this server.")
		}
	}
	return nil
}

func (h handlers) ListTicketPanels(ctx context.Context, req apigen.ListTicketPanelsRequestObject) (apigen.ListTicketPanelsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	list, err := h.tickets.ListTicketPanels(ctx, gid)
	if err != nil {
		return nil, err
	}
	out := apigen.TicketPanelList{Items: make([]apigen.TicketPanel, len(list))}
	for i, p := range list {
		out.Items[i] = toAPITicketPanel(p)
	}
	return apigen.ListTicketPanels200JSONResponse(out), nil
}

func (h handlers) saveTicketPanel(ctx context.Context, rawGuild string, id int64, body *apigen.TicketPanel) (tickets.Panel, error) {
	gid, err := h.managed(ctx, rawGuild)
	if err != nil {
		return tickets.Panel{}, err
	}
	if body == nil {
		return tickets.Panel{}, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	p, err := fromAPITicketPanel(gid, *body)
	if err != nil {
		return tickets.Panel{}, err
	}
	if id != 0 {
		if _, err := h.tickets.GetTicketPanel(ctx, gid, id); errors.Is(err, tickets.ErrPanelNotFound) {
			return tickets.Panel{}, httpErr(http.StatusNotFound, apigen.ErrorCodeNotFound, "Ticket panel not found.")
		} else if err != nil {
			return tickets.Panel{}, err
		}
	}
	if err := h.checkTicketPanelRefs(ctx, gid, p); err != nil {
		return tickets.Panel{}, err
	}
	p.ID = id
	var saved tickets.Panel
	if err := h.rpc.Call(ctx, dc.RPCTicketPanelSave, gid, actorID(ctx), p, &saved); err != nil {
		return tickets.Panel{}, rpcErr(err)
	}
	return saved, nil
}

func (h handlers) CreateTicketPanel(ctx context.Context, req apigen.CreateTicketPanelRequestObject) (apigen.CreateTicketPanelResponseObject, error) {
	p, err := h.saveTicketPanel(ctx, req.GuildId, 0, req.Body)
	if err != nil {
		return nil, err
	}
	return apigen.CreateTicketPanel201JSONResponse(toAPITicketPanel(p)), nil
}

func (h handlers) UpdateTicketPanel(ctx context.Context, req apigen.UpdateTicketPanelRequestObject) (apigen.UpdateTicketPanelResponseObject, error) {
	p, err := h.saveTicketPanel(ctx, req.GuildId, req.PanelId, req.Body)
	if err != nil {
		return nil, err
	}
	return apigen.UpdateTicketPanel200JSONResponse(toAPITicketPanel(p)), nil
}

func (h handlers) DeleteTicketPanel(ctx context.Context, req apigen.DeleteTicketPanelRequestObject) (apigen.DeleteTicketPanelResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if err := h.rpc.Call(ctx, dc.RPCTicketPanelDelete, gid, actorID(ctx), dc.RolePanelIDParams{ID: req.PanelId}, nil); err != nil {
		return nil, rpcErr(err)
	}
	return apigen.DeleteTicketPanel204Response{}, nil
}

// ---- tickets -------------------------------------------------------------------

func toAPITicket(t tickets.Ticket) apigen.Ticket {
	out := apigen.Ticket{Number: t.Number, CategoryLabel: t.CategoryLabel, Mode: apigen.TicketMode(t.Mode), OpenerId: t.OpenerID.String(),
		OpenerName: t.OpenerName, Status: apigen.TicketStatus(t.Status), CreatedAt: t.CreatedAt, HasTranscript: t.HasTranscript, ClosedAt: t.ClosedAt}
	if t.ChannelID != 0 {
		out.ChannelId = ptr(t.ChannelID.String())
	}
	if t.ClaimedByName != "" {
		out.ClaimedByName = ptr(t.ClaimedByName)
	}
	if t.CloseReason != "" {
		out.CloseReason = ptr(t.CloseReason)
	}
	if t.ClosedByName != "" {
		out.ClosedByName = ptr(t.ClosedByName)
	}
	return out
}

func (h handlers) ListTickets(ctx context.Context, req apigen.ListTicketsRequestObject) (apigen.ListTicketsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	limit := 25
	if req.Params.Limit != nil {
		limit = min(max(*req.Params.Limit, 1), 100)
	}
	f := tickets.Filter{Limit: limit + 1}
	if req.Params.Status != nil {
		f.Status = tickets.Status(*req.Params.Status)
	}
	if req.Params.Before != nil {
		f.Before = *req.Params.Before
	}
	list, err := h.tickets.ListTickets(ctx, gid, f)
	if err != nil {
		return nil, err
	}
	out := apigen.TicketList{Items: []apigen.Ticket{}}
	for i, t := range list {
		if i == limit {
			out.NextBefore = ptr(list[i-1].Number)
			break
		}
		out.Items = append(out.Items, toAPITicket(t))
	}
	return apigen.ListTickets200JSONResponse(out), nil
}

func (h handlers) GetTicketTranscript(ctx context.Context, req apigen.GetTicketTranscriptRequestObject) (apigen.GetTicketTranscriptResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	notFound := httpErr(http.StatusNotFound, apigen.ErrorCodeNotFound, "No transcript: the ticket isn't closed or its transcript expired.")
	t, err := h.tickets.GetTicketByNumber(ctx, gid, req.Number)
	if errors.Is(err, tickets.ErrNotFound) {
		return nil, notFound
	}
	if err != nil {
		return nil, err
	}
	tr, err := h.tickets.GetTranscript(ctx, gid, t.ID)
	if errors.Is(err, tickets.ErrNoTranscript) {
		return nil, notFound
	}
	if err != nil {
		return nil, err
	}
	html, err := tr.HTML()
	if err != nil {
		return nil, err
	}
	return apigen.GetTicketTranscript200TexthtmlResponse{
		Body: bytes.NewReader(html), ContentLength: int64(len(html)),
		Headers: apigen.GetTicketTranscript200ResponseHeaders{ContentSecurityPolicy: ptr(transcriptCSP)},
	}, nil
}
