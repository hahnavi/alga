package api

import (
	"expvar"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// expvars must be published once per process; -cpu lists rerun every test in
// the same process, so in-test expvar.NewInt panics with a duplicate name.
var (
	testPromFormatVar = expvar.NewInt("test_prom_format_var")
	testSanitizeVar   = expvar.NewInt("test.sanitize-check/name")
)

func TestMetricsHandler_ReturnsPrometheusFormat(t *testing.T) {
	testPromFormatVar.Set(42)

	h := NewMetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %s", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "# TYPE") || !strings.Contains(body, "# HELP") {
		t.Fatalf("expected Prometheus format with # TYPE and # HELP lines, got:\n%s", body)
	}

	if !strings.Contains(body, "test_prom_format_var 42") {
		t.Fatalf("expected test_prom_format_var metric in output, got:\n%s", body)
	}
}

func TestMetricsHandler_SanitizesNames(t *testing.T) {
	testSanitizeVar.Set(1)

	h := NewMetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# TYPE ") || strings.HasPrefix(line, "# HELP ") {
			continue
		}
		if strings.Contains(line, "test.sanitize-check") {
			t.Fatalf("expected dots/dashes to be replaced with underscores in metric line, got: %s\nfull output:\n%s", line, body)
		}
	}
	if !strings.Contains(body, "test_sanitize_check_name ") {
		t.Fatalf("expected sanitized metric name, got:\n%s", body)
	}
}

func TestMetricsHandler_HandlesEmptyExpvar(t *testing.T) {
	h := NewMetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty body")
	}
}
