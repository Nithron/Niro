// Package cache wraps Valkey: the config-change bus between processes,
// dashboard sessions, and short-lived JSON caches. Nothing in Valkey is the
// source of truth; losing it logs users out and drops caches, nothing more.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

func (c *Client) Delete(ctx context.Context, key string) error {
	return c.c.Do(ctx, c.c.B().Del().Key(key).Build()).Error()
}

// Raw returns the underlying Valkey client (for the RPC bridge).
func (c *Client) Raw() valkey.Client { return c.c }
