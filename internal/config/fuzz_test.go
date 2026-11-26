package config

import "testing"

func FuzzParseBaseURL(f *testing.F) {
	for _, s := range []string{"https://niro.example/", "http://localhost:8080", "localhost", "https://a/b?c", "javascript:alert(1)", "https://[::1]/"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		u, err := parseBaseURL(s)
		if err != nil {
			return
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			t.Fatalf("parseBaseURL(%q) accepted %s", s, u)
		}
	})
}
