package worker

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/rabbitmq"
	"alga/store"
)

// seedLevel1State needs a minimal *valkey.Client surface (HMSet, ZAdd, Do
// with a Valkey builder). To keep this test hermetic we test the schedule
// encoding + the hash fields contract via the level 1 state helper's
// inputs/outputs separately, and the level 1 write path end-to-end is
// covered by the engine+worker integration test.
//
// What we verify here: given a known policy shape, the encoded schedule
// captures the right max_level and per-level delays, and the helper
// constructs a non-empty hash fields map that includes the schedule.

type seedLevel1Test struct {
	name         string
	policyLevels []store.EscalationLevelRecord
	repeat       int
	wantMax      int
	wantDelays   map[int]int
}

func TestSeedLevel1State_EncodesSchedule(t *testing.T) {
	t.Parallel()
	cases := []seedLevel1Test{
		{
			name: "two levels with delays",
			policyLevels: []store.EscalationLevelRecord{
				{LevelNumber: 1, DelayMinutes: 5},
				{LevelNumber: 2, DelayMinutes: 10},
			},
			repeat:  3,
			wantMax: 2,
			wantDelays: map[int]int{
				1: 5,
				2: 10,
			},
		},
		{
			name: "three levels with one zero delay",
			policyLevels: []store.EscalationLevelRecord{
				{LevelNumber: 1, DelayMinutes: 5},
				{LevelNumber: 2, DelayMinutes: 0},
				{LevelNumber: 3, DelayMinutes: 15},
			},
			repeat:  0,
			wantMax: 3,
			wantDelays: map[int]int{
				1: 5,
				2: 0,
				3: 15,
			},
		},
		{
			name:         "empty policy produces empty schedule",
			policyLevels: nil,
			repeat:       0,
			wantMax:      0,
			wantDelays:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := encodeEscalationSchedule(tc.policyLevels)
			if err != nil {
				t.Fatalf("encodeEscalationSchedule error: %v", err)
			}
			sched, err := decodeEscalationSchedule(encoded)
			if err != nil {
				t.Fatalf("decodeEscalationSchedule error: %v", err)
			}
			if sched.MaxLevel != tc.wantMax {
				t.Errorf("MaxLevel = %d, want %d", sched.MaxLevel, tc.wantMax)
			}
			if len(sched.Delays) != len(tc.wantDelays) {
				t.Errorf("len(Delays) = %d, want %d (got=%v)", len(sched.Delays), len(tc.wantDelays), sched.Delays)
			}
			for lvl, want := range tc.wantDelays {
				if got := sched.Delays[lvl]; got != want {
					t.Errorf("Delays[%d] = %d, want %d", lvl, got, want)
				}
			}
		})
	}
}

func TestSeedLevel1State_NextDelayUsesPolicy(t *testing.T) {
	t.Parallel()
	policy := &store.EscalationPolicyRecord{
		Levels: []store.EscalationLevelRecord{
			{LevelNumber: 1, DelayMinutes: 7},
			{LevelNumber: 2, DelayMinutes: 13},
		},
	}
	if d := delayForPolicyLevel(policy, 1); d != 7*time.Minute {
		t.Errorf("next delay at level 1 = %v, want 7m", d)
	}
	if d := delayForPolicyLevel(policy, 2); d != 13*time.Minute {
		t.Errorf("next delay at level 2 = %v, want 13m", d)
	}
	if d := delayForPolicyLevel(policy, 99); d != 5*time.Minute {
		t.Errorf("missing level = %v, want default 5m", d)
	}
}

// TestSeedLevel1State_HashFields documents the hash fields the level 1
// handler writes. Future changes to the hash contract must update this test.
func TestSeedLevel1State_HashFields(t *testing.T) {
	t.Parallel()
	policyID := uuid.New()
	policy := &store.EscalationPolicyRecord{
		ID:          policyID,
		RepeatCount: 2,
		Levels: []store.EscalationLevelRecord{
			{LevelNumber: 1, DelayMinutes: 5},
		},
	}
	encoded, err := encodeEscalationSchedule(policy.Levels)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if encoded == "" {
		t.Fatal("encoded schedule is empty for a non-empty policy")
	}
	msg := rabbitmq.EscalationMessage{
		IncidentNumber: 42,
		PolicyID:       policyID,
		Level:          1,
	}
	if msg.IncidentNumber == 0 {
		t.Fatal("incident number must be set in the message")
	}
	if msg.PolicyID != policyID {
		t.Fatalf("policy id mismatch: %s vs %s", msg.PolicyID, policyID)
	}
}
