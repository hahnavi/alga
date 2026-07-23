package rabbitmq

import (
	"testing"
	"time"
)

func TestRetryScheduleIsAuthoritative(t *testing.T) {
	t.Parallel()
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	if len(RetrySchedule) != len(want) {
		t.Fatalf("RetrySchedule len = %d, want %d", len(RetrySchedule), len(want))
	}
	for i, w := range want {
		if RetrySchedule[i] != w {
			t.Fatalf("RetrySchedule[%d] = %s, want %s", i, RetrySchedule[i], w)
		}
	}
}

func TestJitterStaysWithinBounds(t *testing.T) {
	t.Parallel()
	const samples = 1000
	for _, base := range RetrySchedule {
		lo := time.Duration(float64(base) * (1 - jitterFactor))
		hi := time.Duration(float64(base) * (1 + jitterFactor))
		for range samples {
			got := withJitter(base)
			if got < lo || got > hi {
				t.Fatalf("withJitter(%s) = %s, want within [%s, %s]", base, got, lo, hi)
			}
		}
	}
}

func TestJitterZeroIsNoOp(t *testing.T) {
	t.Parallel()
	if got := withJitter(0); got != 0 {
		t.Fatalf("withJitter(0) = %s, want 0", got)
	}
}

func TestJitterCeilBoundsBackoff(t *testing.T) {
	t.Parallel()
	for _, base := range RetrySchedule {
		if got := jitterCeil(base); got != withJitter(base) && got < withJitter(base) {
			t.Fatalf("jitterCeil(%s) = %s must be >= max jittered value", base, got)
		}
	}
}

func TestRetryExpirationMatchesSchedule(t *testing.T) {
	t.Parallel()
	for i := range RetrySchedule {
		attempt := i + 1
		exp, ok := retryExpiration(attempt)
		if !ok {
			t.Fatalf("retryExpiration(%d) ok = false, want true", attempt)
		}
		lo := time.Duration(float64(RetrySchedule[i]) * (1 - jitterFactor))
		hi := time.Duration(float64(RetrySchedule[i]) * (1 + jitterFactor))
		if exp < lo || exp > hi {
			t.Fatalf("retryExpiration(%d) = %s, want within [%s, %s]", attempt, exp, lo, hi)
		}
	}
}

func TestRetryExpirationRejectsOutOfRange(t *testing.T) {
	t.Parallel()
	if _, ok := retryExpiration(0); ok {
		t.Fatal("retryExpiration(0) ok = true, want false")
	}
	if _, ok := retryExpiration(len(RetrySchedule) + 1); ok {
		t.Fatalf("retryExpiration(%d) ok = true, want false", len(RetrySchedule)+1)
	}
}

func TestRetryQueuesCoverAllStages(t *testing.T) {
	t.Parallel()
	cases := map[string]map[int]string{
		"alert":                alertRetryQueues,
		"investigate":          investigateRetryQueues,
		"triage":               triageRetryQueues,
		"incident":             incidentRetryQueues,
		"escalation":           escalationRetryQueues,
		"notificationDispatch": notificationDispatchRetryQueues,
	}
	for name, queues := range cases {
		if len(queues) != len(RetrySchedule) {
			t.Fatalf("%s retry queues = %d, want %d (one per stage)", name, len(queues), len(RetrySchedule))
		}
		for stage := range RetrySchedule {
			attempt := stage + 1
			if _, ok := queues[attempt]; !ok {
				t.Fatalf("%s missing retry queue for attempt %d", name, attempt)
			}
		}
	}
}

func TestRetryCapsMatchScheduleLength(t *testing.T) {
	t.Parallel()
	caps := map[string]int{
		"alert":                MaxAlertRetries,
		"investigate":          MaxInvestigateRetries,
		"triage":               MaxTriageRetries,
		"incident":             MaxIncidentRetries,
		"escalation":           MaxEscalationRetries,
		"notificationDispatch": MaxNotificationDispatchRetries,
	}
	for name, cap := range caps {
		if cap != len(RetrySchedule) {
			t.Fatalf("%s max retries = %d, want %d", name, cap, len(RetrySchedule))
		}
	}
}
