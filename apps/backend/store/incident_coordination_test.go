package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/ent"
)

func TestIncidentCoordinationStoreCreateListAndDedupSlackTS(t *testing.T) {
	client := newEntTestClient(t)
	incidentStore := newPGIncidentStore(client)
	coordStore := newPGIncidentCoordinationStore(client)

	incidentNumber := int64(4242)
	created, err := incidentStore.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          "Coordination test",
		Description:    "test incident",
		Status:         "active",
		Severity:       "high",
		ImpactLevel:    "medium",
		Priority:       "P2",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	if created == nil {
		t.Fatal("expected created incident")
	}

	actorID := uuid.New()
	msg, err := coordStore.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber:        incidentNumber,
		Kind:                  IncidentCoordinationKindChat,
		ActorType:             IncidentCoordinationActorUser,
		ActorID:               &actorID,
		ActorDisplayName:      "Ada Lovelace",
		Body:                  "Investigating customer impact",
		Source:                IncidentCoordinationSourceAlga,
		SlackChannelID:        "C123",
		SlackMessageTS:        "1716400000.000100",
		SlackThreadTS:         "1716400000.000100",
		ProviderMessageID:     "slack:C123:1716400000.000100",
		LinkedInvestigationID: "",
	})
	if err != nil {
		t.Fatalf("create coordination message: %v", err)
	}
	if msg.ID == uuid.Nil {
		t.Fatal("expected generated message ID")
	}

	duplicate, err := coordStore.FindByProviderMessageID(context.Background(), "slack:C123:1716400000.000100")
	if err != nil {
		t.Fatalf("find by provider id: %v", err)
	}
	if duplicate == nil || duplicate.ID != msg.ID {
		t.Fatalf("expected duplicate lookup to return created message")
	}

	items, err := coordStore.ListMessages(context.Background(), incidentNumber, 50, 0)
	if err != nil {
		t.Fatalf("list coordination messages: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	if items[0].Body != "Investigating customer impact" {
		t.Fatalf("unexpected body %q", items[0].Body)
	}
}

func newEntTestClient(t *testing.T) *ent.Client {
	t.Helper()
	return newTestEntClient(t)
}

func TestIncidentCoordinationListMessagesByKind(t *testing.T) {
	client := newEntTestClient(t)
	incidentStore := newPGIncidentStore(client)
	coordStore := newPGIncidentCoordinationStore(client)

	incidentNumber := int64(5555)
	_, err := incidentStore.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          "Status update test",
		Description:    "test incident",
		Status:         "active",
		Severity:       "high",
		ImpactLevel:    "medium",
		Priority:       "P2",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}

	actorID := uuid.New()

	_, err = coordStore.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             IncidentCoordinationKindChat,
		ActorType:        IncidentCoordinationActorUser,
		ActorID:          &actorID,
		ActorDisplayName: "Alice",
		Body:             "Regular chat message",
		Source:           IncidentCoordinationSourceAlga,
	})
	if err != nil {
		t.Fatalf("create chat message: %v", err)
	}

	_, err = coordStore.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             IncidentCoordinationKindStatusUpdate,
		ActorType:        IncidentCoordinationActorUser,
		ActorID:          &actorID,
		ActorDisplayName: "Alice",
		Body:             "We are investigating.",
		Source:           IncidentCoordinationSourceAlga,
		Metadata:         map[string]any{"status_level": "investigating"},
	})
	if err != nil {
		t.Fatalf("create status update: %v", err)
	}

	_, err = coordStore.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             IncidentCoordinationKindStatusUpdate,
		ActorType:        IncidentCoordinationActorUser,
		ActorID:          &actorID,
		ActorDisplayName: "Alice",
		Body:             "Root cause found.",
		Source:           IncidentCoordinationSourceAlga,
		Metadata:         map[string]any{"status_level": "identified"},
	})
	if err != nil {
		t.Fatalf("create status update 2: %v", err)
	}

	statusUpdates, err := coordStore.ListMessagesByKind(context.Background(), incidentNumber, IncidentCoordinationKindStatusUpdate, 50, 0)
	if err != nil {
		t.Fatalf("list by kind: %v", err)
	}
	if len(statusUpdates) != 2 {
		t.Fatalf("expected 2 status updates, got %d", len(statusUpdates))
	}

	allMessages, err := coordStore.ListMessages(context.Background(), incidentNumber, 50, 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(allMessages) != 1 {
		t.Fatalf("expected 1 total message (chat only), got %d", len(allMessages))
	}

	if statusUpdates[0].Body != "Root cause found." {
		t.Fatalf("expected newest first, got %q", statusUpdates[0].Body)
	}
}

