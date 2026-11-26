package api

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// guildOperations lists every operation in the OpenAPI spec whose path is
// scoped to a server, so the authorization test covers new endpoints by
// itself.
func guildOperations(t *testing.T) [][2]string {
	t.Helper()
	f, err := os.Open("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	pathRe := regexp.MustCompile(`^  (/\S*):\s*$`)
	opRe := regexp.MustCompile(`^    (get|put|post|patch|delete):\s*$`)
	var ops [][2]string
	var cur string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if m := pathRe.FindStringSubmatch(line); m != nil {
			cur = m[1]
			continue
		}
		if m := opRe.FindStringSubmatch(line); m != nil && strings.HasPrefix(cur, "/guilds/{guildId}") {
			ops = append(ops, [2]string{strings.ToUpper(m[1]), cur})
		}
	}
	if len(ops) < 30 {
		t.Fatalf("found only %d guild operations; is the spec parser out of date?", len(ops))
	}
	return ops
}

var pathParam = regexp.MustCompile(`\{([a-zA-Z]+)\}`)

// fill puts plausible values in a spec path for the given server.
func fill(p, guild string) string {
	return pathParam.ReplaceAllStringFunc(p, func(m string) string {
		switch m {
		case "{guildId}":
			return guild
		case "{moduleId}":
			return "logging"
		default:
			return "1"
		}
	})
}

func TestEveryGuildEndpointChecksAccess(t *testing.T) {
	h := newHarnessWith(t, func(d *Deps) { d.Config.RateLimits = RateLimits{API: 1000, Write: 1000} })
	sid := h.login()
	for _, op := range guildOperations(t) {
		method, p := op[0], op[1]
		body := ""
		if method != "GET" && method != "DELETE" {
			body = "{}"
		}
		// Without a session: 401.
		if rec := h.do(method, "/api/v1"+fill(p, "300"), body, "", sameOrigin); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session = %d, want 401", method, p, rec.Code)
		}
		// As a plain member of the server, or for a server the user isn't in: 403.
		for _, g := range []string{"300", "999"} {
			if rec := h.do(method, "/api/v1"+fill(p, g), body, sid, sameOrigin); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as a non-manager of %s = %d, want 403 (%s)", method, p, g, rec.Code, rec.Body.String())
			}
		}
	}
	if len(h.rpc.calls) != 0 {
		t.Fatalf("refused requests reached the bot: %v", h.rpc.calls)
	}
}

