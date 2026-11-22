package core

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type fakeStore struct {
	mu        sync.Mutex
	rows      map[snowflake.ID]map[ModuleID]StoredModuleState
	listCalls int
	known     map[snowflake.ID]bool
}

func newFakeStore(known ...snowflake.ID) *fakeStore {
	s := &fakeStore{rows: map[snowflake.ID]map[ModuleID]StoredModuleState{}, known: map[snowflake.ID]bool{}}
	for _, id := range known {
		s.known[id] = true
	}
	return s
}

func (f *fakeStore) ListModuleStates(_ context.Context, g snowflake.ID) ([]StoredModuleState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	var out []StoredModuleState
	for _, r := range f.rows[g] {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeStore) SetModuleEnabled(_ context.Context, g snowflake.ID, m ModuleID, enabled bool, _ *snowflake.ID, _ ChangeSource) (StoredModuleState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.known[g] {
		return StoredModuleState{}, ErrUnknownGuild
	}
	if f.rows[g] == nil {
		f.rows[g] = map[ModuleID]StoredModuleState{}
	}
	r := f.rows[g][m]
	r.Module, r.Enabled, r.Version, r.UpdatedAt = m, enabled, r.Version+1, time.Unix(100, 0)
	f.rows[g][m] = r
	return r, nil
}

type fakeBus struct {
	mu        sync.Mutex
	published []ConfigChange
	subs      chan func(ConfigChange)
	pubErr    error
}

func newFakeBus() *fakeBus { return &fakeBus{subs: make(chan func(ConfigChange), 1)} }

func (b *fakeBus) PublishConfigChange(_ context.Context, c ConfigChange) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, c)
	return b.pubErr
}

func (b *fakeBus) SubscribeConfigChanges(ctx context.Context, fn func(ConfigChange)) error {
	b.subs <- fn
	<-ctx.Done()
	return ctx.Err()
}

func newService(t *testing.T, store ConfigStore, bus ChangeBus) *ConfigService {
	t.Helper()
	reg, err := NewRegistry(DefaultModules()...)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewConfigService(reg, store, bus, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const guild snowflake.ID = 42

func TestModuleStatesDefaults(t *testing.T) {
	s := newService(t, newFakeStore(guild), newFakeBus())
	states, err := s.ModuleStates(context.Background(), guild)
	if err != nil {
		t.Fatal(err)
	}
	want := map[ModuleID]bool{
		ModuleCore: true, ModuleModeration: true, ModuleGuardian: true,
		ModuleLogging: false, ModuleRoles: false, ModuleWelcome: false, ModuleTickets: false,
	}
	if len(states) != len(want) {
		t.Fatalf("got %d states, want %d", len(states), len(want))
	}
	for _, st := range states {
		if st.Enabled != want[st.Module.ID] || st.Version != 0 {
			t.Errorf("%s: enabled=%v version=%d, want enabled=%v version=0", st.Module.ID, st.Enabled, st.Version, want[st.Module.ID])
		}
	}
}

func TestSetEnabledPersistsInvalidatesAndPublishes(t *testing.T) {
	store, bus := newFakeStore(guild), newFakeBus()
	s := newService(t, store, bus)
	ctx := context.Background()
	actor := snowflake.ID(7)

	if on, _ := s.IsEnabled(ctx, guild, ModuleLogging); on {
		t.Fatal("logging should start disabled")
	}
	st, err := s.SetEnabled(ctx, guild, ModuleLogging, true, &actor, SourceDashboard)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Version != 1 {
		t.Fatalf("returned state = %+v", st)
	}
	// The local cache was invalidated, so the next read sees the new value.
	if on, _ := s.IsEnabled(ctx, guild, ModuleLogging); !on {
		t.Fatal("logging should be enabled after SetEnabled")
	}
	if len(bus.published) != 1 || bus.published[0] != (ConfigChange{GuildID: guild, Module: ModuleLogging, Version: 1}) {
		t.Fatalf("published = %+v", bus.published)
	}
}

func TestSetEnabledRejectsBadInput(t *testing.T) {
	s := newService(t, newFakeStore(guild), newFakeBus())
	ctx := context.Background()
	if _, err := s.SetEnabled(ctx, guild, "nope", true, nil, SourceSystem); !errors.Is(err, ErrUnknownModule) {
		t.Fatalf("unknown module: err = %v", err)
	}
	if _, err := s.SetEnabled(ctx, guild, ModuleCore, false, nil, SourceSystem); !errors.Is(err, ErrModuleAlwaysOn) {
		t.Fatalf("disable core: err = %v", err)
	}
	if _, err := s.SetEnabled(ctx, 999, ModuleLogging, true, nil, SourceSystem); !errors.Is(err, ErrUnknownGuild) {
		t.Fatalf("unknown guild: err = %v", err)
	}
}

func TestSetEnabledSucceedsWhenPublishFails(t *testing.T) {
	bus := newFakeBus()
	bus.pubErr = errors.New("valkey down")
	s := newService(t, newFakeStore(guild), bus)
	if _, err := s.SetEnabled(context.Background(), guild, ModuleTickets, true, nil, SourceSystem); err != nil {
		t.Fatalf("write should succeed even if publish fails: %v", err)
	}
}

func TestCacheServesReadsAndExpires(t *testing.T) {
	store := newFakeStore(guild)
	s := newService(t, store, newFakeBus())
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	ctx := context.Background()

	for range 3 {
		if _, err := s.ModuleStates(ctx, guild); err != nil {
			t.Fatal(err)
		}
	}
	if store.listCalls != 1 {
		t.Fatalf("store hit %d times, want 1 (cached)", store.listCalls)
	}
	now = now.Add(s.ttl)
	_, _ = s.ModuleStates(ctx, guild)
	if store.listCalls != 2 {
		t.Fatalf("store hit %d times after TTL, want 2", store.listCalls)
	}
}

func TestRunInvalidationDropsCachedGuild(t *testing.T) {
	store, bus := newFakeStore(guild), newFakeBus()
	s := newService(t, store, bus)
	ctx := t.Context()

	_, _ = s.ModuleStates(ctx, guild) // fill cache
	got := make(chan ConfigChange, 1)
	go s.RunInvalidation(ctx, func(c ConfigChange) { got <- c })
	deliver := <-bus.subs

	// Another process changes the store directly, then announces it.
	store.mu.Lock()
	store.rows[guild] = map[ModuleID]StoredModuleState{ModuleWelcome: {Module: ModuleWelcome, Enabled: true, Version: 3}}
	store.mu.Unlock()
	deliver(ConfigChange{GuildID: guild, Module: ModuleWelcome, Version: 3})
	<-got

	if on, _ := s.IsEnabled(ctx, guild, ModuleWelcome); !on {
		t.Fatal("cache not invalidated by bus message")
	}
}
