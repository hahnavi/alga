package store

import (
	"context"
	"testing"
	"time"

	"alga/ent/session"
)

// TestSessionStoreAbsoluteLifetimeRejected verifies M1: a session older than
// the configured absolute (max) lifetime is rejected on read even though its
// sliding idle expiry was refreshed recently (ASVS V3.2/V3.3).
func TestSessionStoreAbsoluteLifetimeRejected(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	const idleExpiry = 1 * time.Hour
	const maxLifetime = 30 * time.Minute
	s := newPGSessionStore(client, idleExpiry, maxLifetime)

	userID := mustCreateUser(t, client)
	sess, err := s.CreateSession(userID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Sanity: a fresh session is readable.
	got, err := s.GetSession(sess.ID)
	if err != nil || got == nil {
		t.Fatalf("fresh session should be readable: %v", err)
	}

	// Backdate created_at past the absolute max lifetime while keeping
	// expires_at in the future (simulating a session that has been refreshed
	// within its idle window for longer than the max lifetime).
	ctx := context.Background()
	_, err = client.Session.Update().
		Where(session.IDHash(sess.IDHash)).
		SetCreatedAt(time.Now().Add(-(maxLifetime + time.Minute))).
		SetExpiresAt(time.Now().Add(idleExpiry)).
		Save(ctx)
	if err != nil {
		t.Fatalf("backdate session: %v", err)
	}

	// GetSession must reject the over-age session (absolute cap), even though
	// expires_at is still in the future.
	if got, err := s.GetSession(sess.ID); err != nil {
		t.Fatalf("unexpected error on aged session: %v", err)
	} else if got != nil {
		t.Fatalf("session past absolute max lifetime must be rejected, got %+v", got)
	}

	// GetSessionByRefreshToken must also reject it.
	if got, err := s.GetSessionByRefreshToken(sess.RefreshToken); err != nil {
		t.Fatalf("unexpected error on aged session rt: %v", err)
	} else if got != nil {
		t.Fatalf("refresh-token lookup of aged session must be rejected")
	}
}

// TestSessionStoreRefreshRotatesSessionID verifies M1: RefreshSession rotates
// the session ID so the old cookie no longer authenticates (ASVS V3.3).
func TestSessionStoreRefreshRotatesSessionID(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGSessionStore(client, time.Hour, 0) // no absolute cap for this test

	userID := mustCreateUser(t, client)
	sess, err := s.CreateSession(userID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	oldID := sess.ID

	refreshed, err := s.RefreshSession(oldID, "127.0.0.1", "test")
	if err != nil || refreshed == nil {
		t.Fatalf("refresh session: %v", err)
	}

	// The session ID must have changed.
	if refreshed.ID == oldID {
		t.Fatal("RefreshSession must rotate the session ID; new ID equals old ID")
	}

	// The OLD cookie must no longer authenticate.
	if got, _ := s.GetSession(oldID); got != nil {
		t.Fatal("old session cookie must be invalid after refresh")
	}

	// The NEW cookie must authenticate and carry the rotated refresh token.
	got, err := s.GetSession(refreshed.ID)
	if err != nil || got == nil {
		t.Fatalf("new session cookie must be valid after refresh: %v", err)
	}

	// The old refresh token must no longer resolve (it moved to the prev list).
	if rtGot, _ := s.GetSessionByRefreshToken(sess.RefreshToken); rtGot != nil {
		t.Fatal("old refresh token must be invalidated after rotation")
	}
	// The new refresh token must resolve to the refreshed session.
	if rtGot, _ := s.GetSessionByRefreshToken(refreshed.RefreshToken); rtGot == nil {
		t.Fatal("new refresh token must resolve after refresh")
	}
}

// TestSessionStoreRefreshRespectsAbsoluteLifetime verifies that a refresh is
// refused (returns nil) once the session exceeds its absolute max lifetime,
// even if expires_at is still in the future.
func TestSessionStoreRefreshRespectsAbsoluteLifetime(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	const idleExpiry = 1 * time.Hour
	const maxLifetime = 30 * time.Minute
	s := newPGSessionStore(client, idleExpiry, maxLifetime)

	userID := mustCreateUser(t, client)
	sess, err := s.CreateSession(userID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	ctx := context.Background()
	_, err = client.Session.Update().
		Where(session.IDHash(sess.IDHash)).
		SetCreatedAt(time.Now().Add(-(maxLifetime + time.Minute))).
		SetExpiresAt(time.Now().Add(idleExpiry)).
		Save(ctx)
	if err != nil {
		t.Fatalf("backdate session: %v", err)
	}

	if refreshed, err := s.RefreshSession(sess.ID, "127.0.0.1", "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if refreshed != nil {
		t.Fatalf("refresh of session past absolute max lifetime must return nil, got %+v", refreshed)
	}
}

// TestSessionStoreDeleteExpiredReapsAgedAndIdle verifies the reaper deletes
// sessions past either their idle expiry or their absolute max lifetime.
func TestSessionStoreDeleteExpiredReapsAgedAndIdle(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	const idleExpiry = 1 * time.Hour
	const maxLifetime = 30 * time.Minute
	s := newPGSessionStore(client, idleExpiry, maxLifetime)
	ctx := context.Background()

	// Live session: should survive.
	live, err := s.CreateSession(mustCreateUser(t, client), "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create live: %v", err)
	}

	// Idle-expired session: expires_at in the past.
	idle, err := s.CreateSession(mustCreateUser(t, client), "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create idle: %v", err)
	}
	_, _ = client.Session.Update().Where(session.IDHash(idle.IDHash)).
		SetExpiresAt(time.Now().Add(-time.Minute)).Save(ctx)

	// Absolute-max-expired session: expires_at future, created_at very old.
	aged, err := s.CreateSession(mustCreateUser(t, client), "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create aged: %v", err)
	}
	_, _ = client.Session.Update().Where(session.IDHash(aged.IDHash)).
		SetCreatedAt(time.Now().Add(-(maxLifetime + time.Hour))).
		SetExpiresAt(time.Now().Add(idleExpiry)).Save(ctx)

	n, err := s.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if n < 2 {
		t.Fatalf("expected at least 2 reaped sessions, got %d", n)
	}

	// Live session survives.
	if got, _ := s.GetSession(live.ID); got == nil {
		t.Fatal("live session must survive the reaper")
	}
}
