package api

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"alga/rabbitmq"
	"alga/store"
)

func TestLifecycleCompletesPendingInvestigationWhenAllAlertsResolved(t *testing.T) {
	agentID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   uuid.New(),
			AlertInvestigationID: "AINV-1",
			Status:               store.AlertInvestigationStatusPending,
			AgentID:              agentID.String(),
			AgentName:            "Hermes",
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	alerts := &mockStore{byFP: map[string]store.AlertRecord{
		"fp-1": {Fingerprint: "fp-1", AlertNumber: 1, Status: "resolved"},
	}}
	svc := NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil)
	err := svc.CompleteIfAllAlertsResolved(context.Background(), store.AlertInvestigationLifecycleCompletionRequest{
		AlertNumber: 1,
		Reason:      store.AlertInvestigationCompletedReasonAlertsResolved,
		ActorType:   store.InvestigationActorSystem,
		ActorName:   "system",
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	updates := invStore.statusUpdates["AINV-1"]
	if len(updates) == 0 || updates[len(updates)-1] != store.AlertInvestigationStatusComplete {
		t.Fatalf("updates = %#v, want complete", updates)
	}
}

func TestLifecycleRejectsOtherAgentForCurrentInvestigation(t *testing.T) {
	agentID := uuid.New()
	otherID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   uuid.New(),
			AlertInvestigationID: "AINV-1",
			Status:               store.AlertInvestigationStatusInvestigating,
			AgentID:              otherID.String(),
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	svc := NewAlertInvestigationLifecycleService(&mockStore{}, invStore, nil, nil, nil)
	err := svc.RequireAlertActionAllowed(context.Background(), 1, &store.AgentTokenRecord{ID: agentID, Name: "Hermes"})
	if err == nil || err.Error() != "not assigned to this investigation" {
		t.Fatalf("err = %v, want not assigned", err)
	}
}
