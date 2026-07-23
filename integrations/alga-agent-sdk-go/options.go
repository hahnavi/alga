package alga

import (
	"log/slog"
	"net/http"
	"time"
)

// Logger is the structured logger the SDK calls through. It defaults to
// slog.Default(); callers should inject a sub-logger via WithLogger so SDK
// internal messages carry the right component tag and redaction policy. The
// SDK never logs the bearer token, the agent secret, or request bodies.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Options configure an AlgaClient at construction time. Use the With* helpers
// rather than constructing the struct directly.
type Options struct {
	HTTPClient *http.Client
	Logger     Logger
	Dedup      *MessageDedup
	// UserAgent overrides the default "alga-agent-sdk-go/<go-version>" header.
	UserAgent string
	// MaxRESTRetries is the maximum number of retry attempts for transient
	// REST failures (429, 500, 502, 503, 504, network). 0 disables retries.
	// Negative is invalid and treated as 0. Default 2.
	MaxRESTRetries int
	// HeartbeatInterval overrides the default 30s heartbeat cadence.
	HeartbeatInterval time.Duration
}

// Option mutates Options. Functional-option style keeps NewAlgaClient
// signature stable as we add knobs.
type Option func(*Options)

// WithHTTPClient injects a custom *http.Client (e.g. for test transports or
// custom TLS). Defaults to a client with a 30s timeout.
func WithHTTPClient(h *http.Client) Option {
	return func(o *Options) { o.HTTPClient = h }
}

// WithLogger injects a structured logger. The SDK calls Debug for normal
// lifecycle events, Warn for retries, Error for terminal failures.
func WithLogger(l Logger) Option {
	return func(o *Options) { o.Logger = l }
}

// WithDedup injects a custom dedup cache (e.g. with a larger TTL in
// long-running agents). Defaults to 1000 entries / 5 minutes.
func WithDedup(d *MessageDedup) Option {
	return func(o *Options) { o.Dedup = d }
}

// WithUserAgent sets the User-Agent header on every outbound request.
func WithUserAgent(ua string) Option {
	return func(o *Options) { o.UserAgent = ua }
}

// WithMaxRESTRetries configures per-request retry attempts for transient REST
// failures. Pass 0 to disable retries (fail fast).
func WithMaxRESTRetries(n int) Option {
	return func(o *Options) { o.MaxRESTRetries = n }
}

// WithHeartbeatInterval overrides the default 30s heartbeat cadence. Values
// below 1s are clamped to 1s to protect the backend from accidental tight
// loops.
func WithHeartbeatInterval(d time.Duration) Option {
	return func(o *Options) { o.HeartbeatInterval = d }
}

func defaults(o *Options) {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Dedup == nil {
		o.Dedup = NewMessageDedup(1000, 5*time.Minute)
	}
	if o.UserAgent == "" {
		o.UserAgent = defaultUserAgent
	}
	if o.MaxRESTRetries == 0 {
		o.MaxRESTRetries = 2
	}
	if o.HeartbeatInterval == 0 {
		o.HeartbeatInterval = 30 * time.Second
	} else if o.HeartbeatInterval < time.Second {
		o.HeartbeatInterval = time.Second
	}
}
