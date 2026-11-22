package guardian

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/moderation"
)

// ---- store ------------------------------------------------------------------

type fakeStore struct {
	mu          sync.Mutex
	events      []Event
	quarantines map[[2]snowflake.ID]Quarantine
	raids       []Raid
	locks       []LockState
	feeds       map[string][]string
	feedAt      map[string]time.Time
	now         func() time.Time
}

func newFakeStore(now func() time.Time) *fakeStore {
	return &fakeStore{quarantines: map[[2]snowflake.ID]Quarantine{}, feeds: map[string][]string{}, feedAt: map[string]time.Time{}, now: now}
}

func (s *fakeStore) CreateEvent(_ context.Context, e NewEvent) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := Event{
		ID: int64(len(s.events) + 1), GuildID: e.GuildID, UserID: e.UserID, UserName: e.UserName, Kind: e.Kind,
		Signals: e.Signals, Score: e.Score, Response: e.Response, Evidence: e.Evidence, CaseNumber: e.CaseNumber,
		Status: e.Status, CreatedAt: s.now(),
	}
	s.events = append(s.events, ev)
	return ev, nil
}

func (s *fakeStore) find(g snowflake.ID, id int64) (*Event, error) {
	for i := range s.events {
		if s.events[i].GuildID == g && s.events[i].ID == id {
			return &s.events[i], nil
		}
	}
	return nil, ErrEventNotFound
}

func (s *fakeStore) GetEvent(_ context.Context, g snowflake.ID, id int64) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.find(g, id)
	if err != nil {
		return Event{}, err
	}
	return *e, nil
}

func (s *fakeStore) ListEvents(_ context.Context, g snowflake.ID, f EventFilter) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Event
	for i := len(s.events) - 1; i >= 0; i-- {
		e := s.events[i]
		if e.GuildID == g && (f.Status == "" || e.Status == f.Status) && (f.Before == 0 || e.ID < f.Before) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *fakeStore) SetEventAlert(_ context.Context, g snowflake.ID, id int64, ch, msg snowflake.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.find(g, id)
	if err == nil {
		e.AlertChannelID, e.AlertMessageID = ch, msg
	}
	return err
}

func (s *fakeStore) ResolveEvent(_ context.Context, g snowflake.ID, id int64, st Status, by snowflake.ID, name string) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.find(g, id)
	if err != nil {
		return Event{}, err
	}
	if e.Status != StatusOpen && e.Status != StatusKept {
		return *e, ErrAlreadyResolved
	}
	now := s.now()
	e.Status, e.ResolvedBy, e.ResolvedByName, e.ResolvedAt = st, by, name, &now
	return *e, nil
}

func (s *fakeStore) SetEventStatus(_ context.Context, g snowflake.ID, id int64, st Status, by snowflake.ID, name string) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.find(g, id)
	if err != nil {
		return Event{}, err
	}
	e.Status, e.ResolvedBy, e.ResolvedByName = st, by, name
	return *e, nil
}

func (s *fakeStore) PurgeEventsBefore(context.Context, time.Time) (int64, error) { return 0, nil }

func (s *fakeStore) SaveQuarantine(_ context.Context, q Quarantine) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quarantines[[2]snowflake.ID{q.GuildID, q.UserID}] = q
	return nil
}

func (s *fakeStore) ActiveQuarantine(_ context.Context, g, u snowflake.ID) (Quarantine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.quarantines[[2]snowflake.ID{g, u}]
	if !ok {
		return Quarantine{}, ErrNotQuarantined
	}
	return q, nil
}

func (s *fakeStore) ReleaseQuarantine(_ context.Context, g, u snowflake.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := [2]snowflake.ID{g, u}
	if _, ok := s.quarantines[k]; !ok {
		return ErrNotQuarantined
	}
	delete(s.quarantines, k)
	return nil
}

func (s *fakeStore) activeRaid(g snowflake.ID) *Raid {
	for i := range s.raids {
		if s.raids[i].GuildID == g && s.raids[i].EndedAt == nil {
			return &s.raids[i]
		}
	}
	return nil
}

