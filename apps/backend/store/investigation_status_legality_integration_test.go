//go:build integration

package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"alga/rabbitmq"
)

// TestAlertInvestigationTransitionLegality pins the migrated alert_investigations
// status CHECK (migration 00008) behind the two targets the agent resolve-alert
// finalization path may write: an active investigation must be finalizable to
// `promoted`, while the stray `reviewing` value that the DB always
// rejected must keep failing so it can never silently reappear as a writable
// status.
func TestAlertInvestigationTransitionLegality(t *testing.T) {
	ctx := context.Background()

	rec, err := alertInvStore.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		CorrelationKey: "transition-legality",
		Status:         AlertInvestigationStatusInvestigating,
		Alerts: []rabbitmq.CorrelatedAlert{
			{
				Fingerprint:  "fp-transition-legality",
				Labels:       map[string]string{"alertname": "TransitionLegality"},
				Status:       "firing",
				StartsAt:     time.Now().Format(time.RFC3339),
				GeneratorURL: "http://grafana/transition-legality",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateAlertInvestigation: %v", err)
	}
	t.Cleanup(func() { alertInvStore.DeleteAlertInvestigation(ctx, rec.AlertInvestigationID) })

	if err := alertInvStore.TransitionAlertInvestigationStatus(ctx,
		rec.ID.String(),
		[]string{AlertInvestigationStatusAssigned, AlertInvestigationStatusInvestigating},
		AlertInvestigationStatusPromoted,
	); err != nil {
		t.Fatalf("transition investigating -> promoted must succeed on a migrated DB (writes this value): %v", err)
	}

	got, err := alertInvStore.GetAlertInvestigation(ctx, rec.AlertInvestigationID)
	if err != nil {
		t.Fatalf("GetAlertInvestigation: %v", err)
	}
	if got.Status != AlertInvestigationStatusPromoted {
		t.Errorf("Status = %q, want promoted", got.Status)
	}

	stray, err := alertInvStore.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		CorrelationKey: "transition-legality-reviewing",
		Status:         AlertInvestigationStatusPending,
		Alerts: []rabbitmq.CorrelatedAlert{
			{
				Fingerprint:  "fp-transition-legality-reviewing",
				Labels:       map[string]string{"alertname": "TransitionLegality"},
				Status:       "firing",
				StartsAt:     time.Now().Format(time.RFC3339),
				GeneratorURL: "http://grafana/transition-legality",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateAlertInvestigation stray leg: %v", err)
	}
	t.Cleanup(func() { alertInvStore.DeleteAlertInvestigation(ctx, stray.AlertInvestigationID) })

	err = alertInvStore.TransitionAlertInvestigationStatus(ctx,
		stray.ID.String(),
		[]string{AlertInvestigationStatusPending},
		"reviewing",
	)
	if err == nil {
		t.Fatal("transition to undeclared status `reviewing` must fail on a migrated DB")
	}
	if !strings.Contains(err.Error(), "reviewing") && !strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Errorf("error should name the rejected value or the constraint; got %v", err)
	}

	fetched, err := alertInvStore.GetAlertInvestigation(ctx, stray.AlertInvestigationID)
	if err != nil {
		t.Fatalf("GetAlertInvestigation after rejected transition: %v", err)
	}
	if fetched.Status != AlertInvestigationStatusPending {
		t.Errorf("Status = %q after rejected transition, want pending (unchanged)", fetched.Status)
	}
}
