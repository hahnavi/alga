package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"

	"alga/logger"
)

// presenceKeyPrefix is the Valkey hash prefix tracking active agent
// SSE sessions. Each agent token id has one hash; each connected
// session (potentially across replicas) is a hash field.
const presenceKeyPrefix = "alga:agent-presence:"

// AgentEventsChannel is the pub/sub channel used to notify all replicas about
// agent online/offline transitions. Subscribers (the scheduler) use the
// stream to wake up scheduling and apply disconnect grace.
const AgentEventsChannel = "alga:agent-events"

// AgentEventType enumerates AgentEvent.Type values.
const (
	AgentEventOnline       = "online"
	AgentEventOffline      = "offline"
	AgentEventSessionEnded = "session_ended"
)

// AgentEvent is the JSON payload broadcast on AgentEventsChannel.
type AgentEvent struct {
	Type      string    `json:"type"`
	AgentID   string    `json:"agent_id"`
	SessionID string    `json:"session_id,omitempty"`
	Replica   string    `json:"replica,omitempty"`
	At        time.Time `json:"at"`
}

// AgentSessionMeta is the JSON value stored per session field in the agent's
// presence hash. It carries enough context for operators / debugging without
// being authoritative; the source of truth is always the SSE connection itself.
type AgentSessionMeta struct {
	SessionID string    `json:"session_id"`
	Replica   string    `json:"replica,omitempty"`
	AgentType string    `json:"agent_type,omitempty"`
	StartedAt time.Time `json:"started_at"`
	LastSeen  time.Time `json:"last_seen"`
}

func presenceKey(agentID string) string { return presenceKeyPrefix + agentID }

// Presence is a Valkey-backed presence registry.
//
// Single-replica deployments may run with client == nil; in that case
// IsAgentOnline always returns false (callers must fall back to the local
// connection map in the SSE handler — this is what AgentOnline does in
// investigation_forwarder.go).
type Presence struct {
	client  *Client
	ttl     time.Duration
	replica string
}

// NewPresence builds a presence helper. ttl controls how long a session
// registration survives without a renewal; the SSE handler renews it via
// its 15s keepalive cadence, so a 90s TTL is comfortable.
// Replica is a free-form identifier (hostname/pod name) embedded in events.
func NewPresence(client *Client, ttl time.Duration, replica string) *Presence {
	if ttl <= 0 {
		ttl = 90 * time.Second
	}
	return &Presence{client: client, ttl: ttl, replica: replica}
}

// Available reports whether a Valkey client is configured. Callers use this
// to decide whether to fall back to in-process presence.
func (p *Presence) Available() bool { return p != nil && p.client != nil }

// TTL returns the session TTL.
func (p *Presence) TTL() time.Duration {
	if p == nil {
		return 0
	}
	return p.ttl
}

// Register marks an agent session as online. Idempotent: re-calling for the
// same (agent, session) refreshes the TTL and updates LastSeen.
func (p *Presence) Register(ctx context.Context, agentID, sessionID, agentType string) error {
	if !p.Available() || agentID == "" || sessionID == "" {
		return nil
	}
	now := time.Now().UTC()
	meta := AgentSessionMeta{
		SessionID: sessionID,
		Replica:   p.replica,
		AgentType: agentType,
		StartedAt: now,
		LastSeen:  now,
	}
	val, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal session meta: %w", err)
	}
	key := presenceKey(agentID)
	if err := p.client.Do(ctx, p.client.Builder().Hset().Key(key).
		FieldValue().FieldValue(sessionID, string(val)).Build()).Error(); err != nil {
		logger.Error("failed to register agent presence", "component", "valkey", "agent_id", agentID, "error", err)
		return fmt.Errorf("hset presence: %w", err)
	}
	if err := p.client.Do(ctx, p.client.Builder().Expire().Key(key).
		Seconds(int64(p.ttl.Seconds())).Build()).Error(); err != nil {
		logger.Error("failed to set presence TTL", "component", "valkey", "agent_id", agentID, "error", err)
		return fmt.Errorf("expire presence: %w", err)
	}
	return nil
}

