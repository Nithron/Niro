package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const (
	// sessionCookie is the cookie name over plain http (development). Over
	// https it carries the __Host- prefix, which browsers only accept for a
	// Secure, host-only cookie on path /.
	sessionCookie    = "niro_session"
	stateCookie      = "niro_oauth_state"
	maxSessionLength = 7 * 24 * time.Hour
	// idleTimeout ends a session nobody has used for this long.
	idleTimeout = 24 * time.Hour
	// touchEvery limits how often a session's last-seen time is written back.
	touchEvery = 5 * time.Minute
)

// KV is the key/value store sessions and caches live in (Valkey in production).
type KV interface {
	GetJSON(ctx context.Context, key string, v any) (bool, error)
	SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// Session is a logged-in dashboard user. The Discord access token is kept
// server-side only, never sent to the browser. No refresh token is stored:
// when the access token expires, the user logs in again.
type Session struct {
	User        Profile   `json:"user"`
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeen    time.Time `json:"last_seen"`
}

type sessions struct{ kv KV }

// The cookie holds a random ID; Valkey stores it hashed, so reading Valkey
// does not reveal usable cookie values.
func sessionKey(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "niro:session:" + hex.EncodeToString(sum[:])
}

func newRandomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(b)
}

// epochKey holds the time of a user's last "sign out everywhere": sessions
// created before it are no longer valid.
func epochKey(user string) string { return "niro:session-epoch:" + user }

func (s sessions) create(ctx context.Context, sess Session) (id string, err error) {
	id = newRandomToken()
	now := time.Now()
	sess.CreatedAt, sess.LastSeen = now, now
	ttl := max(time.Until(sess.ExpiresAt), time.Minute)
	return id, s.kv.SetJSON(ctx, sessionKey(id), sess, ttl)
}

// get returns the live session for a cookie value, or nil when it doesn't
// exist, has expired, has been idle too long or was signed out everywhere.
func (s sessions) get(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, nil
	}
	var sess Session
	found, err := s.kv.GetJSON(ctx, sessionKey(id), &sess)
	if err != nil || !found {
		return nil, err
	}
	now := time.Now()
	if now.After(sess.ExpiresAt) || now.Sub(sess.LastSeen) > idleTimeout {
		_ = s.kv.Delete(ctx, sessionKey(id))
		return nil, nil
	}
	var epoch time.Time
	found, err = s.kv.GetJSON(ctx, epochKey(sess.User.ID.String()), &epoch)
	if err != nil {
		return nil, err
	}
	if found && !sess.CreatedAt.After(epoch) {
		_ = s.kv.Delete(ctx, sessionKey(id))
		return nil, nil
	}
	if now.Sub(sess.LastSeen) > touchEvery {
		sess.LastSeen = now
		// Best effort: failing to record activity must not fail the request.
		_ = s.kv.SetJSON(ctx, sessionKey(id), sess, max(time.Until(sess.ExpiresAt), time.Minute))
	}
	return &sess, nil
}

// endAll ends every session of a user, including ones on other devices.
func (s sessions) endAll(ctx context.Context, user string) error {
	return s.kv.SetJSON(ctx, epochKey(user), time.Now(), maxSessionLength)
}

func (s sessions) delete(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	return s.kv.Delete(ctx, sessionKey(id))
}

type ctxKey struct{}

func withSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

func sessionFrom(ctx context.Context) *Session {
	s, _ := ctx.Value(ctxKey{}).(*Session)
	return s
}
