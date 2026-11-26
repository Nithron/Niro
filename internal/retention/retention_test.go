package retention

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeStore struct {
	cutoffs map[string]time.Time
	fail    string
}

func (f *fakeStore) record(name string, t time.Time) (int64, error) {
	if f.cutoffs == nil {
		f.cutoffs = map[string]time.Time{}
	}
	f.cutoffs[name] = t
	if f.fail == name {
		return 0, errors.New("boom")
	}
	return 1, nil
}

func (f *fakeStore) PurgeLeftGuilds(_ context.Context, t time.Time) (int64, error) {
	return f.record("guilds", t)
}
func (f *fakeStore) PurgeDoneScheduledActions(_ context.Context, t time.Time) (int64, error) {
	return f.record("actions", t)
}
func (f *fakeStore) PurgeEndedRaids(_ context.Context, t time.Time) (int64, error) {
	return f.record("raids", t)
}
func (f *fakeStore) PurgeConfigAudit(_ context.Context, t time.Time) (int64, error) {
	return f.record("audit", t)
}

func TestRunOnceUsesThePolicy(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	fs := &fakeStore{fail: "actions"}
	r := Runner{Store: fs, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }}
	err := r.RunOnce(context.Background())
	if err == nil {
		t.Fatal("a failing rule should be reported")
	}
	want := map[string]time.Time{
		"guilds": now.Add(-30 * day), "actions": now.Add(-30 * day),
		"raids": now.Add(-90 * day), "audit": now.Add(-365 * day),
	}
	for k, v := range want {
		if !fs.cutoffs[k].Equal(v) {
			t.Errorf("%s cutoff = %s, want %s", k, fs.cutoffs[k], v)
		}
	}
}
