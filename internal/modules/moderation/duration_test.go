package moderation

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	good := map[string]time.Duration{
		"10m": 10 * time.Minute, "2h": 2 * time.Hour, "1d12h": 36 * time.Hour,
		"1w": 7 * 24 * time.Hour, " 3D ": 72 * time.Hour, "1h 30m": 90 * time.Minute, "45s": 45 * time.Second,
	}
	for in, want := range good {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "10", "m", "10x", "1.5h", "-5m", "h10", "999999999d"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) accepted", in)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second: "30s", 10 * time.Minute: "10m", 36 * time.Hour: "1d 12h",
		15 * 24 * time.Hour: "2w 1d", 90 * time.Minute: "1h 30m",
	}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}
