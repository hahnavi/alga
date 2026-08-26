package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"alga/rabbitmq"
)

type fakeSLASweepPublisher struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (p *fakeSLASweepPublisher) PublishSLASweep(ctx context.Context, msg rabbitmq.SLASweepMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.err
}

func (p *fakeSLASweepPublisher) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// TestSLASweepTickPublishesRequest verifies each tick publishes one sweep
// request and that publish failures do not panic the loop.
func TestSLASweepTickPublishesRequest(t *testing.T) {
	s := NewInvestigationScheduler(nil, nil, nil, 1)
	pub := &fakeSLASweepPublisher{}
	s.SetSLAPublisher(pub)
	s.SetSLASweepInterval(time.Minute)

	s.slaSweepTick(context.Background())
	s.slaSweepTick(context.Background())

	if got := pub.callCount(); got != 2 {
		t.Fatalf("expected 2 published ticks, got %d", got)
	}
}

func TestSLASweepTickContinuesAfterPublishError(t *testing.T) {
	s := NewInvestigationScheduler(nil, nil, nil, 1)
	pub := &fakeSLASweepPublisher{err: context.DeadlineExceeded}
	s.SetSLAPublisher(pub)
	s.SetSLASweepInterval(time.Minute)

	s.slaSweepTick(context.Background())
	if got := pub.callCount(); got != 1 {
		t.Fatalf("expected 1 attempted tick after error, got %d", got)
	}
}

func TestSLASweepIntervalSetterSemantics(t *testing.T) {
	s := NewInvestigationScheduler(nil, nil, nil, 1)

	// Zero/negative disables publication (gate in Start stays closed).
	s.SetSLASweepInterval(0)
	s.SetSLASweepInterval(-time.Minute)
	if s.slaSweepInterval != 0 {
		t.Fatalf("expected disabled interval to stay 0, got %v", s.slaSweepInterval)
	}

	// Positive intervals below 5s clamp to 5s to avoid queue churn.
	s = NewInvestigationScheduler(nil, nil, nil, 1)
	s.SetSLASweepInterval(250 * time.Millisecond)
	if s.slaSweepInterval != 5*time.Second {
		t.Fatalf("expected sub-5s interval clamped to 5s, got %v", s.slaSweepInterval)
	}

	// A configured interval is honored as-is.
	s.SetSLASweepInterval(90 * time.Second)
	if s.slaSweepInterval != 90*time.Second {
		t.Fatalf("expected 90s interval preserved, got %v", s.slaSweepInterval)
	}
}

// TestSLASweepRunLoop exercises the real goroutine loop at a short interval
// by setting the field directly (bypassing the setter's 5s floor for tests).
func TestSLASweepRunLoop(t *testing.T) {
	s := NewInvestigationScheduler(nil, nil, nil, 1)
	pub := &fakeSLASweepPublisher{}
	s.SetSLAPublisher(pub)
	s.slaSweepInterval = 15 * time.Millisecond

	s.Start()
	time.Sleep(100 * time.Millisecond)
	s.Stop()

	if got := pub.callCount(); got < 2 {
		t.Fatalf("expected multiple ticks from run loop, got %d", got)
	}
}
