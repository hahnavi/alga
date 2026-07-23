package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}
	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestStrictTransportSecurity(t *testing.T) {
	handler := StrictTransportSecurity(http.HandlerFunc(okHandler))

	// HSTS is HTTPS-conditional (ASVS V12.5): emit only on TLS or
	// X-Forwarded-Proto=https, never over plain HTTP.
	t.Run("https_tls", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		req.TLS = &tls.ConnectionState{} //nolint:exhaustruct // test zero value
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		got := rec.Header().Get("Strict-Transport-Security")
		if !strings.Contains(got, "max-age=63072000") || !strings.Contains(got, "includeSubDomains") || !strings.Contains(got, "preload") {
			t.Fatalf("Strict-Transport-Security on HTTPS: got %q", got)
		}
	})

	t.Run("http_not_emitted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
			t.Fatalf("HSTS must not be emitted over HTTP, got %q", got)
		}
	})
}
