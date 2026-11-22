package guardian

import (
	"context"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// MemoryTracker is an in-process Tracker for tests and the replay harness.
// Production uses the Valkey implementation in internal/cache, which has the
// same semantics.
type MemoryTracker struct {
	mu      sync.Mutex
	Now     func() time.Time
	msgs    map[[2]snowflake.ID][]RecentEntry
	active  map[[2]snowflake.ID]time.Time
	since   map[snowflake.ID]time.Time
	joins   map[snowflake.ID][]time.Time
	claimed map[string]time.Time
}

var _ Tracker = (*MemoryTracker)(nil)

func NewMemoryTracker(now func() time.Time) *MemoryTracker {
	return &MemoryTracker{
		Now: now, msgs: map[[2]snowflake.ID][]RecentEntry{}, active: map[[2]snowflake.ID]time.Time{},
		since: map[snowflake.ID]time.Time{}, joins: map[snowflake.ID][]time.Time{}, claimed: map[string]time.Time{},
	}
}

func (t *MemoryTracker) RecordMessage(_ context.Context, g, u snowflake.ID, e RecentEntry, window time.Duration) ([]RecentEntry, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := [2]snowflake.ID{g, u}
	var keep []RecentEntry
	for _, x := range append(t.msgs[k], e) {
		if !x.At.Before(e.At.Add(-window)) {
			keep = append(keep, x)
		}
	}
	t.msgs[k] = keep
	return append([]RecentEntry(nil), keep...), nil
}

func (t *MemoryTracker) MarkActive(_ context.Context, g, u snowflake.ID, at time.Time, ttl time.Duration) (time.Time, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := [2]snowflake.ID{g, u}
	prev := t.active[k]
	t.active[k] = at
	if !prev.IsZero() && at.Sub(prev) > ttl {
		prev = time.Time{} // expired
	}
	return prev, nil
}

// SetActive seeds activity (tests).
func (t *MemoryTracker) SetActive(g, u snowflake.ID, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active[[2]snowflake.ID{g, u}] = at
}

// SetWatchingSince seeds the watch start (tests).
func (t *MemoryTracker) SetWatchingSince(g snowflake.ID, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.since[g] = at
}

func (t *MemoryTracker) WatchingSince(_ context.Context, g snowflake.ID, now time.Time) (time.Time, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.since[g]; ok {
		return s, nil
	}
	t.since[g] = now
	return now, nil
}

func (t *MemoryTracker) RecordJoin(_ context.Context, g, _ snowflake.ID, at time.Time, window time.Duration) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var keep []time.Time
	for _, x := range append(t.joins[g], at) {
		if !x.Before(at.Add(-window)) {
			keep = append(keep, x)
		}
	}
	t.joins[g] = keep
	return len(keep), nil
}

func (t *MemoryTracker) Claim(_ context.Context, key string, ttl time.Duration) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.Now()
	if exp, ok := t.claimed[key]; ok && now.Before(exp) {
		return false, nil
	}
	t.claimed[key] = now.Add(ttl)
	return true, nil
}
