// Package api serves the dashboard: the Discord OAuth2 login, the JSON API
// described in api/openapi.yaml, and the embedded single-page app.
package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/moderation"
)

// Config is what the API needs from the process configuration.
type Config struct {
	PublicURL     *url.URL
	DashboardURL  *url.URL
	SecureCookies bool
	ClientID      snowflake.ID
}

// Deps are the server's collaborators.
type Deps struct {
	Config  Config
	Log     *slog.Logger
	OAuth   OAuthClient
	KV      KV
	Guilds  core.GuildDirectory
	Configs *core.ConfigService
	Cases   moderation.CaseStore
	RPC     RPCCaller
	// Dashboard is the built SPA (index.html + assets); may be empty.
	Dashboard fs.FS
	Metrics   prometheus.Registerer
}

type Server struct {
	cfg      Config
	log      *slog.Logger
	oauth    OAuthClient
	kv       KV
	sessions sessions
	guilds   core.GuildDirectory
	config   *core.ConfigService
	cases    moderation.CaseStore
	rpc      RPCCaller
	spa      fs.FS
	requests *prometheus.CounterVec
}

// New builds the HTTP handler.
func New(d Deps) (http.Handler, error) {
	s := &Server{
		cfg: d.Config, log: d.Log.With("component", "api"), oauth: d.OAuth, kv: d.KV,
		sessions: sessions{kv: d.KV}, guilds: d.Guilds, config: d.Configs, cases: d.Cases, rpc: d.RPC, spa: d.Dashboard,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_api_requests_total", Help: "API requests by route and status class.",
		}, []string{"route", "status"}),
	}
	if d.Metrics != nil {
		if err := d.Metrics.Register(s.requests); err != nil {
			return nil, err
		}
	}

	r := chi.NewRouter()
	// No RealIP middleware: it trusts client-supplied headers. Add a trusted-proxy
	// aware variant if client IPs are ever needed (e.g. rate limiting in M5).
	r.Use(middleware.RequestID, s.logRequests, middleware.Recoverer)
	r.Use(s.securityHeaders, s.checkOrigin)
	r.Use(middleware.RequestSize(64 << 10))

	r.Route("/auth", func(r chi.Router) {
		r.Get("/login", s.handleLogin)
		r.Get("/callback", s.handleCallback)
		r.Post("/logout", s.handleLogout)
	})

	strict := apigen.NewStrictHandlerWithOptions(handlers{s}, nil, apigen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			var ae *apiError
			if errors.As(err, &ae) {
				writeError(w, ae.status, string(ae.code), ae.msg)
				return
			}
			s.log.ErrorContext(r.Context(), "api request failed", "path", r.URL.Path, "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		},
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.requireSession)
		apigen.HandlerWithOptions(strict, apigen.ChiServerOptions{
			BaseRouter: r,
			ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			},
		})
	})
	r.NotFound(s.serveSPA)
	return r, nil
}

// checkOrigin rejects state-changing requests that don't come from the
// dashboard's own origin (CSRF defence in depth, on top of SameSite cookies).
func (s *Server) checkOrigin(next http.Handler) http.Handler {
	allowed := map[string]bool{origin(s.cfg.PublicURL): true, origin(s.cfg.DashboardURL): true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		o := r.Header.Get("Origin")
		ok := (o != "" && allowed[o]) || (o == "" && r.Header.Get("Sec-Fetch-Site") == "same-origin")
		if !ok {
			writeError(w, http.StatusForbidden, "forbidden", "Cross-site request rejected.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func origin(u *url.URL) string { return u.Scheme + "://" + u.Host }

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https://cdn.discordapp.com data:; "+
			"style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self'; "+
			"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if s.cfg.SecureCookies {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		s.requests.WithLabelValues(route, statusClass(status)).Inc()
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			s.log.DebugContext(r.Context(), "request", "method", r.Method, "route", route, "status", status, "duration", time.Since(start))
		}
	})
}

func statusClass(code int) string { return string(rune('0'+code/100)) + "xx" }

// serveSPA serves the built dashboard: real files directly (hashed assets
// cached forever), everything else falls back to index.html for client-side
// routing. Unknown /api and /auth paths stay 404 JSON.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "bad_request", "Method not allowed.")
		return
	}
	if s.spa == nil {
		http.Error(w, "dashboard not built: run `make web-build`", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if f, err := fs.Stat(s.spa, name); err == nil && !f.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.spa, name)
			return
		}
	}
	index, err := fs.ReadFile(s.spa, "index.html")
	if errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "dashboard not built: run `make web-build`", http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

// compile-time check that writeError's body matches the API's Error schema.
var _ = func() bool {
	b, _ := json.Marshal(apigen.Error{Code: "x", Message: "y"})
	return string(b) == `{"code":"x","message":"y"}`
}()
