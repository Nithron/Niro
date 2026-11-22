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
	sessionCookie    = "nithro_session"
	stateCookie      = "nithro_oauth_state"
	maxSessionLength = 7 * 24 * time.Hour
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
}

type sessions struct{ kv KV }

// The cookie holds a random ID; Valkey stores it hashed, so reading Valkey
// does not reveal usable cookie values.
func sessionKey(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "nithro:session:" + hex.EncodeToString(sum[:])
}

func newRandomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s sessions) create(ctx context.Context, sess Session) (id string, err error) {
	id = newRandomToken()
	ttl := max(time.Until(sess.ExpiresAt), time.Minute)
	return id, s.kv.SetJSON(ctx, sessionKey(id), sess, ttl)
}

func (s sessions) get(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, nil
	}
	var sess Session
	found, err := s.kv.GetJSON(ctx, sessionKey(id), &sess)
	if err != nil || !found {
		return nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, nil
	}
	return &sess, nil
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
