package alga

import (
	"log/slog"
	"math/rand/v2"
)

// randInt64N returns a non-negative pseudo-random int64 in [0, n). Wraps
// math/rand/v2 (Go 1.22+, auto-seeded, goroutine-safe) so callers don't need
// to import it directly.
func randInt64N(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int64N(n)
}

// slogDefault returns the SDK's Logger backed by slog.Default(). Kept as a
// function (not a var) so test setups that swap slog.Default pick up the
// change at call time.
func slogDefault() Logger { return AsLogger(slog.Default()) }
