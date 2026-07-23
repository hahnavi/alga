package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"alga/ent"
	"alga/ent/alertinvestigationalert"
	"alga/rabbitmq"
)

func TestAlertInvestigationStoreCreatesCorrelatedAlertGroup(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)

	created, err := store.CreateAlertInvestigation(context.Background(), AlertInvestigationRecord{
		AlertInvestigationID: "AINV-100",
		CorrelationKey:       "service=checkout",
		Alerts: []rabbitmq.CorrelatedAlert{
			testCorrelatedAlert("fp-1", 101, "CheckoutDown"),
			testCorrelatedAlert("fp-2", 102, "CheckoutLatency"),
		},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}

	got, err := store.GetAlertInvestigation(context.Background(), created.AlertInvestigationID)
	if err != nil {
		t.Fatalf("get alert investigation: %v", err)
	}
	if got == nil {
		t.Fatal("expected alert investigation")
	}
	if len(got.Alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(got.Alerts))
	}

	seen := make(map[string]rabbitmq.CorrelatedAlert, len(got.Alerts))
	for _, alert := range got.Alerts {
		seen[alert.Fingerprint] = alert
	}
	if seen["fp-1"].AlertNumber != 101 {
		t.Fatalf("expected fp-1 alert number 101, got %d", seen["fp-1"].AlertNumber)
	}
	if seen["fp-2"].Labels["alertname"] != "CheckoutLatency" {
		t.Fatalf("expected fp-2 alertname CheckoutLatency, got %q", seen["fp-2"].Labels["alertname"])
	}
}

func TestAlertInvestigationStoreAppendsPromotionSystemUpdate(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	incidentStore := newPGIncidentStore(client)
	incidentInvestigationStore := newPGIncidentInvestigationStore(client)

	created, err := store.CreateAlertInvestigation(context.Background(), AlertInvestigationRecord{
		AlertInvestigationID: "AINV-101",
		CorrelationKey:       "service=billing",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-3", 201, "BillingDown")},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}

	const incidentNumber int64 = 4242
	incident, err := incidentStore.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          "Promotion target",
		Status:         "active",
		Severity:       "high",
		ImpactLevel:    "medium",
		Priority:       "P2",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	inv, err := incidentInvestigationStore.CreateIncidentInvestigation(context.Background(), IncidentInvestigationRecord{
		IncidentNumber: incidentNumber,
		Status:         IncidentInvestigationStatusPending,
		Updates:        []InvestigationUpdate{},
	})
	if err != nil {
		t.Fatalf("create incident investigation: %v", err)
	}

	incidentID := incident.ID.String()
	incidentInvestigationID := inv.ID.String()
	promoted, err := store.MarkAlertInvestigationPromoted(
		context.Background(),
		created.AlertInvestigationID,
		incidentID,
		incidentNumber,
		incidentInvestigationID,
	)
	if err != nil {
		t.Fatalf("mark promoted: %v", err)
	}
	if promoted.PromotedIncidentID == nil || promoted.PromotedIncidentID.String() != incidentID {
		t.Fatalf("expected promoted incident ID %q, got %#v", incidentID, promoted.PromotedIncidentID)
	}
	if promoted.PromotedIncidentInvestigationID == nil || promoted.PromotedIncidentInvestigationID.String() != incidentInvestigationID {
		t.Fatalf("expected promoted incident investigation ID %q, got %#v", incidentInvestigationID, promoted.PromotedIncidentInvestigationID)
	}
	if len(promoted.Updates) != 1 {
		t.Fatalf("expected 1 promotion update, got %d", len(promoted.Updates))
	}

	update := promoted.Updates[0]
	if update.Source != UpdateSourceSystem {
		t.Fatalf("expected system update source, got %q", update.Source)
	}
	if !strings.Contains(update.Message, "#4242") {
		t.Fatalf("expected update message to mention incident number #4242, got %q", update.Message)
	}
	// The progress note must not leak the un-linkable incident / incident
	// investigation UUIDs into the investigation thread.
	if strings.Contains(update.Message, incidentID) {
		t.Fatalf("update message must not leak incident UUID, got %q", update.Message)
	}
	if strings.Contains(update.Message, incidentInvestigationID) {
		t.Fatalf("update message must not leak incident investigation UUID, got %q", update.Message)
	}
}

func TestDeleteAlertHardDeletesActiveLinkedInvestigation(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	alerts := newPGAlertStore(client)
	investigations := newPGAlertInvestigationStore(client)
	ctx := context.Background()

	fingerprint := "fp-delete-active-investigation"
	alertNumber, err := alerts.Create(AlertRecord{
		Fingerprint: fingerprint,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "DeleteMe"},
	})
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}

	created, err := investigations.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-DELETE",
		Status:               AlertInvestigationStatusInvestigating,
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert(fingerprint, alertNumber, "DeleteMe")},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}

	if err := alerts.DeleteAlert(fingerprint); err != nil {
		t.Fatalf("delete alert: %v", err)
	}

	if got, _ := investigations.GetAlertInvestigation(ctx, created.AlertInvestigationID); got != nil {
		t.Fatalf("investigation should be hard-deleted, got status=%q", got.Status)
	}
}

