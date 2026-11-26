package tickets

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"
)

// Actor is a member acting on a ticket.
type Actor struct {
	ID      snowflake.ID
	Name    string
	RoleIDs []snowflake.ID
	// Manager: the owner, an Administrator or someone with Manage Server;
	// managers count as staff in every category.
	Manager bool
}

// Announcement is a line Niro posts in a ticket.
type Announcement string

const (
	AnnounceClaimed  Announcement = "claimed"
	AnnounceClosed   Announcement = "closed"
	AnnounceReopened Announcement = "reopened"
)

// Platform does the chat-platform parts.
type Platform interface {
	PostPanel(ctx context.Context, p Panel) (snowflake.ID, error)
	EditPanel(ctx context.Context, p Panel) error // ErrMessageGone if deleted
	DeleteMessage(ctx context.Context, channelID, messageID snowflake.ID) error
	RoleExists(ctx context.Context, guildID, roleID snowflake.ID) bool

	// OpenThread creates a private thread in channelID for the ticket and
	// adds the opener (and staff it can find); OpenChannel creates a channel
	// only the opener, staff and Niro can see. Both return the new channel.
	OpenThread(ctx context.Context, t Ticket, channelID snowflake.ID, staff []snowflake.ID) (snowflake.ID, error)
	OpenChannel(ctx context.Context, t Ticket, parentID snowflake.ID, staff []snowflake.ID) (snowflake.ID, error)
	// PostOpening posts the first message (category message, answers,
	// Claim and Close buttons), notifying the opener and staff.
	PostOpening(ctx context.Context, t Ticket, c Category, answers []Answer) error
	Announce(ctx context.Context, t Ticket, a Announcement, by Actor) error
	// Close hides the ticket from the opener (thread: archive and lock;
	// channel: remove their access); Reopen undoes it; Delete removes it.
	Close(ctx context.Context, t Ticket) error
	Reopen(ctx context.Context, t Ticket) error
	Delete(ctx context.Context, t Ticket) error
	AddMember(ctx context.Context, t Ticket, userID snowflake.ID) error
	RemoveMember(ctx context.Context, t Ticket, userID snowflake.ID) error
	// Messages returns the ticket's messages, oldest first, at most limit;
	// more is true if there were more.
	Messages(ctx context.Context, channelID snowflake.ID, limit int) (msgs []TranscriptMessage, more bool, err error)
	PostTranscript(ctx context.Context, channelID snowflake.ID, t Ticket, htmlGzip []byte) error
	ServerName(guildID snowflake.ID) string
	// MemberRoles are a member's role IDs; BotID is Niro's own user ID.
	MemberRoles(ctx context.Context, guildID, userID snowflake.ID) ([]snowflake.ID, error)
	BotID() snowflake.ID
}

// Answer is one answered question.
type Answer struct {
	Question string
	Answer   string
}

// Deps are the service's collaborators.
type Deps struct {
	Store    Store
	Platform Platform
	Settings func(ctx context.Context, guildID snowflake.ID) (s Settings, enabled bool, err error)
	Log      *slog.Logger
	Now      func() time.Time
}

// Service runs tickets.
type Service struct {
	d Deps
	// locks are striped by key (per member for opening, per ticket for
	// changes), so their number stays fixed however many tickets there are.
	locks [64]sync.Mutex
}

func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d}
}

func (s *Service) lock(key string) func() {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	m := &s.locks[h.Sum32()%uint32(len(s.locks))]
	m.Lock()
	return m.Unlock
}

func (s *Service) on(ctx context.Context, guildID snowflake.ID) (Settings, error) {
	st, on, err := s.d.Settings(ctx, guildID)
	if err != nil {
		return Settings{}, err
	}
	if !on {
		return Settings{}, ErrModuleOff
	}
	return st, nil
}

// ---- panels --------------------------------------------------------------------

