package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/modules/moderation"
)

// ---- fakes ----------------------------------------------------------------

type memKV struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (k *memKV) GetJSON(_ context.Context, key string, v any) (bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	b, ok := k.m[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(b, v)
}

func (k *memKV) SetJSON(_ context.Context, key string, v any, _ time.Duration) error {
	b, err := json.Marshal(v)
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[key] = b
	return err
}

func (k *memKV) Delete(_ context.Context, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, key)
	return nil
}

type fakeOAuth struct {
	guilds     []UserGuild
	guildCalls int
}

func (f *fakeOAuth) AuthorizeURL(state string) string {
	return "https://discord.com/oauth2/authorize?state=" + state
}

func (f *fakeOAuth) Exchange(_ context.Context, code string) (Token, error) {
	return Token{AccessToken: "tok-" + code, ExpiresIn: time.Hour}, nil
}

func (f *fakeOAuth) CurrentUser(context.Context, string) (Profile, error) {
	return Profile{ID: 7, Username: "ana", AvatarURL: "https://cdn.discordapp.com/a.png"}, nil
}

func (f *fakeOAuth) CurrentUserGuilds(context.Context, string) ([]UserGuild, error) {
	f.guildCalls++
	return f.guilds, nil
}

type fakeDir struct{ active map[snowflake.ID]bool }

func (f fakeDir) UpsertGuild(context.Context, core.GuildInfo) error { return nil }
func (f fakeDir) MarkGuildLeft(context.Context, snowflake.ID) error { return nil }
func (f fakeDir) IsGuildActive(_ context.Context, id snowflake.ID) (bool, error) {
	return f.active[id], nil
}
func (f fakeDir) ActiveGuilds(_ context.Context, ids []snowflake.ID) (map[snowflake.ID]bool, error) {
	out := map[snowflake.ID]bool{}
	for _, id := range ids {
		if f.active[id] {
			out[id] = true
		}
	}
	return out, nil
}

type fakeConfigStore struct {
	mu   sync.Mutex
	rows map[core.ModuleID]core.StoredModuleState
}

func (s *fakeConfigStore) ListModuleStates(context.Context, snowflake.ID) ([]core.StoredModuleState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []core.StoredModuleState
	for _, r := range s.rows {
		out = append(out, r)
	}
	return out, nil
}

func (s *fakeConfigStore) SetModuleEnabled(_ context.Context, _ snowflake.ID, m core.ModuleID, on bool, _ *snowflake.ID, _ core.ChangeSource) (core.StoredModuleState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rows[m]
	r.Module, r.Enabled, r.Version, r.UpdatedAt = m, on, r.Version+1, time.Unix(1700000000, 0).UTC()
	s.rows[m] = r
	return r, nil
}

func (s *fakeConfigStore) SetModuleSettings(_ context.Context, _ snowflake.ID, m core.ModuleID, cfg []byte, def bool, _ *snowflake.ID, _ core.ChangeSource) (core.StoredModuleState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[m]
	if !ok {
		r.Enabled = def
	}
	r.Module, r.Config, r.Version = m, cfg, r.Version+1
	s.rows[m] = r
	return r, nil
}

type nopBus struct{}

func (nopBus) PublishConfigChange(context.Context, core.ConfigChange) error { return nil }
func (nopBus) SubscribeConfigChanges(ctx context.Context, _ func(core.ConfigChange)) error {
	<-ctx.Done()
	return nil
}

// ---- harness --------------------------------------------------------------

const (
	managedGuild    snowflake.ID = 100 // user has Manage Server, bot present
	noBotGuild      snowflake.ID = 200 // user owns it, bot absent
	memberOnlyGuild snowflake.ID = 300 // user is a plain member
)