func TestDeleteAlertByNumberHardDeletesActiveLinkedInvestigation(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	alerts := newPGAlertStore(client)
	investigations := newPGAlertInvestigationStore(client)
	ctx := context.Background()

	fingerprint := "fp-delete-number-active-investigation"
	alertNumber, err := alerts.Create(AlertRecord{
		Fingerprint: fingerprint,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "DeleteNumber"},
	})
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}

	created, err := investigations.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-DELETE-NUMBER",
		Status:               AlertInvestigationStatusAssigned,
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert(fingerprint, alertNumber, "DeleteNumber")},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}

	if err := alerts.DeleteAlertByNumber(alertNumber); err != nil {
		t.Fatalf("delete alert by number: %v", err)
	}

	if got, _ := investigations.GetAlertInvestigation(ctx, created.AlertInvestigationID); got != nil {
		t.Fatalf("investigation should be hard-deleted, got status=%q", got.Status)
	}
}

func TestAlertInvestigationStoreListsByAlertNumber(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)

	_, err := store.CreateAlertInvestigation(context.Background(), AlertInvestigationRecord{
		AlertInvestigationID: "AINV-102",
		CorrelationKey:       "service=api",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-4", 301, "APIDown")},
	})
	if err != nil {
		t.Fatalf("create first alert investigation: %v", err)
	}
	_, err = store.CreateAlertInvestigation(context.Background(), AlertInvestigationRecord{
		AlertInvestigationID: "AINV-103",
		CorrelationKey:       "service=worker",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-5", 302, "WorkerDown")},
	})
	if err != nil {
		t.Fatalf("create second alert investigation: %v", err)
	}

	items, err := store.ListAlertInvestigationsByAlertNumber(context.Background(), 301)
	if err != nil {
		t.Fatalf("list by alert number: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 alert investigation, got %d", len(items))
	}
	if items[0].AlertInvestigationID != "AINV-102" {
		t.Fatalf("expected AINV-102, got %q", items[0].AlertInvestigationID)
	}
	if len(items[0].Alerts) != 1 || items[0].Alerts[0].AlertNumber != 301 {
		t.Fatalf("expected returned investigation to include alert number 301, got %#v", items[0].Alerts)
	}
}

func TestCurrentAlertInvestigationLookupIsDeterministic(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()

	older, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-CURRENT-OLDER",
		CorrelationKey:       "service=current",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-current", 401, "CurrentDown")},
	})
	if err != nil {
		t.Fatalf("create older alert investigation: %v", err)
	}
	newer, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-CURRENT-NEWER",
		CorrelationKey:       "service=current",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-current", 401, "CurrentDown")},
	})
	if err != nil {
		t.Fatalf("create newer alert investigation: %v", err)
	}

	if err := store.MarkAlertInvestigationAlertsCurrent(ctx, older.AlertInvestigationID, false); err != nil {
		t.Fatalf("mark older not current: %v", err)
	}
	for range 5 {
		got, err := store.GetCurrentAlertInvestigationByAlertNumber(ctx, 401)
		if err != nil {
			t.Fatalf("get current alert investigation: %v", err)
		}
		if got.AlertInvestigationID != newer.AlertInvestigationID {
			t.Fatalf("current alert investigation = %q, want %q", got.AlertInvestigationID, newer.AlertInvestigationID)
		}
	}

	if err := store.MarkAlertInvestigationAlertsCurrent(ctx, newer.AlertInvestigationID, false); err != nil {
		t.Fatalf("mark newer not current: %v", err)
	}
	if _, err := store.GetCurrentAlertInvestigationByAlertNumber(ctx, 401); !errors.Is(err, ErrInvestigationNotFound) {
		t.Fatalf("expected ErrInvestigationNotFound when no current link remains, got %v", err)
	}
}

