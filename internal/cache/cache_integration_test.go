package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/testcontainers/testcontainers-go"
	tcvalkey "github.com/testcontainers/testcontainers-go/modules/valkey"

	"github.com/nithron/niro/internal/cache"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/testutil"
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

func TestRateLimitHit(t *testing.T) {
	c := newClient(t)
	ctx := t.Context()
	for want := int64(1); want <= 3; want++ {
		n, reset, err := c.Hit(ctx, "niro:ratelimit:test", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if n != want || reset <= 0 || reset > time.Minute {
			t.Fatalf("hit %d = (%d, %s)", want, n, reset)
		}
	}
	// A new window starts once the old one expires.
	if _, _, err := c.Hit(ctx, "niro:ratelimit:short", 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	if n, _, err := c.Hit(ctx, "niro:ratelimit:short", 50*time.Millisecond); err != nil || n != 1 {
		t.Fatalf("after the window = (%d, %v), want 1", n, err)
	}
}

func TestReplaceJSONNeverCreates(t *testing.T) {
	c := newClient(t)
	ctx := t.Context()
	if ok, err := c.ReplaceJSON(ctx, "niro:test:missing", map[string]int{"a": 1}); err != nil || ok {
		t.Fatalf("replace of a missing key = (%v, %v), want (false, nil)", ok, err)
	}
	if found, _ := c.GetJSON(ctx, "niro:test:missing", &map[string]int{}); found {
		t.Fatal("ReplaceJSON created a key")
	}
	if err := c.SetJSON(ctx, "niro:test:k", 1, time.Minute); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.ReplaceJSON(ctx, "niro:test:k", 2); err != nil || !ok {
		t.Fatalf("replace = (%v, %v)", ok, err)
	}
	var v int
	if _, err := c.GetJSON(ctx, "niro:test:k", &v); err != nil || v != 2 {
		t.Fatalf("value = %d (%v)", v, err)
	}
	ttl, err := c.Raw().Do(ctx, c.Raw().B().Pttl().Key("niro:test:k").Build()).AsInt64()
	if err != nil || ttl <= 0 {
		t.Fatalf("TTL lost on replace: %d (%v)", ttl, err)
	}
}

func TestForgetGuildAndMessages(t *testing.T) {
	c := newClient(t)
	ctx := t.Context()
	set := func(k string) {
		if err := c.Raw().Do(ctx, c.Raw().B().Set().Key(k).Value("x").Build()).Error(); err != nil {
			t.Fatal(err)
		}
	}
	mine := []string{
		"niro:guardian:msgs:1:5", "niro:guardian:seen:1:5", "niro:guardian:since:1", "niro:guardian:joins:1",
		"niro:logging:invites:1", "niro:logging:msg:10:99", "niro:logging:msg:11:98",
	}
	other := []string{"niro:guardian:msgs:2:5", "niro:guardian:msgs:12:5", "niro:logging:msg:20:97", "niro:guardian:since:12"}
	for _, k := range append(append([]string{}, mine...), other...) {
		set(k)
	}
	if err := c.ForgetGuild(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.ForgetMessages(ctx, []snowflake.ID{10, 11}); err != nil {
		t.Fatal(err)
	}
	exists := func(k string) bool {
		n, err := c.Raw().Do(ctx, c.Raw().B().Exists().Key(k).Build()).AsInt64()
		if err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	for _, k := range mine {
		if exists(k) {
			t.Errorf("%s survived", k)
		}
	}
	for _, k := range other {
		if !exists(k) {
			t.Errorf("%s of another server was deleted", k)
		}
	}
}
