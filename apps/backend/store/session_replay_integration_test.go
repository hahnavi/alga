//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/db"
	"alga/db/models"
)

// TestSessionFamilyReplayDetection pins the store contract: rotation
// remembers rotated-out session IDs and refresh tokens for the sliding window,
// a replayed cookie resolves via FindRotatedOutSession, a replayed refresh
// token surfaces ErrRefreshTokenReused with the owning session, and unknown
// credentials resolve to nothing.
func TestSessionFamilyReplayDetection(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	user, err := stores.User.CreateUser(
		fmt.Sprintf("replay-%s@example.com", uuid.NewString()[:8]),
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.Session)(nil)).Where("user_id = ?", user.ID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.User)(nil)).Where("id = ?", user.ID).Exec(context.Background())
	})

	first, err := stores.Session.CreateSession(user.ID, "127.0.0.1", "replay-test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Rotation invalidates the old cookie and remembers its hash.
	second, err := stores.Session.RefreshSession(first.ID, "127.0.0.1", "replay-test")
	if err != nil || second == nil {
		t.Fatalf("refresh session: err=%v rec=%v", err, second)
	}
	if second.ID == first.ID || second.RefreshToken == first.RefreshToken {
		t.Fatal("rotation must mint a new session ID and refresh token")
	}
	if rec, _ := stores.Session.GetSession(first.ID); rec != nil {
		t.Fatal("rotated-out session ID must no longer authenticate")
	}

	rotated, err := stores.Session.FindRotatedOutSession(first.ID)
	if err != nil {
		t.Fatalf("FindRotatedOutSession: %v", err)
	}
	if rotated == nil || rotated.UserID != user.ID {
		t.Fatalf("rotated-out cookie must resolve to the owning session, got %+v", rotated)
	}

	// The live refresh token rotates through the refresh-token path.
	third, err := stores.Session.RefreshSessionByRefreshToken(second.RefreshToken, "127.0.0.1", "replay-test")
	if err != nil || third == nil {
		t.Fatalf("RefreshSessionByRefreshToken(live): err=%v rec=%v", err, third)
	}

	// Presenting the just-rotated-out refresh token again is replay.
	owner, err := stores.Session.RefreshSessionByRefreshToken(second.RefreshToken, "127.0.0.1", "replay-test")
	if !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("replayed refresh token err = %v, want ErrRefreshTokenReused", err)
	}
	if owner == nil || owner.UserID != user.ID {
		t.Fatalf("replay must return the owning session for revocation, got %+v", owner)
	}

	// Unknown credentials resolve to nothing.
	if unknown, err := stores.Session.FindRotatedOutSession("never-issued"); err != nil || unknown != nil {
		t.Fatalf("FindRotatedOutSession(unknown) = %+v, %v; want nil, nil", unknown, err)
	}
	if rec, err := stores.Session.RefreshSessionByRefreshToken("never-issued", "127.0.0.1", "replay-test"); err != nil || rec != nil {
		t.Fatalf("RefreshSessionByRefreshToken(unknown) = %+v, %v; want nil, nil", rec, err)
	}
}
