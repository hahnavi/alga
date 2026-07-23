package worker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/rabbitmq"
	"alga/store"
	"alga/valkey"
)

const (
	escSortedSet  = "alga:esc:pending"
	escHashPrefix = "alga:esc:"
	escSweepTick  = 10 * time.Second
)

// EscalationSweepWorker is a pure timer. It claims expired entries from the
// Valkey sorted set, advances the level in the hash, and publishes an
// EscalationMessage to RabbitMQ. All on-call resolution, notification
// dispatch, and timeline logging live in EscalationWorker, which consumes
// that message.
//
// The schedule (max level + per-level delay) is normally captured in the
// hash at level 1 by EscalationWorker. If the schedule is missing — because
// the level 1 message predated this change, the hash was evicted by Valkey,
// or an operator PATCHed a policy mid-incident — the sweep worker falls
// back to a single GetPolicy read so the in-flight escalation does not
// stall. The schedule is re-cached so subsequent ticks stay on the
// Valkey-only path.
type EscalationSweepWorker struct {
	escalationStore store.EscalationStore
	publisher       *rabbitmq.Publisher
	vkClient        *valkey.Client
}

func NewEscalationSweepWorker(escalationStore store.EscalationStore, publisher *rabbitmq.Publisher, vkClient *valkey.Client) *EscalationSweepWorker {
	return &EscalationSweepWorker{
		escalationStore: escalationStore,
		publisher:       publisher,
		vkClient:        vkClient,
	}
}

func (w *EscalationSweepWorker) Run(ctx context.Context) {
	runTickerLoop(ctx, escSweepTick, "escalation-sweep", w.tick)
}

func (w *EscalationSweepWorker) tick(ctx context.Context) {
	if w.vkClient == nil {
		return
	}

	now := float64(time.Now().Unix())
	results, err := w.vkClient.ZRangeByScore(ctx, escSortedSet, 0, now)
	if err != nil {
		logger.Error("Escalation sweep: failed to query sorted set", "component", "escalation-sweep", "error", err)
		return
	}

	for _, incidentID := range results {
		if w.atomicClaim(ctx, incidentID, now) {
			w.processOne(ctx, incidentID)
		}
	}
}

func (w *EscalationSweepWorker) atomicClaim(ctx context.Context, incidentID string, now float64) bool {
	if w.vkClient == nil {
		return true
	}

	resp := claimLua.Exec(ctx, w.vkClient.Client(), []string{escSortedSet}, []string{incidentID, fmt.Sprintf("%.0f", now)})
	if resp.Error() != nil {
		logger.Warn("Escalation sweep: atomic claim script error", "component", "escalation-sweep", "incident_id", incidentID, "error", resp.Error())
		return false
	}
	n, err := resp.AsInt64()
	if err != nil {
		logger.Warn("Escalation sweep: atomic claim script returned non-integer; treating as not-claimed", "component", "escalation-sweep", "incident_id", incidentID, "value", resp.String(), "error", err)
		return false
	}
	return n == 1
}

// processOne advances the level in the Valkey hash and republishes the timer
// at the next level's delay. The level 1 message handler is responsible for
// having captured the schedule in the hash; if it cannot be decoded the
// entry is dropped (no dispatch, no schedule refresh — the next manual
// /escalate or auto-assign will re-arm).
func (w *EscalationSweepWorker) processOne(ctx context.Context, incidentID string) {
	hashKey := escHashPrefix + incidentID

	acknowledged, err := w.vkClient.HGet(ctx, hashKey, "acknowledged")
	if err != nil {
		logger.Warn("escalation sweep: failed to check acknowledged status, skipping", "component", "escalation-sweep", "incident_id", incidentID, "error", err)
		return
	}
	if acknowledged == "1" {
		return
	}

	currentLevelStr, err := w.vkClient.HGet(ctx, hashKey, "current_level")
	if err != nil || currentLevelStr == "" {
		logger.Warn("Escalation sweep: missing current_level in hash", "component", "escalation-sweep", "incident_id", incidentID)
		return
	}
	currentLevel, perr := strconv.Atoi(currentLevelStr)
	if perr != nil {
		logger.Warn("Escalation sweep: unparseable current_level in hash", "component", "escalation-sweep", "incident_id", incidentID, "current_level", currentLevelStr, "error", perr)
		return
	}

	silencedUntilStr, _ := w.vkClient.HGet(ctx, hashKey, "silenced_until")
	if silencedUntilStr != "" {
		silencedUntil, perr := strconv.ParseInt(silencedUntilStr, 10, 64)
		if perr == nil && silencedUntil > time.Now().Unix() {
			nextFire := float64(silencedUntil)
			w.atomicEscalateState(ctx, hashKey, escSortedSet, currentLevel, nextFire, incidentID)
			logger.Info("Escalation sweep: incident silenced, rescheduling after silence window", "component", "escalation-sweep", "incident_id", incidentID, "silenced_until", silencedUntil)
			return
		}
	}

	policyIDStr, _ := w.vkClient.HGet(ctx, hashKey, "policy_id")
	policyID, err := uuid.Parse(policyIDStr)
	if err != nil {
		logger.Warn("Escalation sweep: invalid policy_id", "component", "escalation-sweep", "incident_id", incidentID, "policy_id", policyIDStr)
		return
	}

	repeatRemainingStr, _ := w.vkClient.HGet(ctx, hashKey, "repeat_remaining")
	repeatRemaining, _ := strconv.Atoi(repeatRemainingStr)

	scheduleRaw, _ := w.vkClient.HGet(ctx, hashKey, "level_schedule")
	schedule, err := decodeEscalationSchedule(scheduleRaw)
	if err != nil || schedule.MaxLevel == 0 {
		// Schedule missing or undecodable. Fall back to a single GetPolicy
		// read so an in-flight escalation does not stall when the hash was
		// evicted or the operator PATCHed the policy mid-incident. We
		// re-cache the schedule so subsequent ticks stay on the Valkey path.
		if w.escalationStore == nil {
			logger.Warn("Escalation sweep: schedule missing and store unavailable, dropping entry",
				"component", "escalation-sweep", "incident_id", incidentID, "policy_id", policyID, "error", err)
			w.dropEntry(ctx, incidentID)
			return
		}
		policy, perr := w.escalationStore.GetPolicy(ctx, policyID)
		if perr != nil || policy == nil || len(policy.Levels) == 0 {
			logger.Warn("Escalation sweep: schedule missing and policy lookup failed, dropping entry",
				"component", "escalation-sweep", "incident_id", incidentID, "policy_id", policyID, "decode_error", err, "lookup_error", perr)
			w.dropEntry(ctx, incidentID)
			return
		}
		schedule = cacheScheduleFromPolicy(ctx, w.vkClient, hashKey, policy.Levels)
		if schedule.MaxLevel == 0 {
			logger.Warn("Escalation sweep: policy has no usable levels, dropping entry",
				"component", "escalation-sweep", "incident_id", incidentID, "policy_id", policyID)
			w.dropEntry(ctx, incidentID)
			return
		}
	}

	nextLevel := currentLevel + 1
	if nextLevel > schedule.MaxLevel {
		if repeatRemaining > 0 {
			nextLevel = 1
			_ = w.vkClient.HSet(ctx, hashKey, "repeat_remaining", strconv.Itoa(repeatRemaining-1))
		} else {
			w.dropEntry(ctx, incidentID)
			logger.Info("Escalation sweep: all levels exhausted", "component", "escalation-sweep", "incident_id", incidentID)
			return
		}
	}

	if err := w.publishAdvance(ctx, incidentID, policyID, nextLevel); err != nil {
		logger.Error("Escalation sweep: failed to publish escalation", "component", "escalation-sweep", "incident_id", incidentID, "level", nextLevel, "error", err)
		w.dropEntry(ctx, incidentID)
		return
	}

	nextDelay := scheduleDelayForLevel(schedule, nextLevel)
	nextFire := float64(time.Now().Add(nextDelay).Unix())
	w.atomicEscalateState(ctx, hashKey, escSortedSet, nextLevel, nextFire, incidentID)

	logger.Info("Escalation sweep: advanced to level", "component", "escalation-sweep", "incident_id", incidentID, "level", nextLevel, "next_delay_seconds", int(nextDelay.Seconds()))
}