// SavePanel creates (ID 0) or updates a panel and posts or edits its message.
func (s *Service) SavePanel(ctx context.Context, p Panel) (Panel, error) {
	if p.ID != 0 {
		old, err := s.d.Store.GetTicketPanel(ctx, p.GuildID, p.ID)
		if err != nil {
			return Panel{}, err
		}
		// Keep category IDs stable: buttons already posted carry them.
		for i := range p.Categories {
			if p.Categories[i].ID != "" && !slices.ContainsFunc(old.Categories, func(c Category) bool { return c.ID == p.Categories[i].ID }) {
				p.Categories[i].ID = ""
			}
		}
		p.MessageID = old.MessageID
		if old.ChannelID != p.ChannelID && old.MessageID != 0 {
			_ = s.d.Platform.DeleteMessage(ctx, old.ChannelID, old.MessageID)
			p.MessageID = 0
			if err := s.d.Store.SetTicketPanelMessage(ctx, p.GuildID, p.ID, old.ChannelID, 0); err != nil {
				return Panel{}, err
			}
		}
	} else {
		for i := range p.Categories {
			p.Categories[i].ID = ""
		}
	}
	if err := p.Validate(); err != nil {
		return Panel{}, err
	}
	for _, c := range p.Categories {
		for _, r := range c.StaffRoleIDs {
			if !s.d.Platform.RoleExists(ctx, p.GuildID, r) {
				return Panel{}, fmt.Errorf("%w: category %q: a staff role doesn't exist", ErrInvalid, c.Label)
			}
		}
	}
	if p.ID == 0 {
		n, err := s.d.Store.CountTicketPanels(ctx, p.GuildID)
		if err != nil {
			return Panel{}, err
		}
		if n >= MaxPanels {
			return Panel{}, ErrTooManyPanels
		}
		saved, err := s.d.Store.CreateTicketPanel(ctx, p)
		if err != nil {
			return Panel{}, err
		}
		posted, err := s.publish(ctx, saved)
		if err != nil {
			_ = s.d.Store.DeleteTicketPanel(ctx, p.GuildID, saved.ID)
			return Panel{}, err
		}
		return posted, nil
	}
	saved, err := s.d.Store.UpdateTicketPanel(ctx, p)
	if err != nil {
		return Panel{}, err
	}
	saved.MessageID = p.MessageID
	return s.publish(ctx, saved)
}

func (s *Service) publish(ctx context.Context, p Panel) (Panel, error) {
	if p.MessageID != 0 {
		err := s.d.Platform.EditPanel(ctx, p)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ErrMessageGone) {
			return p, err
		}
	}
	id, err := s.d.Platform.PostPanel(ctx, p)
	if err != nil {
		return p, err
	}
	p.MessageID = id
	return p, s.d.Store.SetTicketPanelMessage(ctx, p.GuildID, p.ID, p.ChannelID, id)
}

// DeletePanel removes a panel and its message (open tickets stay).
func (s *Service) DeletePanel(ctx context.Context, guildID snowflake.ID, id int64) error {
	p, err := s.d.Store.GetTicketPanel(ctx, guildID, id)
	if err != nil {
		return err
	}
	if p.MessageID != 0 {
		_ = s.d.Platform.DeleteMessage(ctx, p.ChannelID, p.MessageID)
	}
	return s.d.Store.DeleteTicketPanel(ctx, guildID, id)
}

// ---- opening -------------------------------------------------------------------

// Form returns the category to open (to show its questions first).
func (s *Service) Form(ctx context.Context, guildID snowflake.ID, panelID int64, categoryID string) (Category, error) {
	if _, err := s.on(ctx, guildID); err != nil {
		return Category{}, err
	}
	p, err := s.d.Store.GetTicketPanel(ctx, guildID, panelID)
	if err != nil {
		return Category{}, err
	}
	c, ok := p.Category(categoryID)
	if !ok {
		return Category{}, ErrCategoryRemoved
	}
	return c, nil
}

// LimitError: the member already has open tickets; Channels are where.
type LimitError struct{ Channels []snowflake.ID }

func (e *LimitError) Error() string { return ErrLimit.Error() }
func (e *LimitError) Unwrap() error { return ErrLimit }

