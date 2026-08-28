package webhook

import (
	"context"
	"strconv"
	"testing"

	"alga/store"
)

type ackCancelStateFake struct {
	hSetFields map[string]string
	zRemCalls  []string
}

func (f *ackCancelStateFake) HSet(_ context.Context, key, field, value string) error {
	if f.hSetFields == nil {
		f.hSetFields = map[string]string{}
	}
	f.hSetFields[key+"#"+field] = value
	return nil
}

func (f *ackCancelStateFake) ZRem(_ context.Context, key, member string) error {
	f.zRemCalls = append(f.zRemCalls, key+"#"+member)
	return nil
}

type ackCancelTimelineFake struct {
	entries []store.IncidentTimelineEntryRecord
}

func (f *ackCancelTimelineFake) AddTimelineEntry(_ context.Context, record *store.IncidentTimelineEntryRecord) error {
	f.entries = append(f.entries, *record)
	return nil
}

// ackCancelAlertStore stubs the two Store methods cancellation uses; the
// embedded nil interface keeps every other method out of the test's concern.
type ackCancelAlertStore struct {
	store.Store
	record    *store.AlertRecord
	incidents []store.IncidentRecord
}

func (s *ackCancelAlertStore) GetByFingerprint(string) (*store.AlertRecord, error) {
	return s.record, nil
}

func (s *ackCancelAlertStore) GetIncidentsByAlertNumber(context.Context, int64) ([]store.IncidentRecord, error) {
	return s.incidents, nil
}

// TestSlackAckCancelsPendingEscalations verifies that a Slack acknowledge on an
// alert cancels pending escalations for its linked non-terminal incidents — the
// contract documented in docs/on-call/escalation-policies.md ("via the UI,
// Slack, or the API").
func TestSlackAckCancelsPendingEscalations(t *testing.T) {
	vk := &ackCancelStateFake{}
	timeline := &ackCancelTimelineFake{}
	h := &SlackWebhookHandler{
		alertStore: &ackCancelAlertStore{
			record: &store.AlertRecord{Fingerprint: "fp-1", AlertNumber: 7},
			incidents: []store.IncidentRecord{
				{IncidentNumber: 101, Status: "active"},
				{IncidentNumber: 102, Status: "resolved"}, // terminal: skipped
				{IncidentNumber: 103, Status: "mitigated"},
			},
		},
		vkClient:           vk,
		escalationTimeline: timeline,
	}

	h.cancelPendingEscalationsCtx(context.Background(), "fp-1", "alert acknowledged via Slack")

	for _, incidentNumber := range []int64{101, 103} {
		id := "alga:esc:" + strconv.FormatInt(incidentNumber, 10)
		if vk.hSetFields[id+"#acknowledged"] != "1" {
			t.Errorf("incident %d: escalation hash not marked acknowledged", incidentNumber)
		}
		wantRem := "alga:esc:pending#" + strconv.FormatInt(incidentNumber, 10)
		found := false
		for _, call := range vk.zRemCalls {
			if call == wantRem {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("incident %d: not removed from pending set (calls: %v)", incidentNumber, vk.zRemCalls)
		}
	}
	if _, ok := vk.hSetFields["alga:esc:102#acknowledged"]; ok {
		t.Error("terminal incident 102 must be skipped")
	}
	if len(timeline.entries) != 2 {
		t.Fatalf("timeline entries = %d, want 2", len(timeline.entries))
	}
	for _, entry := range timeline.entries {
		if entry.EventType != "escalation_cancelled" || entry.Message != "Escalation stopped — alert acknowledged via Slack" {
			t.Errorf("unexpected timeline entry: %+v", entry)
		}
	}
}

// TestSlackAckCancelWithoutLinkedIncidents verifies alert-only acknowledgements
// (no linked incidents) stay a clean no-op instead of erroring.
func TestSlackAckCancelWithoutLinkedIncidents(t *testing.T) {
	vk := &ackCancelStateFake{}
	h := &SlackWebhookHandler{
		alertStore: &ackCancelAlertStore{
			record:    &store.AlertRecord{Fingerprint: "fp-2", AlertNumber: 9},
			incidents: nil,
		},
		vkClient: vk,
	}

	h.cancelPendingEscalationsCtx(context.Background(), "fp-2", "reason")

	if len(vk.zRemCalls) != 0 || len(vk.hSetFields) != 0 {
		t.Errorf("expected no state changes, got HSet=%v ZRem=%v", vk.hSetFields, vk.zRemCalls)
	}
}

// TestSlackAckCancelNilValkeyIsNoOp guards the deployment case where Slack is
// configured but Valkey is not: the response must still succeed and nothing
// may panic.
func TestSlackAckCancelNilValkeyIsNoOp(t *testing.T) {
	timeline := &ackCancelTimelineFake{}
	h := &SlackWebhookHandler{
		alertStore: &ackCancelAlertStore{
			record: &store.AlertRecord{Fingerprint: "fp-3", AlertNumber: 11},
			incidents: []store.IncidentRecord{
				{IncidentNumber: 201, Status: "active"},
			},
		},
		escalationTimeline: timeline,
	}
	h.cancelPendingEscalationsCtx(context.Background(), "fp-3", "reason")
	if len(timeline.entries) != 0 {
		t.Errorf("nil Valkey must skip cancellation entirely; got %d entries", len(timeline.entries))
	}
}
