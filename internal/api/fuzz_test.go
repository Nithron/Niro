package api

import (
	"net/http"
	"net/netip"
	"testing"
)

// FuzzClientIP checks that a client can never make Niro see one of its trusted
// proxies, and that headers from untrusted peers are ignored.
func FuzzClientIP(f *testing.F) {
	f.Add("10.0.0.2:1", "198.51.100.7, 10.0.0.3")
	f.Add("203.0.113.9:1", "10.0.0.1")
	f.Add("10.0.0.2:1", "fe80::1%eth0")
	f.Add("[::ffff:10.0.0.2]:1", "::ffff:1.2.3.4")
	s := &Server{cfg: Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("fd00::/8")}}}
	f.Fuzz(func(t *testing.T, remote, xff string) {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", xff)
		got := s.clientIP(r)
		peer := parseAddr(remote)
		if !s.trusted(peer) && got != peer {
			t.Fatalf("untrusted peer %s set its own IP to %s", remote, got)
		}
		if got != peer && s.trusted(got) {
			t.Fatalf("client IP %s is a trusted proxy (peer %s, XFF %q)", got, remote, xff)
		}
		if got.Zone() != "" {
			t.Fatalf("zone kept: %s", got)
		}
		_ = s.clientKey(r)
	})
}
