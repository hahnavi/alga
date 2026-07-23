package valkey

import (
	"context"
	"testing"
	"time"
)

// TestPresenceNilClientFallback confirms the Available() guard so callers
// can treat the registry as a no-op when Valkey isn't configured.
func TestPresenceNilClientFallback(t *testing.T) {
	t.Parallel()
	p := NewPresence(nil, 0, "replica-a")
	if p.Available() {
		t.Fatalf("Available() must be false for nil client")
	}
	if p.TTL() != 90*time.Second {
		t.Fatalf("default TTL = %v want 90s", p.TTL())
	}

	ctx := context.Background()
	if err := p.Register(ctx, "agent-1", "session-1", "hermes"); err != nil {
		t.Fatalf("Register(nil)=%v want nil", err)
	}
	if err := p.Renew(ctx, "agent-1", "session-1"); err != nil {
		t.Fatalf("Renew(nil)=%v want nil", err)
	}
	empty, err := p.Unregister(ctx, "agent-1", "session-1")
	if err != nil || empty {
		t.Fatalf("Unregister(nil)=%v,%v want false,nil", empty, err)
	}
	if p.IsAgentOnline(ctx, "agent-1") {
		t.Fatalf("IsAgentOnline(nil) should be false")
	}
	got, err := p.ListOnlineAgents(ctx)
	if err != nil || got != nil {
		t.Fatalf("ListOnlineAgents(nil)=%v,%v want nil,nil", got, err)
	}
	if err := p.PublishEvent(ctx, AgentEvent{Type: AgentEventOnline, AgentID: "agent-1"}); err != nil {
		t.Fatalf("PublishEvent(nil)=%v want nil", err)
	}
}

// TestPresenceNilReceiver protects callers that may pass a nil *Presence
// (e.g. during partial wiring in tests).
func TestPresenceNilReceiver(t *testing.T) {
	t.Parallel()
	var p *Presence
	if p.Available() {
		t.Fatalf("nil receiver Available() must be false")
	}
	if p.TTL() != 0 {
		t.Fatalf("nil receiver TTL must be 0")
	}
}