type harness struct {
	t     *testing.T
	h     http.Handler
	kv    *memKV
	oauth *fakeOAuth
	rpc   *fakeRPC
	cases *fakeCases
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pub, _ := url.Parse("http://localhost:8080")
	kv := &memKV{m: map[string][]byte{}}
	oauth := &fakeOAuth{guilds: []UserGuild{
		{ID: managedGuild, Name: "Beta", Permissions: discord.PermissionManageGuild},
		{ID: noBotGuild, Name: "alpha", Owner: true},
		{ID: memberOnlyGuild, Name: "Gamma", Permissions: discord.PermissionSendMessages},
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg, _ := core.NewRegistry(core.WithSettings(core.DefaultModules(), core.ModuleModeration, moderation.SettingsSpec{})...)
	frpc := &fakeRPC{handlers: map[string]func(params any, out any) error{}}
	fcases := &fakeCases{}
	cfgs, _ := core.NewConfigService(reg, &fakeConfigStore{rows: map[core.ModuleID]core.StoredModuleState{}}, nopBus{}, log)

	h, err := New(Deps{
		Config: Config{PublicURL: pub, DashboardURL: pub, ClientID: 999},
		Log:    log, OAuth: oauth, KV: kv,
		Guilds:    fakeDir{active: map[snowflake.ID]bool{managedGuild: true}},
		Configs:   cfgs,
		Cases:     fcases,
		RPC:       frpc,
		Dashboard: fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}, "assets/app.js": {Data: []byte("js")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, h: h, kv: kv, oauth: oauth, rpc: frpc, cases: fcases}
}

func (h *harness) do(method, target, body, cookie string, hdr map[string]string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	return rec
}

// login runs the real OAuth flow against the fake Discord and returns the session cookie.
func (h *harness) login() string {
	h.t.Helper()
	rec := h.do("GET", "/auth/login", "", "", nil)
	if rec.Code != http.StatusFound {
		h.t.Fatalf("/auth/login = %d", rec.Code)
	}
	var state string
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie {
			state = c.Value
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				h.t.Fatal("state cookie must be HttpOnly and SameSite=Lax")
			}
		}
	}
	if state == "" || !strings.Contains(rec.Header().Get("Location"), "state="+state) {
		h.t.Fatal("state not set or not sent to Discord")
	}

	req := httptest.NewRequest("GET", "/auth/callback?code=abc&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: state})
	cb := httptest.NewRecorder()
	h.h.ServeHTTP(cb, req)
	if cb.Code != http.StatusFound || cb.Header().Get("Location") != "http://localhost:8080/" {
		h.t.Fatalf("callback = %d → %s", cb.Code, cb.Header().Get("Location"))
	}
	for _, c := range cb.Result().Cookies() {
		if c.Name == sessionCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				h.t.Fatal("session cookie must be HttpOnly and SameSite=Lax")
			}
			return c.Value
		}
	}
	h.t.Fatal("no session cookie set")
	return ""
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return v
}

var sameOrigin = map[string]string{"Origin": "http://localhost:8080"}

// ---- tests ----------------------------------------------------------------

func TestAPIRequiresLogin(t *testing.T) {
	h := newHarness(t)
	for _, p := range []string{"/api/v1/me", "/api/v1/guilds", "/api/v1/guilds/100/modules"} {
		if rec := h.do("GET", p, "", "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without session = %d, want 401", p, rec.Code)
		}
	}
	if rec := h.do("GET", "/api/v1/me", "", "forged-session-id", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("unknown session id = %d, want 401", rec.Code)
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest("GET", "/auth/callback?code=abc&state=attacker", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "victim"})
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "login=failed") {
		t.Fatalf("mismatched state should fail, redirected to %q", loc)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			t.Fatal("session created despite bad state")
		}
	}
}

func TestLoginMeAndLogout(t *testing.T) {
	h := newHarness(t)
	sid := h.login()

	me := decode[map[string]any](t, h.do("GET", "/api/v1/me", "", sid, nil))
	if me["id"] != "7" || me["username"] != "ana" {
		t.Fatalf("/me = %v", me)
	}
	for k := range h.kv.m {
		if strings.Contains(k, sid) {
			t.Fatal("raw session id stored in Valkey; it must be hashed")
		}
	}

	if rec := h.do("POST", "/auth/logout", "", sid, sameOrigin); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := h.do("GET", "/api/v1/me", "", sid, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session still valid after logout: %d", rec.Code)
	}
}

func TestListGuildsFiltersAndSorts(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	guilds := decode[[]map[string]any](t, h.do("GET", "/api/v1/guilds", "", sid, nil))
	if len(guilds) != 2 {
		t.Fatalf("got %d guilds, want 2 (member-only server hidden): %v", len(guilds), guilds)
	}
	if guilds[0]["id"] != "100" || guilds[0]["botPresent"] != true || guilds[1]["id"] != "200" || guilds[1]["botPresent"] != false {
		t.Fatalf("want bot-present first: %v", guilds)
	}
	h.do("GET", "/api/v1/guilds", "", sid, nil)
	if h.oauth.guildCalls != 1 {
		t.Fatalf("Discord called %d times, want 1 (cached)", h.oauth.guildCalls)
	}
}