func (s *fakeStore) StartRaid(_ context.Context, r Raid) (Raid, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a := s.activeRaid(r.GuildID); a != nil {
		return *a, false, nil
	}
	r.ID = int64(len(s.raids) + 1)
	s.raids = append(s.raids, r)
	return r, true, nil
}

func (s *fakeStore) UpdateRaid(_ context.Context, g snowflake.ID, id, eventID int64, st RaidState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.raids {
		if s.raids[i].ID == id {
			s.raids[i].EventID, s.raids[i].State = eventID, st
		}
	}
	return nil
}

func (s *fakeStore) ActiveRaid(_ context.Context, g snowflake.ID) (Raid, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a := s.activeRaid(g); a != nil {
		return *a, nil
	}
	return Raid{}, ErrNoRaid
}

func (s *fakeStore) EndRaid(_ context.Context, g, by snowflake.ID) (Raid, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.activeRaid(g)
	if a == nil {
		return Raid{}, ErrNoRaid
	}
	now := s.now()
	a.EndedAt, a.EndedBy = &now, by
	return *a, nil
}

func (s *fakeStore) DueRaids(_ context.Context, now time.Time) ([]Raid, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Raid
	for _, r := range s.raids {
		if r.EndedAt == nil && !r.EndsAt.After(now) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *fakeStore) SaveLockdowns(_ context.Context, st []LockState) error {
	s.locks = append(s.locks, st...)
	return nil
}
func (s *fakeStore) Lockdowns(context.Context, snowflake.ID) ([]LockState, error) { return s.locks, nil }
func (s *fakeStore) DeleteLockdowns(context.Context, snowflake.ID, []snowflake.ID) error {
	s.locks = nil
	return nil
}

func (s *fakeStore) ReplaceFeedDomains(_ context.Context, src string, d []string) (int, error) {
	s.feeds[src], s.feedAt[src] = d, s.now()
	return len(d), nil
}

func (s *fakeStore) ScamDomains(context.Context) ([]string, error) {
	var out []string
	for _, d := range s.feeds {
		out = append(out, d...)
	}
	return out, nil
}

func (s *fakeStore) FeedUpdatedAt(_ context.Context, src string) (time.Time, error) {
	return s.feedAt[src], nil
}

// ---- platform ---------------------------------------------------------------

type fakePlatform struct {
	mu           sync.Mutex
	members      map[snowflake.ID]*Member
	bot          Member
	deleted      []MessageRef
	timeouts     map[snowflake.ID]time.Time
	kicked       []snowflake.ID
	invitesUntil time.Time
	verification int
	channels     []Channel
	failDelete   bool
}

func (p *fakePlatform) GuildName(snowflake.ID) string { return "Test Server" }

func (p *fakePlatform) Member(_ context.Context, _, u snowflake.ID) (Member, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.members[u]
	if !ok {
		return Member{}, moderation.ErrNotMember
	}
	out := *m
	out.RoleIDs = slices.Clone(m.RoleIDs)
	if t, ok := p.timeouts[u]; ok {
		out.TimedOutUntil = &t
	}
	return out, nil
}

func (p *fakePlatform) BotMember(context.Context, snowflake.ID) (Member, error) { return p.bot, nil }

func (p *fakePlatform) DeleteMessages(_ context.Context, ch snowflake.ID, ids []snowflake.ID, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failDelete {
		return fmt.Errorf("missing access")
	}
	for _, id := range ids {
		p.deleted = append(p.deleted, MessageRef{ChannelID: ch, MessageID: id})
	}
	return nil
}

func (p *fakePlatform) Timeout(_ context.Context, _, u snowflake.ID, until time.Time, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.timeouts[u] = until
	return nil
}

func (p *fakePlatform) RemoveTimeout(_ context.Context, _, u snowflake.ID, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.timeouts, u)
	return nil
}

func (p *fakePlatform) SetRoles(_ context.Context, _, u snowflake.ID, roles []snowflake.ID, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.members[u].RoleIDs = slices.Clone(roles)
	return nil
}

func (p *fakePlatform) Kick(_ context.Context, _, u snowflake.ID, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.kicked = append(p.kicked, u)
	delete(p.members, u)
	return nil
}

