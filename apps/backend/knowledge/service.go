// Package knowledge aggregates cross-investigation knowledge (past episodes,
// runbook/KB notes, concurrent investigations, peer-ask hints) and renders a
// single deterministic prompt block that is injected into the outbound agent
// prompt by all prompt builders.
//
// The package is agent-agnostic: Hermes, OpenClaw, or any future agent_type
// receives the same shared-knowledge block without any client-side changes.
package knowledge

import (
	"context"
	"time"

	"alga/store"
)

// Service aggregates shared knowledge for an investigation prompt.
type Service interface {
	BuildContext(ctx context.Context, inv *store.AlertInvestigationRecord) *Context
}

// EpisodicEntry is a summary of a past investigation that is similar to the
// current one.
type EpisodicEntry struct {
	InvestigationID string
	AgentType       string
	Severity        string
	Status          string
	AlertName       string
	Namespace       string
	RootCause       string
	Resolution      string
	Summary         string
	CompletedAt     time.Time
}

// NoteEntry represents an applicable knowledge-base note (runbook,
// known_issue, service_owner, fact). Populated by Phase 2.
type NoteEntry struct {
	ID    string
	Kind  string
	Title string
	Body  string
	Tags  []string
}

// ConcurrentEntry describes another currently in-flight investigation that
// overlaps on at least one discriminating label. Populated by Phase 3.
type ConcurrentEntry struct {
	InvestigationID string
	AgentType       string
	AlertName       string
	Namespace       string
	Severity        string
	StartedAgo      time.Duration
}

// Context holds the aggregated shared knowledge for one investigation.
// It is always safe to render via PromptBlock, even when empty.
type Context struct {
	Past       []EpisodicEntry
	Notes      []NoteEntry
	Concurrent []ConcurrentEntry
	Memories   []MemoryEntry

	PeerAskAvailable bool
}

// IsEmpty reports whether the context has no injectable content.
func (c *Context) IsEmpty() bool {
	if c == nil {
		return true
	}
	return len(c.Past) == 0 && len(c.Notes) == 0 && len(c.Concurrent) == 0 && len(c.Memories) == 0
}
