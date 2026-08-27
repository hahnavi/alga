//go:build integration

package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/db"
	"alga/db/models"
)

// TestRetentionDeleteFamily pins the DT-E3 store contract: audit logs,
// triage results, and notification delivery logs purge strictly older than
// the cutoff (in bounded batches) while fresh rows survive; password-reset
// tokens purge when used OR more than a week past expiry.
func TestRetentionDeleteFamily(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	ctx := context.Background()
	user, err := stores.User.CreateUser(
		fmt.Sprintf("retention-%s@example.com", uuid.NewString()[:8]),
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	userID := user.ID
	old := time.Now().UTC().AddDate(-120, 0, 0)
	fresh := time.Now().UTC().Add(time.Hour)
	cutoff := time.Now().UTC().AddDate(0, 0, -90)

	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.AuditLog)(nil)).Where("user_id = ?", userID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.TriageResult)(nil)).Where("correlation_key LIKE ?", "retention-test-%").Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.NotificationDeliveryLog)(nil)).Where("user_id = ?", userID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.PasswordResetToken)(nil)).Where("user_id = ?", userID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.User)(nil)).Where("id = ?", userID).Exec(context.Background())
	})

	// Audit logs: rows are inserted directly (LogRecord is fire-and-forget),
	// old deleted, fresh kept.
	for _, ts := range []time.Time{old, fresh} {
		audit := &models.AuditLog{
			IDModel:   models.IDModel{ID: models.NewUUID()},
			Timestamp: ts,
			Event:     "retention_test",
			UserID:    &userID,
		}
		if _, err := bunDB.NewInsert().Model(audit).Exec(ctx); err != nil {
			t.Fatalf("insert audit fixture: %v", err)
		}
	}
	if n, err := stores.Audit.DeleteOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("audit DeleteOlderThan: %v", err)
	} else if n == 0 {
		t.Error("audit DeleteOlderThan deleted nothing, want the old row")
	}
	var auditLeft int
	if err := bunDB.NewSelect().Model((*models.AuditLog)(nil)).
		Where("user_id = ?", userID).Where("timestamp >= ?", cutoff).
		ColumnExpr("count(*)").Scan(ctx, &auditLeft); err != nil {
		t.Fatalf("count fresh audit rows: %v", err)
	}
	if auditLeft != 1 {
		t.Errorf("fresh audit rows = %d, want 1", auditLeft)
	}

	// Triage results: the old row goes in directly because Create stamps
	// created_at to now; numbers come from the real sequence.
	for _, ts := range []time.Time{old, fresh} {
		var triageNumber int64
		if err := bunDB.NewSelect().ColumnExpr("nextval('triage_number_seq')").Scan(ctx, &triageNumber); err != nil {
			t.Fatalf("allocate triage number: %v", err)
		}
		tr := &models.TriageResult{
			ID:             models.NewUUID(),
			TriageNumber:   triageNumber,
			CorrelationKey: "retention-test-" + uuid.NewString(),
			Decision:       "investigate",
			Outcome:        TriageResultOutcomePending,
			CreatedAt:      ts,
			UpdatedAt:      ts,
		}
		if _, err := bunDB.NewInsert().Model(tr).Exec(ctx); err != nil {
			t.Fatalf("insert triage fixture: %v", err)
		}
	}
	if n, err := stores.TriageResult.DeleteOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("triage DeleteOlderThan: %v", err)
	} else if n != 1 {
		t.Errorf("triage DeleteOlderThan = %d, want 1 (the old row)", n)
	}

	// Notification delivery logs.
	for _, ts := range []time.Time{old, fresh} {
		if _, err := stores.Delivery.Create(ctx, &NotificationDeliveryRecord{
			UserID:           userID,
			NotificationType: "info",
			Channel:          "email",
			Status:           "queued",
			CreatedAt:        ts,
		}); err != nil {
			t.Fatalf("insert delivery fixture: %v", err)
		}
	}
	if n, err := stores.Delivery.DeleteOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("delivery DeleteOlderThan: %v", err)
	} else if n != 1 {
		t.Errorf("delivery DeleteOlderThan = %d, want 1 (the old row)", n)
	}

	// Password-reset tokens: used rows and rows expired before the cutoff
	// go; fresh unexpired unused rows stay.
	live, err := stores.PasswordReset.CreateToken(ctx, userID, "hash-live-"+uuid.NewString(), fresh)
	if err != nil {
		t.Fatalf("create live token: %v", err)
	}
	used, err := stores.PasswordReset.CreateToken(ctx, userID, "hash-used-"+uuid.NewString(), fresh)
	if err != nil {
		t.Fatalf("create used token: %v", err)
	}
	if err := stores.PasswordReset.MarkUsed(ctx, used.ID); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	if _, err := stores.PasswordReset.CreateToken(ctx, userID, "hash-stale-"+uuid.NewString(), old); err != nil {
		t.Fatalf("create stale token: %v", err)
	}
	if n, err := stores.PasswordReset.DeleteConsumedExpired(ctx, time.Now().UTC().AddDate(0, 0, -7)); err != nil {
		t.Fatalf("DeleteConsumedExpired: %v", err)
	} else if n != 2 {
		t.Errorf("DeleteConsumedExpired = %d, want 2 (used + week-stale expired)", n)
	}
	var tokensLeft int
	if err := bunDB.NewSelect().Model((*models.PasswordResetToken)(nil)).
		Where("user_id = ?", userID).ColumnExpr("count(*)").Scan(ctx, &tokensLeft); err != nil {
		t.Fatalf("count remaining tokens: %v", err)
	}
	if tokensLeft != 1 {
		t.Errorf("remaining tokens = %d, want 1 (the live unused one)", tokensLeft)
	}
	got, err := stores.PasswordReset.GetByTokenHash(ctx, live.TokenHash)
	if err != nil {
		t.Fatalf("GetByTokenHash(live): %v", err)
	}
	if got.UserID != userID || got.Used {
		t.Errorf("live token = %+v, want the unused fixture row", got)
	}
}
