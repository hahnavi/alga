package worker

import (
	"testing"

	"alga/rabbitmq"
)

// TestEscalationRetryBudget pins the MaxRetries clamping contract: a
// publisher may lower the retry budget per message (down to 0 retries) but
// never exceed the wired retry-ladder depth; unset or negative values fall
// back to the constant.
func TestEscalationRetryBudget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		max  int
		want int
	}{
		{"constant passthrough", rabbitmq.MaxEscalationRetries, rabbitmq.MaxEscalationRetries},
		{"lower budget honored", 2, 2},
		{"one retry honored", 1, 1},
		{"zero falls back", 0, rabbitmq.MaxEscalationRetries},
		{"negative falls back", -3, rabbitmq.MaxEscalationRetries},
		{"above ladder clamped", rabbitmq.MaxEscalationRetries + 7, rabbitmq.MaxEscalationRetries},
	}
	for _, tc := range cases {
		if got := escalationRetryBudget(tc.max); got != tc.want {
			t.Errorf("%s: escalationRetryBudget(%d) = %d, want %d", tc.name, tc.max, got, tc.want)
		}
	}
}
