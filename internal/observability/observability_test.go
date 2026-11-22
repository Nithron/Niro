package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpsEndpoints(t *testing.T) {
	checks := NewChecks()
	checks.Add("postgres", func(context.Context) error { return nil })
	h := OpsHandler(NewRegistry(), checks)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	if rec := get("/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("/healthz = %d", rec.Code)
	}
	if rec := get("/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("/readyz with passing checks = %d", rec.Code)
	}
	if rec := get("/metrics"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatalf("/metrics = %d", rec.Code)
	}

	checks.Add("valkey", func(context.Context) error { return errors.New("connection refused") })
	rec := get("/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz with failing check = %d", rec.Code)
	}
	var body struct {
		Ready  bool
		Checks map[string]string
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Ready || body.Checks["valkey"] != "connection refused" || body.Checks["postgres"] != "" {
		t.Fatalf("readyz body = %+v", body)
	}
}
