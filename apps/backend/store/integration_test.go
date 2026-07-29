//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestAlertCRUD(t *testing.T) {
	stores := newTestStores(t)

	rec := AlertRecord{
		Fingerprint: "test-fingerprint-001",
		Status:      "firing",
		Labels:      map[string]string{"alertname": "TestAlert", "severity": "critical"},
		Annotations: map[string]string{"summary": "test alert"},
		StartsAt:    time.Now().UTC(),
	}

	alertNumber, err := stores.Alert.Create(rec)
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}
	if alertNumber == 0 {
		t.Fatal("expected non-zero alert_number")
	}

	got, err := stores.Alert.GetByFingerprint(rec.Fingerprint)
	if err != nil {
		t.Fatalf("get by fingerprint: %v", err)
	}
	if got == nil {
		t.Fatal("expected alert, got nil")
	}
	if got.AlertNumber != alertNumber {
		t.Fatalf("alert_number mismatch: got %d, want %d", got.AlertNumber, alertNumber)
	}

	alerts, err := stores.Alert.QueryAlerts(map[string]any{"$limit": 10, "$skip": 0})
	if err != nil {
		t.Fatalf("query alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
}

func TestUserCRUD(t *testing.T) {
	stores := newTestStores(t)

	user, err := stores.User.CreateUser("test@example.com", "hashedpass", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Email != "test@example.com" {
		t.Fatalf("email mismatch: got %q", user.Email)
	}

	got, err := stores.User.GetByEmail("test@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.Role != "admin" {
		t.Fatalf("role mismatch: got %q", got.Role)
	}
}

func TestMigrationsCreateTables(t *testing.T) {
	skipIfNoDocker(t)
	bunDB := newTestDB(t)

	var count int
	err := bunDB.NewRaw("SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(context.Background(), &count)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if count < 58 {
		t.Fatalf("expected at least 58 tables, got %d", count)
	}
}
