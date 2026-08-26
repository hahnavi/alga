package worker

import (
	"context"
	"sync"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/ics"
	"alga/sse"
)

type stubICSIncidentStore struct {
	err error
}

func (s *stubICSIncidentStore) GetIncident(ctx context.Context, incidentNumber int64) (*ics.IncidentRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &ics.IncidentRecord{IncidentNumber: incidentNumber}, nil
}

func (s *stubICSIncidentStore) AddTimelineEntry(ctx context.Context, entry *ics.TimelineEntry) error {
	return nil
}

func (s *stubICSIncidentStore) SetWarRoomMeet(ctx context.Context, incidentNumber int64, spaceName, conferenceURL string) error {
	return nil
}

type capturingSSEPublisher struct {
	mu     sync.Mutex
	events []sse.Event
}

func (p *capturingSSEPublisher) Publish(ev sse.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *capturingSSEPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

// stubAcknowledger records how the worker settled each delivery so the tests
// can assert ack/nack routing without a broker.
type stubAcknowledger struct {
	mu     sync.Mutex
	acked  int
	nacked int
}

func (a *stubAcknowledger) Ack(tag uint64, multiple bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acked++
	return nil
}

func (a *stubAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nacked++
	return nil
}

func (a *stubAcknowledger) Reject(tag uint64, requeue bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nacked++
	return nil
}

func (a *stubAcknowledger) settled() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.acked, a.nacked
}

func newTestProvisionDelivery(t *testing.T, body []byte) (amqp.Delivery, *stubAcknowledger) {
	t.Helper()
	d := amqp.Delivery{Body: body}
	ack := &stubAcknowledger{}
	d.Acknowledger = ack
	return d, ack
}

func newTestICSWorker(storeErr error, capture *capturingSSEPublisher) *ICSWorker {
	store := &stubICSIncidentStore{err: storeErr}
	w := NewICSWorker(ics.NewWarRoomProvisioner(store, nil, nil, nil), nil)
	w.SetSSEPublisher(capture)
	return w
}

// TestICSWorkerPublishesWarRoomCreated verifies a successful provisioning
// delivery announces war_room_created exactly once and acks.
func TestICSWorkerPublishesWarRoomCreated(t *testing.T) {
	capture := &capturingSSEPublisher{}
	w := newTestICSWorker(nil, capture)

	d, ack := newTestProvisionDelivery(t, []byte(`{"incident_number":42,"retry_count":0}`))
	w.Handle(context.Background(), d)

	if got := capture.count(); got != 1 {
		t.Fatalf("expected exactly 1 war_room_created event, got %d", got)
	}
	if ev := capture.events[0]; ev.Type != "war_room_created" {
		t.Fatalf("unexpected event type %q", ev.Type)
	} else if data, ok := ev.Data.(map[string]any); !ok || data["incident_number"] != int64(42) {
		t.Fatalf("unexpected event payload %#v", ev.Data)
	}
	if acked, nacked := ack.settled(); acked != 1 || nacked != 0 {
		t.Fatalf("expected delivery acked once, got acked=%d nacked=%d", acked, nacked)
	}
}

// TestICSWorkerFailedProvisioningNoEvent verifies the failure path does not
// announce war_room_created.
func TestICSWorkerFailedProvisioningNoEvent(t *testing.T) {
	capture := &capturingSSEPublisher{}
	w := newTestICSWorker(context.DeadlineExceeded, capture)

	d, ack := newTestProvisionDelivery(t, []byte(`{"incident_number":42,"retry_count":0}`))
	w.Handle(context.Background(), d)

	if got := capture.count(); got != 0 {
		t.Fatalf("expected 0 events on failed provisioning, got %d", got)
	}
	if acked, nacked := ack.settled(); acked != 0 || nacked != 1 {
		t.Fatalf("expected failed delivery nacked, got acked=%d nacked=%d", acked, nacked)
	}
}

// TestICSWorkerMalformedMessageNoEvent verifies malformed bodies are dropped
// without announcing anything.
func TestICSWorkerMalformedMessageNoEvent(t *testing.T) {
	capture := &capturingSSEPublisher{}
	w := newTestICSWorker(nil, capture)

	d, ack := newTestProvisionDelivery(t, []byte("{invalid"))
	w.Handle(context.Background(), d)

	if got := capture.count(); got != 0 {
		t.Fatalf("expected 0 events on malformed message, got %d", got)
	}
	if acked, nacked := ack.settled(); acked != 0 || nacked != 1 {
		t.Fatalf("expected malformed delivery nacked, got acked=%d nacked=%d", acked, nacked)
	}
}
