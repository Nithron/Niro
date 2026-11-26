package tickets

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
)

const (
	guild snowflake.ID = 1
	panCh snowflake.ID = 50
	staff snowflake.ID = 60
	user  snowflake.ID = 100
	mod   snowflake.ID = 200
)

// ---- in-memory store ----------------------------------------------------------

type memStore struct {
	mu          sync.Mutex
	panels      map[int64]Panel
	tickets     map[int64]Ticket
	transcripts map[int64]Transcript
	nextPanel   int64
	nextTicket  int64
	numbers     map[snowflake.ID]int
}

func newMemStore() *memStore {
	return &memStore{panels: map[int64]Panel{}, tickets: map[int64]Ticket{}, transcripts: map[int64]Transcript{}, numbers: map[snowflake.ID]int{}}
}

func (m *memStore) CreateTicketPanel(_ context.Context, p Panel) (Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextPanel++
	p.ID = m.nextPanel
	m.panels[p.ID] = p
	return p, nil
}

func (m *memStore) UpdateTicketPanel(_ context.Context, p Panel) (Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.panels[p.ID] = p
	return p, nil
}

func (m *memStore) GetTicketPanel(_ context.Context, g snowflake.ID, id int64) (Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.panels[id]
	if !ok || p.GuildID != g {
		return Panel{}, ErrPanelNotFound
	}
	return p, nil
}

func (m *memStore) ListTicketPanels(_ context.Context, g snowflake.ID) ([]Panel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Panel
	for _, p := range m.panels {
		if p.GuildID == g {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memStore) CountTicketPanels(ctx context.Context, g snowflake.ID) (int, error) {
	l, _ := m.ListTicketPanels(ctx, g)
	return len(l), nil
}

func (m *memStore) DeleteTicketPanel(_ context.Context, _ snowflake.ID, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.panels, id)
	return nil
}

func (m *memStore) SetTicketPanelMessage(_ context.Context, _ snowflake.ID, id int64, ch, msg snowflake.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.panels[id]
	p.ChannelID, p.MessageID = ch, msg
	m.panels[id] = p
	return nil
}

func (m *memStore) CreateTicket(_ context.Context, n NewTicket) (Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextTicket++
	m.numbers[n.GuildID]++
	t := Ticket{ID: m.nextTicket, GuildID: n.GuildID, Number: m.numbers[n.GuildID], PanelID: n.PanelID, CategoryID: n.CategoryID,
		CategoryLabel: n.CategoryLabel, Mode: n.Mode, OpenerID: n.OpenerID, OpenerName: n.OpenerName, Status: StatusOpen, CreatedAt: time.Now()}
	m.tickets[t.ID] = t
	return t, nil
}

func (m *memStore) DeleteTicket(_ context.Context, _ snowflake.ID, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tickets, id)
	return nil
}

func (m *memStore) SetTicketChannel(_ context.Context, _ snowflake.ID, id int64, ch snowflake.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tickets[id]
	t.ChannelID = ch
	m.tickets[id] = t
	return nil
}

func (m *memStore) GetTicket(_ context.Context, g snowflake.ID, id int64) (Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[id]
	if !ok || t.GuildID != g {
		return Ticket{}, ErrNotFound
	}
	_, t.HasTranscript = m.transcripts[id]
	return t, nil
}

func (m *memStore) GetTicketByNumber(_ context.Context, g snowflake.ID, n int) (Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tickets {
		if t.GuildID == g && t.Number == n {
			return t, nil
		}
	}
	return Ticket{}, ErrNotFound
}

func (m *memStore) GetTicketByChannel(_ context.Context, g, ch snowflake.ID) (Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tickets {
		if t.GuildID == g && t.ChannelID == ch {
			return t, nil
		}
	}
	return Ticket{}, ErrNotFound
}