// Open opens a ticket. answers are the form answers, in question order.
func (s *Service) Open(ctx context.Context, guildID snowflake.ID, panelID int64, categoryID string, opener Actor, answers []string) (Ticket, error) {
	if _, err := s.on(ctx, guildID); err != nil {
		return Ticket{}, err
	}
	p, err := s.d.Store.GetTicketPanel(ctx, guildID, panelID)
	if err != nil {
		return Ticket{}, err
	}
	c, ok := p.Category(categoryID)
	if !ok {
		return Ticket{}, ErrCategoryRemoved
	}
	answered, err := matchAnswers(c.Questions, answers)
	if err != nil {
		return Ticket{}, err
	}
	unlock := s.lock(fmt.Sprintf("open:%s:%s", guildID, opener.ID))
	defer unlock()
	n, err := s.d.Store.CountOpenTickets(ctx, guildID, opener.ID, panelID, categoryID)
	if err != nil {
		return Ticket{}, err
	}
	if n >= c.MaxOpen {
		open, _ := s.d.Store.ListTickets(ctx, guildID, Filter{Limit: 100})
		le := &LimitError{}
		for _, t := range open {
			if t.OpenerID == opener.ID && t.Status != StatusClosed && t.PanelID == panelID && t.CategoryID == categoryID {
				le.Channels = append(le.Channels, t.ChannelID)
			}
		}
		return Ticket{}, le
	}
	t, err := s.d.Store.CreateTicket(ctx, NewTicket{
		GuildID: guildID, PanelID: panelID, CategoryID: c.ID, CategoryLabel: c.Label, Mode: c.Mode,
		OpenerID: opener.ID, OpenerName: opener.Name,
	})
	if err != nil {
		return Ticket{}, err
	}
	var ch snowflake.ID
	if c.Mode == ModeThread {
		ch, err = s.d.Platform.OpenThread(ctx, t, p.ChannelID, c.StaffRoleIDs)
	} else {
		ch, err = s.d.Platform.OpenChannel(ctx, t, c.ParentID, c.StaffRoleIDs)
	}
	if err != nil {
		_ = s.d.Store.DeleteTicket(ctx, guildID, t.ID) // nothing was created: forget it
		return Ticket{}, fmt.Errorf("creating the ticket's %s: %w", c.Mode, err)
	}
	t.ChannelID = ch
	if err := s.d.Store.SetTicketChannel(ctx, guildID, t.ID, ch); err != nil {
		return t, err
	}
	if err := s.d.Platform.PostOpening(ctx, t, c, answered); err != nil {
		s.d.Log.WarnContext(ctx, "posting the ticket's first message failed", "guild_id", guildID, "ticket", t.Number, "err", err)
	}
	return t, nil
}

func matchAnswers(qs []Question, answers []string) ([]Answer, error) {
	if len(answers) != len(qs) {
		return nil, ErrBadAnswers
	}
	out := make([]Answer, len(qs))
	for i, q := range qs {
		a := strings.TrimSpace(answers[i])
		if (q.Required && a == "") || utf8.RuneCountInString(a) > MaxAnswer {
			return nil, ErrBadAnswers
		}
		out[i] = Answer{Question: q.Label, Answer: a}
	}
	return out, nil
}

// ---- lifecycle -----------------------------------------------------------------

// isStaff: a manager, or a member with one of the category's staff roles. If
// the panel or category is gone, only managers are staff.
func (s *Service) isStaff(ctx context.Context, t Ticket, a Actor) bool {
	if a.Manager {
		return true
	}
	p, err := s.d.Store.GetTicketPanel(ctx, t.GuildID, t.PanelID)
	if err != nil {
		return false
	}
	c, ok := p.Category(t.CategoryID)
	if !ok {
		return false
	}
	for _, r := range a.RoleIDs {
		if slices.Contains(c.StaffRoleIDs, r) {
			return true
		}
	}
	return false
}

// memberIsStaff: userID holds one of the category's staff roles (unknown
// counts as staff, so nobody is removed by mistake).
func (s *Service) memberIsStaff(ctx context.Context, t Ticket, userID snowflake.ID) bool {
	roles, err := s.d.Platform.MemberRoles(ctx, t.GuildID, userID)
	if err != nil {
		return false // not a member: nothing to protect
	}
	return s.isStaff(ctx, t, Actor{ID: userID, RoleIDs: roles})
}

// IsStaff reports whether a may manage ticket t.
func (s *Service) IsStaff(ctx context.Context, t Ticket, a Actor) bool { return s.isStaff(ctx, t, a) }

func (s *Service) ticket(ctx context.Context, guildID snowflake.ID, id int64) (Ticket, func(), error) {
	unlock := s.lock(fmt.Sprintf("ticket:%d", id))
	t, err := s.d.Store.GetTicket(ctx, guildID, id)
	if err != nil {
		unlock()
		return Ticket{}, nil, err
	}
	return t, unlock, nil
}

