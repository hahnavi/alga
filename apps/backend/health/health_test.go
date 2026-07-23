package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

var okCheck Check = func(ctx context.Context) error { return nil }

var failCheck Check = func(ctx context.Context) error { return context.DeadlineExceeded }

func decode(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var r Response
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("response not valid JSON: %v (body=%s)", err, w.Body.String())
	}
	return r
}

func TestLive(t *testing.T) {
	h := NewHandler(0, nil)
	w := httptest.NewRecorder()
	h.Live(w, httptest.NewRequest(http.MethodGet, "/live", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := decode(t, w); got.Status != "ok" {
		t.Fatalf("expected status ok, got %q", got.Status)
	}
}

func TestLiveRejectsNonGet(t *testing.T) {
	h := NewHandler(0, nil)
	w := httptest.NewRecorder()
	h.Live(w, httptest.NewRequest(http.MethodPost, "/live", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestReadyAllHealthy(t *testing.T) {
	h := NewHandler(0, map[string]Check{
		"postgres": okCheck,
		"rabbitmq": okCheck,
		"valkey":   okCheck,
	})
	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := decode(t, w)
	if got.Status != "ok" {
		t.Fatalf("expected status ok, got %q", got.Status)
	}
	for name, dep := range got.Dependencies {
		if dep.Status != "ok" || !dep.Checked {
			t.Fatalf("dependency %q not healthy: %+v", name, dep)
		}
	}
}

func TestReadyDegraded(t *testing.T) {
	h := NewHandler(0, map[string]Check{
		"postgres": okCheck,
		"rabbitmq": failCheck,
		"valkey":   okCheck,
	})
	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	got := decode(t, w)
	if got.Status != "degraded" {
		t.Fatalf("expected status degraded, got %q", got.Status)
	}
	if got.Dependencies["rabbitmq"].Status != "degraded" {
		t.Fatalf("rabbitmq should be degraded: %+v", got.Dependencies["rabbitmq"])
	}
	if got.Dependencies["postgres"].Status != "ok" {
		t.Fatalf("postgres should be ok: %+v", got.Dependencies["postgres"])
	}
}

func TestReadyNilChecksAreOptional(t *testing.T) {
	h := NewHandler(0, map[string]Check{
		"postgres": okCheck,
		"valkey":   nil, // not configured
	})
	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := decode(t, w)
	if got.Dependencies["valkey"].Checked {
		t.Fatalf("nil check should report checked=false, got %+v", got.Dependencies["valkey"])
	}
	if got.Dependencies["valkey"].Status != "ok" {
		t.Fatalf("nil check should report ok, got %+v", got.Dependencies["valkey"])
	}
}

func TestHealthAliasRoutesToReady(t *testing.T) {
	h := NewHandler(0, map[string]Check{"postgres": failCheck})
	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health to behave like /ready (503), got %d", w.Code)
	}
}

func TestMuxRoutes(t *testing.T) {
	h := NewHandler(0, nil)
	mux := h.Mux()

	for _, path := range []string{"/live", "/ready", "/health"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code == 0 {
			t.Fatalf("no response for %s", path)
		}
	}
}