func TestCreateAlertInvestigationRetiresExistingCurrentLinks(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()

	older, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-CREATE-CURRENT-OLDER",
		CorrelationKey:       "service=create-current",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-create-current", 451, "CreateCurrentDown")},
	})
	if err != nil {
		t.Fatalf("create older alert investigation: %v", err)
	}

	newer, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-CREATE-CURRENT-NEWER",
		CorrelationKey:       "service=create-current",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-create-current", 451, "CreateCurrentDown")},
	})
	if err != nil {
		t.Fatalf("create replacement alert investigation: %v", err)
	}

	got, err := store.GetCurrentAlertInvestigationByAlertNumber(ctx, 451)
	if err != nil {
		t.Fatalf("get current alert investigation: %v", err)
	}
	if got.AlertInvestigationID != newer.AlertInvestigationID {
		t.Fatalf("current alert investigation = %q, want %q", got.AlertInvestigationID, newer.AlertInvestigationID)
	}

	oldCurrent, err := client.AlertInvestigationAlert.Query().
		Where(alertinvestigationalert.AlertInvestigationUUID(older.ID)).
		Only(ctx)
	if err != nil {
		t.Fatalf("query older alert link: %v", err)
	}
	if oldCurrent.Current {
		t.Fatal("older alert link should no longer be current")
	}
}

func TestAppendAlertsToAlertInvestigationRetiresExistingCurrentLinks(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()

	older, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-APPEND-CURRENT-OLDER",
		CorrelationKey:       "service=append-current-a",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-append-current-1", 501, "AppendCurrentDown")},
	})
	if err != nil {
		t.Fatalf("create older alert investigation: %v", err)
	}

	newer, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-APPEND-CURRENT-NEWER",
		CorrelationKey:       "service=append-current-b",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-append-current-2", 502, "AppendCurrentOtherDown")},
	})
	if err != nil {
		t.Fatalf("create newer alert investigation: %v", err)
	}

	if err := store.AppendAlertsToAlertInvestigation(ctx, newer.AlertInvestigationID, []rabbitmq.CorrelatedAlert{
		testCorrelatedAlert("fp-append-current-1", 501, "AppendCurrentDown"),
	}); err != nil {
		t.Fatalf("append existing current alert: %v", err)
	}

	got, err := store.GetCurrentAlertInvestigationByAlertNumber(ctx, 501)
	if err != nil {
		t.Fatalf("get current alert investigation: %v", err)
	}
	if got.AlertInvestigationID != newer.AlertInvestigationID {
		t.Fatalf("current alert investigation = %q, want %q", got.AlertInvestigationID, newer.AlertInvestigationID)
	}

	oldCurrent, err := client.AlertInvestigationAlert.Query().
		Where(alertinvestigationalert.AlertInvestigationUUID(older.ID)).
		Only(ctx)
	if err != nil {
		t.Fatalf("query older alert link: %v", err)
	}
	if oldCurrent.Current {
		t.Fatal("older alert link should no longer be current")
	}
}

func TestDeleteAlertInvestigationDeletesLifecycleEvents(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()

	created, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-DELETE-EVENTS",
		CorrelationKey:       "service=delete-events",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-delete-events", 701, "DeleteEventsDown")},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}
	if err := store.AppendAlertInvestigationEvent(ctx, created.ID, AlertInvestigationEvent{
		EventType: AlertInvestigationEventRequeued,
		Reason:    "delete regression",
	}); err != nil {
		t.Fatalf("append lifecycle event: %v", err)
	}

	if err := store.DeleteAlertInvestigation(ctx, created.AlertInvestigationID); err != nil {
		t.Fatalf("delete alert investigation: %v", err)
	}
}

