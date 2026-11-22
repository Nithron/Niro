// Package observability sets up logging, Prometheus metrics and the ops HTTP
// server (liveness, readiness, metrics) that every Niro process exposes.
package observability

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewLogger returns a slog logger writing text (development) or JSON (production).
func NewLogger(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

// NewRegistry returns a Prometheus registry with Go runtime and process collectors.
func NewRegistry() *prometheus.Registry {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return r
}

// Check is a readiness probe; it should return quickly.
type Check func(ctx context.Context) error

// Checks is a concurrency-safe set of named readiness checks. Components
// register their checks as they start.
type Checks struct {
	mu     sync.RWMutex
	checks map[string]Check
}

func NewChecks() *Checks { return &Checks{checks: map[string]Check{}} }

func (c *Checks) Add(name string, check Check) {
	c.mu.Lock()
	c.checks[name] = check
	c.mu.Unlock()
}

// Run executes every check with a shared timeout and returns name → error text ("" if OK).
func (c *Checks) Run(ctx context.Context) (map[string]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	results := make(map[string]string, len(c.checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	ok := true
	for name, check := range c.checks {
		wg.Go(func() {
			err := check(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results[name] = err.Error()
				ok = false
			} else {
				results[name] = ""
			}
		})
	}
	wg.Wait()
	return results, ok
}

// OpsHandler serves /healthz (process is alive), /readyz (dependencies are
// reachable) and /metrics. Bind it to an internal address only.
func OpsHandler(reg *prometheus.Registry, checks *Checks) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		results, ok := checks.Run(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": ok, "checks": results})
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	return mux
}
