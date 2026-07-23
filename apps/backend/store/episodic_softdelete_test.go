//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"alga/rabbitmq"
)

// TestEpisodicFinder_ExcludesSoftDeletedAlert verifies that an investigation
// whose primary alert has been soft-deleted is NOT returned by the episodic
// finder, so the agent's auto-injected PAST INCIDENTS block never cites
// soft-deleted context.
func TestEpisodicFinder_ExcludesSoftDeletedAlert(t *testing.T) {
	ctx := context.Background()
	fp := "episodic-softdel-" + t.Name()
	correlationKey := "episodic-softdel-key-" + t.Name()

	created, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "EpisodicSoftDel"},
	})
	if err != nil {
		t.Fatalf("Create alert: %v", err)
	}
	if created == 0 {
		t.Fatal("expected alert number from Create")
	}

	rec, err := alertInvStore.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		CorrelationKey: correlationKey,
		Status:         AlertInvestigationStatusComplete,
		CompletedAt:    ptrTime(time.Now().Add(-1 * time.Hour)),
		Alerts: []rabbitmq.CorrelatedAlert{{
			Fingerprint: fp,
			AlertNumber: created,
			Labels:      map[string]string{"alertname": "EpisodicSoftDel"},
			Status:      "firing",
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	})
	if err != nil {
		t.Fatalf("CreateAlertInvestigation: %v", err)
	}

	if err := alertsStore.DeleteAlert(fp); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	similar, err := alertInvStore.FindSimilarAlertInvestigations(ctx, SimilarAlertInvestigationsQuery{
		CorrelationKey: correlationKey,
		Limit:          5,
	})
	if err != nil {
		t.Fatalf("FindSimilarAlertInvestigations: %v", err)
	}
	for _, s := range similar {
		if s.AlertInvestigationID == rec.AlertInvestigationID {
			t.Fatal("episodic finder returned an investigation whose alert was soft-deleted")
		}
	}
}

// TestEpisodicFinder_KeepsInvestigationWithLiveAlert is a positive control: an
// investigation whose primary alert was NOT soft-deleted must still be returned.
// Protects against a regression where the filter accidentally excludes
// everything.
func TestEpisodicFinder_KeepsInvestigationWithLiveAlert(t *testing.T) {
	ctx := context.Background()
	fp := "episodic-live-" + t.Name()

	// Seed a live (NOT soft-deleted) alert + completed investigation.
	if _, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "EpisodicLive"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := alertInvStore.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		CorrelationKey: "episodic-live-key",
		Status:         AlertInvestigationStatusComplete,
		CompletedAt:    ptrTime(time.Now().Add(-1 * time.Hour)),
		Alerts: []rabbitmq.CorrelatedAlert{{
			Fingerprint: fp,
			Labels:      map[string]string{"alertname": "EpisodicLive"},
			Status:      "firing",
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}); err != nil {
		t.Fatalf("CreateAlertInvestigation: %v", err)
	}

	similar, err := alertInvStore.FindSimilarAlertInvestigations(ctx, SimilarAlertInvestigationsQuery{
		CorrelationKey: "episodic-live-key",
		Limit:          5,
	})
	if err != nil {
		t.Fatalf("FindSimilarAlertInvestigations: %v", err)
	}
	if len(similar) == 0 {
		t.Fatal("expected episodic finder to return the live investigation; got 0 results")
	}
	found := false
	for _, s := range similar {
		for _, a := range s.Alerts {
			if a.Labels["alertname"] == "EpisodicLive" {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("episodic finder excluded the investigation with a live (non-deleted) alert — the filter is over-eager")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
