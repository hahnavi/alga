//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db"
	"alga/db/models"
)

// TestSetSlackIdentityDuplicateBinding pins the uniqueness contract:
// binding a Slack identity that another user already owns fails with the
// typed ErrSlackIdentityTaken (the users_slack_user_id partial unique index
// from migration 00002) rather than a raw driver error, re-binding by the
// current owner is a plain update, and clearing the owner frees the identity.
func TestSetSlackIdentityDuplicateBinding(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	ctx := context.Background()
	first, err := stores.User.CreateUser(
		"slack-bind-first-"+uuid.NewString()[:8]+"@example.com",
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}
	second, err := stores.User.CreateUser(
		"slack-bind-second-"+uuid.NewString()[:8]+"@example.com",
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.User)(nil)).
			Where("id IN (?)", bun.List([]uuid.UUID{first.ID, second.ID})).
			Exec(context.Background())
	})

	slackID := "U-DUP-" + uuid.NewString()[:8]

	if err := stores.User.SetSlackIdentity(ctx, first.ID, slackID, "First"); err != nil {
		t.Fatalf("first binding: %v", err)
	}

	if err := stores.User.SetSlackIdentity(ctx, second.ID, slackID, "Second"); !errors.Is(err, ErrSlackIdentityTaken) {
		t.Fatalf("duplicate binding error = %v, want ErrSlackIdentityTaken", err)
	}

	if err := stores.User.SetSlackIdentity(ctx, first.ID, slackID, "First Renamed"); err != nil {
		t.Fatalf("re-bind by current owner: %v", err)
	}

	if err := stores.User.ClearSlackIdentity(ctx, first.ID); err != nil {
		t.Fatalf("clear identity: %v", err)
	}
	if err := stores.User.SetSlackIdentity(ctx, second.ID, slackID, "Second"); err != nil {
		t.Fatalf("bind after owner cleared: %v", err)
	}
}