// Renew refreshes the LastSeen timestamp + TTL for an existing session.
// Cheaper than Register because it does not re-encode the full meta on every
// pong; we just bump the hash TTL and update LastSeen via HSET.
func (p *Presence) Renew(ctx context.Context, agentID, sessionID string) error {
	if !p.Available() || agentID == "" || sessionID == "" {
		return nil
	}
	key := presenceKey(agentID)
	// Read existing meta so we preserve StartedAt/AgentType.
	prev, err := p.client.Do(ctx, p.client.Builder().Hget().Key(key).Field(sessionID).Build()).ToString()
	var meta AgentSessionMeta
	if err == nil && prev != "" {
		_ = json.Unmarshal([]byte(prev), &meta)
	} else if err != nil && !errors.Is(err, valkey.Nil) {
		logger.Error("failed to read presence for renewal", "component", "valkey", "agent_id", agentID, "error", err)
		return fmt.Errorf("hget presence: %w", err)
	}
	meta.SessionID = sessionID
	if meta.Replica == "" {
		meta.Replica = p.replica
	}
	if meta.StartedAt.IsZero() {
		meta.StartedAt = time.Now().UTC()
	}
	meta.LastSeen = time.Now().UTC()
	val, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal session meta: %w", err)
	}
	if err := p.client.Do(ctx, p.client.Builder().Hset().Key(key).
		FieldValue().FieldValue(sessionID, string(val)).Build()).Error(); err != nil {
		logger.Error("failed to renew agent presence", "component", "valkey", "agent_id", agentID, "error", err)
		return fmt.Errorf("hset presence: %w", err)
	}
	if err := p.client.Do(ctx, p.client.Builder().Expire().Key(key).
		Seconds(int64(p.ttl.Seconds())).Build()).Error(); err != nil {
		logger.Error("failed to renew presence TTL", "component", "valkey", "agent_id", agentID, "error", err)
		return fmt.Errorf("expire presence: %w", err)
	}
	return nil
}

// Unregister removes a session. Returns true when the agent has no remaining
// sessions (caller should then publish an Offline AgentEvent).
func (p *Presence) Unregister(ctx context.Context, agentID, sessionID string) (becameEmpty bool, err error) {
	if !p.Available() || agentID == "" || sessionID == "" {
		return false, nil
	}
	key := presenceKey(agentID)
	if err := p.client.Do(ctx, p.client.Builder().Hdel().Key(key).Field(sessionID).Build()).Error(); err != nil {
		return false, fmt.Errorf("hdel presence: %w", err)
	}
	n, err := p.client.Do(ctx, p.client.Builder().Hlen().Key(key).Build()).AsInt64()
	if err != nil {
		return false, fmt.Errorf("hlen presence: %w", err)
	}
	if n == 0 {
		_ = p.client.Do(ctx, p.client.Builder().Del().Key(key).Build()).Error()
		return true, nil
	}
	return false, nil
}

// IsAgentOnline reports whether at least one live session exists for the
// agent. Returns false on error / when client is nil.
func (p *Presence) IsAgentOnline(ctx context.Context, agentID string) bool {
	if !p.Available() || agentID == "" {
		return false
	}
	n, err := p.client.Do(ctx, p.client.Builder().Hlen().Key(presenceKey(agentID)).Build()).AsInt64()
	if err != nil {
		return false
	}
	return n > 0
}

// ListOnlineAgents returns the agentIDs that currently have at least one
// live session, scanning Valkey with SCAN (never KEYS).
func (p *Presence) ListOnlineAgents(ctx context.Context) ([]string, error) {
	if !p.Available() {
		return nil, nil
	}
	out := make([]string, 0, 32)
	var cursor uint64
	for {
		entry, err := p.client.Do(ctx, p.client.Builder().Scan().Cursor(cursor).
			Match(presenceKeyPrefix+"*").Count(200).Build()).AsScanEntry()
		if err != nil {
			return nil, fmt.Errorf("scan presence: %w", err)
		}
		for _, k := range entry.Elements {
			if !strings.HasPrefix(k, presenceKeyPrefix) {
				continue
			}
			out = append(out, strings.TrimPrefix(k, presenceKeyPrefix))
		}
		cursor = entry.Cursor
		if cursor == 0 {
			break
		}
	}
	return out, nil
}

// PublishEvent announces a presence transition on AgentEventsChannel.
func (p *Presence) PublishEvent(ctx context.Context, ev AgentEvent) error {
	if !p.Available() {
		return nil
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	if ev.Replica == "" {
		ev.Replica = p.replica
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal agent event: %w", err)
	}
	return p.client.Do(ctx, p.client.Builder().Publish().
		Channel(AgentEventsChannel).Message(string(data)).Build()).Error()
}

// SubscribeEvents blocks until ctx is cancelled, invoking handler for each
// received AgentEvent.
func (p *Presence) SubscribeEvents(ctx context.Context, handler func(AgentEvent)) error {
	if !p.Available() {
		<-ctx.Done()
		return ctx.Err()
	}
	return p.client.Receive(ctx,
		p.client.Builder().Subscribe().Channel(AgentEventsChannel).Build(),
		func(msg valkey.PubSubMessage) {
			var ev AgentEvent
			if err := json.Unmarshal([]byte(msg.Message), &ev); err != nil {
				logger.Warn("failed to unmarshal agent event", "component", "valkey", "error", err)
				return
			}
			handler(ev)
		})
}
