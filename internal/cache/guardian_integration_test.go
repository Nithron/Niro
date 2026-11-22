package cache_test

import (
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/guardian"
)

// The Valkey tracker must behave exactly like the in-memory tracker used by
// the replay harness, so the scenarios there hold in production.
func TestGuardianTrackerMatchesMemoryTracker(t *testing.T) {
	c := newClient(t)
	ctx := t.Context()
	base := time.Now().Truncate(time.Millisecond)
	trackers := map[string]guardian.Tracker{
		"valkey": c.Guardian(),
		"memory": guardian.NewMemoryTracker(func() time.Time { return base }),
	}
	for name, tr := range trackers {
		t.Run(name, func(t *testing.T) {
			g := snowflake.ID(1)
			if name == "valkey" {
				g = 2 // separate keys from other runs
			}
			u := snowflake.ID(100)
			window := 10 * time.Second
			entry := func(ch, msg snowflake.ID, at time.Duration, fp string) guardian.RecentEntry {
				return guardian.RecentEntry{ChannelID: ch, MessageID: msg, Fingerprints: []string{fp}, At: base.Add(at)}
			}
			got, err := tr.RecordMessage(ctx, g, u, entry(11, 1, 0, "a"), window)
			if err != nil || len(got) != 1 {
				t.Fatalf("first = %+v, %v", got, err)
			}
			got, _ = tr.RecordMessage(ctx, g, u, entry(12, 2, 5*time.Second, "a"), window)
			if len(got) != 2 {
				t.Fatalf("second = %+v", got)
			}
			// 12s later the first entry is outside the window.
			got, _ = tr.RecordMessage(ctx, g, u, entry(13, 3, 12*time.Second, "b"), window)
			if len(got) != 2 || got[0].MessageID != 2 || got[1].MessageID != 3 || got[1].Fingerprints[0] != "b" {
				t.Fatalf("after window = %+v", got)
			}
			// Other users are separate.
			if got, _ = tr.RecordMessage(ctx, g, 200, entry(11, 4, 12*time.Second, "a"), window); len(got) != 1 {
				t.Fatalf("other user = %+v", got)
			}

			prev, err := tr.MarkActive(ctx, g, u, base, time.Hour)
			if err != nil || !prev.IsZero() {
				t.Fatalf("first activity prev = %v, %v", prev, err)
			}
			if prev, _ = tr.MarkActive(ctx, g, u, base.Add(time.Minute), time.Hour); !prev.Equal(base) {
				t.Fatalf("prev = %v, want %v", prev, base)
			}

			since, err := tr.WatchingSince(ctx, g, base)
			if err != nil || !since.Equal(base) {
				t.Fatalf("since = %v, %v", since, err)
			}
			if since, _ = tr.WatchingSince(ctx, g, base.Add(time.Hour)); !since.Equal(base) {
				t.Fatalf("since must not move: %v", since)
			}

			for i, at := range []time.Duration{0, time.Second, 2 * time.Second} {
				n, err := tr.RecordJoin(ctx, g, snowflake.ID(500+i), base.Add(at), 5*time.Second)
				if err != nil || n != i+1 {
					t.Fatalf("join %d: n = %d, %v", i, n, err)
				}
			}
			if n, _ := tr.RecordJoin(ctx, g, 600, base.Add(6500*time.Millisecond), 5*time.Second); n != 2 {
				t.Fatalf("joins after window = %d", n)
			}

			key := name + "-claim"
			if ok, err := tr.Claim(ctx, key, time.Minute); err != nil || !ok {
				t.Fatalf("claim = %v, %v", ok, err)
			}
			if ok, _ := tr.Claim(ctx, key, time.Minute); ok {
				t.Fatal("claimed twice")
			}
		})
	}
}
