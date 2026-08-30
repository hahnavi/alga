package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"alga/config"
)

// fixedClock doubles time.Now for deterministic window rollover.
type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }
func (c *fixedClock) Advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func newTestLimiter(limit int, window time.Duration) (*memoryRateLimiter, *fixedClock) {
	clock := &fixedClock{t: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	rl := &memoryRateLimiter{
		counters: make(map[string]*windowCounter),
		limit:    limit,
		window:   window,
		ttl:      5 * time.Minute,
		nowFn:    clock.Now,
		stopCh:   make(chan struct{}),
	}
	return rl, clock
}

// TestMemoryRateLimiterFixedWindow covers the acceptance boundary: request
// #limit+1 within one window is denied; the first request of the next window
// is allowed again.
func TestMemoryRateLimiterFixedWindow(t *testing.T) {
	t.Parallel()

	rl, clock := newTestLimiter(20, time.Minute)

	for i := range 20 {
		if !rl.Allow("10.0.0.1") {
			t.Fatalf("request %d denied, want allowed", i+1)
		}
	}
	if rl.Allow("10.0.0.1") {
		t.Fatal("request 21 allowed, want denied")
	}

	clock.Advance(time.Minute)
	if !rl.Allow("10.0.0.1") {
		t.Fatal("first request of next window denied, want allowed")
	}
}

func TestMemoryRateLimiterPerKeyIsolation(t *testing.T) {
	t.Parallel()

	rl, _ := newTestLimiter(3, time.Minute)

	for range 3 {
		if !rl.Allow("10.0.0.1") {
			t.Fatal("first IP denied within limit")
		}
	}
	if rl.Allow("10.0.0.1") {
		t.Fatal("first IP allowed past limit")
	}
	if !rl.Allow("10.0.0.2") {
		t.Fatal("second IP denied despite separate budget")
	}
}

func TestMemoryRateLimiterPartialWindowRollover(t *testing.T) {
	t.Parallel()

	rl, clock := newTestLimiter(5, time.Minute)

	for range 5 {
		rl.Allow("10.0.0.1")
	}
	if rl.Allow("10.0.0.1") {
		t.Fatal("allowed past limit")
	}
	clock.Advance(30 * time.Second) // half a window: must NOT reset
	if rl.Allow("10.0.0.1") {
		t.Fatal("mid-window reset leaked budget")
	}
	clock.Advance(31 * time.Second) // past window edge: reset
	if !rl.Allow("10.0.0.1") {
		t.Fatal("post-window request denied")
	}
}

func TestMemoryRateLimiterConcurrentAllow(t *testing.T) {
	t.Parallel()

	rl, _ := newTestLimiter(5, time.Minute)

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0

	// 2000 concurrent requests across 200 keys (10 each): every key's budget
	// of 5 must hold under race, so at most 200*5 = 1000 pass.
	for i := range 2000 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ip := "10.1." + itoa(n%200) + ".1"
			if rl.Allow(ip) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if allowed > 1000 {
		t.Fatalf("allowed = %d, want <= 1000", allowed)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// fakeValkeyRateLimiter mirrors valkey.ValkeyRateLimiter's INCR+EXPIRE fixed
// window so parity can be asserted without a container harness.
type fakeValkeyRateLimiter struct {
	counts map[string]int
	burst  int
	nowFn  func() time.Time
	starts map[string]time.Time
	window time.Duration
}

func newFakeValkeyRateLimiter(burst int, nowFn func() time.Time) *fakeValkeyRateLimiter {
	return &fakeValkeyRateLimiter{
		counts: make(map[string]int),
		starts: make(map[string]time.Time),
		burst:  burst,
		nowFn:  nowFn,
		window: time.Minute,
	}
}

func (f *fakeValkeyRateLimiter) Allow(ip string) bool {
	now := f.nowFn()
	start, ok := f.starts[ip]
	if !ok || now.Sub(start) >= f.window {
		f.counts[ip] = 0
		f.starts[ip] = now
	}
	f.counts[ip]++
	return f.counts[ip] <= f.burst
}

// TestRateLimiterParity drives both implementations through identical request
// sequences and asserts identical allow/deny decisions — the contract
// that memory and Valkey modes cannot diverge.
func TestRateLimiterParity(t *testing.T) {
	t.Parallel()

	clock := &fixedClock{t: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	mem := &memoryRateLimiter{
		counters: make(map[string]*windowCounter),
		limit:    20,
		window:   time.Minute,
		ttl:      5 * time.Minute,
		nowFn:    clock.Now,
		stopCh:   make(chan struct{}),
	}
	valkeyLike := newFakeValkeyRateLimiter(20, clock.Now)

	type step struct {
		ip      string
		advance time.Duration
	}
	steps := []step{
		{"a", 0}, {"a", 0}, {"a", 0}, {"b", 0},
		{"a", 10 * time.Second}, {"a", 0}, {"a", 0},
		{"c", 25 * time.Second}, {"a", 0}, {"a", 0}, {"a", 0},
		{"a", 30 * time.Second}, {"a", 0}, {"b", 0}, {"a", 0},
	}

	for i, s := range steps {
		clock.Advance(s.advance)
		gotMem := mem.Allow(s.ip)
		gotVk := valkeyLike.Allow(s.ip)
		if gotMem != gotVk {
			t.Fatalf("step %d ip %s: memory=%v valkey=%v (parity broken)", i, s.ip, gotMem, gotVk)
		}
	}
}

// TestMemoryRateLimiterEnvCeiling asserts the configured ceiling is honored
// (the env-override acceptance criterion, via constructor seam).
func TestMemoryRateLimiterEnvCeiling(t *testing.T) {
	t.Parallel()

	rl, _ := newTestLimiter(300, time.Minute)
	for range 300 {
		if !rl.Allow("collector") {
			t.Fatalf("denied within configured ceiling of 300")
		}
	}
	if rl.Allow("collector") {
		t.Fatal("allowed past configured ceiling")
	}
}

// extractClientIP builds the extractor under test and resolves the client IP
// for a synthetic request with the given RemoteAddr and X-Forwarded-For.
func extractClientIP(t *testing.T, trusted []string, remoteAddr, xff string) string {
	t.Helper()
	ex := newIPExtractor(&config.Config{TrustedProxies: trusted})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return ex.ClientIP(req)
}

func TestIPExtractorNoTrustedProxiesIgnoresXFF(t *testing.T) {
	t.Parallel()

	// Without TRUSTED_PROXIES the header is never consulted, so a directly
	// exposed endpoint can't rotate the rate-limit key via a spoofed header.
	got := extractClientIP(t, nil, "203.0.113.9:443", "198.51.100.1, 198.51.100.2")
	if got != "203.0.113.9" {
		t.Fatalf("got %q, want RemoteAddr 203.0.113.9", got)
	}
}

func TestIPExtractorUntrustedRemoteIgnoresXFF(t *testing.T) {
	t.Parallel()

	// Even with trusted proxies configured, a request whose RemoteAddr is
	// not one of them must not have its XFF honored.
	got := extractClientIP(t, []string{"10.0.0.0/8"}, "203.0.113.9:443", "198.51.100.1")
	if got != "203.0.113.9" {
		t.Fatalf("got %q, want RemoteAddr 203.0.113.9", got)
	}
}

func TestIPExtractorRightmostUntrustedWins(t *testing.T) {
	t.Parallel()

	// Proxy appends the real client after the client-supplied (spoofable)
	// leftmost entry; the rightmost untrusted entry is the only trustworthy
	// one.
	got := extractClientIP(t, []string{"10.0.0.1"}, "10.0.0.1:5555", "198.51.100.1, 198.51.100.7")
	if got != "198.51.100.7" {
		t.Fatalf("got %q, want 198.51.100.7", got)
	}
}

func TestIPExtractorSkipsTrustedChain(t *testing.T) {
	t.Parallel()

	// Chained trusted proxies each append themselves; all trusted entries
	// are skipped down to the first untrusted (client) IP.
	got := extractClientIP(t, []string{"10.0.0.0/8"}, "10.0.0.2:5555", "198.51.100.7, 10.0.0.1, 10.0.0.3")
	if got != "198.51.100.7" {
		t.Fatalf("got %q, want 198.51.100.7", got)
	}
}

func TestIPExtractorAllTrustedFallsBackToRemote(t *testing.T) {
	t.Parallel()

	got := extractClientIP(t, []string{"10.0.0.0/8"}, "10.0.0.1:5555", "10.0.0.2, 10.0.0.3")
	if got != "10.0.0.1" {
		t.Fatalf("got %q, want RemoteAddr 10.0.0.1", got)
	}
}

func TestIPExtractorSkipsJunkEntries(t *testing.T) {
	t.Parallel()

	got := extractClientIP(t, []string{"10.0.0.1"}, "10.0.0.1:5555", "junk, 198.51.100.7")
	if got != "198.51.100.7" {
		t.Fatalf("got %q, want 198.51.100.7", got)
	}
}

func TestIPExtractorSingleIPTrustedEntry(t *testing.T) {
	t.Parallel()

	// A bare IP (no CIDR suffix) is accepted as a /32 or /128 entry.
	got := extractClientIP(t, []string{"10.0.0.1"}, "10.0.0.1:5555", "198.51.100.7")
	if got != "198.51.100.7" {
		t.Fatalf("got %q, want 198.51.100.7", got)
	}
}
