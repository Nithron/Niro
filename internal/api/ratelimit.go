package api

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// Limiter counts hits per key in fixed windows (Valkey in production). The
// first hit of a window starts it; resetIn is the time left in the window.
type Limiter interface {
	Hit(ctx context.Context, key string, window time.Duration) (count int64, resetIn time.Duration, err error)
}

// RateLimits are request budgets per one-minute window. Zero fields use the
// defaults.
type RateLimits struct {
	API    int64 // all /api/v1 requests, per user
	Write  int64 // non-GET /api/v1 requests, per user (these change settings or reach the bot)
	Auth   int64 // /auth/login and /auth/callback, per client IP
	Public int64 // /api/v1 requests without a session (the invite link), per client IP
}

const rateWindow = time.Minute

func (l RateLimits) withDefaults() RateLimits {
	def := func(v *int64, d int64) {
		if *v <= 0 {
			*v = d
		}
	}
	def(&l.API, 120)
	def(&l.Write, 30)
	def(&l.Auth, 10)
	def(&l.Public, 60)
	return l
}

// limitAuth throttles the login flow per client IP.
func (s *Server) limitAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/login" || r.URL.Path == "/auth/callback" {
			if !s.allow(w, r, "auth", "ip:"+s.clientIP(r).String(), s.limits.Auth) {
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// limitAPI throttles API calls per user (per IP for the public routes). It
// runs after requireSession.
func (s *Server) limitAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		if sess == nil {
			if !s.allow(w, r, "public", "ip:"+s.clientIP(r).String(), s.limits.Public) {
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		who := "user:" + sess.User.ID.String()
		if !s.allow(w, r, "api", who, s.limits.API) {
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.allow(w, r, "write", who, s.limits.Write) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow counts one hit and answers 429 when the budget is spent. If the
// limiter is unavailable the request goes through: an outage of the counter
// store should not take the dashboard down with it.
func (s *Server) allow(w http.ResponseWriter, r *http.Request, rule, who string, limit int64) bool {
	if s.limiter == nil {
		return true
	}
	n, reset, err := s.limiter.Hit(r.Context(), "niro:ratelimit:"+rule+":"+who, rateWindow)
	if err != nil {
		s.log.WarnContext(r.Context(), "rate limiter unavailable", "rule", rule, "err", err)
		return true
	}
	if n <= limit {
		return true
	}
	s.rateLimited.WithLabelValues(rule).Inc()
	secs := max(int((reset+time.Second-1)/time.Second), 1)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Please wait a moment and try again.")
	return false
}

// clientIP is the connecting address, or, when that is a trusted reverse
// proxy, the right-most X-Forwarded-For entry that isn't one. Client-supplied
// headers are never trusted from anyone else.
func (s *Server) clientIP(r *http.Request) netip.Addr {
	remote := parseAddr(r.RemoteAddr)
	if !s.trusted(remote) {
		return remote
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return remote
		}
		a = a.Unmap()
		if !s.trusted(a) {
			return a
		}
	}
	return remote
}

func (s *Server) trusted(a netip.Addr) bool {
	for _, p := range s.cfg.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func parseAddr(hostport string) netip.Addr {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.IPv4Unspecified()
	}
	return a.Unmap()
}