func TestCompleteAlertInvestigationPreservesAgentAndWritesMetadata(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-time.Hour)

	created, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-COMPLETE",
		Status:               AlertInvestigationStatusInvestigating,
		AgentID:              "agent-1",
		AgentName:            "Agent One",
		AgentType:            "hermes",
		StartedAt:            &startedAt,
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-complete", 501, "CompleteDown")},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}

	err = store.CompleteAlertInvestigation(ctx, created.ID.String(), AlertInvestigationCompletion{
		Reason:      AlertInvestigationCompletedReasonAgentResolved,
		ActorType:   InvestigationActorAgent,
		ActorID:     "agent-1",
		ActorName:   "Agent One",
		EventReason: "agent reported resolved",
	})
	if err != nil {
		t.Fatalf("complete alert investigation: %v", err)
	}

	got, err := store.GetAlertInvestigation(ctx, created.AlertInvestigationID)
	if err != nil {
		t.Fatalf("get alert investigation: %v", err)
	}
	if got.Status != AlertInvestigationStatusComplete {
		t.Fatalf("status = %q, want %q", got.Status, AlertInvestigationStatusComplete)
	}
	if got.CompletedAt == nil {
		t.Fatal("completed_at should be set")
	}
	if got.AgentID != "agent-1" || got.AgentName != "Agent One" || got.AgentType != "hermes" {
		t.Fatalf("agent fields were not preserved: %#v", got)
	}
	if got.CompletedReason != AlertInvestigationCompletedReasonAgentResolved {
		t.Fatalf("completed reason = %q, want %q", got.CompletedReason, AlertInvestigationCompletedReasonAgentResolved)
	}
	if got.CompletedByType != InvestigationActorAgent || got.CompletedByID != "agent-1" || got.CompletedByName != "Agent One" {
		t.Fatalf("completion actor not stored: type=%q id=%q name=%q", got.CompletedByType, got.CompletedByID, got.CompletedByName)
	}
	if len(got.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Events))
	}
	event := got.Events[0]
	if event.EventType != AlertInvestigationEventCompleted || event.Reason != "agent reported resolved" {
		t.Fatalf("completion event = type %q reason %q", event.EventType, event.Reason)
	}
	if event.ActorType != InvestigationActorAgent || event.ActorID != "agent-1" || event.ActorName != "Agent One" {
		t.Fatalf("completion event actor not stored: %#v", event)
	}
	if event.AgentID != "agent-1" || event.AgentName != "Agent One" || event.AgentType != "hermes" {
		t.Fatalf("completion event agent not stored: %#v", event)
	}
}

func TestCompleteAlertInvestigationIdempotentWhenAlreadyTerminal(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-time.Hour)

	created, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-IDEMP",
		Status:               AlertInvestigationStatusInvestigating,
		AgentID:              "agent-1",
		AgentName:            "Agent One",
		AgentType:            "hermes",
		StartedAt:            &startedAt,
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-idemp", 701, "IdempDown")},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}

	first := AlertInvestigationCompletion{
		Reason:      AlertInvestigationCompletedReasonAgentResolved,
		ActorType:   InvestigationActorAgent,
		ActorID:     "agent-1",
		ActorName:   "Agent One",
		EventReason: "first caller resolved",
	}
	if err := store.CompleteAlertInvestigation(ctx, created.ID.String(), first); err != nil {
		t.Fatalf("first complete: %v", err)
	}

	raced := AlertInvestigationCompletion{
		Reason:      AlertInvestigationCompletedReasonAlertsResolved,
		ActorType:   InvestigationActorAgent,
		ActorID:     "agent-2",
		ActorName:   "Agent Two",
		EventReason: "concurrent caller resolved",
	}
	if err := store.CompleteAlertInvestigation(ctx, created.ID.String(), raced); err != nil {
		t.Fatalf("second complete should be idempotent, got: %v", err)
	}

	got, err := store.GetAlertInvestigation(ctx, created.AlertInvestigationID)
	if err != nil {
		t.Fatalf("get alert investigation: %v", err)
	}
	if got.Status != AlertInvestigationStatusComplete {
		t.Fatalf("status = %q, want %q", got.Status, AlertInvestigationStatusComplete)
	}
	if got.CompletedByID != "agent-1" || got.CompletedByName != "Agent One" {
		t.Fatalf("completion actor overwritten by idempotent call: id=%q name=%q", got.CompletedByID, got.CompletedByName)
	}
	if len(got.Events) != 1 {
		t.Fatalf("expected exactly 1 completion event (no duplicate from idempotent call), got %d", len(got.Events))
	}
}

