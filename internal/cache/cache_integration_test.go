package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcvalkey "github.com/testcontainers/testcontainers-go/modules/valkey"

	"github.com/nithron/nithro/internal/cache"
	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/testutil"
)

func newClient(t *testing.T) *cache.Client {
	t.Helper()
	testutil.RequireDocker(t)
	ctx := context.Background()
	ctr, err := tcvalkey.Run(ctx, "valkey/valkey:8-alpine")
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := ctr.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	c, err := cache.Connect(ctx, host+":"+port.Port(), "", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestConfigChangeBus(t *testing.T) {
	c := newClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	got := make(chan core.ConfigChange, 1)
	go func() { _ = c.SubscribeConfigChanges(ctx, func(ch core.ConfigChange) { got <- ch }) }()

	want := core.ConfigChange{GuildID: 1234567890123456789, Module: core.ModuleTickets, Version: 4}
	// The subscription may not be active yet; publish until it is received.
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case ch := <-got:
			if ch != want {
				t.Fatalf("received %+v, want %+v", ch, want)
			}
			return
		case <-tick.C:
			if err := c.PublishConfigChange(ctx, want); err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("config change not received")
		}
	}
}

func TestJSONRoundTripAndTTL(t *testing.T) {
	c := newClient(t)
	ctx := t.Context()
	type v struct{ A string }

	var out v
	if found, err := c.GetJSON(ctx, "missing", &out); err != nil || found {
		t.Fatalf("missing key: found=%v err=%v", found, err)
	}
	if err := c.SetJSON(ctx, "k", v{A: "x"}, time.Second); err != nil {
		t.Fatal(err)
	}
	if found, err := c.GetJSON(ctx, "k", &out); err != nil || !found || out.A != "x" {
		t.Fatalf("round trip: found=%v err=%v out=%+v", found, err, out)
	}
	if err := c.SetJSON(ctx, "k", v{}, time.Millisecond); err == nil {
		t.Fatal("sub-second TTL accepted")
	}
	time.Sleep(1500 * time.Millisecond)
	if found, _ := c.GetJSON(ctx, "k", &out); found {
		t.Fatal("key did not expire")
	}
	if err := c.SetJSON(ctx, "d", v{A: "y"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, "d"); err != nil {
		t.Fatal(err)
	}
	if found, _ := c.GetJSON(ctx, "d", &out); found {
		t.Fatal("deleted key still present")
	}
}
