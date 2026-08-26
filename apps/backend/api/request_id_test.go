package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDAcceptsWellFormedClientID(t *testing.T) {
	t.Parallel()

	handler := RequestIDMiddleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "0af7651916cd43dd8448eb211c80319c")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-ID")
	if got != "0af7651916cd43dd8448eb211c80319c" {
		t.Fatalf("valid client request ID echoed as %q", got)
	}
}

func TestRequestIDRejectsForgedValues(t *testing.T) {
	t.Parallel()

	forged := []string{
		"../../etc/passwd%0AInjected",
		"with spaces and\ttabs",
		strings.Repeat("x", 129), // over the 128-char cap
		"ansi\x1b[31mred",
	}

	for _, id := range forged {
		handler := RequestIDMiddleware(http.HandlerFunc(okHandler))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", id)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		got := rec.Header().Get("X-Request-ID")
		if got == id {
			t.Fatalf("forged request ID %q was echoed verbatim", id)
		}
		if len(got) != 36 || strings.Count(got, "-") != 4 {
			t.Fatalf("expected server-generated UUID, got %q", got)
		}
	}
}

func TestRequestIDMintedWhenAbsent(t *testing.T) {
	t.Parallel()

	handler := RequestIDMiddleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-ID")
	if len(got) != 36 || strings.Count(got, "-") != 4 {
		t.Fatalf("expected minted UUID, got %q", got)
	}
}
