package escalation

import (
	"context"
	"testing"

	"alga/store"
)

type fakeStateClient struct {
	hSetFields map[string]string
	zRemKeys   []string
}

func (f *fakeStateClient) HSet(_ context.Context, key, field, value string) error {
	if f.hSetFields == nil {
		f.hSetFields = map[string]string{}
	}
	f.hSetFields[key+"#"+field] = value
	return nil
}

func (f *fakeStateClient) ZRem(_ context.Context, key, member string) error {
	f.zRemKeys = append(f.zRemKeys, key+"#"+member)
	return nil
}

type fakeTimeline struct {
	entries []store.IncidentTimelineEntryRecord
}

func (f *fakeTimeline) AddTimelineEntry(_ context.Context, record *store.IncidentTimelineEntryRecord) error {
	f.entries = append(f.entries, *record)
	return nil
}

// TestCancelForIncident pins the shared cancellation contract used by the API
// acknowledge path and the Slack interactive button: mark the state hash as
// acknowledged, remove the incident from the pending sweep set, and record a
// system timeline entry.
func TestCancelForIncident(t *testing.T) {
	vk := &fakeStateClient{}
	timeline := &fakeTimeline{}

	CancelForIncident(context.Background(), vk, timeline, "42", "alert acknowledged via Slack")

	if got := vk.hSetFields[StateHashPrefix+"42#acknowledged"]; got != "1" {
		t.Errorf("acknowledged flag = %q, want \"1\" (fields: %v)", got, vk.hSetFields)
	}
	wantRem := PendingSetKey + "#42"
	if len(vk.zRemKeys) != 1 || vk.zRemKeys[0] != wantRem {
		t.Errorf("ZRem calls = %v, want [%q]", vk.zRemKeys, wantRem)
	}
	if len(timeline.entries) != 1 {
		t.Fatalf("timeline entries = %d, want 1", len(timeline.entries))
	}
	entry := timeline.entries[0]
	if entry.IncidentNumber != 42 || entry.EventType != "escalation_cancelled" || entry.ActorType != "system" {
		t.Errorf("unexpected timeline entry: %+v", entry)
	}
	wantMsg := "Escalation stopped — alert acknowledged via Slack"
	if entry.Message != wantMsg {
		t.Errorf("timeline message = %q, want %q", entry.Message, wantMsg)
	}
}

// TestCancelForIncident_MalformedIncidentID verifies a non-numeric incident id
// still flips the Valkey state but writes no timeline entry instead of failing.
func TestCancelForIncident_MalformedIncidentID(t *testing.T) {
	vk := &fakeStateClient{}
	timeline := &fakeTimeline{}

	CancelForIncident(context.Background(), vk, timeline, "not-a-number", "reason")

	if _, ok := vk.hSetFields[StateHashPrefix+"not-a-number#acknowledged"]; !ok {
		t.Error("state hash not updated despite valid client")
	}
	if len(timeline.entries) != 0 {
		t.Errorf("timeline entries = %d, want 0 for malformed incident id", len(timeline.entries))
	}
}

// TestCancelForIncident_NilClient verifies the helper is a no-op — never a
// panic — when Valkey is unavailable.
func TestCancelForIncident_NilClient(t *testing.T) {
	timeline := &fakeTimeline{}
	CancelForIncident(context.Background(), nil, timeline, "1", "reason")
	if len(timeline.entries) != 0 {
		t.Fatalf("nil Valkey client must skip cancellation entirely")
	}
}

func TestIsTerminalIncidentStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"resolved", "closed", "cancelled"} {
		if !IsTerminalIncidentStatus(status) {
			t.Errorf("IsTerminalIncidentStatus(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"detected", "triaging", "active", "mitigated", ""} {
		if IsTerminalIncidentStatus(status) {
			t.Errorf("IsTerminalIncidentStatus(%q) = true, want false", status)
		}
	}
}