func TestGuildAuthorization(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	cases := []struct {
		path string
		want int
	}{
		{"/api/v1/guilds/100", 200},
		{"/api/v1/guilds/200", 200},         // can manage; bot absent is fine here
		{"/api/v1/guilds/300", 403},         // member only
		{"/api/v1/guilds/999", 403},         // not in it at all: don't reveal existence
		{"/api/v1/guilds/100/modules", 200}, //
		{"/api/v1/guilds/200/modules", 409}, // bot not in guild
		{"/api/v1/guilds/300/modules", 403},
		{"/api/v1/guilds/abc", 403}, // malformed IDs are treated like unknown servers
	}
	for _, c := range cases {
		if rec := h.do("GET", c.path, "", sid, nil); rec.Code != c.want {
			t.Errorf("GET %s = %d, want %d (%s)", c.path, rec.Code, c.want, rec.Body.String())
		}
	}
}

func TestUpdateModule(t *testing.T) {
	h := newHarness(t)
	sid := h.login()
	put := func(path, body string, hdr map[string]string) *httptest.ResponseRecorder {
		return h.do("PUT", path, body, sid, hdr)
	}

	rec := put("/api/v1/guilds/100/modules/logging", `{"enabled":true}`, sameOrigin)
	if rec.Code != 200 {
		t.Fatalf("enable logging = %d %s", rec.Code, rec.Body.String())
	}
	st := decode[map[string]any](t, rec)
	if st["enabled"] != true || st["version"] != float64(1) {
		t.Fatalf("state = %v", st)
	}

	mods := decode[[]map[string]any](t, h.do("GET", "/api/v1/guilds/100/modules", "", sid, nil))
	for _, m := range mods {
		if m["id"] == "logging" && m["enabled"] != true {
			t.Fatal("change not visible in module list")
		}
	}

	for _, c := range []struct {
		path, body string
		hdr        map[string]string
		want       int
	}{
		{"/api/v1/guilds/100/modules/core", `{"enabled":false}`, sameOrigin, 400},
		{"/api/v1/guilds/100/modules/nope", `{"enabled":true}`, sameOrigin, 404},
		{"/api/v1/guilds/300/modules/logging", `{"enabled":true}`, sameOrigin, 403},
		{"/api/v1/guilds/100/modules/logging", `{"enabled":true}`, map[string]string{"Origin": "https://evil.example"}, 403},
		{"/api/v1/guilds/100/modules/logging", `{"enabled":true}`, nil, 403}, // no Origin, no Sec-Fetch-Site
		{"/api/v1/guilds/100/modules/logging", `{"enabled":true}`, map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"/api/v1/guilds/100/modules/logging", `not json`, sameOrigin, 400},
	} {
		if rec := put(c.path, c.body, c.hdr); rec.Code != c.want {
			t.Errorf("PUT %s %s %v = %d, want %d (%s)", c.path, c.body, c.hdr, rec.Code, c.want, rec.Body.String())
		}
	}
}

func TestInviteIsPublic(t *testing.T) {
	h := newHarness(t)
	rec := h.do("GET", "/api/v1/invite?guildId=100", "", "", nil)
	if rec.Code != 200 {
		t.Fatalf("invite = %d", rec.Code)
	}
	u := decode[map[string]string](t, rec)["url"]
	for _, want := range []string{"client_id=999", "guild_id=100", "disable_guild_select=true", "scope=bot+applications.commands"} {
		if !strings.Contains(u, want) {
			t.Errorf("invite URL %s missing %s", u, want)
		}
	}
}

func TestSPAAndSecurityHeaders(t *testing.T) {
	h := newHarness(t)
	rec := h.do("GET", "/guilds/100", "", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "spa") {
		t.Fatalf("client route should serve index.html: %d", rec.Code)
	}
	if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers missing")
	}
	if rec := h.do("GET", "/assets/app.js", "", "", nil); !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("hashed assets should be cached immutably")
	}
	if rec := h.do("GET", "/api/v1/nope", "", "", nil); rec.Code != 404 && rec.Code != 401 {
		t.Fatalf("unknown API path = %d", rec.Code)
	}
}
