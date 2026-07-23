package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
)

type fwdMockAgentTokenStore struct {
	store.AgentTokenStore
}

type fwdMockAlertInvestigationStore struct {
	store.AlertInvestigationStore
	inv *store.AlertInvestigationRecord
}

func (m *fwdMockAlertInvestigationStore) GetAlertInvestigation(ctx context.Context, id string) (*store.AlertInvestigationRecord, error) {
	return m.inv, nil
}

type fwdMockIncidentStore struct {
	store.IncidentStore
	inc *store.IncidentRecord
}

func (m *fwdMockIncidentStore) GetIncidentByID(ctx context.Context, id uuid.UUID) (*store.IncidentRecord, error) {
	if m.inc != nil && m.inc.ID == id {
		return m.inc, nil
	}
	return nil, fmt.Errorf("not found")
}

func TestInvestigationForwarderChatIDResolution(t *testing.T) {
	agentID := uuid.New()
	incidentUUID := uuid.New()

	t.Run("promoted alert investigation with incident resolved", func(t *testing.T) {
		invStore := &fwdMockAlertInvestigationStore{
			inv: &store.AlertInvestigationRecord{
				AlertInvestigationID: "AINV-1",
				PromotedIncidentID:   &incidentUUID,
			},
		}
		incStore := &fwdMockIncidentStore{
			inc: &store.IncidentRecord{
				ID:             incidentUUID,
				IncidentNumber: 42,
			},
		}
		broker := sse.NewBroker()
		ch := make(chan sse.Event, 1)
		broker.SubscribeAgent(agentID.String(), "client-1", ch)

		f := &DefaultInvestigationForwarder{
			AgentTokens:             &fwdMockAgentTokenStore{},
			AgentSSE:                agent.NewAgentSSEHandler(broker, nil, nil, nil, nil),
			AlertInvestigationStore: invStore,
			IncidentStore:           incStore,
		}

		err := f.ForwardToAgent(agentID.String(), "AINV-1", "sender-1", "Sender", "hello")
		if err != nil {
			t.Fatalf("ForwardToAgent failed: %v", err)
		}

		select {
		case ev := <-ch:
			data, ok := ev.Data.(map[string]any)
			if !ok {
				t.Fatalf("expected map[string]any, got %T", ev.Data)
			}
			if chatID := data["chat_id"]; chatID != "incident_inv_42" {
				t.Errorf("expected chat_id incident_inv_42, got %q", chatID)
			}
		default:
			t.Fatal("expected event, got none")
		}
	})

	t.Run("alert investigation with no incident, resolving alert number", func(t *testing.T) {
		invStore := &fwdMockAlertInvestigationStore{
			inv: &store.AlertInvestigationRecord{
				AlertInvestigationID: "AINV-2",
				Alerts: []rabbitmq.CorrelatedAlert{
					{AlertNumber: 101},
				},
			},
		}
		broker := sse.NewBroker()
		ch := make(chan sse.Event, 1)
		broker.SubscribeAgent(agentID.String(), "client-2", ch)

		f := &DefaultInvestigationForwarder{
			AgentTokens:             &fwdMockAgentTokenStore{},
			AgentSSE:                agent.NewAgentSSEHandler(broker, nil, nil, nil, nil),
			AlertInvestigationStore: invStore,
		}

		err := f.ForwardToAgent(agentID.String(), "AINV-2", "sender-1", "Sender", "hello")
		if err != nil {
			t.Fatalf("ForwardToAgent failed: %v", err)
		}

		select {
		case ev := <-ch:
			data, ok := ev.Data.(map[string]any)
			if !ok {
				t.Fatalf("expected map[string]any, got %T", ev.Data)
			}
			if chatID := data["chat_id"]; chatID != "alert_101" {
				t.Errorf("expected chat_id alert_101, got %q", chatID)
			}
		default:
			t.Fatal("expected event, got none")
		}
	})
}