func TestRateLimits(t *testing.T) {
	h := newHarnessWith(t, func(d *Deps) {
		d.Config.RateLimits = RateLimits{API: 5, Write: 2, Auth: 3, Public: 2}
	})
	sid := h.login() // two auth hits: /auth/login and /auth/callback

	for i := range 5 {
		if rec := h.do("GET", "/api/v1/me", "", sid, nil); rec.Code != 200 {
			t.Fatalf("request %d = %d", i+1, rec.Code)
		}
	}
	rec := h.do("GET", "/api/v1/me", "", sid, nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over the API budget = %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if e := decode[map[string]string](t, rec); e["code"] != "rate_limited" {
		t.Fatalf("error = %v", e)
	}

	// The login flow is limited per client IP.
	if rec := h.do("GET", "/auth/login", "", "", nil); rec.Code != http.StatusFound {
		t.Fatalf("third auth hit = %d", rec.Code)
	}
	if rec := h.do("GET", "/auth/login", "", "", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("fourth auth hit = %d, want 429", rec.Code)
	}

	// The public invite route is limited per IP, without a session.
	for i, want := range []int{200, 200, 429} {
		if rec := h.do("GET", "/api/v1/invite", "", "", nil); rec.Code != want {
			t.Fatalf("invite %d = %d, want %d", i+1, rec.Code, want)
		}
	}
}

func TestWriteBudgetIsSeparate(t *testing.T) {
	h := newHarnessWith(t, func(d *Deps) { d.Config.RateLimits = RateLimits{Write: 1} })
	sid := h.login()
	put := func() int {
		return h.do("PUT", "/api/v1/guilds/100/modules/logging", `{"enabled":true}`, sid, sameOrigin).Code
	}
	if c := put(); c != 200 {
		t.Fatalf("first write = %d", c)
	}
	if c := put(); c != http.StatusTooManyRequests {
		t.Fatalf("second write = %d, want 429", c)
	}
	if c := h.do("GET", "/api/v1/me", "", sid, nil).Code; c != 200 {
		t.Fatalf("reads still allowed: %d", c)
	}
}

func TestClientIP(t *testing.T) {
	s := &Server{cfg: Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}}
	cases := []struct {
		remote, xff, want string
	}{
		{"203.0.113.9:4000", "", "203.0.113.9"},
		{"203.0.113.9:4000", "1.2.3.4", "203.0.113.9"},             // untrusted peer: header ignored
		{"10.0.0.2:4000", "198.51.100.7", "198.51.100.7"},          // trusted proxy
		{"10.0.0.2:4000", "6.6.6.6, 198.51.100.7", "198.51.100.7"}, // right-most untrusted entry, not a spoofed left one
		{"10.0.0.2:4000", "198.51.100.7, 10.0.0.3", "198.51.100.7"},
		{"10.0.0.2:4000", "garbage", "10.0.0.2"},
	}
	for _, c := range cases {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := s.clientIP(r).String(); got != c.want {
			t.Errorf("clientIP(%s, %q) = %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	if rec := h.do("POST", "/auth/logout", "", sid, sameOrigin); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if len(h.oauth.revoked) != 1 || h.oauth.revoked[0] != "tok-abc" {
		t.Fatalf("revoked = %v", h.oauth.revoked)
	}
	if rec := h.do("GET", "/api/v1/me", "", sid, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout = %d", rec.Code)
	}
}

func TestLogoutEverywhere(t *testing.T) {
	h := newHarness(t)
	laptop, phone := h.login(), h.login()
	if rec := h.do("POST", "/auth/logout-everywhere", "", "", sameOrigin); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a session = %d", rec.Code)
	}
	if rec := h.do("POST", "/auth/logout-everywhere", "", phone, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site = %d, want 403", rec.Code)
	}
	if rec := h.do("POST", "/auth/logout-everywhere", "", phone, sameOrigin); rec.Code != http.StatusNoContent {
		t.Fatalf("logout everywhere = %d", rec.Code)
	}
	for name, sid := range map[string]string{"laptop": laptop, "phone": phone} {
		if rec := h.do("GET", "/api/v1/me", "", sid, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s session after sign-out everywhere = %d", name, rec.Code)
		}
	}
	time.Sleep(time.Millisecond) // the next session must start after the sign-out
	if rec := h.do("GET", "/api/v1/me", "", h.login(), nil); rec.Code != 200 {
		t.Fatalf("new login after sign-out everywhere = %d", rec.Code)
	}
}

func TestIdleSessionEnds(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	key := sessionKey(sid)
	var sess Session
	if ok, _ := h.kv.GetJSON(t.Context(), key, &sess); !ok {
		t.Fatal("session not stored")
	}
	sess.LastSeen = time.Now().Add(-idleTimeout - time.Minute)
	_ = h.kv.SetJSON(t.Context(), key, sess, time.Hour)
	if rec := h.do("GET", "/api/v1/me", "", sid, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("idle session = %d, want 401", rec.Code)
	}
}

func TestSecureSessionCookie(t *testing.T) {
	pub, _ := url.Parse("https://niro.example")
	h := newHarnessWith(t, func(d *Deps) {
		d.Config.PublicURL, d.Config.DashboardURL, d.Config.SecureCookies = pub, pub, true
	})
	rec := h.do("GET", "/auth/login", "", "", nil)
	var state string
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie {
			state = c.Value
		}
	}
	req, _ := http.NewRequest("GET", "/auth/callback?code=abc&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: state})
	cb := serve(h, req)
	var found bool
	for _, c := range cb.Result().Cookies() {
		if c.Name == "__Host-niro_session" {
			found = c.Secure && c.Path == "/" && c.Domain == "" && c.HttpOnly
		}
		if c.Name == sessionCookie {
			t.Fatal("over https the cookie must carry the __Host- prefix")
		}
	}
	if !found {
		t.Fatal("no Secure __Host- session cookie")
	}
	if !strings.Contains(cb.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Fatal("HSTS missing over https")
	}
}

func serve(h *harness, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	return rec
}

func TestAPIHeadersAndErrors(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	rec := h.do("GET", "/api/v1/me", "", sid, nil)
	for k, want := range map[string]string{
		"Cache-Control":                "no-store",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if rec.Header().Get("Permissions-Policy") == "" {
		t.Error("Permissions-Policy missing")
	}
	if rec := h.do("GET", "/", "", "", nil); rec.Header().Get("Cache-Control") == "no-store" {
		t.Error("the SPA shell should not be no-store")
	}
	// Parser errors are not echoed back.
	rec = h.do("PUT", "/api/v1/guilds/100/modules/logging", `{"enabled":"yes"}`, sid, sameOrigin)
	if rec.Code != 400 {
		t.Fatalf("bad body = %d", rec.Code)
	}
	if e := decode[map[string]string](t, rec); e["message"] != "The request was not valid." {
		t.Fatalf("bad request message leaks parser detail: %q", e["message"])
	}
}
