package config

import (
	"testing"
)

// TestMemoryEnvPlumbing pins the WP-A11 contract: the memory knobs are parsed
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
