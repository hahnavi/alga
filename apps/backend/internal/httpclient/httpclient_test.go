package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTimeoutClient(t *testing.T) {
	c := NewTimeoutClient(7 * time.Second)
	if c.Timeout != 7*time.Second {
		t.Fatalf("timeout = %v, want 7s", c.Timeout)
	}
}

func TestDoJSONSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("X-Custom"); got != "v" {
			t.Errorf("X-Custom header = %q, want %q", got, "v")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "ping" {
			t.Errorf("body = %q, want %q", string(body), "ping")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`pong`))
	}))
	defer srv.Close()

	status, respBody, err := DoJSON(context.Background(), srv.Client(), http.MethodPost, srv.URL,
		map[string]string{"X-Custom": "v"}, io.NopCloser(newStrReader("ping")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusCreated {
		t.Errorf("status = %d, want %d", status, http.StatusCreated)
	}
	if string(respBody) != "pong" {
		t.Errorf("respBody = %q, want %q", respBody, "pong")
	}
}

func TestDoJSONDoesNotCheckStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	status, respBody, err := DoJSON(context.Background(), srv.Client(), http.MethodGet, srv.URL, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for 500: %v", err)
	}
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", status, http.StatusInternalServerError)
	}
	if string(respBody) != "boom" {
		t.Errorf("respBody = %q, want %q", respBody, "boom")
	}
}

func TestDoJSONDefaultUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	if _, _, err := DoJSON(context.Background(), srv.Client(), http.MethodGet, srv.URL, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUA != DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, DefaultUserAgent)
	}
}

func TestDoJSONCallerUserAgentWins(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	const custom = "telnyx-sdk/2.3"
	// Also verifies case-insensitivity: a lowercase key must override the default.
	if _, _, err := DoJSON(context.Background(), srv.Client(), http.MethodGet, srv.URL,
		map[string]string{"user-agent": custom}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUA != custom {
		t.Errorf("User-Agent = %q, want %q", gotUA, custom)
	}
}

func TestDoJSONRequestError(t *testing.T) {
	_, _, err := DoJSON(context.Background(), http.DefaultClient, http.MethodGet, "http://invalid.\x7f", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestDoJSONTransportError(t *testing.T) {
	// A closed server yields a connection error on Do.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()
	_, _, err := DoJSON(context.Background(), srv.Client(), http.MethodGet, srv.URL, nil, nil)
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

type strReader struct {
	s string
	i int
}

func newStrReader(s string) *strReader { return &strReader{s: s} }

func (r *strReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
