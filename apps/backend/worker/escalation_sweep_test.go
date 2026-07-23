package worker

import (
	"context"
	"testing"
	"time"

	"alga/store"
)

func TestDefaultDelayForLevel(t *testing.T) {
	t.Parallel()
	policy := &store.EscalationPolicyRecord{
		Levels: []store.EscalationLevelRecord{
			{LevelNumber: 1, DelayMinutes: 5},
			{LevelNumber: 2, DelayMinutes: 10},
			{LevelNumber: 3, DelayMinutes: 0},
		},
	}

	if d := delayForPolicyLevel(policy, 1); d != 5*time.Minute {
		t.Errorf("level 1 delay = %v, want 5m", d)
	}
	if d := delayForPolicyLevel(policy, 2); d != 10*time.Minute {
		t.Errorf("level 2 delay = %v, want 10m", d)
	}
	// DelayMinutes=0 is clamped to a 1-minute floor so the 10s sweep tick
	// cannot page at 10s intervals.
	if d := delayForPolicyLevel(policy, 3); d != 1*time.Minute {
		t.Errorf("level 3 delay = %v, want clamped 1m", d)
	}
	if d := delayForPolicyLevel(policy, 99); d != 5*time.Minute {
		t.Errorf("unknown level delay = %v, want default 5m", d)
	}
}

func TestDefaultDelayForLevelEmptyPolicy(t *testing.T) {
	t.Parallel()
	policy := &store.EscalationPolicyRecord{}
	if d := delayForPolicyLevel(policy, 1); d != 5*time.Minute {
		t.Errorf("empty policy delay = %v, want default 5m", d)
	}
}

func TestDefaultDelayForLevelSingleLevel(t *testing.T) {
	t.Parallel()
	policy := &store.EscalationPolicyRecord{
		Levels: []store.EscalationLevelRecord{
			{LevelNumber: 1, DelayMinutes: 15},
		},
	}
	if d := delayForPolicyLevel(policy, 1); d != 15*time.Minute {
		t.Errorf("level 1 delay = %v, want 15m", d)
	}
	if d := delayForPolicyLevel(policy, 2); d != 5*time.Minute {
		t.Errorf("missing level 2 delay = %v, want default 5m", d)
	}
}

func TestDefaultDelayForLevelLargeDelay(t *testing.T) {
	t.Parallel()
	policy := &store.EscalationPolicyRecord{
		Levels: []store.EscalationLevelRecord{
			{LevelNumber: 1, DelayMinutes: 120},
		},
	}
	if d := delayForPolicyLevel(policy, 1); d != 120*time.Minute {
		t.Errorf("level 1 delay = %v, want 120m", d)
	}
}

func TestDefaultDelayForLevelNegativeLevel(t *testing.T) {
	t.Parallel()
	policy := &store.EscalationPolicyRecord{
		Levels: []store.EscalationLevelRecord{
			{LevelNumber: 1, DelayMinutes: 5},
		},
	}
	if d := delayForPolicyLevel(policy, -1); d != 5*time.Minute {
		t.Errorf("negative level delay = %v, want default 5m", d)
	}
}

func TestDefaultDelayForLevelLevelZero(t *testing.T) {
	t.Parallel()
	policy := &store.EscalationPolicyRecord{
		Levels: []store.EscalationLevelRecord{
			{LevelNumber: 0, DelayMinutes: 3},
		},
	}
	if d := delayForPolicyLevel(policy, 0); d != 3*time.Minute {
		t.Errorf("level 0 delay = %v, want 3m", d)
	}
}

func TestEscalationSweepWorkerNilValkeyTick(t *testing.T) {
	t.Parallel()
	w := &EscalationSweepWorker{vkClient: nil}
	w.tick(context.TODO())
}

func TestEscalationSweepConstants(t *testing.T) {
	t.Parallel()
	if escHashPrefix != "alga:esc:" {
		t.Fatalf("escHashPrefix = %q, want %q", escHashPrefix, "alga:esc:")
	}
	if escSortedSet != "alga:esc:pending" {
		t.Fatalf("escSortedSet = %q, want %q", escSortedSet, "alga:esc:pending")
	}
}

func TestEscalationSweepWorkerBuilder(t *testing.T) {
	t.Parallel()
	w := NewEscalationSweepWorker(nil, nil, nil)
	if w == nil {
		t.Fatalf("NewEscalationSweepWorker returned nil")
	}
}

