package webhook

import (
	"context"
	"sync"
	"testing"

	"alga/config"
	"alga/rabbitmq"
	"alga/routing"
	"alga/store"
	"alga/types"
)

// correlatorSpy records every ProcessAlert call; the embedded nil interface
// keeps the Correlator contract to just the one method.
type correlatorSpy struct {
	Correlator
	mu    sync.Mutex
	calls []rabbitmq.CorrelatedAlert
}

func (c *correlatorSpy) ProcessAlert(_ context.Context, alert rabbitmq.CorrelatedAlert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, alert)
	return nil
}

// firingStoreFake stubs the Store methods handleFiring and IngestManualAlert
// need; the embedded nil interface keeps every other method out of scope.
type firingStoreFake struct {
	store.Store
	nextNumber int64
	created    map[string]*store.AlertRecord
}

func (s *firingStoreFake) Create(record store.AlertRecord) (int64, error) {
	s.nextNumber++
	record.AlertNumber = s.nextNumber
	if s.created == nil {
		s.created = map[string]*store.AlertRecord{}
	}
	stored := record
	s.created[record.Fingerprint] = &stored
	return s.nextNumber, nil
}

func (s *firingStoreFake) GetByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	return s.created[fingerprint], nil
}

func (s *firingStoreFake) GetOpenByFingerprint(string) (*store.AlertRecord, error) {
	return nil, nil
}

func (s *firingStoreFake) UpdateDeliveryTargets(string, []store.DeliveryTarget) error {
	return nil
}

// silencedEngine returns a routing engine whose single rule silences alerts
// labelled team=noise; a plain engine routes everything to a channel.
func silencedEngine() (*routing.Engine, *routing.Engine) {
	silenced := routing.NewEngine([]config.RouteConfig{{
		Conditions: []config.RouteCondition{{Source: "label", Field: "team", Operator: "=", Value: "noise"}},
		Silenced:   true,
	}})
	loud := routing.NewEngine([]config.RouteConfig{{
		Conditions: []config.RouteCondition{{Source: "label", Field: "team", Operator: "=", Value: "noise"}},
		Targets:    []config.RouteTarget{{Provider: "mattermost", Channel: "ops"}},
	}})
	return silenced, loud
}

func noiseAlert() types.Alert {
	return types.Alert{
		Fingerprint: "fp-noise",
		Status:      "firing",
		Labels:      map[string]string{"team": "noise", "alertname": "TooLoud"},
	}
}

// TestHandleFiringSilencedSkipsCorrelator asserts silence means silence: a
// firing alert matched by a silenced routing rule is stored but never forwarded
// to the correlator, so no LLM investigation can be opened for it (WP-C6).
func TestHandleFiringSilencedSkipsCorrelator(t *testing.T) {
	engine, _ := silencedEngine()
	correlator := &correlatorSpy{}
	r := NewReceiver(engine, nil, nil, &firingStoreFake{}, nil, nil, false)
	r.SetCorrelator(correlator)

	r.handleFiring(context.Background(), noiseAlert(), nil)

	if len(correlator.calls) != 0 {
		t.Fatalf("silenced alert reached correlator %d time(s), want 0", len(correlator.calls))
	}
}

// TestHandleFiringNonSilencedStillCorrelates proves the WP-C6 change did not
// mute the normal path: non-silenced firing alerts must still reach the
// correlator with their assigned alert number.
func TestHandleFiringNonSilencedStillCorrelates(t *testing.T) {
	_, loud := silencedEngine()
	correlator := &correlatorSpy{}
	storeFake := &firingStoreFake{}
	r := NewReceiver(loud, nil, nil, storeFake, nil, nil, false)
	r.SetCorrelator(correlator)

	r.handleFiring(context.Background(), noiseAlert(), nil)

	if len(correlator.calls) != 1 {
		t.Fatalf("non-silenced alert correlator calls = %d, want 1", len(correlator.calls))
	}
	if correlator.calls[0].AlertNumber == 0 {
		t.Fatal("correlated alert missing store-assigned alert number")
	}
}

// TestIngestManualAlertSilencedSkipsCorrelator extends the suppression
// invariant to the manual ingestion path: a user-authored alert routed as
// silenced is stored without triggering correlation.
func TestIngestManualAlertSilencedSkipsCorrelator(t *testing.T) {
	engine, _ := silencedEngine()
	correlator := &correlatorSpy{}
	r := NewReceiver(engine, nil, nil, &firingStoreFake{}, nil, nil, false)
	r.SetCorrelator(correlator)

	alert := noiseAlert()
	alert.Fingerprint = "manual-test-1"
	if _, err := r.IngestManualAlert(context.Background(), alert, nil); err != nil {
		t.Fatalf("IngestManualAlert: %v", err)
	}

	if len(correlator.calls) != 0 {
		t.Fatalf("manual silenced alert reached correlator %d time(s), want 0", len(correlator.calls))
	}
}
