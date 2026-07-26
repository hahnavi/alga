// Package agent implements the conversation loop, session memory, and system
// prompt assembly. It is channel-agnostic: channels deliver messages and
// receive responses through the AgentCore.
package agent

import (
	"sync"
	"time"

	"alga-agent/internal/llm"
)

// AlgaContext carries the current investigation/incident context into the agent
// so it can resolve IDs and inject context into the system prompt. Populated by
// the Alga channel; empty for Telegram-initiated requests.
type AlgaContext struct {
	InvestigationID     string   `json:"investigation_id,omitempty"`
	IncidentID          string   `json:"incident_id,omitempty"`
	AlertFingerprints   []string `json:"alert_fingerprints,omitempty"`
	InvestigationStatus string   `json:"investigation_status,omitempty"`
	Severity            string   `json:"severity,omitempty"`
}

// Session is a single conversation. Each session has its own message history
// (ring buffer) and metadata. Two mutexes guard access:
//   - dispatchMu serializes message processing for a single chat (held by the
//     router for the duration of Process, preventing interleaved tool calls).
//   - mu guards the session fields (messages, AlgaCtx, lastActive).
//
// They are separate so that Process can hold dispatchMu while still acquiring
// mu for individual field reads/writes without self-deadlock.
type Session struct {
	dispatchMu sync.Mutex
	mu         sync.Mutex
	messages   []llm.Message
	maxTurns   int
	// AlgaCtx is the most recent Alga context for this session (investigation
	// threads). Refreshed on each inbound Alga message.
	AlgaCtx AlgaContext
	// created tracks when the session was first seen, for idle-eviction.
	created time.Time
	// lastActive is updated on each inbound message.
	lastActive time.Time
}

// Lock/Unlock expose the dispatch mutex so the router can serialize message
// processing for a single chat (preventing interleaved tool executions).
func (s *Session) Lock()   { s.dispatchMu.Lock() }
func (s *Session) Unlock() { s.dispatchMu.Unlock() }

// Messages returns a copy of the current message history.
func (s *Session) Messages() []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]llm.Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// AppendMessage appends a message to the session and trims the ring buffer.
func (s *Session) AppendMessage(m llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
	s.lastActive = time.Now()
	s.trim()
}

// ReplaceMessages replaces the entire message history (used after summarization).
func (s *Session) ReplaceMessages(msgs []llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = msgs
	s.lastActive = time.Now()
	s.trim()
}

// SetAlgaCtx updates the Alga context for this session.
func (s *Session) SetAlgaCtx(ctx AlgaContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AlgaCtx = ctx
}

// AlgaContext returns the current Alga context.
func (s *Session) AlgaContext() AlgaContext {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.AlgaCtx
}

// trim enforces the ring buffer cap. Leading system messages are always
// preserved. Non-system messages are dropped from the oldest end in complete
// conversational units so that assistant+tool-call groups are never split
// (splitting them orphans tool result messages and corrupts the history sent
// to the LLM, which the OpenAI API rejects).
//
// A "unit" is one of:
//   - a single user message
//   - an assistant message with no tool_calls
//   - an assistant message with tool_calls, plus every trailing tool-role
//     message until the next non-tool message
//
// Caller must hold s.mu.
func (s *Session) trim() {
	if len(s.messages) <= s.maxTurns {
		return
	}
	// Preserve leading system messages.
	start := 0
	for start < len(s.messages) && s.messages[start].Role == "system" {
		start++
	}
	// Drop whole units from the front until under cap. We must not leave a
	// tool-role message without its preceding assistant tool_call.
	for len(s.messages)-start > s.maxTurns && start < len(s.messages) {
		drop := 1
		// If this is an assistant with tool_calls, also drop its trailing
		// tool result messages atomically.
		if s.messages[start].Role == "assistant" && len(s.messages[start].ToolCalls) > 0 {
			for start+drop < len(s.messages) && s.messages[start+drop].Role == "tool" {
				drop++
			}
		}
		s.messages = append(s.messages[:start], s.messages[start+drop:]...)
	}
}

// SessionStore is a thread-safe map of sessions keyed by session id.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
	maxTurns int
	// persistDir, when non-empty, enables JSON-file persistence (persist.go).
	persistDir string
}

// NewSessionStore returns a session store that retains up to maxTurns messages
// per session.
func NewSessionStore(maxTurns int) *SessionStore {
	if maxTurns < 4 {
		maxTurns = 4
	}
	return &SessionStore{
		sessions: make(map[string]*Session),
		maxTurns: maxTurns,
	}
}

// Get returns the session for id, creating it if missing. With persistence
// enabled, a missing session is first reloaded from disk (resume after
// restart or idle eviction) before falling back to a fresh one.
func (ss *SessionStore) Get(id string) *Session {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s, ok := ss.sessions[id]
	if !ok {
		if s = ss.loadSession(id); s == nil {
			s = &Session{
				maxTurns:   ss.maxTurns,
				created:    time.Now(),
				lastActive: time.Now(),
			}
		}
		ss.sessions[id] = s
	}
	return s
}

// Has reports whether a session exists for id.
func (ss *SessionStore) Has(id string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	_, ok := ss.sessions[id]
	return ok
}

// Clear removes the session for id (e.g. on /clear command), including its
// persisted file — clearing means the conversation is forgotten.
func (ss *SessionStore) Clear(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.sessions, id)
	ss.removeSessionFile(id)
}

// Size returns the number of active sessions.
func (ss *SessionStore) Size() int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return len(ss.sessions)
}

// EvictIdle removes sessions that have been inactive for longer than maxIdle.
// Returns the number of sessions evicted.
func (ss *SessionStore) EvictIdle(maxIdle time.Duration) int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	cutoff := time.Now().Add(-maxIdle)
	var n int
	for id, s := range ss.sessions {
		s.mu.Lock()
		idle := s.lastActive.Before(cutoff)
		s.mu.Unlock()
		if idle {
			delete(ss.sessions, id)
			n++
		}
	}
	return n
}