func TestEncodeEscalationSchedule(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns empty string", func(t *testing.T) {
		t.Parallel()
		got, err := encodeEscalationSchedule(nil)
		if err != nil {
			t.Fatalf("encodeEscalationSchedule(nil) error: %v", err)
		}
		if got != "" {
			t.Errorf("want empty, got %q", got)
		}
	})

	t.Run("captures max level and per-level delays", func(t *testing.T) {
		t.Parallel()
		levels := []store.EscalationLevelRecord{
			{LevelNumber: 1, DelayMinutes: 5},
			{LevelNumber: 2, DelayMinutes: 10},
			{LevelNumber: 3, DelayMinutes: 15},
		}
		raw, err := encodeEscalationSchedule(levels)
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		sched, err := decodeEscalationSchedule(raw)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if sched.MaxLevel != 3 {
			t.Errorf("MaxLevel = %d, want 3", sched.MaxLevel)
		}
		if got := sched.Delays[1]; got != 5 {
			t.Errorf("Delays[1] = %d, want 5", got)
		}
		if got := sched.Delays[2]; got != 10 {
			t.Errorf("Delays[2] = %d, want 10", got)
		}
		if got := sched.Delays[3]; got != 15 {
			t.Errorf("Delays[3] = %d, want 15", got)
		}
	})

	t.Run("skips non-positive level numbers", func(t *testing.T) {
		t.Parallel()
		levels := []store.EscalationLevelRecord{
			{LevelNumber: 0, DelayMinutes: 7},
			{LevelNumber: 1, DelayMinutes: 5},
		}
		raw, err := encodeEscalationSchedule(levels)
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		sched, err := decodeEscalationSchedule(raw)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if sched.MaxLevel != 1 {
			t.Errorf("MaxLevel = %d, want 1 (level 0 must be ignored)", sched.MaxLevel)
		}
	})

	t.Run("decode of empty string is zero value", func(t *testing.T) {
		t.Parallel()
		sched, err := decodeEscalationSchedule("")
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if sched.MaxLevel != 0 {
			t.Errorf("MaxLevel = %d, want 0", sched.MaxLevel)
		}
		if len(sched.Delays) != 0 {
			t.Errorf("Delays = %v, want empty", sched.Delays)
		}
	})

	t.Run("decode of malformed JSON returns error", func(t *testing.T) {
		t.Parallel()
		_, err := decodeEscalationSchedule("not-json")
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})
}

func TestScheduleDelayForLevel(t *testing.T) {
	t.Parallel()
	sched := escalationLevelSchedule{
		MaxLevel: 3,
		Delays:   map[int]int{1: 5, 2: 10, 3: 0},
	}

	if d := scheduleDelayForLevel(sched, 1); d != 5*time.Minute {
		t.Errorf("level 1 = %v, want 5m", d)
	}
	if d := scheduleDelayForLevel(sched, 2); d != 10*time.Minute {
		t.Errorf("level 2 = %v, want 10m", d)
	}
	if d := scheduleDelayForLevel(sched, 3); d != 1*time.Minute {
		t.Errorf("level 3 with 0 delay = %v, want 1m clamp", d)
	}
	if d := scheduleDelayForLevel(sched, 99); d != 5*time.Minute {
		t.Errorf("unknown level = %v, want default 5m", d)
	}
}

func TestEscalationSweepWorkerHasNoEngineOrDispatcher(t *testing.T) {
	// The sweep worker is now a pure timer. It must not carry an engine or
	// notification dispatcher so the dispatch path is single-source-of-truth
	// (EscalationWorker only). This test pins the field set so a future
	// "convenience" addition breaks it loudly.
	t.Parallel()

	w := &EscalationSweepWorker{}
	if w.publisher != nil {
		t.Errorf("publisher = %v, want nil until SetPublisher", w.publisher)
	}
	if w.vkClient != nil {
		t.Errorf("vkClient = %v, want nil until SetVKClient", w.vkClient)
	}
	if w.escalationStore != nil {
		t.Errorf("escalationStore = %v, want nil until SetEscalationStore", w.escalationStore)
	}
}

func TestCacheScheduleFromPolicyEmpty(t *testing.T) {
	t.Parallel()
	if got := cacheScheduleFromPolicy(context.TODO(), nil, "k", nil); got.MaxLevel != 0 {
		t.Errorf("MaxLevel = %d, want 0 for nil levels", got.MaxLevel)
	}
	if got := cacheScheduleFromPolicy(context.TODO(), nil, "k", []store.EscalationLevelRecord{}); got.MaxLevel != 0 {
		t.Errorf("MaxLevel = %d, want 0 for empty levels", got.MaxLevel)
	}
}

func TestCacheScheduleFromPolicyWritesHash(t *testing.T) {
	t.Parallel()
	fake := &recordingHashWriter{}
	levels := []store.EscalationLevelRecord{
		{LevelNumber: 1, DelayMinutes: 5},
		{LevelNumber: 2, DelayMinutes: 10},
	}
	got := cacheScheduleFromPolicy(context.TODO(), fake, "alga:esc:42", levels)
	if got.MaxLevel != 2 {
		t.Errorf("MaxLevel = %d, want 2", got.MaxLevel)
	}
	if fake.key != "alga:esc:42" {
		t.Errorf("key = %q, want alga:esc:42", fake.key)
	}
	if fake.field != "level_schedule" {
		t.Errorf("field = %q, want level_schedule", fake.field)
	}
	if fake.value == "" {
		t.Error("value is empty, want JSON schedule")
	}
}

type recordingHashWriter struct {
	key, field, value string
}

func (r *recordingHashWriter) HSet(_ context.Context, key, field, value string) error {
	r.key, r.field, r.value = key, field, value
	return nil
}

func TestActionItemSweepWorkerBuilder(t *testing.T) {
	t.Parallel()
	w := NewActionItemSweepWorker(nil, nil)
	if w == nil {
		t.Fatalf("NewActionItemSweepWorker returned nil")
	}
}
