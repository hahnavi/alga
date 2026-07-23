package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"alga/config"
	"alga/health"
)

func newHealthTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{cfg: &config.Config{}}
	s.SetHealthHandler(health.NewHandler(0, map[string]health.Check{
		"postgres": func(ctx context.Context) error { return nil },
	}))
	return s
}

func TestRootHealthEndpoints(t *testing.T) {
	s := newHealthTestServer(t)
	mux := http.NewServeMux()
	s.Register(mux)

	cases := []struct {
		path       string
		wantStatus int
	}{
		{"/live", http.StatusOK},
		{"/ready", http.StatusOK},
		{"/health", http.StatusOK},
		{"/api/v1/readiness", http.StatusOK},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.path, nil))
		if w.Code != c.wantStatus {
			t.Fatalf("%s: expected %d, got %d (body=%s)", c.path, c.wantStatus, w.Code, w.Body.String())
		}
	}
}