// ByChannel finds the ticket in a channel (for slash commands).
func (s *Service) ByChannel(ctx context.Context, guildID, channelID snowflake.ID) (Ticket, error) {
	return s.d.Store.GetTicketByChannel(ctx, guildID, channelID)
}

// Claim marks a ticket as taken by a staff member.
func (s *Service) Claim(ctx context.Context, guildID snowflake.ID, id int64, by Actor) (Ticket, error) {
	t, unlock, err := s.ticket(ctx, guildID, id)
	if err != nil {
		return Ticket{}, err
	}
	defer unlock()
	if !s.isStaff(ctx, t, by) {
		return Ticket{}, ErrNotStaff
	}
	t, err = s.d.Store.ClaimTicket(ctx, guildID, id, by.ID, by.Name)
	if err != nil {
		return Ticket{}, err
	}
	if err := s.d.Platform.Announce(ctx, t, AnnounceClaimed, by); err != nil {
		s.d.Log.WarnContext(ctx, "announcing claim failed", "err", err)
	}
	return t, nil
}

// CloseProblems are things that went wrong after a ticket was closed.
type CloseProblems struct {
	TranscriptFailed bool // the transcript couldn't be saved
	HideFailed       bool // the opener may still see the ticket
}

// Close closes a ticket (the opener or staff), stores its transcript and
// hides it from the opener.
func (s *Service) Close(ctx context.Context, guildID snowflake.ID, id int64, by Actor, reason string) (Ticket, CloseProblems, error) {
	t, p, err := s.close(ctx, guildID, id, by, reason)
	return t, p, err
}

