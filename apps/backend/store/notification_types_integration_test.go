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

// emittedNotificationTypes mirrors every NotificationType string shipped
// producers publish, including
// the test endpoint's "test" type.
var emittedNotificationTypes = []string{
	"escalation",
	"oncall_handoff",
	"post_mortem_review_requested",
	"action_item_assigned",
	"mention",
	"info",
	"incident_acknowledged",
	"incident_mitigated",
	"incident_resolved",
	"incident_reopened",
	"oncall_reminder",
	"test",
}

// TestNotificationChecksAcceptEmittedTypes verifies the migration-00017 CHECK
// constraints accept every type and resource_type shipped producers emit —
// the dispatch worker inserts through the same paths exercised here, so any
// rejection here dead-letters real notifications.
func TestNotificationChecksAcceptEmittedTypes(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	ctx := context.Background()
	user, err := stores.User.CreateUser(
		fmt.Sprintf("notification-types-%s@example.com", uuid.NewString()[:8]),
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	userID := user.ID.String()
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.Notification)(nil)).Where("user_id = ?", userID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.NotificationDeliveryLog)(nil)).Where("user_id = ?", userID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.User)(nil)).Where("id = ?", userID).Exec(context.Background())
	})

	for _, nt := range emittedNotificationTypes {
		rec := &NotificationRecord{UserID: userID, Type: nt, Title: "t " + nt, Message: "m"}
		if _, err := stores.Notification.Create(ctx, rec); err != nil {
			t.Errorf("notifications.type CHECK rejects emitted type %q: %v", nt, err)
		}
	}

	resourceTypes := []string{"incident", "investigation", "post_mortem", "action_item", "alert", "handoff", "schedule", "system"}
	for i, rt := range resourceTypes {
		rec := &NotificationRecord{
			UserID:       userID,
			Type:         "info",
			Title:        fmt.Sprintf("resource %d", i),
			Message:      "m",
			ResourceType: rt,
		}
		if _, err := stores.Notification.Create(ctx, rec); err != nil {
			t.Errorf("notifications.resource_type CHECK rejects emitted resource_type %q: %v", rt, err)
		}
	}

	for _, nt := range emittedNotificationTypes {
		rec := &NotificationDeliveryRecord{
			UserID:           user.ID,
			NotificationType: nt,
			Channel:          "email",
			Status:           "queued",
		}
		if _, err := stores.Delivery.Create(ctx, rec); err != nil {
			t.Errorf("notification_delivery_logs.notification_type CHECK rejects emitted type %q: %v", nt, err)
		}
	}

	bogus := &NotificationRecord{UserID: userID, Type: "carrier_pigeon", Title: "t", Message: "m"}
	if _, err := stores.Notification.Create(ctx, bogus); err == nil {
		t.Error("notifications.type CHECK accepted unknown type \"carrier_pigeon\"")
	}
	bogusResource := &NotificationRecord{UserID: userID, Type: "info", Title: "t", Message: "m", ResourceType: "smoke_signal"}
	if _, err := stores.Notification.Create(ctx, bogusResource); err == nil {
		t.Error("notifications.resource_type CHECK accepted unknown resource_type \"smoke_signal\"")
	}
}
