package logging

import (
	"context"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// MemoryCache is an in-process MessageCache and InviteSnapshots for tests.
// Production uses the Valkey implementation in internal/cache, which has the
// same semantics.
type MemoryCache struct {
	mu      sync.Mutex
	Now     func() time.Time
	msgs    map[[2]snowflake.ID]memEntry
	invites map[snowflake.ID][]Invite
}

type memEntry struct {
	m   CachedMessage
	exp time.Time
}

var (
	_ MessageCache    = (*MemoryCache)(nil)
	_ InviteSnapshots = (*MemoryCache)(nil)
)

func NewMemoryCache(now func() time.Time) *MemoryCache {
	if now == nil {
		now = time.Now
	}
	return &MemoryCache{Now: now, msgs: map[[2]snowflake.ID]memEntry{}, invites: map[snowflake.ID][]Invite{}}
}

func (c *MemoryCache) Put(_ context.Context, m CachedMessage, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs[[2]snowflake.ID{m.ChannelID, m.ID}] = memEntry{m: m, exp: c.Now().Add(ttl)}
	return nil
}

func (c *MemoryCache) Get(_ context.Context, ch, id snowflake.ID) (CachedMessage, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.msgs[[2]snowflake.ID{ch, id}]
	if !ok || c.Now().After(e.exp) {
		return CachedMessage{}, false, nil
	}
	return e.m, true, nil
}

func (c *MemoryCache) Take(_ context.Context, ch snowflake.ID, ids []snowflake.ID) ([]CachedMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []CachedMessage
	for _, id := range ids {
		k := [2]snowflake.ID{ch, id}
		if e, ok := c.msgs[k]; ok {
			delete(c.msgs, k)
			if !c.Now().After(e.exp) {
				out = append(out, e.m)
			}
		}
	}
	return out, nil
}

// Len is the number of cached messages (tests).
func (c *MemoryCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

func (c *MemoryCache) SwapInvites(_ context.Context, g snowflake.ID, cur []Invite) ([]Invite, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, ok := c.invites[g]
	c.invites[g] = append([]Invite(nil), cur...)
	return prev, ok, nil
}
