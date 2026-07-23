package worker

import (
	"testing"
	"time"

	"alga/rabbitmq"
	"alga/store"
)

func TestComputePriority(t *testing.T) {
	t.Parallel()
	now := time.Now()
	critical := store.AlertInvestigationRecord{
		Alerts:    []rabbitmq.CorrelatedAlert{{Labels: map[string]string{"severity": "critical"}}},
		CreatedAt: now,
	}
	warning := store.AlertInvestigationRecord{
		Alerts:    []rabbitmq.CorrelatedAlert{{Labels: map[string]string{"severity": "warning"}}},
		CreatedAt: now,
	}
	info := store.AlertInvestigationRecord{
		Alerts:    []rabbitmq.CorrelatedAlert{{Labels: map[string]string{"severity": "info"}}},
		CreatedAt: now,
	}
	cp := computePriority(critical)
	wp := computePriority(warning)
	ip := computePriority(info)
	if cp <= wp {
		t.Fatalf("critical priority %f should be > warning %f", cp, wp)
	}
	if wp <= ip {
		t.Fatalf("warning priority %f should be > info %f", wp, ip)
	}
}

func TestComputePriorityAging(t *testing.T) {
	t.Parallel()
	old := store.AlertInvestigationRecord{
		Alerts:    []rabbitmq.CorrelatedAlert{{Labels: map[string]string{"severity": "info"}}},
		CreatedAt: time.Now().Add(-30 * time.Minute),
	}
	recent := store.AlertInvestigationRecord{
		Alerts:    []rabbitmq.CorrelatedAlert{{Labels: map[string]string{"severity": "info"}}},
		CreatedAt: time.Now(),
	}
	op := computePriority(old)
	rp := computePriority(recent)
	if op <= rp {
		t.Fatalf("older investigation should have higher priority: old=%f recent=%f", op, rp)
	}
}

func TestExponentialBackoff(t *testing.T) {
	t.Parallel()
	cases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
		{4, 240 * time.Second},
		{5, 480 * time.Second},
		{6, 480 * time.Second},
		{10, 480 * time.Second},
	}
	for _, tc := range cases {
		got := backoffDuration(tc.attempt)
		if got != tc.expected {
			t.Errorf("attempt %d: got %v, want %v", tc.attempt, got, tc.expected)
		}
	}
}