func TestCreateStatusUpdateRejectsInvalidLevel(t *testing.T) {
	client := newEntTestClient(t)
	coordStore := newPGIncidentCoordinationStore(client)

	actorID := uuid.New()
	_, err := coordStore.CreateStatusUpdate(context.Background(), 9999, "bogus", "body", false, actorID, "Alice")
	if err == nil {
		t.Fatal("expected error for invalid status_level")
	}
}

// TestDeleteIncidentPreservesCoordinationMessages verifies that soft-deleting
// an incident preserves the incident_coordination_messages rows attached to
// it (so the tombstone can still show historical coordination context). The
// old hard-delete cascade had to clean these up to avoid FK violations; under
// soft-delete the parent row is preserved and the FK constraint never fires.
func TestDeleteIncidentPreservesCoordinationMessages(t *testing.T) {
	client := newEntTestClient(t)
	incidentStore := newPGIncidentStore(client)
	coordStore := newPGIncidentCoordinationStore(client)

	incidentNumber := int64(6464)
	_, err := incidentStore.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          "Delete coordination test",
		Description:    "test incident",
		Status:         "active",
		Severity:       "high",
		ImpactLevel:    "medium",
		Priority:       "P2",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}

	actorID := uuid.New()
	if _, err := coordStore.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             IncidentCoordinationKindChat,
		ActorType:        IncidentCoordinationActorUser,
		ActorID:          &actorID,
		ActorDisplayName: "Ada",
		Body:             "Looking into it",
		Source:           IncidentCoordinationSourceAlga,
	}); err != nil {
		t.Fatalf("create coordination message: %v", err)
	}

	if err := incidentStore.DeleteIncident(context.Background(), incidentNumber); err != nil {
		t.Fatalf("delete incident with coordination messages: %v", err)
	}

	remaining, err := coordStore.ListMessages(context.Background(), incidentNumber, 50, 0)
	if err != nil {
		t.Fatalf("list coordination messages after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 preserved coordination message after soft-delete, got %d", len(remaining))
	}
}

func setupCoordinationStore(t *testing.T) (IncidentCoordinationStore, int64, func()) {
	t.Helper()
	client := newEntTestClient(t)
	incidentStore := newPGIncidentStore(client)
	coordStore := newPGIncidentCoordinationStore(client)

	incidentNumber := time.Now().UnixNano()
	_, err := incidentStore.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          "Newest query test",
		Description:    "test incident",
		Status:         "active",
		Severity:       "high",
		ImpactLevel:    "medium",
		Priority:       "P2",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	return coordStore, incidentNumber, func() {}
}

func mustCreateMessage(t *testing.T, s IncidentCoordinationStore, incidentNumber int64, kind string, createdAt time.Time) *IncidentCoordinationMessageRecord {
	t.Helper()
	rec, err := s.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber: incidentNumber,
		Kind:           kind,
		ActorType:      IncidentCoordinationActorAgent,
		Body:           "body",
		Source:         IncidentCoordinationSourceAgent,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	return rec
}

func TestNewestStatusUpdateReturnsMostRecent(t *testing.T) {
	coordStore, incidentID, cleanup := setupCoordinationStore(t)
	defer cleanup()

	mustCreateMessage(t, coordStore, incidentID, IncidentCoordinationKindStatusUpdate, time.Now().Add(-1*time.Hour))
	newer := mustCreateMessage(t, coordStore, incidentID, IncidentCoordinationKindStatusUpdate, time.Now())

	got, err := coordStore.NewestStatusUpdate(context.Background(), incidentID)
	if err != nil {
		t.Fatalf("NewestStatusUpdate: %v", err)
	}
	if got == nil || got.ID != newer.ID {
		t.Fatalf("NewestStatusUpdate = %+v, want newest %s", got, newer.ID)
	}
}

func TestNewestStatusUpdateReturnsNilWhenAbsent(t *testing.T) {
	coordStore, incidentID, cleanup := setupCoordinationStore(t)
	defer cleanup()

	got, err := coordStore.NewestStatusUpdate(context.Background(), incidentID)
	if err != nil {
		t.Fatalf("NewestStatusUpdate: %v", err)
	}
	if got != nil {
		t.Fatalf("NewestStatusUpdate = %+v, want nil", got)
	}
}

func TestNewestAgentCoordinationReplyReturnsLatestAgentReply(t *testing.T) {
	coordStore, incidentID, cleanup := setupCoordinationStore(t)
	defer cleanup()

	now := time.Now()
	mustCreate(t, coordStore, incidentID, IncidentCoordinationKindStatusUpdate, IncidentCoordinationActorSystem, now.Add(-30*time.Minute))
	mustCreate(t, coordStore, incidentID, IncidentCoordinationKindAgentReply, IncidentCoordinationActorUser, now.Add(-20*time.Minute))
	mustCreate(t, coordStore, incidentID, IncidentCoordinationKindAgentReply, IncidentCoordinationActorAgent, now.Add(-10*time.Minute))
	want := mustCreate(t, coordStore, incidentID, IncidentCoordinationKindAgentReply, IncidentCoordinationActorAgent, now)

	got, err := coordStore.NewestAgentCoordinationReply(context.Background(), incidentID)
	if err != nil {
		t.Fatalf("NewestAgentCoordinationReply: %v", err)
	}
	if got == nil || got.ID != want.ID {
		t.Fatalf("NewestAgentCoordinationReply = %+v, want %s", got, want.ID)
	}
}

