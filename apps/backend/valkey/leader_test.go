package valkey

import (
	"context"
	"testing"
	"time"
)

// TestLeaderLeaseNilClientFallback exercises the single-replica fallback:
// with a nil client, every Acquire/Renew must succeed and IsLeader must
// reflect the held state without contacting Valkey.
func TestLeaderLeaseNilClientFallback(t *testing.T) {
	t.Parallel()
	l := NewLeaderLease(nil, "alga:test:leader", 5*time.Second)
	if !l.IsLeader() {
		t.Fatalf("nil-client leader should report IsLeader=true")
	}
	ok, err := l.Acquire(context.Background())
	if err != nil || !ok {
		t.Fatalf("Acquire(nil-client)=%v,%v want true,nil", ok, err)
	}
	ok, err = l.Renew(context.Background())
	if err != nil || !ok {
		t.Fatalf("Renew(nil-client)=%v,%v want true,nil", ok, err)
	}
	l.Release(context.Background())
	// After release on nil-client, IsLeader continues to return true (we
	// special-case nil clients as "always leader" since there's no one to
	// argue with).
	if !l.IsLeader() {
		t.Fatalf("nil-client leader should still report leader after release")
	}
	if l.Identity() == "" {
		t.Fatalf("identity should be non-empty")
	}
}

// TestLeaderLeaseDefaults confirms the constructor's defaulting behavior.
func TestLeaderLeaseDefaults(t *testing.T) {
	t.Parallel()
	l := NewLeaderLease(nil, "k", 0)
	if l.ttl != 15*time.Second {
		t.Fatalf("default ttl = %v want 15s", l.ttl)
	}
	l = NewLeaderLease(nil, "k", -time.Second)
	if l.ttl != 15*time.Second {
		t.Fatalf("negative ttl should default to 15s, got %v", l.ttl)
	}
}

// TestLeaderLeaseNilReceiver guards against panics when callers pass a
// nil *LeaderLease (e.g. when leader election was never wired up).
func TestLeaderLeaseNilReceiver(t *testing.T) {
	t.Parallel()
	var l *LeaderLease
	if l.IsLeader() {
		t.Fatalf("nil receiver IsLeader should be false")
	}
	ok, err := l.Acquire(context.Background())
	if ok || err != nil {
		t.Fatalf("nil receiver Acquire=%v,%v", ok, err)
	}
	ok, err = l.Renew(context.Background())
	if ok || err != nil {
		t.Fatalf("nil receiver Renew=%v,%v", ok, err)
	}
	l.Release(context.Background())
}