func (m *memStore) ListTickets(_ context.Context, g snowflake.ID, f Filter) ([]Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Ticket
	for _, t := range m.tickets {
		if t.GuildID == g && (f.Status == "" || t.Status == f.Status) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) CountOpenTickets(_ context.Context, g, opener snowflake.ID, panel int64, cat string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, t := range m.tickets {
		if t.GuildID == g && t.OpenerID == opener && t.PanelID == panel && t.CategoryID == cat && t.Status != StatusClosed {
			n++
		}
	}
	return n, nil
}

func (m *memStore) update(g snowflake.ID, id int64, allowed []Status, fn func(*Ticket)) (Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[id]
	if !ok || t.GuildID != g {
		return Ticket{}, ErrNotFound
	}
	if !slices.Contains(allowed, t.Status) {
		return Ticket{}, ErrWrongStatus
	}
	fn(&t)
	m.tickets[id] = t
	return t, nil
}

func (m *memStore) ClaimTicket(_ context.Context, g snowflake.ID, id int64, by snowflake.ID, name string) (Ticket, error) {
	return m.update(g, id, []Status{StatusOpen}, func(t *Ticket) { t.Status, t.ClaimedBy, t.ClaimedByName = StatusClaimed, by, name })
}

func (m *memStore) CloseTicket(_ context.Context, g snowflake.ID, id int64, by snowflake.ID, name, reason string) (Ticket, error) {
	return m.update(g, id, []Status{StatusOpen, StatusClaimed}, func(t *Ticket) {
		now := time.Now()
		t.Status, t.ClosedBy, t.ClosedByName, t.CloseReason, t.ClosedAt = StatusClosed, by, name, reason, &now
	})
}

func (m *memStore) ReopenTicket(_ context.Context, g snowflake.ID, id int64) (Ticket, error) {
	return m.update(g, id, []Status{StatusClosed}, func(t *Ticket) { t.Status, t.ClosedAt = StatusOpen, nil })
}

func (m *memStore) SaveTranscript(_ context.Context, tr Transcript) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transcripts[tr.TicketID] = tr
	return nil
}

func (m *memStore) GetTranscript(_ context.Context, g snowflake.ID, id int64) (Transcript, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr, ok := m.transcripts[id]
	if !ok || tr.GuildID != g {
		return Transcript{}, ErrNoTranscript
	}
	return tr, nil
}

func (m *memStore) PurgeTranscripts(_ context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, tr := range m.transcripts {
		if tr.ExpiresAt.Before(before) {
			delete(m.transcripts, id)
			n++
		}
	}
	return n, nil
}

// ---- fake platform ------------------------------------------------------------

type fakePlatform struct {
	mu        sync.Mutex
	nextCh    snowflake.ID
	openErr   error
	openings  []Answer
	announced []Announcement
	closed    []int64
	reopened  []int64
	deleted   []int64
	added     []snowflake.ID
	posted    []snowflake.ID
	messages  []TranscriptMessage
}

func (f *fakePlatform) PostPanel(context.Context, Panel) (snowflake.ID, error) { return 900, nil }
func (f *fakePlatform) EditPanel(context.Context, Panel) error                 { return nil }
func (f *fakePlatform) DeleteMessage(context.Context, snowflake.ID, snowflake.ID) error {
	return nil
}
func (f *fakePlatform) RoleExists(_ context.Context, _, r snowflake.ID) bool { return r == staff }
func (f *fakePlatform) open() (snowflake.ID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.openErr != nil {
		return 0, f.openErr
	}
	f.nextCh++
	return 700 + f.nextCh, nil
}
func (f *fakePlatform) OpenThread(context.Context, Ticket, snowflake.ID, []snowflake.ID) (snowflake.ID, error) {
	return f.open()
}
func (f *fakePlatform) OpenChannel(context.Context, Ticket, snowflake.ID, []snowflake.ID) (snowflake.ID, error) {
	return f.open()
}
func (f *fakePlatform) PostOpening(_ context.Context, _ Ticket, _ Category, a []Answer) error {
	f.openings = append(f.openings, a...)
	return nil
}
func (f *fakePlatform) Announce(_ context.Context, _ Ticket, a Announcement, _ Actor) error {
	f.announced = append(f.announced, a)
	return nil
}
func (f *fakePlatform) Close(_ context.Context, t Ticket) error {
	f.closed = append(f.closed, t.ID)
	return nil
}
func (f *fakePlatform) Reopen(_ context.Context, t Ticket) error {
	f.reopened = append(f.reopened, t.ID)
	return nil
}
func (f *fakePlatform) Delete(_ context.Context, t Ticket) error {
	f.deleted = append(f.deleted, t.ID)
	return nil
}
func (f *fakePlatform) AddMember(_ context.Context, _ Ticket, u snowflake.ID) error {
	f.added = append(f.added, u)
	return nil
}
func (f *fakePlatform) RemoveMember(context.Context, Ticket, snowflake.ID) error { return nil }
func (f *fakePlatform) Messages(context.Context, snowflake.ID, int) ([]TranscriptMessage, bool, error) {
	return f.messages, false, nil
}
func (f *fakePlatform) PostTranscript(_ context.Context, ch snowflake.ID, _ Ticket, _ []byte) error {
	f.posted = append(f.posted, ch)
	return nil
}
func (f *fakePlatform) ServerName(snowflake.ID) string { return "Test <Server>" }
func (f *fakePlatform) BotID() snowflake.ID            { return 999 }
func (f *fakePlatform) MemberRoles(_ context.Context, _, u snowflake.ID) ([]snowflake.ID, error) {
	if u == mod {
		return []snowflake.ID{staff}, nil
	}
	return nil, nil
}

