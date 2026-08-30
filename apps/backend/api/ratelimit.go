package api

import (
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"alga/api/platform"
	"alga/config"
	"alga/logger"
)

// RateLimiting is the interface for per-IP rate limiting.
type RateLimiting interface {
	Allow(ip string) bool
	Stop()
}

type LoginRateLimiting interface {
	CheckLoginAllowed(ip string) (allowed bool, remaining int, lockedUntil *time.Time)
	Reset(ip string)
	Stop()
}

// ipExtractor handles trusted proxy configuration for IP extraction
type ipExtractor struct {
	trustedProxies []*net.IPNet
}

// NewIPExtractor builds the trusted-proxy-aware client-IP extractor from
// TRUSTED_PROXIES for consumers outside this package (webhook ingress rate
// limiting). With no trusted proxies configured it never consults
// X-Forwarded-For.
func NewIPExtractor(cfg *config.Config) platform.IPExtractor {
	return newIPExtractor(cfg)
}

func newIPExtractor(cfg *config.Config) *ipExtractor {
	ex := &ipExtractor{}
	for _, cidr := range cfg.TrustedProxies {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		// Parse as CIDR
		if strings.Contains(cidr, "/") {
			_, ipNet, err := net.ParseCIDR(cidr)
			if err == nil {
				ex.trustedProxies = append(ex.trustedProxies, ipNet)
			}
		} else {
			// Single IP - convert to /32 or /128
			ip := net.ParseIP(cidr)
			if ip != nil {
				var mask net.IPMask
				if ip.To4() != nil {
					mask = net.CIDRMask(32, 32)
				} else {
					mask = net.CIDRMask(128, 128)
				}
				ex.trustedProxies = append(ex.trustedProxies, &net.IPNet{IP: ip, Mask: mask})
			}
		}
	}
	return ex
}

// ClientIP is the exported accessor delegating to clientIP so *ipExtractor
// satisfies platform.IPExtractor.
func (ex *ipExtractor) ClientIP(r *http.Request) string {
	return ex.clientIP(r)
}

// clientIP extracts the client IP from the request, respecting trusted proxies.

func (ex *ipExtractor) clientIP(r *http.Request) string {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	// If no trusted proxies configured, only use RemoteAddr
	if len(ex.trustedProxies) == 0 {
		return remoteIP
	}

	// Check if the request came from a trusted proxy
	remoteAddrIP := net.ParseIP(remoteIP)
	if remoteAddrIP == nil {
		return remoteIP
	}

	isTrusted := false
	for _, trusted := range ex.trustedProxies {
		if trusted.Contains(remoteAddrIP) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		// Request not from trusted proxy - ignore X-Forwarded-For
		return remoteIP
	}

	// Request from trusted proxy - use rightmost entry from X-Forwarded-For
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remoteIP
	}

	// XFF format: client, proxy1, proxy2, ...
	// Use rightmost untrusted entry (the one before our trusted proxy)
	ips := strings.Split(xff, ",")
	for i := len(ips) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(ips[i])
		if ip == "" {
			continue
		}
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			continue
		}

		// Check if this IP is from a trusted proxy
		isTrustedProxy := false
		for _, trusted := range ex.trustedProxies {
			if trusted.Contains(parsedIP) {
				isTrustedProxy = true
				break
			}
		}

		// Return the first non-trusted IP from the right
		if !isTrustedProxy {
			return ip
		}
	}

	// All IPs in XFF are trusted proxies, use RemoteAddr
	return remoteIP
}

// memoryRateLimiter implements per-IP fixed-window counting, matching the
// Valkey implementation's semantics exactly: each key gets `limit`
// requests per `window`, resetting on window rollover. Previously this was a
// token bucket whose effective allowance diverged ~30x from the Valkey path
// depending on deployment mode. `nowFn` is a seam for tests that double the
// clock.
type memoryRateLimiter struct {
	mu       sync.RWMutex
	counters map[string]*windowCounter
	limit    int
	window   time.Duration
	ttl      time.Duration
	nowFn    func() time.Time
	stopCh   chan struct{}
}

type windowCounter struct {
	count       int
	windowStart time.Time
	lastSeen    time.Time
}

// NewRateLimiter creates a fixed-window rate limiter (e.g. limit=20,
// window=time.Minute for the documented public-surface contract).
func NewRateLimiter(limit int, window time.Duration) RateLimiting {
	if limit <= 0 || window <= 0 {
		logger.Error("rate limiter misconfigured; allowing all traffic", "component", "api", "limit", limit, "window", window.String())
		return allowAllLimiter{}
	}
	rl := &memoryRateLimiter{
		counters: make(map[string]*windowCounter),
		limit:    limit,
		window:   window,
		ttl:      5 * time.Minute,
		nowFn:    time.Now,
		stopCh:   make(chan struct{}),
	}

	go rl.cleanup()

	return rl
}

