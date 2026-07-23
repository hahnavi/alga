package app

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"alga/config"
)

// TestHTTPServerReadHeaderTimeoutDropsSlowloris verifies the H1 hardening:
// the http.Server configured with config defaults drops a client that stalls
// sending request headers past ServerReadHeaderTimeout. This is the slowloris
// protection (ASVS V12.1). It also confirms the timeout fields are non-zero so
// the server cannot be constructed with slowloris-vulnerable zero timeouts.
func TestHTTPServerReadHeaderTimeoutDropsSlowloris(t *testing.T) {
	cfg := config.Defaults()

	if cfg.ServerReadHeaderTimeout <= 0 {
		t.Fatalf("ServerReadHeaderTimeout must be non-zero, got %v", cfg.ServerReadHeaderTimeout)
	}
	if cfg.ServerReadTimeout <= 0 || cfg.ServerWriteTimeout <= 0 || cfg.ServerIdleTimeout <= 0 {
		t.Fatalf("server timeouts must all be non-zero: read=%v write=%v idle=%v",
			cfg.ServerReadTimeout, cfg.ServerWriteTimeout, cfg.ServerIdleTimeout)
	}
	if cfg.ServerMaxHeaderBytes <= 0 {
		t.Fatalf("ServerMaxHeaderBytes must be non-zero, got %d", cfg.ServerMaxHeaderBytes)
	}

	// Use small timeouts to keep the test fast; the configured defaults
	// (10s/30s) are what ship, but the protection holds for any positive value.
	cfg.ServerReadHeaderTimeout = 200 * time.Millisecond
	cfg.ServerReadTimeout = 400 * time.Millisecond

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           mux,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		ReadTimeout:       cfg.ServerReadTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
		MaxHeaderBytes:    cfg.ServerMaxHeaderBytes,
	}
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	// Sanity: a well-behaved request succeeds (status line contains 200).
	if resp := doRawRequest(t, ln.Addr().String(), "GET /ping HTTP/1.1\r\nHost: localhost\r\n\r\n"); !strings.Contains(resp, " 200 ") {
		t.Fatalf("normal request expected 200, got: %q", resp)
	}

	// Slowloris: open a connection, send only the request line, then stall.
	// The server must close the connection within ~ReadHeaderTimeout.
	start := time.Now()
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 1*time.Second)
	if err != nil {
		t.Fatalf("dial slow client: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("GET /ping HTTP/1.1\r\n")); err != nil {
		t.Fatalf("write partial request line: %v", err)
	}

	// A read should return EOF/error once the server drops the stalled
	// connection, well before a generous safety bound.
	deadline := time.After(2 * time.Second)
	buf := make([]byte, 256)
	readCh := make(chan readResult, 1)
	go func() {
		n, err := conn.Read(buf)
		readCh <- readResult{n: n, err: err}
	}()
	select {
	case res := <-readCh:
		elapsed := time.Since(start)
		if res.err == nil && res.n > 0 {
			t.Fatalf("server responded to slowloris connection instead of dropping (elapsed=%v, reply=%q)",
				elapsed, string(buf[:res.n]))
		}
		if elapsed > 1*time.Second {
			t.Fatalf("server took too long (%v) to drop slowloris connection", elapsed)
		}
		t.Logf("server dropped stalled headers after %v", elapsed)
	case <-deadline:
		t.Fatal("server did not drop the slowloris connection within 2s")
	}

	// The dropped connection must not have killed the server.
	select {
	case err := <-serverErr:
		t.Fatalf("server.Serve returned unexpectedly: %v", err)
	default:
	}
}

type readResult struct {
	n   int
	err error
}

func doRawRequest(t *testing.T, addr, raw string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	return string(buf[:n])
}
