package webhook

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"alga/store"
)

type fakeIncidentLookupStore struct {
	incident *store.IncidentRecord
}

func (f *fakeIncidentLookupStore) GetIncidentBySlackChannel(_ context.Context, channelID string) (*store.IncidentRecord, error) {
	if f.incident != nil && f.incident.SlackChannelID == channelID {
		return f.incident, nil
	}
	return nil, nil
}

type fakeCoordinationStore struct {
	created []store.IncidentCoordinationMessageRecord
	seen    map[string]*store.IncidentCoordinationMessageRecord
}

func (f *fakeCoordinationStore) CreateMessage(_ context.Context, record *store.IncidentCoordinationMessageRecord) (*store.IncidentCoordinationMessageRecord, error) {
	f.created = append(f.created, *record)
	return record, nil
}

func (f *fakeCoordinationStore) ListMessages(_ context.Context, _ int64, _ int, _ int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordinationStore) FindByProviderMessageID(_ context.Context, providerID string) (*store.IncidentCoordinationMessageRecord, error) {
	if f.seen == nil {
		return nil, nil
	}
	return f.seen[providerID], nil
}

func (f *fakeCoordinationStore) SetSlackMessageTS(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}

func (f *fakeCoordinationStore) ListMessagesByKind(_ context.Context, _ int64, _ string, _ int, _ int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordinationStore) NewestStatusUpdate(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordinationStore) NewestAgentCoordinationReply(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordinationStore) CreateStatusUpdate(_ context.Context, incidentNumber int64, statusLevel string, body string, internal bool, actorID uuid.UUID, actorDisplayName string) (*store.IncidentCoordinationMessageRecord, error) {
	return f.CreateMessage(context.Background(), &store.IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             store.IncidentCoordinationKindStatusUpdate,
		ActorType:        store.IncidentCoordinationActorUser,
		ActorID:          &actorID,
		ActorDisplayName: actorDisplayName,
		Body:             body,
		Internal:         internal,
		Source:           store.IncidentCoordinationSourceAlga,
		Metadata:         map[string]any{"status_level": statusLevel},
	})
}

func TestSlackWebhookCreatesIncidentCoordinationMessage(t *testing.T) {
	coord := &fakeCoordinationStore{}
	h := NewSlackWebhookHandler(nil, nil, "secret")
	h.SetIncidentCoordinationStore(coord)
	h.SetIncidentLookupStore(&fakeIncidentLookupStore{incident: &store.IncidentRecord{IncidentNumber: 1, SlackChannelID: "C123"}})

	ev := slackMessageEvent{Type: "message", User: "U123", Text: "IC update from Slack", Channel: "C123", TS: "1716400000.000100"}
	if handled := h.handleIncidentCoordinationSlackMessage(context.Background(), ev); !handled {
		t.Fatal("expected incident coordination message to be handled")
	}
	if len(coord.created) != 1 {
		t.Fatalf("expected 1 created message, got %d", len(coord.created))
	}
	msg := coord.created[0]
	if msg.IncidentNumber != 1 || msg.Body != "IC update from Slack" || msg.Source != store.IncidentCoordinationSourceSlack {
		t.Fatalf("unexpected message: %#v", msg)
	}
}

func TestSlackIncidentCoordinationDedup(t *testing.T) {
	providerID := store.SlackProviderMessageID("C123", "1716400000.000100")
	coord := &fakeCoordinationStore{
		seen: map[string]*store.IncidentCoordinationMessageRecord{
			providerID: {IncidentNumber: 1},
		},
	}
	h := NewSlackWebhookHandler(nil, nil, "secret")
	h.SetIncidentCoordinationStore(coord)
	h.SetIncidentLookupStore(&fakeIncidentLookupStore{incident: &store.IncidentRecord{IncidentNumber: 1, SlackChannelID: "C123"}})

	ev := slackMessageEvent{Type: "message", User: "U123", Text: "duplicate", Channel: "C123", TS: "1716400000.000100"}
	if handled := h.handleIncidentCoordinationSlackMessage(context.Background(), ev); !handled {
		t.Fatal("expected dedup to return handled=true")
	}
	if len(coord.created) != 0 {
		t.Fatalf("expected 0 created messages on dedup, got %d", len(coord.created))
	}
}

func TestSlackIncidentCoordinationNoMatch(t *testing.T) {
	coord := &fakeCoordinationStore{}
	h := NewSlackWebhookHandler(nil, nil, "secret")
	h.SetIncidentCoordinationStore(coord)
	h.SetIncidentLookupStore(&fakeIncidentLookupStore{incident: &store.IncidentRecord{IncidentNumber: 1, SlackChannelID: "C_OTHER"}})

	ev := slackMessageEvent{Type: "message", User: "U123", Text: "no match", Channel: "C999", TS: "1716400000.000200"}
	if handled := h.handleIncidentCoordinationSlackMessage(context.Background(), ev); handled {
		t.Fatal("expected no match to return handled=false")
	}
	if len(coord.created) != 0 {
		t.Fatalf("expected 0 created messages on no match, got %d", len(coord.created))
	}
}

func TestSlackIncidentCoordinationNilStores(t *testing.T) {
	h := NewSlackWebhookHandler(nil, nil, "secret")
	ev := slackMessageEvent{Type: "message", User: "U123", Text: "hello", Channel: "C123", TS: "1716400000.000100"}
	if handled := h.handleIncidentCoordinationSlackMessage(context.Background(), ev); handled {
		t.Fatal("expected nil stores to return handled=false")
	}
}

func TestSlackIncidentCoordinationThreadTSEqualsTS(t *testing.T) {
	coord := &fakeCoordinationStore{}
	h := NewSlackWebhookHandler(nil, nil, "secret")
	h.SetIncidentCoordinationStore(coord)
	h.SetIncidentLookupStore(&fakeIncidentLookupStore{incident: &store.IncidentRecord{IncidentNumber: 2, SlackChannelID: "C456"}})

	ev := slackMessageEvent{Type: "message", User: "U456", Text: "top-level message", Channel: "C456", TS: "1716400000.000300"}
	if handled := h.handleIncidentCoordinationSlackMessage(context.Background(), ev); !handled {
		t.Fatal("expected handled")
	}
	if len(coord.created) != 1 {
		t.Fatalf("expected 1 message, got %d", len(coord.created))
	}
	msg := coord.created[0]
	if msg.SlackThreadTS != ev.TS {
		t.Fatalf("expected SlackThreadTS to equal TS when no ThreadTS, got %q", msg.SlackThreadTS)
	}
}
