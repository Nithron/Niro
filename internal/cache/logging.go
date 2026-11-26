package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/valkey-io/valkey-go"

	"github.com/nithron/niro/internal/modules/logging"
)

// LoggingCache implements logging.MessageCache and logging.InviteSnapshots
// on Valkey. Message text lives only here, under a TTL (plan §5), and is
// removed as soon as the message is deleted.
type LoggingCache struct{ c valkey.Client }

var (
	_ logging.MessageCache    = (*LoggingCache)(nil)
	_ logging.InviteSnapshots = (*LoggingCache)(nil)
)

func (c *Client) Logging() *LoggingCache { return &LoggingCache{c: c.c} }

func logMsgKey(ch, id snowflake.ID) string { return fmt.Sprintf("niro:logging:msg:%s:%s", ch, id) }
func invitesKey(g snowflake.ID) string     { return "niro:logging:invites:" + g.String() }

// inviteSnapshotTTL: snapshots are refreshed on every join and invite
// change; an idle server's snapshot may expire (its next join is "unknown").
const inviteSnapshotTTL = 30 * 24 * time.Hour

func (l *LoggingCache) Put(ctx context.Context, m logging.CachedMessage, ttl time.Duration) error {
	if ttl < time.Second {
		return errors.New("cache: TTL must be at least one second")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return l.c.Do(ctx, l.c.B().Set().Key(logMsgKey(m.ChannelID, m.ID)).Value(string(b)).Ex(ttl).Build()).Error()
}

func (l *LoggingCache) Get(ctx context.Context, ch, id snowflake.ID) (logging.CachedMessage, bool, error) {
	s, err := l.c.Do(ctx, l.c.B().Get().Key(logMsgKey(ch, id)).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return logging.CachedMessage{}, false, nil
	}
	if err != nil {
		return logging.CachedMessage{}, false, err
	}
	var m logging.CachedMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return logging.CachedMessage{}, false, nil // unreadable: treat as not cached
	}
	return m, true, nil
}

func (l *LoggingCache) Take(ctx context.Context, ch snowflake.ID, ids []snowflake.ID) ([]logging.CachedMessage, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cmds := make(valkey.Commands, len(ids))
	for i, id := range ids {
		cmds[i] = l.c.B().Getdel().Key(logMsgKey(ch, id)).Build()
	}
	var out []logging.CachedMessage
	for _, r := range l.c.DoMulti(ctx, cmds...) {
		s, err := r.ToString()
		if valkey.IsValkeyNil(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var m logging.CachedMessage
		if json.Unmarshal([]byte(s), &m) == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

func (l *LoggingCache) SwapInvites(ctx context.Context, g snowflake.ID, cur []logging.Invite) ([]logging.Invite, bool, error) {
	if cur == nil {
		cur = []logging.Invite{}
	}
	b, err := json.Marshal(cur)
	if err != nil {
		return nil, false, err
	}
	s, err := l.c.Do(ctx, l.c.B().Set().Key(invitesKey(g)).Value(string(b)).Get().Ex(inviteSnapshotTTL).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var prev []logging.Invite
	if err := json.Unmarshal([]byte(s), &prev); err != nil {
		return nil, false, nil
	}
	return prev, true, nil
}
