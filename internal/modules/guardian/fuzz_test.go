package guardian

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzExtractLinks(f *testing.F) {
	for _, s := range []string{
		"free nitro https://dlscord.gift/abc", "<https://www.Example.COM./x>", "xn--80ak6aa92e.com/a",
		"[x](https://evil.co.uk)", "hxxps://a.b", "https://[::1]:80/", "http://%zz", "https://ex\u0430mple.com",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, l := range ExtractLinks(s) {
			if l.Host == "" || !utf8.ValidString(l.Host) {
				t.Fatalf("link %+v from %q has a bad host", l, s)
			}
			// Hosts are already normalized (IDNA decides letter case, e.g. Cherokee).
			if again, ok := NormalizeHost(l.Host); !ok || again != l.Host {
				t.Fatalf("host %q from %q is not normalized (%q, %v)", l.Host, s, again, ok)
			}
		}
	})
}

func FuzzNormalizeHost(f *testing.F) {
	for _, s := range []string{"WWW.Example.com.", "xn--80ak6aa92e.com", "a..b", "", ".", "localhost:80"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		h, ok := NormalizeHost(s)
		if !ok {
			return
		}
		if again, ok2 := NormalizeHost(h); !ok2 || again != h {
			t.Fatalf("NormalizeHost is not idempotent: %q -> %q -> %q (%v)", s, h, again, ok2)
		}
	})
}

func FuzzNormalizeText(f *testing.F) {
	for _, s := range []string{"FREE N1TRO", "ｆｒｅｅ", "a\u200bb", "Ｓｔｅａｍ", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = normalizeText(s)
	})
}

func FuzzNormalizeDomains(f *testing.F) {
	f.Add("example.com\nhttps://www.Evil.co.uk/path\n*.x.y")
	f.Fuzz(func(t *testing.T, s string) {
		out, err := normalizeDomains("allow", strings.Split(s, "\n"))
		if err != nil {
			return
		}
		for _, d := range out {
			if d == "" || strings.ContainsAny(d, " /") {
				t.Fatalf("normalizeDomains(%q) kept %q", s, d)
			}
		}
	})
}

func FuzzParseFeed(f *testing.F) {
	f.Add([]byte(`["a.com","b.org"]`))
	f.Add([]byte(`{"domains":["a.com"]}`))
	f.Add([]byte("a.com\nb.org\n# comment"))
	f.Fuzz(func(t *testing.T, b []byte) {
		domains, err := ParseFeed(b)
		if err != nil {
			return
		}
		for _, d := range domains {
			if d == "" {
				t.Fatal("ParseFeed returned an empty domain")
			}
		}
	})
}
