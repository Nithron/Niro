package moderation

import "testing"

func FuzzParseDuration(f *testing.F) {
	for _, s := range []string{"10m", "1d12h", "2w", " 3 H ", "0s", "99999999999d", "100000w", "1", "h", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := ParseDuration(s)
		if err == nil && (d < 0 || d > maxParsedDuration) {
			t.Fatalf("ParseDuration(%q) = %s without an error", s, d)
		}
	})
}