func TestNewestAgentCoordinationReplySkipsUserReplies(t *testing.T) {
	coordStore, incidentID, cleanup := setupCoordinationStore(t)
	defer cleanup()

	now := time.Now()
	mustCreate(t, coordStore, incidentID, IncidentCoordinationKindAgentReply, IncidentCoordinationActorUser, now)
	olderAgent := mustCreate(t, coordStore, incidentID, IncidentCoordinationKindAgentReply, IncidentCoordinationActorAgent, now.Add(-15*time.Minute))
	mustCreate(t, coordStore, incidentID, IncidentCoordinationKindAgentReply, IncidentCoordinationActorUser, now.Add(time.Minute))

	got, err := coordStore.NewestAgentCoordinationReply(context.Background(), incidentID)
	if err != nil {
		t.Fatalf("NewestAgentCoordinationReply: %v", err)
	}
	if got == nil || got.ID != olderAgent.ID {
		t.Fatalf("NewestAgentCoordinationReply = %+v, want the older agent reply %s", got, olderAgent.ID)
	}
}

func TestNewestAgentCoordinationReplyNilWhenNoAgentActivity(t *testing.T) {
	coordStore, incidentID, cleanup := setupCoordinationStore(t)
	defer cleanup()

	mustCreate(t, coordStore, incidentID, IncidentCoordinationKindStatusUpdate, IncidentCoordinationActorSystem, time.Now())
	mustCreate(t, coordStore, incidentID, IncidentCoordinationKindAgentReply, IncidentCoordinationActorUser, time.Now())

	got, err := coordStore.NewestAgentCoordinationReply(context.Background(), incidentID)
	if err != nil {
		t.Fatalf("NewestAgentCoordinationReply: %v", err)
	}
	if got != nil {
		t.Fatalf("NewestAgentCoordinationReply = %+v, want nil", got)
	}
}

// mustCreate creates a coordination message with a configurable actor_type so
// tests can distinguish agent-authored replies from user/system ones (the
// older mustCreateMessageWithMeta helper hardcodes actor_type=agent).
func mustCreate(t *testing.T, s IncidentCoordinationStore, incidentNumber int64, kind, actorType string, createdAt time.Time) *IncidentCoordinationMessageRecord {
	t.Helper()
	rec, err := s.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             kind,
		ActorType:        actorType,
		ActorDisplayName: actorType,
		Body:             "body",
		Source:           IncidentCoordinationSourceAgent,
		Metadata:         map[string]any{},
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	return rec
}

func TestListMessagesExcludesSystemStatusUpdates(t *testing.T) {
	coordStore, incidentID, cleanup := setupCoordinationStore(t)
	defer cleanup()

	_, err := coordStore.CreateMessage(context.Background(), &IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentID,
		Kind:             IncidentCoordinationKindStatusUpdate,
		ActorType:        IncidentCoordinationActorSystem,
		ActorDisplayName: "System",
		Body:             "We're investigating this incident",
		Source:           IncidentCoordinationSourceSystem,
		Metadata:         map[string]any{"status_level": "investigating", "auto": true},
	})
	if err != nil {
		t.Fatalf("create system status update: %v", err)
	}

	mustCreateMessage(t, coordStore, incidentID, IncidentCoordinationKindChat, time.Now())
	mustCreateMessage(t, coordStore, incidentID, IncidentCoordinationKindStatusUpdate, time.Now())

	messages, err := coordStore.ListMessages(context.Background(), incidentID, 50, 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, m := range messages {
		if m.Kind == IncidentCoordinationKindStatusUpdate {
			t.Fatalf("ListMessages returned status update, but all status updates should be excluded: %+v", m)
		}
	}

	statusUpdates, err := coordStore.ListMessagesByKind(context.Background(), incidentID, IncidentCoordinationKindStatusUpdate, 50, 0)
	if err != nil {
		t.Fatalf("ListMessagesByKind: %v", err)
	}
	hasSystem := false
	for _, su := range statusUpdates {
		if su.ActorType == IncidentCoordinationActorSystem {
			hasSystem = true
			break
		}
	}
	if !hasSystem {
		t.Fatal("ListMessagesByKind should still return system status updates for the status card")
	}
}