type harness struct {
	svc *Service
	st  *memStore
	pf  *fakePlatform
	set Settings
	on  bool
	now time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{st: newMemStore(), pf: &fakePlatform{}, set: DefaultSettings(), on: true, now: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)}
	h.svc = NewService(Deps{
		Store: h.st, Platform: h.pf, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return h.now },
		Settings: func(context.Context, snowflake.ID) (Settings, bool, error) { return h.set, h.on, nil },
	})
	return h
}

func testPanel(cats ...Category) Panel {
	if len(cats) == 0 {
		cats = []Category{{Label: "Support", Mode: ModeThread, StaffRoleIDs: []snowflake.ID{staff}, Opening: message.Simple("Staff will be with you soon.")}}
	}
	return Panel{GuildID: guild, Name: "Help", ChannelID: panCh, Message: message.Simple("Need help?"), Categories: cats}
}

func (h *harness) panel(t *testing.T, p Panel) Panel {
	t.Helper()
	saved, err := h.svc.SavePanel(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

var (
	opener   = Actor{ID: user, Name: "ana"}
	staffer  = Actor{ID: mod, Name: "mo", RoleIDs: []snowflake.ID{staff}}
	stranger = Actor{ID: 300, Name: "x"}
	manager  = Actor{ID: 400, Name: "owner", Manager: true}
)

// ---- tests --------------------------------------------------------------------

func TestPanelValidation(t *testing.T) {
	good := testPanel()
	if err := good.Validate(); err != nil || good.Categories[0].ID == "" || good.Categories[0].MaxOpen != 1 {
		t.Fatalf("good panel: %v %+v", err, good.Categories[0])
	}
	bad := map[string]func(p *Panel){
		"no categories": func(p *Panel) { p.Categories = nil },
		"no staff":      func(p *Panel) { p.Categories[0].StaffRoleIDs = nil },
		"bad mode":      func(p *Panel) { p.Categories[0].Mode = "dm" },
		"long question": func(p *Panel) { p.Categories[0].Questions = []Question{{Label: strings.Repeat("q", 46)}} },
		"6 questions":   func(p *Panel) { p.Categories[0].Questions = make([]Question, 6) },
		"max open":      func(p *Panel) { p.Categories[0].MaxOpen = 9 },
		"6 categories": func(p *Panel) {
			for range 5 {
				p.Categories = append(p.Categories, p.Categories[0])
			}
		},
	}
	for name, edit := range bad {
		p := testPanel()
		edit(&p)
		if err := p.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestPanelSaveKeepsCategoryIDs(t *testing.T) {
	h := newHarness(t)
	p := h.panel(t, testPanel())
	id := p.Categories[0].ID
	p.Categories = append(p.Categories, Category{ID: "forged", Label: "Billing", Mode: ModeChannel, StaffRoleIDs: []snowflake.ID{staff}, Opening: message.Simple("x")})
	p2, err := h.svc.SavePanel(context.Background(), p)
	if err != nil || p2.Categories[0].ID != id || p2.Categories[1].ID == "forged" || p2.Categories[1].ID == "" {
		t.Errorf("ids: %v %+v", err, p2.Categories)
	}
	bad := testPanel()
	bad.Categories[0].StaffRoleIDs = []snowflake.ID{999}
	if _, err := h.svc.SavePanel(context.Background(), bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown staff role: %v", err)
	}
}

func TestOpenClaimCloseReopenDelete(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.set.TranscriptChannelID = 77
	q := testPanel(Category{Label: "Report", Mode: ModeChannel, StaffRoleIDs: []snowflake.ID{staff}, Opening: message.Simple("Thanks"),
		Questions: []Question{{Label: "Who?", Required: true}, {Label: "Details", Long: true}}})
	p := h.panel(t, q)
	cat := p.Categories[0].ID

	if _, err := h.svc.Open(ctx, guild, p.ID, cat, opener, []string{""}); !errors.Is(err, ErrBadAnswers) {
		t.Errorf("missing answers: %v", err)
	}
	if _, err := h.svc.Open(ctx, guild, p.ID, cat, opener, []string{"  ", "x"}); !errors.Is(err, ErrBadAnswers) {
		t.Errorf("required answer blank: %v", err)
	}
	tk, err := h.svc.Open(ctx, guild, p.ID, cat, opener, []string{"bob", ""})
	if err != nil || tk.Number != 1 || tk.ChannelID == 0 || tk.Mode != ModeChannel || len(h.pf.openings) != 2 {
		t.Fatalf("open: %+v %v", tk, err)
	}
	var le *LimitError
	if _, err := h.svc.Open(ctx, guild, p.ID, cat, opener, []string{"bob", ""}); !errors.As(err, &le) || len(le.Channels) != 1 {
		t.Errorf("second ticket: %v", err)
	}

	if _, err := h.svc.Claim(ctx, guild, tk.ID, opener); !errors.Is(err, ErrNotStaff) {
		t.Errorf("opener claimed: %v", err)
	}
	if c, err := h.svc.Claim(ctx, guild, tk.ID, staffer); err != nil || c.Status != StatusClaimed || c.ClaimedByName != "mo" {
		t.Errorf("claim: %+v %v", c, err)
	}
	if _, _, err := h.svc.Close(ctx, guild, tk.ID, stranger, ""); !errors.Is(err, ErrNotStaff) {
		t.Errorf("stranger closed: %v", err)
	}
	h.pf.messages = []TranscriptMessage{{AuthorName: "ana", Content: "<script>alert(1)</script>", At: h.now}}
	closed, probs, err := h.svc.Close(ctx, guild, tk.ID, opener, "solved")
	if err != nil || closed.Status != StatusClosed || closed.CloseReason != "solved" || !slices.Contains(h.pf.closed, tk.ID) || probs != (CloseProblems{}) {
		t.Fatalf("close: %+v %+v %v", closed, probs, err)
	}
	tr, err := h.st.GetTranscript(ctx, guild, tk.ID)
	if err != nil || !tr.ExpiresAt.Equal(h.now.Add(90*24*time.Hour)) || !slices.Equal(h.pf.posted, []snowflake.ID{77}) {
		t.Fatalf("transcript: %+v %v posted %v", tr, err, h.pf.posted)
	}
	html, _ := tr.HTML()
	page := string(html)
	if strings.Contains(page, "<script>alert") || !strings.Contains(page, "&lt;script&gt;alert(1)&lt;/script&gt;") ||
		!strings.Contains(page, "Test &lt;Server&gt;") || !strings.Contains(page, "solved") {
		t.Errorf("transcript not escaped or incomplete:\n%s", page)
	}
	if _, _, err := h.svc.Close(ctx, guild, tk.ID, staffer, ""); !errors.Is(err, ErrWrongStatus) {
		t.Errorf("closed twice: %v", err)
	}
	if _, err := h.svc.Reopen(ctx, guild, tk.ID, opener); !errors.Is(err, ErrNotStaff) {
		t.Errorf("opener reopened: %v", err)
	}
	if r, err := h.svc.Reopen(ctx, guild, tk.ID, staffer); err != nil || r.Status != StatusOpen {
		t.Errorf("reopen: %+v %v", r, err)
	}
	if err := h.svc.Delete(ctx, guild, tk.ID, staffer); !errors.Is(err, ErrWrongStatus) {
		t.Errorf("deleted an open ticket: %v", err)
	}
	_, _, _ = h.svc.Close(ctx, guild, tk.ID, manager, "")
	if err := h.svc.Delete(ctx, guild, tk.ID, manager); err != nil || !slices.Contains(h.pf.deleted, tk.ID) {
		t.Errorf("delete: %v", err)
	}
	want := []Announcement{AnnounceClaimed, AnnounceClosed, AnnounceReopened, AnnounceClosed}
	if !slices.Equal(h.pf.announced, want) {
		t.Errorf("announcements = %v", h.pf.announced)
	}
}

func TestOpenFailureLeavesNoTicket(t *testing.T) {
	h := newHarness(t)
	p := h.panel(t, testPanel())
	h.pf.openErr = errors.New("missing permissions")
	if _, err := h.svc.Open(context.Background(), guild, p.ID, p.Categories[0].ID, opener, nil); err == nil {
		t.Fatal("open succeeded")
	}
	if len(h.st.tickets) != 0 {
		t.Error("a ticket without a channel was kept")
	}
}

func TestConcurrentOpensRespectTheLimit(t *testing.T) {
	h := newHarness(t)
	p := h.panel(t, testPanel())
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { _, _ = h.svc.Open(context.Background(), guild, p.ID, p.Categories[0].ID, opener, nil) })
	}
	wg.Wait()
	if n := len(h.st.tickets); n != 1 {
		t.Errorf("%d tickets opened, limit is 1", n)
	}
}

func TestMembersAndModuleOff(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	p := h.panel(t, testPanel())
	tk, _ := h.svc.Open(ctx, guild, p.ID, p.Categories[0].ID, opener, nil)
	if _, err := h.svc.AddMember(ctx, guild, tk.ChannelID, opener, 5); !errors.Is(err, ErrNotStaff) {
		t.Errorf("opener added someone: %v", err)
	}
	if _, err := h.svc.AddMember(ctx, guild, tk.ChannelID, staffer, 5); err != nil || !slices.Contains(h.pf.added, 5) {
		t.Errorf("add: %v", err)
	}
	for _, who := range []snowflake.ID{user, mod, 999} { // opener, staff, Niro
		if _, err := h.svc.RemoveMember(ctx, guild, tk.ChannelID, staffer, who); !errors.Is(err, ErrProtectedMember) {
			t.Errorf("removed %s: %v", who, err)
		}
	}
	if _, err := h.svc.RemoveMember(ctx, guild, tk.ChannelID, staffer, 5); err != nil {
		t.Errorf("remove a helper: %v", err)
	}
	if _, err := h.svc.AddMember(ctx, 999, tk.ChannelID, manager, 5); !errors.Is(err, ErrNotFound) {
		t.Errorf("other guild: %v", err)
	}
	h.on = false
	if _, err := h.svc.Open(ctx, guild, p.ID, p.Categories[0].ID, opener, nil); !errors.Is(err, ErrModuleOff) {
		t.Errorf("module off: %v", err)
	}
	// Open tickets can still be closed with the module off.
	if _, _, err := h.svc.Close(ctx, guild, tk.ID, opener, ""); err != nil {
		t.Errorf("close with module off: %v", err)
	}
}

func TestPurge(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	_ = h.st.SaveTranscript(ctx, Transcript{TicketID: 1, GuildID: guild, ExpiresAt: h.now.Add(-time.Hour)})
	_ = h.st.SaveTranscript(ctx, Transcript{TicketID: 2, GuildID: guild, ExpiresAt: h.now.Add(time.Hour)})
	if n, err := h.svc.PurgeExpired(ctx); err != nil || n != 1 || len(h.st.transcripts) != 1 {
		t.Errorf("purge: %d %v", n, err)
	}
}

func TestSettings(t *testing.T) {
	for raw, ok := range map[string]bool{"": true, `{"retention_days":365}`: true, `{"retention_days":0}`: false, `{"retention_days":400}`: false, `{"x":1}`: false} {
		var in []byte
		if raw != "" {
			in = []byte(raw)
		}
		if _, err := (SettingsSpec{}).Normalize(in); (err == nil) != ok {
			t.Errorf("%q: %v", raw, err)
		}
	}
}

type failingMessages struct{ *fakePlatform }

func (failingMessages) Messages(context.Context, snowflake.ID, int) ([]TranscriptMessage, bool, error) {
	return nil, false, errors.New("missing access")
}

func TestFailedTranscriptBlocksDelete(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.svc.d.Platform = failingMessages{h.pf}
	p := h.panel(t, testPanel())
	tk, _ := h.svc.Open(ctx, guild, p.ID, p.Categories[0].ID, opener, nil)
	_, probs, err := h.svc.Close(ctx, guild, tk.ID, staffer, "")
	if err != nil || !probs.TranscriptFailed {
		t.Fatalf("close: %+v %v", probs, err)
	}
	if err := h.svc.Delete(ctx, guild, tk.ID, staffer); !errors.Is(err, ErrNoTranscript) {
		t.Errorf("deleted without a transcript: %v", err)
	}
}
