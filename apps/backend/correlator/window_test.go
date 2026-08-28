package correlator

import (
	"testing"
	"time"
)

// TestEffectiveWindow pins the window-resolution contract: a matched
// rule with Window > 0 overrides the global window (seconds); the global
// window is the fallback, including 0 = immediate flush.
func TestEffectiveWindow(t *testing.T) {
	t.Parallel()

	global := 15 * time.Second
	c := &Correlator{cfg: Config{Window: global}}

	if got := c.effectiveWindow(nil); got != global {
		t.Fatalf("no rule: got %v, want global %v", got, global)
	}
	if got := c.effectiveWindow(&CorrelationRule{Window: 0}); got != global {
		t.Fatalf("rule with zero window: got %v, want global %v", got, global)
	}
	if got := c.effectiveWindow(&CorrelationRule{Window: -5}); got != global {
		t.Fatalf("rule with negative window: got %v, want global %v", got, global)
	}
	if got := c.effectiveWindow(&CorrelationRule{Window: 45}); got != 45*time.Second {
		t.Fatalf("rule override: got %v, want 45s", got)
	}

	// Explicit CORRELATION_WINDOW=0 (legacy escape hatch): no rule override
	// means immediate flush.
	zero := &Correlator{cfg: Config{Window: 0}}
	if got := zero.effectiveWindow(nil); got != 0 {
		t.Fatalf("zero global window: got %v, want 0 (immediate flush)", got)
	}
	if got := zero.effectiveWindow(&CorrelationRule{Window: 30}); got != 30*time.Second {
		t.Fatalf("zero global window with override: got %v, want 30s", got)
	}
}
