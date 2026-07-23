package worker

import (
	"sync"
)

type healthEntry struct {
	successes int
	failures  int
}

type AgentHealthTracker struct {
	mu      sync.Mutex
	window  int
	entries map[string]*healthEntry
	circuit map[string]bool
}

func NewAgentHealthTracker(window int) *AgentHealthTracker {
	if window <= 0 {
		window = 50
	}
	return &AgentHealthTracker{
		window:  window,
		entries: make(map[string]*healthEntry),
		circuit: make(map[string]bool),
	}
}

func (t *AgentHealthTracker) RecordSuccess(agentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.getOrCreate(agentID)
	e.successes++
	delete(t.circuit, agentID)
}

func (t *AgentHealthTracker) RecordFailure(agentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.getOrCreate(agentID)
	e.failures++
	total := e.successes + e.failures
	if total >= 3 && float64(e.successes)/float64(total) < 0.3 {
		t.circuit[agentID] = true
	}
}

func (t *AgentHealthTracker) Health(agentID string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[agentID]
	if !ok || (e.successes+e.failures) == 0 {
		return 1.0
	}
	return float64(e.successes) / float64(e.successes+e.failures)
}

func (t *AgentHealthTracker) IsCircuitBroken(agentID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.circuit[agentID]
}

func (t *AgentHealthTracker) getOrCreate(agentID string) *healthEntry {
	e, ok := t.entries[agentID]
	if !ok {
		e = &healthEntry{}
		t.entries[agentID] = e
	}
	total := e.successes + e.failures
	if total > t.window {
		ratio := float64(e.successes) / float64(total)
		keep := t.window / 2
		e.successes = int(float64(keep) * ratio)
		e.failures = keep - e.successes
	}
	return e
}