func (p *fakePlatform) PauseInvites(_ context.Context, _ snowflake.ID, until time.Time) error {
	p.invitesUntil = until
	return nil
}

func (p *fakePlatform) VerificationLevel(context.Context, snowflake.ID) (int, error) {
	return p.verification, nil
}

func (p *fakePlatform) SetVerificationLevel(_ context.Context, _ snowflake.ID, l int, _ string) error {
	p.verification = l
	return nil
}

func (p *fakePlatform) TextChannels(context.Context, snowflake.ID) ([]Channel, error) {
	return slices.Clone(p.channels), nil
}

func (p *fakePlatform) SetSlowmode(_ context.Context, ch snowflake.ID, sec int, _ string) error {
	for i := range p.channels {
		if p.channels[i].ID == ch {
			p.channels[i].Slowmode = sec
		}
	}
	return nil
}

// ---- alerts and moderation --------------------------------------------------

type fakeAlerts struct {
	mu     sync.Mutex
	dms    []int64
	posted []int64
	edited []Event
}

func (a *fakeAlerts) RecoveryDM(_ context.Context, ev Event, _ string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dms = append(a.dms, ev.ID)
	return nil
}

func (a *fakeAlerts) PostAlert(_ context.Context, _ snowflake.ID, ev Event) (snowflake.ID, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.posted = append(a.posted, ev.ID)
	return snowflake.ID(9000 + ev.ID), nil
}

func (a *fakeAlerts) EditAlert(_ context.Context, _, _ snowflake.ID, ev Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.edited = append(a.edited, ev)
	return nil
}

type fakeModeration struct {
	mu       sync.Mutex
	cases    []moderation.NewCase
	executed []moderation.Request
}

func (m *fakeModeration) Record(_ context.Context, nc moderation.NewCase) (moderation.Case, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cases = append(m.cases, nc)
	return moderation.Case{Number: len(m.cases)}, nil
}

func (m *fakeModeration) Execute(_ context.Context, r moderation.Request) (moderation.Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executed = append(m.executed, r)
	return moderation.Outcome{}, nil
}

// ---- harness ----------------------------------------------------------------

type harness struct {
	now      time.Time
	store    *fakeStore
	platform *fakePlatform
	alerts   *fakeAlerts
	mod      *fakeModeration
	tracker  *MemoryTracker
	settings Settings
	enabled  bool
	svc      *Service
}

const testGuild snowflake.ID = 1000

func newHarness(s Settings) *harness {
	h := &harness{now: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC), settings: s, enabled: true}
	clock := func() time.Time { return h.now }
	h.store = newFakeStore(clock)
	h.platform = &fakePlatform{
		members:  map[snowflake.ID]*Member{},
		timeouts: map[snowflake.ID]time.Time{},
		bot:      Member{Rank: core.Rank{ID: 1, TopRolePosition: 50}, Name: "Niro"},
	}
	h.alerts = &fakeAlerts{}
	h.mod = &fakeModeration{}
	h.tracker = NewMemoryTracker(clock)
	h.svc = NewService(Deps{
		Store: h.store, Platform: h.platform, Alerts: h.alerts, Tracker: h.tracker, Moderation: h.mod,
		Blocklist: NewDomainSet(),
		Settings: func(context.Context, snowflake.ID) (Settings, bool, error) {
			return h.settings, h.enabled, nil
		},
		FallbackAlertChannel: func(context.Context, snowflake.ID) snowflake.ID { return 77 },
		Log:                  discardLogger(),
		Now:                  clock,
	})
	return h
}

// addMember creates a member whose account is ageDays old and who joined joinedDaysAgo.
func (h *harness) addMember(name string, ageDays, rolePos int, roles ...snowflake.ID) snowflake.ID {
	id := snowflake.New(h.now.Add(-time.Duration(ageDays) * 24 * time.Hour))
	for h.platform.members[id] != nil {
		id++
	}
	h.platform.members[id] = &Member{Rank: core.Rank{ID: id, TopRolePosition: rolePos}, Name: name, RoleIDs: roles}
	return id
}