func (s *Service) close(ctx context.Context, guildID snowflake.ID, id int64, by Actor, reason string) (Ticket, CloseProblems, error) {
	var problems CloseProblems
	st, err := s.on(ctx, guildID)
	if err != nil && !errors.Is(err, ErrModuleOff) {
		return Ticket{}, problems, err
	}
	if st.RetentionDays == 0 {
		st = DefaultSettings() // module turned off: open tickets can still be closed
	}
	t, unlock, err := s.ticket(ctx, guildID, id)
	if err != nil {
		return Ticket{}, problems, err
	}
	defer unlock()
	if by.ID != t.OpenerID && !s.isStaff(ctx, t, by) {
		return Ticket{}, problems, ErrNotStaff
	}
	if t.Status == StatusClosed {
		return Ticket{}, problems, ErrWrongStatus
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > MaxCloseReason {
		reason = string([]rune(reason)[:MaxCloseReason])
	}
	closed, err := s.d.Store.CloseTicket(ctx, guildID, id, by.ID, by.Name, reason)
	if err != nil {
		return Ticket{}, problems, err
	}
	if err := s.saveTranscript(ctx, st, closed); err != nil {
		problems.TranscriptFailed = true
		s.d.Log.WarnContext(ctx, "saving transcript failed", "guild_id", guildID, "ticket", t.Number, "err", err)
	} else {
		closed.HasTranscript = true
	}
	if err := s.d.Platform.Announce(ctx, closed, AnnounceClosed, by); err != nil {
		s.d.Log.WarnContext(ctx, "announcing close failed", "err", err)
	}
	if err := s.d.Platform.Close(ctx, closed); err != nil {
		problems.HideFailed = true
		s.d.Log.WarnContext(ctx, "hiding the closed ticket failed", "guild_id", guildID, "ticket", t.Number, "err", err)
	}
	return closed, problems, nil
}

func (s *Service) saveTranscript(ctx context.Context, st Settings, t Ticket) error {
	msgs, more, err := s.d.Platform.Messages(ctx, t.ChannelID, MaxTranscriptMessages)
	if err != nil {
		return err
	}
	now := s.d.Now()
	closedAt := now
	if t.ClosedAt != nil {
		closedAt = *t.ClosedAt
	}
	expires := now.Add(time.Duration(st.RetentionDays) * 24 * time.Hour)
	gz, err := RenderTranscript(TranscriptInfo{Server: s.d.Platform.ServerName(t.GuildID), Ticket: t, ClosedAt: closedAt, Truncated: more}, msgs, expires)
	if err != nil {
		return err
	}
	if err := s.d.Store.SaveTranscript(ctx, Transcript{TicketID: t.ID, GuildID: t.GuildID, HTMLGzip: gz, Messages: len(msgs), ExpiresAt: expires}); err != nil {
		return err
	}
	if st.TranscriptChannelID != 0 {
		if err := s.d.Platform.PostTranscript(ctx, st.TranscriptChannelID, t, gz); err != nil {
			s.d.Log.WarnContext(ctx, "posting transcript failed", "guild_id", t.GuildID, "err", err)
		}
	}
	return nil
}

// Reopen reopens a closed ticket (staff).
func (s *Service) Reopen(ctx context.Context, guildID snowflake.ID, id int64, by Actor) (Ticket, error) {
	if _, err := s.on(ctx, guildID); err != nil {
		return Ticket{}, err
	}
	t, unlock, err := s.ticket(ctx, guildID, id)
	if err != nil {
		return Ticket{}, err
	}
	defer unlock()
	if !s.isStaff(ctx, t, by) {
		return Ticket{}, ErrNotStaff
	}
	if t.Status != StatusClosed {
		return Ticket{}, ErrWrongStatus
	}
	if err := s.d.Platform.Reopen(ctx, t); err != nil {
		return Ticket{}, err
	}
	t, err = s.d.Store.ReopenTicket(ctx, guildID, id)
	if err != nil {
		return Ticket{}, err
	}
	if err := s.d.Platform.Announce(ctx, t, AnnounceReopened, by); err != nil {
		s.d.Log.WarnContext(ctx, "announcing reopen failed", "err", err)
	}
	return t, nil
}

// Delete removes a closed ticket's thread or channel (staff). The ticket
// record and its transcript stay until the transcript expires.
func (s *Service) Delete(ctx context.Context, guildID snowflake.ID, id int64, by Actor) error {
	t, unlock, err := s.ticket(ctx, guildID, id)
	if err != nil {
		return err
	}
	defer unlock()
	if !s.isStaff(ctx, t, by) {
		return ErrNotStaff
	}
	if t.Status != StatusClosed {
		return ErrWrongStatus
	}
	if !t.HasTranscript {
		// Deleting would lose the conversation: reopen and close again to
		// retry the transcript (or delete the channel by hand).
		return ErrNoTranscript
	}
	return s.d.Platform.Delete(ctx, t)
}

// AddMember adds someone to the ticket in channelID (staff).
func (s *Service) AddMember(ctx context.Context, guildID, channelID snowflake.ID, by Actor, userID snowflake.ID) (Ticket, error) {
	return s.member(ctx, guildID, channelID, by, func(t Ticket) error { return s.d.Platform.AddMember(ctx, t, userID) })
}

// RemoveMember removes someone (not the opener) from the ticket in channelID (staff).
func (s *Service) RemoveMember(ctx context.Context, guildID, channelID snowflake.ID, by Actor, userID snowflake.ID) (Ticket, error) {
	return s.member(ctx, guildID, channelID, by, func(t Ticket) error {
		if userID == t.OpenerID || userID == s.d.Platform.BotID() || s.memberIsStaff(ctx, t, userID) {
			return ErrProtectedMember
		}
		return s.d.Platform.RemoveMember(ctx, t, userID)
	})
}

func (s *Service) member(ctx context.Context, guildID, channelID snowflake.ID, by Actor, fn func(Ticket) error) (Ticket, error) {
	t, err := s.d.Store.GetTicketByChannel(ctx, guildID, channelID)
	if err != nil {
		return Ticket{}, err
	}
	if !s.isStaff(ctx, t, by) {
		return Ticket{}, ErrNotStaff
	}
	if t.Status == StatusClosed {
		return Ticket{}, ErrWrongStatus
	}
	return t, fn(t)
}

// PurgeExpired deletes transcripts past their retention.
func (s *Service) PurgeExpired(ctx context.Context) (int64, error) {
	return s.d.Store.PurgeTranscripts(ctx, s.d.Now())
}

// RunMaintenance purges expired transcripts every interval until ctx ends.
func (s *Service) RunMaintenance(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := s.PurgeExpired(ctx); err != nil {
			s.d.Log.WarnContext(ctx, "purging expired transcripts failed", "err", err)
		} else if n > 0 {
			s.d.Log.InfoContext(ctx, "expired transcripts deleted", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
