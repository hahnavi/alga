package store

import (
	"context"
	"testing"
	"time"

	"alga/ent"
)

const testIncidentNumber int64 = 1

func TestIncidentInvestigationStoreCreatesForIncident(t *testing.T) {
	client := newIncidentInvestigationEntTestClient(t)
	store := newPGIncidentInvestigationStore(client)

	created, err := store.CreateIncidentInvestigation(context.Background(), IncidentInvestigationRecord{
		IncidentNumber: testIncidentNumber,
		Status:         IncidentInvestigationStatusPending,
		Updates:        []InvestigationUpdate{},
	})
	if err != nil {
		t.Fatalf("create incident investigation: %v", err)
	}
	if created.IncidentInvestigationID == "" {
		t.Fatal("id is empty")
	}
	if created.IncidentNumber != testIncidentNumber {
		t.Errorf("incident number mismatch: got %d, want %d", created.IncidentNumber, testIncidentNumber)
	}
	if created.Status != IncidentInvestigationStatusPending {
		t.Errorf("status mismatch: got %s, want %s", created.Status, IncidentInvestigationStatusPending)
	}
}

func TestIncidentInvestigationStoreAllowsOnlyOneActivePerIncident(t *testing.T) {
	client := newIncidentInvestigationEntTestClient(t)
	store := newPGIncidentInvestigationStore(client)

	record1 := IncidentInvestigationRecord{
		IncidentNumber: testIncidentNumber,
		Status:         IncidentInvestigationStatusPending,
		Updates:        []InvestigationUpdate{},
	}

	_, err := store.CreateIncidentInvestigation(context.Background(), record1)
	if err != nil {
		t.Fatalf("failed to create first: %v", err)
	}

	record2 := IncidentInvestigationRecord{
		IncidentNumber: testIncidentNumber,
		Status:         IncidentInvestigationStatusPending,
		Updates:        []InvestigationUpdate{},
	}

	_, err = store.CreateIncidentInvestigation(context.Background(), record2)
	if err != ErrActiveIncidentInvestigationExists {
		t.Errorf("expected ErrActiveIncidentInvestigationExists, got: %v", err)
	}
}

func TestIncidentInvestigationStoreCreatesInitialAlertSummaryUpdate(t *testing.T) {
	client := newIncidentInvestigationEntTestClient(t)
	store := newPGIncidentInvestigationStore(client)

	update := InvestigationUpdate{
		Type:      UpdateTypeProgress,
		Message:   "Initial investigation from alert investigation AINV-1",
		Source:    UpdateSourceSystem,
		Internal:  false,
		CreatedAt: time.Now().UTC(),
	}

	record := IncidentInvestigationRecord{
		IncidentNumber: testIncidentNumber,
		Status:         IncidentInvestigationStatusPending,
		Updates:        []InvestigationUpdate{update},
	}

	created, err := store.CreateIncidentInvestigation(context.Background(), record)
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	if len(created.Updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(created.Updates))
	}
	if created.Updates[0].Message != update.Message {
		t.Errorf("message mismatch: got %s, want %s", created.Updates[0].Message, update.Message)
	}
}

func newIncidentInvestigationEntTestClient(t *testing.T) *ent.Client {
	t.Helper()
	client := newTestEntClient(t)
	incidentStore := newPGIncidentStore(client)
	if _, err := incidentStore.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: testIncidentNumber,
		Title:          "Test Incident",
		Status:         "active",
		Severity:       "critical",
		Priority:       "P2",
		CreatedAt:      time.Now(),
	}); err != nil {
		t.Fatalf("create test incident: %v", err)
	}
	return client
}
