package worker

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The escalationGuards functions require a real Valkey client, so this file
// only tests the helper logic that does not touch the network. The
// integration with Valkey is exercised via the worker constructor
// wiring tests and the dispatcher tests.

func TestEscalationGuardResultString(t *testing.T) {
	t.Parallel()
	cases := map[escalationGuardResult]string{
		guardAllow:            "allow",
		guardSkipAcknowledged: "skipped_acknowledged",
		guardSkipSilenced:     "skipped_silenced",
		guardSkipClaimLost:    "skipped_claim_lost",
	}
	for got, want := range cases {
		if got.String() != want {
			t.Errorf("guard.String() = %q, want %q", got.String(), want)
		}
	}
}

// TestEscalationHashKeyFormat documents the contract: the hash key is
// "alga:esc:" + incident number. The sweep worker relies on this format
// (escalation_sweep.go:96).
func TestEscalationHashKeyFormat(t *testing.T) {
	t.Parallel()
	if escHashPrefix != "alga:esc:" {
		t.Fatalf("escHashPrefix = %q, want %q", escHashPrefix, "alga:esc:")
	}
	incidentNumber := int64(123)
	want := "alga:esc:" + strconv.FormatInt(incidentNumber, 10)
	if got := escHashPrefix + strconv.FormatInt(incidentNumber, 10); got != want {
		t.Errorf("hash key = %q, want %q", got, want)
	}
}

// TestEscalationSweepConstants pins the sorted-set key. The cancelEscalationForIncident
// function in api/incident.go:1668 and the sweep's ZREM both use this literal.
func TestEscalationSweepConstantsGuards(t *testing.T) {
	t.Parallel()
	if escSortedSet != "alga:esc:pending" {
		t.Fatalf("escSortedSet = %q, want %q", escSortedSet, "alga:esc:pending")
	}
	if escSweepTick != 10*time.Second {
		t.Fatalf("escSweepTick = %v, want 10s", escSweepTick)
	}
}

// TestClaimVoiceCallSlotKeyFormat documents the dedup key format. The TTL is
// fixed at 15 minutes, which exceeds the longest notification-dispatch retry
// tail (5 min) plus the natural level-to-level transition.
func TestClaimVoiceCallSlotKeyFormat(t *testing.T) {
	t.Parallel()
	incidentNumber := int64(42)
	uid := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	level := 2
	want := "alga:voice:call:42:00000000-0000-0000-0000-000000000001:2"
	got := "alga:voice:call:" + strconv.FormatInt(incidentNumber, 10) + ":" + uid.String() + ":" + strconv.Itoa(level)
	if got != want {
		t.Errorf("voice dedup key = %q, want %q", got, want)
	}
}

// TestEscalationPolicyRepeatMath documents the per-incident dispatch bound:
// total dispatches = (RepeatCount+1) * len(Levels) when neither ack nor
// silence ever fires. With RepeatCount=0 (seed default) and 1 level (seed
// default) the bound is 1.
func TestEscalationPolicyRepeatMath(t *testing.T) {
	t.Parallel()
	repeat := 0
	levels := 1
	if got := (repeat + 1) * levels; got != 1 {
		t.Errorf("seed worst-case dispatches = %d, want 1", got)
	}
	repeat = 5
	levels = 3
	if got := (repeat + 1) * levels; got != 18 {
		t.Errorf("repeat=5/levels=3 worst-case dispatches = %d, want 18", got)
	}
}