// publishAdvance converts the Valkey incident_id back to int64 and enqueues
// the EscalationMessage for EscalationWorker to consume.
func (w *EscalationSweepWorker) publishAdvance(ctx context.Context, incidentID string, policyID uuid.UUID, level int) error {
	if w.publisher == nil {
		return fmt.Errorf("rabbitmq publisher not configured")
	}
	incidentNumber, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid incident id %q: %w", incidentID, err)
	}
	return w.publisher.PublishEscalation(ctx, rabbitmq.EscalationMessage{
		IncidentNumber: incidentNumber,
		PolicyID:       policyID,
		Level:          level,
	})
}

// dropEntry removes the incident from the pending sorted set so the next tick
// does not reclaim and re-process it. The hash is left in place so a manual
// /escalate (which reads the state) still sees the incident as already-
// escalated. It will expire on its 24h TTL.
func (w *EscalationSweepWorker) dropEntry(ctx context.Context, incidentID string) {
	if err := w.vkClient.ZRem(ctx, escSortedSet, incidentID); err != nil {
		logger.Warn("Escalation sweep: failed to remove exhausted entry", "component", "escalation-sweep", "incident_id", incidentID, "error", err)
	}
}

// scheduleDelayForLevel returns the next-fire delay for a level, clamped to
// at least 1 minute so a misconfigured 0-minute level cannot pair with the
// 10s sweep tick to page at 10s intervals. Mirrors the floor in
// delayForPolicyLevel so the two stay consistent.
func scheduleDelayForLevel(sched escalationLevelSchedule, level int) time.Duration {
	const minLevelDelay = 1 * time.Minute
	if delayMin, ok := sched.Delays[level]; ok {
		d := time.Duration(delayMin) * time.Minute
		if d < minLevelDelay {
			return minLevelDelay
		}
		return d
	}
	return 5 * time.Minute
}

var claimLua = valkey.NewLuaScript(`
		local member = ARGV[1]
		local max_score = tonumber(ARGV[2])
		local score = redis.call('ZSCORE', KEYS[1], member)
		if score and tonumber(score) <= max_score then
			redis.call('ZREM', KEYS[1], member)
			return 1
		end
		return 0
	`)

var escalateLua = valkey.NewLuaScript(`local hashKey = KEYS[1]
local sortedSetKey = KEYS[2]
redis.call('HSET', hashKey, 'current_level', ARGV[1])
redis.call('ZADD', sortedSetKey, ARGV[2], ARGV[3])
return 1`)

func (w *EscalationSweepWorker) atomicEscalateState(ctx context.Context, hashKey, sortedSetKey string, level int, nextFire float64, incidentID string) {
	if w.vkClient == nil {
		return
	}
	resp := escalateLua.Exec(ctx, w.vkClient.Client(), []string{hashKey, sortedSetKey}, []string{
		strconv.Itoa(level),
		fmt.Sprintf("%.0f", nextFire),
		incidentID,
	})
	if resp.Error() != nil {
		logger.Error("Escalation sweep: atomic state update failed", "component", "escalation-sweep", "incident_id", incidentID, "error", resp.Error())
	}
}
