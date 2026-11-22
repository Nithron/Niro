package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/valkey-io/valkey-go"

	"github.com/nithron/niro/internal/modules/guardian"
)

// GuardianTracker implements guardian.Tracker on Valkey. Every key expires
// on its own; losing them only means Guardian briefly sees less history.
type GuardianTracker struct{ c valkey.Client }

var _ guardian.Tracker = (*GuardianTracker)(nil)

func (c *Client) Guardian() *GuardianTracker { return &GuardianTracker{c: c.c} }

func msgsKey(g, u snowflake.ID) string   { return fmt.Sprintf("niro:guardian:msgs:%s:%s", g, u) }
func activeKey(g, u snowflake.ID) string { return fmt.Sprintf("niro:guardian:seen:%s:%s", g, u) }
func sinceKey(g snowflake.ID) string     { return "niro:guardian:since:" + g.String() }
func joinsKey(g snowflake.ID) string     { return "niro:guardian:joins:" + g.String() }
func claimKey(k string) string           { return "niro:guardian:claim:" + k }

// keyGrace keeps sorted sets a little longer than their window.
const keyGrace = time.Minute

func ms(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

func (t *GuardianTracker) RecordMessage(ctx context.Context, g, u snowflake.ID, e guardian.RecentEntry, window time.Duration) ([]guardian.RecentEntry, error) {
	member, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	k := msgsKey(g, u)
	c := t.c
	res := c.DoMulti(ctx,
		c.B().Zadd().Key(k).ScoreMember().ScoreMember(float64(e.At.UnixMilli()), string(member)).Build(),
		c.B().Zremrangebyscore().Key(k).Min("-inf").Max("("+ms(e.At.Add(-window))).Build(),
		c.B().Zrange().Key(k).Min("0").Max("-1").Build(),
		c.B().Pexpire().Key(k).Milliseconds((window + keyGrace).Milliseconds()).Build(),
	)
	for _, r := range res {
		if err := r.Error(); err != nil {
			return nil, err
		}
	}
	raw, err := res[2].AsStrSlice()
	if err != nil {
		return nil, err
	}
	out := make([]guardian.RecentEntry, 0, len(raw))
	for _, s := range raw {
		var x guardian.RecentEntry
		if json.Unmarshal([]byte(s), &x) == nil {
			out = append(out, x)
		}
	}
	return out, nil
}

func (t *GuardianTracker) MarkActive(ctx context.Context, g, u snowflake.ID, at time.Time, ttl time.Duration) (time.Time, error) {
	c := t.c
	prev, err := c.Do(ctx, c.B().Set().Key(activeKey(g, u)).Value(ms(at)).Get().Px(ttl).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	n, err := strconv.ParseInt(prev, 10, 64)
	if err != nil {
		return time.Time{}, nil
	}
	return time.UnixMilli(n), nil
}

func (t *GuardianTracker) WatchingSince(ctx context.Context, g snowflake.ID, now time.Time) (time.Time, error) {
	c := t.c
	prev, err := c.Do(ctx, c.B().Set().Key(sinceKey(g)).Value(ms(now)).Nx().Get().Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return now, nil // first time: just set
	}
	if err != nil {
		return time.Time{}, err
	}
	n, err := strconv.ParseInt(prev, 10, 64)
	if err != nil {
		return now, nil
	}
	return time.UnixMilli(n), nil
}

func (t *GuardianTracker) RecordJoin(ctx context.Context, g, u snowflake.ID, at time.Time, window time.Duration) (int, error) {
	k := joinsKey(g)
	c := t.c
	res := c.DoMulti(ctx,
		c.B().Zadd().Key(k).ScoreMember().ScoreMember(float64(at.UnixMilli()), u.String()).Build(),
		c.B().Zremrangebyscore().Key(k).Min("-inf").Max("("+ms(at.Add(-window))).Build(),
		c.B().Zcard().Key(k).Build(),
		c.B().Pexpire().Key(k).Milliseconds((window + keyGrace).Milliseconds()).Build(),
	)
	for _, r := range res {
		if err := r.Error(); err != nil {
			return 0, err
		}
	}
	n, err := res[2].AsInt64()
	return int(n), err
}

func (t *GuardianTracker) Claim(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	c := t.c
	err := c.Do(ctx, c.B().Set().Key(claimKey(key)).Value("1").Nx().Px(ttl).Build()).Error()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	return err == nil, err
}
