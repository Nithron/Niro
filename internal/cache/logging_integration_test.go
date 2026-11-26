package cache_test

import (
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/logging"
)

// The Valkey message cache must behave like the in-memory one used by the
// logging service tests.
func TestLoggingCacheMatchesMemoryCache(t *testing.T) {
	c := newClient(t)
	ctx := t.Context()
	type cacheImpl interface {
		logging.MessageCache
		logging.InviteSnapshots
	}
	impls := map[string]cacheImpl{
		"valkey": c.Logging(),
		"memory": logging.NewMemoryCache(nil),
	}
	for name, mc := range impls {
		t.Run(name, func(t *testing.T) {
			ch := snowflake.ID(10)
			g := snowflake.ID(1)
			if name == "valkey" {
				ch, g = 20, 2 // separate keys from other runs
			}
			at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
			m := logging.CachedMessage{ID: 5, ChannelID: ch, AuthorID: 7, AuthorName: "alice", Content: "hello",
				Attachments: []string{"a.png"}, AuthorRoleIDs: []snowflake.ID{3}, CreatedAt: at}
			if err := mc.Put(ctx, m, time.Minute); err != nil {
				t.Fatal(err)
			}
			got, ok, err := mc.Get(ctx, ch, 5)
			if err != nil || !ok || got.Content != "hello" || got.AuthorName != "alice" || !got.CreatedAt.Equal(at) ||
				len(got.Attachments) != 1 || len(got.AuthorRoleIDs) != 1 {
				t.Fatalf("Get = %+v, %v, %v", got, ok, err)
			}
			if _, ok, _ := mc.Get(ctx, ch, 6); ok {
				t.Fatal("found a message that was never cached")
			}
			m2 := m
			m2.ID, m2.Content = 6, "second"
			_ = mc.Put(ctx, m2, time.Minute)
			taken, err := mc.Take(ctx, ch, []snowflake.ID{5, 6, 99})
			if err != nil || len(taken) != 2 {
				t.Fatalf("Take = %+v, %v", taken, err)
			}
			if _, ok, _ := mc.Get(ctx, ch, 5); ok {
				t.Fatal("taken message still cached")
			}
			if again, _ := mc.Take(ctx, ch, []snowflake.ID{5}); len(again) != 0 {
				t.Fatalf("taken twice: %+v", again)
			}

			prev, found, err := mc.SwapInvites(ctx, g, []logging.Invite{{Code: "abc", Uses: 2, MaxUses: 5, InviterID: 7}})
			if err != nil || found || prev != nil {
				t.Fatalf("first swap = %v, %v, %v", prev, found, err)
			}
			prev, found, _ = mc.SwapInvites(ctx, g, nil)
			if !found || len(prev) != 1 || prev[0].Code != "abc" || prev[0].Uses != 2 || prev[0].MaxUses != 5 {
				t.Fatalf("second swap = %+v, %v", prev, found)
			}
			if prev, found, _ = mc.SwapInvites(ctx, g, nil); !found || len(prev) != 0 {
				t.Fatalf("empty snapshot = %+v, %v", prev, found)
			}
		})
	}
}

func TestLoggingCacheExpires(t *testing.T) {
	c := newClient(t).Logging()
	ctx := t.Context()
	m := logging.CachedMessage{ID: 1, ChannelID: 30, Content: "short-lived"}
	if err := c.Put(ctx, m, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(ctx, m, time.Millisecond); err == nil {
		t.Error("sub-second TTL accepted")
	}
	time.Sleep(1500 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, 30, 1); ok {
		t.Error("message outlived its TTL")
	}
}
