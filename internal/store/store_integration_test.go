package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/store"
	"github.com/nithron/niro/internal/testutil"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	testutil.RequireDocker(t)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("niro"), postgres.WithUsername("niro"), postgres.WithPassword("niro"),
		postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := store.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return store.New(pool)
}

func TestGuildLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	g := core.GuildInfo{ID: 111, Name: "Test", OwnerID: 1}

	if err := s.UpsertGuild(ctx, g); err != nil {
		t.Fatal(err)
	}
	active, err := s.ActiveGuilds(ctx, []snowflake.ID{111, 222})
	if err != nil {
		t.Fatal(err)
	}
	if !active[111] || active[222] {
		t.Fatalf("active = %v", active)
	}
	if err := s.MarkGuildLeft(ctx, 111); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsGuildActive(ctx, 111); ok {
		t.Fatal("guild still active after leaving")
	}
	if err := s.UpsertGuild(ctx, g); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsGuildActive(ctx, 111); !ok {
		t.Fatal("re-added guild not active")
	}
}

func TestSetModuleEnabledWritesAudit(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	if err := s.UpsertGuild(ctx, core.GuildInfo{ID: 5, Name: "G", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	actor := snowflake.ID(77)

	st, err := s.SetModuleEnabled(ctx, 5, core.ModuleLogging, true, &actor, core.SourceDashboard)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Version != 1 {
		t.Fatalf("first write = %+v", st)
	}
	st, err = s.SetModuleEnabled(ctx, 5, core.ModuleLogging, false, nil, core.SourceCommand)
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.Version != 2 {
		t.Fatalf("second write = %+v", st)
	}

	states, err := s.ListModuleStates(ctx, 5)
	if err != nil || len(states) != 1 || states[0].Enabled {
		t.Fatalf("states = %+v, err = %v", states, err)
	}

	audit, err := s.ListConfigAudit(ctx, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 {
		t.Fatalf("audit entries = %d, want 2", len(audit))
	}
	newest, oldest := audit[0], audit[1]
	if newest.Source != "command" || newest.ActorID != nil {
		t.Errorf("newest entry = %+v", newest)
	}
	if oldest.Source != "dashboard" || oldest.ActorID == nil || *oldest.ActorID != actor {
		t.Errorf("oldest entry = %+v", oldest)
	}
	var change struct {
		Enabled struct {
			From *bool `json:"from"`
			To   bool  `json:"to"`
		} `json:"enabled"`
	}
	if err := json.Unmarshal(oldest.Change, &change); err != nil || change.Enabled.From != nil || !change.Enabled.To {
		t.Errorf("first change should be default→true: %s", oldest.Change)
	}
	if err := json.Unmarshal(newest.Change, &change); err != nil || change.Enabled.From == nil || !*change.Enabled.From || change.Enabled.To {
		t.Errorf("second change should be true→false: %s", newest.Change)
	}
}

func TestSetModuleEnabledUnknownGuild(t *testing.T) {
	s := newStore(t)
	_, err := s.SetModuleEnabled(t.Context(), 999, core.ModuleLogging, true, nil, core.SourceSystem)
	if !errors.Is(err, core.ErrUnknownGuild) {
		t.Fatalf("err = %v, want ErrUnknownGuild", err)
	}
}

func snow(n int64) snowflake.ID { return snowflake.ID(n) }
