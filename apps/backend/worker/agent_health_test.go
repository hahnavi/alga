package worker

import (
	"testing"
)

func TestAgentHealthTrackerRecord(t *testing.T) {
	t.Parallel()
	tr := NewAgentHealthTracker(50)
	tr.RecordSuccess("agent-1")
	tr.RecordSuccess("agent-1")
	tr.RecordFailure("agent-1")

	score := tr.Health("agent-1")
	if score < 0.6 || score > 0.7 {
		t.Fatalf("expected ~0.67, got %.2f", score)
	}
}

func TestAgentHealthTrackerNoData(t *testing.T) {
	t.Parallel()
	tr := NewAgentHealthTracker(50)
	score := tr.Health("agent-unknown")
	if score != 1.0 {
		t.Fatalf("unknown agent should default to 1.0, got %.2f", score)
	}
}

func TestAgentHealthTrackerCircuitBreaker(t *testing.T) {
	t.Parallel()
	tr := NewAgentHealthTracker(50)
	tr.RecordFailure("agent-1")
	tr.RecordFailure("agent-1")
	tr.RecordFailure("agent-1")

	if !tr.IsCircuitBroken("agent-1") {
		t.Fatal("expected circuit breaker to be tripped after 3 failures")
	}
	if tr.IsCircuitBroken("agent-2") {
		t.Fatal("agent-2 should not be broken")
	}
}

func TestAgentHealthTrackerRecovery(t *testing.T) {
	t.Parallel()
	tr := NewAgentHealthTracker(50)
	tr.RecordFailure("agent-1")
	tr.RecordFailure("agent-1")
	tr.RecordFailure("agent-1")
	tr.RecordSuccess("agent-1")

	if tr.IsCircuitBroken("agent-1") {
		t.Fatal("success should break the circuit")
	}
}
