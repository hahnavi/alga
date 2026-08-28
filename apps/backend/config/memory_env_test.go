package config

import (
	"testing"
	"time"
)

// TestMemoryEnvPlumbing pins the contract: the memory knobs are parsed
// from env into the config fields the memory service consumes.
func TestMemoryEnvPlumbing(t *testing.T) {
	t.Setenv("MEMORY_MAX_PER_INVESTIGATION", "25")
	t.Setenv("MEMORY_SIMILARITY_THRESHOLD", "0.8")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.MemoryMaxPerInvestigation != 25 {
		t.Fatalf("MemoryMaxPerInvestigation = %d, want 25", cfg.MemoryMaxPerInvestigation)
	}
	if cfg.MemorySimilarityThreshold != 0.8 {
		t.Fatalf("MemorySimilarityThreshold = %v, want 0.8", cfg.MemorySimilarityThreshold)
	}
}

// TestCorrelationWindowDefaultAndOverride pins the semantics: the
// built-in default buffers bursts for 15s, an explicit value overrides it, and
// CORRELATION_WINDOW=0 restores immediate flush (legacy escape hatch).
func TestCorrelationWindowDefaultAndOverride(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CorrelationWindow != 15*time.Second {
		t.Fatalf("default CorrelationWindow = %v, want 15s", cfg.CorrelationWindow)
	}

	t.Setenv("CORRELATION_WINDOW", "0")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load with override: %v", err)
	}
	if cfg.CorrelationWindow != 0 {
		t.Fatalf("explicit zero CorrelationWindow = %v, want 0 (immediate flush)", cfg.CorrelationWindow)
	}
}