func TestRequeueAlertInvestigationPreservesAgent(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-30 * time.Minute)

	created, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-REQUEUE",
		Status:               AlertInvestigationStatusAssigned,
		AgentID:              "agent-2",
		AgentName:            "Agent Two",
		AgentType:            "openclaw",
		StartedAt:            &startedAt,
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-requeue", 601, "RequeueDown")},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}

	err = store.RequeueAlertInvestigation(ctx, created.ID.String(), AlertInvestigationRequeue{
		Reason:    "agent unavailable",
		ActorType: InvestigationActorSystem,
		ActorName: "scheduler",
		Metadata:  map[string]any{"attempt": float64(2)},
	})
	if err != nil {
		t.Fatalf("requeue alert investigation: %v", err)
	}

	got, err := store.GetAlertInvestigation(ctx, created.AlertInvestigationID)
	if err != nil {
		t.Fatalf("get alert investigation: %v", err)
	}
	if got.Status != AlertInvestigationStatusPending {
		t.Fatalf("status = %q, want %q", got.Status, AlertInvestigationStatusPending)
	}
	if got.StartedAt != nil {
		t.Fatalf("started_at should be cleared, got %v", got.StartedAt)
	}
	if got.AgentID != "agent-2" || got.AgentName != "Agent Two" || got.AgentType != "openclaw" {
		t.Fatalf("agent fields were not preserved: %#v", got)
	}
	if len(got.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Events))
	}
	event := got.Events[0]
	if event.EventType != AlertInvestigationEventRequeued || event.Reason != "agent unavailable" {
		t.Fatalf("requeue event = type %q reason %q", event.EventType, event.Reason)
	}
	if event.ActorType != InvestigationActorSystem || event.ActorName != "scheduler" {
		t.Fatalf("requeue event actor not stored: %#v", event)
	}
	if event.AgentID != "agent-2" || event.AgentName != "Agent Two" || event.AgentType != "openclaw" {
		t.Fatalf("requeue event agent not stored: %#v", event)
	}
	if event.Metadata["attempt"] != float64(2) {
		t.Fatalf("requeue event metadata = %#v", event.Metadata)
	}
}

func TestAlertInvestigationSummariesBatchLookup(t *testing.T) {
	client := newAlertInvestigationEntTestClient(t)
	store := newPGAlertInvestigationStore(client)
	ctx := context.Background()

	_, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-SUM-1",
		CorrelationKey:       "service=api",
		Status:               AlertInvestigationStatusInvestigating,
		AgentID:              "agent-1",
		AgentName:            "Agent One",
		AgentType:            "hermes",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-sum-1", 601, "SummaryOne")},
	})
	if err != nil {
		t.Fatalf("create first investigation: %v", err)
	}
	_, err = store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-SUM-2",
		CorrelationKey:       "service=worker",
		Status:               AlertInvestigationStatusAssigned,
		AgentID:              "agent-2",
		AgentName:            "Agent Two",
		AgentType:            "openclaw",
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-sum-2", 602, "SummaryTwo")},
	})
	if err != nil {
		t.Fatalf("create second investigation: %v", err)
	}
	if _, err := store.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-SUM-3",
		CorrelationKey:       "service=db",
		Status:               AlertInvestigationStatusPending,
		Alerts:               []rabbitmq.CorrelatedAlert{testCorrelatedAlert("fp-sum-3", 603, "SummaryThree")},
	}); err != nil {
		t.Fatalf("create third investigation: %v", err)
	}

	got, err := store.GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx, []int64{601, 602, 603, 999})
	if err != nil {
		t.Fatalf("batch lookup: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 summaries, got %d (%#v)", len(got), got)
	}

	if got[601].AlertInvestigationID != "AINV-SUM-1" ||
		got[601].Status != AlertInvestigationStatusInvestigating ||
		got[601].AgentID != "agent-1" ||
		got[601].AgentName != "Agent One" ||
		got[601].AgentType != "hermes" {
		t.Errorf("summary for 601 = %#v", got[601])
	}
	if got[602].AlertInvestigationID != "AINV-SUM-2" ||
		got[602].AgentName != "Agent Two" ||
		got[602].AgentType != "openclaw" {
		t.Errorf("summary for 602 = %#v", got[602])
	}
	if got[603].AlertInvestigationID != "AINV-SUM-3" {
		t.Errorf("summary for 603 = %#v", got[603])
	}
	if got[603].AgentID != "" || got[603].AgentName != "" || got[603].AgentType != "" {
		t.Errorf("pending investigation should have empty agent fields, got %#v", got[603])
	}

	empty, err := store.GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx, nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty map for nil input, got %#v", empty)
	}
}

func testCorrelatedAlert(fingerprint string, alertNumber int64, alertname string) rabbitmq.CorrelatedAlert {
	return rabbitmq.CorrelatedAlert{
		Fingerprint:  fingerprint,
		AlertNumber:  alertNumber,
		Status:       "firing",
		StartsAt:     "2026-05-24T00:00:00Z",
		GeneratorURL: "https://grafana.example.test/alert/" + fingerprint,
		Labels: map[string]string{
			"alertname": alertname,
			"namespace": "prod",
		},
		Annotations: map[string]string{
			"summary": alertname + " is firing",
		},
	}
}

func newAlertInvestigationEntTestClient(t *testing.T) *ent.Client {
	t.Helper()
	return newTestEntClient(t)
}