type allowAllLimiter struct{}

func (allowAllLimiter) Allow(string) bool { return true }
func (allowAllLimiter) Stop()             {}

func (rl *memoryRateLimiter) Stop() {
	close(rl.stopCh)
}

// Allow checks if a request from the given IP is allowed
func (rl *memoryRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.nowFn()
	c, exists := rl.counters[ip]
	if !exists {
		c = &windowCounter{count: 0, windowStart: now, lastSeen: now}
		rl.counters[ip] = c
	}
	c.lastSeen = now

	if now.Sub(c.windowStart) >= rl.window {
		c.count = 0
		c.windowStart = now
	}

	if c.count >= rl.limit {
		return false
	}
	c.count++
	return true
}

// cleanup removes stale limiters periodically
func (rl *memoryRateLimiter) cleanup() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("rate limiter cleanup panicked", "component", "api", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
		}
		rl.mu.Lock()
		for ip, c := range rl.counters {
			if time.Since(c.lastSeen) > rl.ttl {
				delete(rl.counters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// memoryLoginRateLimiter implements stricter rate limiting for login attempts
type memoryLoginRateLimiter struct {
	mu              sync.RWMutex
	attempts        map[string]*loginAttempts
	maxAttempts     int
	window          time.Duration
	lockoutDuration time.Duration
	stopCh          chan struct{}
}

type loginAttempts struct {
	count       int
	firstSeen   time.Time
	lastSeen    time.Time
	lockedUntil *time.Time
}

// NewLoginRateLimiter creates a login-specific rate limiter
func NewLoginRateLimiter(maxAttempts int, window, lockoutDuration time.Duration) LoginRateLimiting {
	lrl := &memoryLoginRateLimiter{
		attempts:        make(map[string]*loginAttempts),
		maxAttempts:     maxAttempts,
		window:          window,
		lockoutDuration: lockoutDuration,
		stopCh:          make(chan struct{}),
	}

	go lrl.cleanup()

	return lrl
}

func (lrl *memoryLoginRateLimiter) Stop() {
	close(lrl.stopCh)
}

// CheckLoginAllowed checks if login is allowed and increments the attempt counter
func (lrl *memoryLoginRateLimiter) CheckLoginAllowed(ip string) (allowed bool, remaining int, lockedUntil *time.Time) {
	lrl.mu.Lock()
	defer lrl.mu.Unlock()

	now := time.Now()
	attempts, exists := lrl.attempts[ip]

	if !exists {
		lrl.attempts[ip] = &loginAttempts{
			count:     1,
			firstSeen: now,
			lastSeen:  now,
		}
		return true, lrl.maxAttempts - 1, nil
	}

	// Check if locked out
	if attempts.lockedUntil != nil && now.Before(*attempts.lockedUntil) {
		return false, 0, attempts.lockedUntil
	}

	// Reset if window has passed
	if now.Sub(attempts.firstSeen) > lrl.window {
		attempts.count = 1
		attempts.firstSeen = now
		attempts.lockedUntil = nil
		attempts.lastSeen = now
		return true, lrl.maxAttempts - 1, nil
	}

	attempts.count++
	attempts.lastSeen = now

	// Lock account if max attempts reached
	if attempts.count >= lrl.maxAttempts {
		lockoutEnd := now.Add(lrl.lockoutDuration)
		attempts.lockedUntil = &lockoutEnd
		return false, 0, &lockoutEnd
	}

	return true, lrl.maxAttempts - attempts.count, nil
}

// Reset resets the attempt counter for an IP (called on successful login)
func (lrl *memoryLoginRateLimiter) Reset(ip string) {
	lrl.mu.Lock()
	defer lrl.mu.Unlock()
	delete(lrl.attempts, ip)
}

// cleanup removes stale entries
func (lrl *memoryLoginRateLimiter) cleanup() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("login rate limiter cleanup panicked", "component", "api", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-lrl.stopCh:
			return
		case <-ticker.C:
		}
		lrl.mu.Lock()
		now := time.Now()
		for ip, attempts := range lrl.attempts {
			if now.Sub(attempts.lastSeen) > lrl.window+lrl.lockoutDuration {
				delete(lrl.attempts, ip)
			}
		}
		lrl.mu.Unlock()
	}
}
