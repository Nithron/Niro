// Package cache wraps Valkey: the config-change bus between processes,
// dashboard sessions, and short-lived JSON caches. Nothing in Valkey is the
// source of truth; losing it logs users out and drops caches, nothing more.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/valkey-io/valkey-go"

	"github.com/nithron/niro/internal/core"
)

const configChannel = "niro:config"

// Client implements core.ChangeBus plus key/value helpers.
type Client struct {
	c valkey.Client
}

var _ core.ChangeBus = (*Client)(nil)

// Connect opens a client and verifies it, retrying for up to timeout.
func Connect(ctx context.Context, addr, password string, timeout time.Duration) (*Client, error) {
	deadline := time.Now().Add(timeout)
	for {
		c, err := valkey.NewClient(valkey.ClientOption{
			InitAddress:  []string{addr},
			Password:     password,
			DisableCache: true, // we use explicit keys, not client-side caching
		})
		if err == nil {
			cl := &Client{c: c}
			if err = cl.Ping(ctx); err == nil {
				return cl, nil
			}
			c.Close()
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, fmt.Errorf("connecting to valkey at %s: %w", addr, err)
		}
		time.Sleep(time.Second)
	}
}

func (c *Client) Close() { c.c.Close() }

func (c *Client) Ping(ctx context.Context) error {
	return c.c.Do(ctx, c.c.B().Ping().Build()).Error()
}

func (c *Client) PublishConfigChange(ctx context.Context, change core.ConfigChange) error {
	b, err := json.Marshal(change)
	if err != nil {
		return err
	}
	return c.c.Do(ctx, c.c.B().Publish().Channel(configChannel).Message(string(b)).Build()).Error()
}

func (c *Client) SubscribeConfigChanges(ctx context.Context, fn func(core.ConfigChange)) error {
	return c.c.Receive(ctx, c.c.B().Subscribe().Channel(configChannel).Build(), func(msg valkey.PubSubMessage) {
		var change core.ConfigChange
		if err := json.Unmarshal([]byte(msg.Message), &change); err != nil {
			return // ignore malformed messages; they cannot come from Niro
		}
		fn(change)
	})
}

// GetJSON loads key into v. found is false if the key does not exist.
func (c *Client) GetJSON(ctx context.Context, key string, v any) (found bool, err error) {
	s, err := c.c.Do(ctx, c.c.B().Get().Key(key).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(s), v)
}

// SetJSON stores v under key with a TTL (which must be at least a second).
func (c *Client) SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error {
	if ttl < time.Second {
		return errors.New("cache: TTL must be at least one second")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.c.Do(ctx, c.c.B().Set().Key(key).Value(string(b)).Ex(ttl).Build()).Error()
}

// ForgetGuild deletes the Valkey keys that belong to one server: Guardian's
// counters and watch state and the invite snapshot. The message cache is keyed
// by channel and expires on its own within 7 days.
func (c *Client) ForgetGuild(ctx context.Context, g snowflake.ID) error {
	keys := []string{sinceKey(g), joinsKey(g), invitesKey(g)}
	for _, pattern := range []string{"niro:guardian:msgs:" + g.String() + ":*", "niro:guardian:seen:" + g.String() + ":*"} {
		var cursor uint64
		for {
			e, err := c.c.Do(ctx, c.c.B().Scan().Cursor(cursor).Match(pattern).Count(500).Build()).AsScanEntry()
			if err != nil {
				return err
			}
			keys = append(keys, e.Elements...)
			if cursor = e.Cursor; cursor == 0 {
				break
			}
		}
	}
	for chunk := range slices.Chunk(keys, 500) {
		if err := c.c.Do(ctx, c.c.B().Del().Key(chunk...).Build()).Error(); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceJSON overwrites an existing key, keeping its TTL; it never creates one.
func (c *Client) ReplaceJSON(ctx context.Context, key string, v any) (bool, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	err = c.c.Do(ctx, c.c.B().Set().Key(key).Value(string(b)).Xx().Keepttl().Build()).Error()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	return err == nil, err
}

func (c *Client) Delete(ctx context.Context, key string) error {
	return c.c.Do(ctx, c.c.B().Del().Key(key).Build()).Error()
}

// hitScript increments a fixed-window counter, starting the window on the
// first hit, and returns the count and the milliseconds left in the window.
var hitScript = valkey.NewLuaScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then redis.call('PEXPIRE', KEYS[1], ARGV[1]); ttl = tonumber(ARGV[1]) end
return {n, ttl}`)

// Hit counts one request against a rate-limit window (api.Limiter).
func (c *Client) Hit(ctx context.Context, key string, window time.Duration) (int64, time.Duration, error) {
	vals, err := hitScript.Exec(ctx, c.c, []string{key}, []string{strconv.FormatInt(window.Milliseconds(), 10)}).ToArray()
	if err != nil {
		return 0, 0, err
	}
	if len(vals) != 2 {
		return 0, 0, fmt.Errorf("rate limit script returned %d values", len(vals))
	}
	n, err := vals[0].AsInt64()
	if err != nil {
		return 0, 0, err
	}
	ms, err := vals[1].AsInt64()
	if err != nil {
		return 0, 0, err
	}
	return n, time.Duration(ms) * time.Millisecond, nil
}

// Raw returns the underlying Valkey client (for the RPC bridge).
func (c *Client) Raw() valkey.Client { return c.c }
