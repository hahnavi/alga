package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHeartbeatCreateReturnsTokenAndDeadline(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGHeartbeatStore(client)

	out, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "api-health",
		IntervalSeconds: 60,
		GraceSeconds:    10,
		Severity:        "critical",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.ID == uuid.Nil {
		t.Fatal("expected non-nil ID")
	}
	if out.PingToken == "" {
		t.Fatal("expected ping token to be returned once")
	}
	if out.Status != HeartbeatStatusHealthy {
		t.Fatalf("status = %q, want healthy", out.Status)
	}
	if out.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set")
	}
	// Deadline is now + interval + grace (within a few seconds of tolerance).
	want := time.Now().Add(70 * time.Second)
	if out.ExpiresAt.Sub(want).Abs() > 5*time.Second {
		t.Fatalf("expires_at = %v, want ~%v", out.ExpiresAt, want)
	}
}

func TestHeartbeatGetByPingToken(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGHeartbeatStore(client)

	out, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "cron-job",
		IntervalSeconds: 30,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetByPingToken(context.Background(), out.PingToken)
	if err != nil {
		t.Fatalf("GetByPingToken: %v", err)
	}
	if got == nil || got.ID != out.ID {
		t.Fatalf("expected lookup to return created heartbeat")
	}

	if got2, _ := s.GetByPingToken(context.Background(), "alga_hb_bogus"); got2 != nil {
		t.Fatal("expected nil for unknown token")
	}
}

func TestHeartbeatRecordPingReArmsDeadline(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGHeartbeatStore(client)

	out, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "worker",
		IntervalSeconds: 60,
		GraceSeconds:    0,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pingTime := time.Now().Add(2 * time.Minute).UTC()
	updated, err := s.RecordPing(context.Background(), out.ID, pingTime)
	if err != nil {
		t.Fatalf("RecordPing: %v", err)
	}
	if updated.LastPingAt == nil || updated.LastPingAt.Sub(pingTime).Abs() > time.Second {
		t.Fatalf("last_ping_at = %v, want ~%v", updated.LastPingAt, pingTime)
	}
	want := pingTime.Add(60 * time.Second)
	if updated.ExpiresAt == nil || updated.ExpiresAt.Sub(want).Abs() > time.Second {
		t.Fatalf("expires_at = %v, want ~%v", updated.ExpiresAt, want)
	}
}

func TestHeartbeatListExpiredAndMarkExpired(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGHeartbeatStore(client)
	now := time.Now().UTC()

	// Healthy heartbeat whose deadline is forced into the past via a back-dated ping.
	hb, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "stale",
		IntervalSeconds: 60,
		GraceSeconds:    0,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.RecordPing(context.Background(), hb.ID, now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("RecordPing: %v", err)
	}

	// A fresh heartbeat that is still healthy.
	fresh, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "fresh",
		IntervalSeconds: 3600,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("Create fresh: %v", err)
	}

	expired, err := s.ListExpired(context.Background(), now)
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != hb.ID {
		t.Fatalf("expected only stale heartbeat expired, got %+v", expired)
	}

	marked, err := s.MarkExpired(context.Background(), hb.ID, now)
	if err != nil {
		t.Fatalf("MarkExpired: %v", err)
	}
	if marked.Status != HeartbeatStatusExpired {
		t.Fatalf("status = %q, want expired", marked.Status)
	}

	// Once marked expired it must not be returned again (idempotent sweep).
	expired, _ = s.ListExpired(context.Background(), now)
	if len(expired) != 0 {
		t.Fatalf("expected no expired heartbeats after marking, got %d", len(expired))
	}

	// Disabled heartbeats are excluded even when overdue.
	if _, err := s.RecordPing(context.Background(), fresh.ID, now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("RecordPing fresh: %v", err)
	}
	if _, err := s.Update(context.Background(), fresh.ID, &HeartbeatRecord{Enabled: false, EnabledSet: true}); err != nil {
		t.Fatalf("Update disable: %v", err)
	}
	expired, _ = s.ListExpired(context.Background(), now)
	if len(expired) != 0 {
		t.Fatalf("expected disabled heartbeat excluded from sweep, got %d", len(expired))
	}
}

func TestHeartbeatRegenerateTokenInvalidatesOldToken(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGHeartbeatStore(client)

	out, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "rotated",
		IntervalSeconds: 30,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldToken := out.PingToken

	regen, err := s.RegenerateToken(context.Background(), out.ID)
	if err != nil {
		t.Fatalf("RegenerateToken: %v", err)
	}
	if regen.PingToken == "" || regen.PingToken == oldToken {
		t.Fatal("expected a new distinct token")
	}

	if got, _ := s.GetByPingToken(context.Background(), oldToken); got != nil {
		t.Fatal("old token must no longer resolve after regeneration")
	}
	if got, _ := s.GetByPingToken(context.Background(), regen.PingToken); got == nil {
		t.Fatal("new token must resolve")
	}
}

func TestHeartbeatUpdateChangesIntervalAndReArms(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGHeartbeatStore(client)

	out, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "tunable",
		IntervalSeconds: 60,
		GraceSeconds:    0,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newInterval := 600
	updated, err := s.Update(context.Background(), out.ID, &HeartbeatRecord{IntervalSeconds: newInterval})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.IntervalSeconds != newInterval {
		t.Fatalf("interval = %d, want %d", updated.IntervalSeconds, newInterval)
	}
	if updated.Status != HeartbeatStatusHealthy {
		t.Fatalf("status = %q, want healthy after re-arm", updated.Status)
	}
}

func TestHeartbeatDelete(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGHeartbeatStore(client)

	out, err := s.Create(context.Background(), &HeartbeatRecord{
		Name:            "gone",
		IntervalSeconds: 30,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Delete(context.Background(), out.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(context.Background(), out.ID); err == nil {
		t.Fatal("expected error deleting missing heartbeat")
	}
}
