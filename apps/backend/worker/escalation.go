package worker

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/cancellation"
	"alga/escalation"
	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

type EscalationWorker struct {
	escalationStore store.EscalationStore
	incidentStore   store.IncidentStore
	engine          *escalation.PolicyEngine
	ssePublisher    *sse.DualPublisher
	publisher       *rabbitmq.Publisher
	vkClient        *valkey.Client
	cancelSet       *valkey.CancelSet
	dispatcher      escalationDispatcher
}

func NewEscalationWorker(escalationStore store.EscalationStore, incidentStore store.IncidentStore, engine *escalation.PolicyEngine, ssePublisher *sse.DualPublisher, publisher *rabbitmq.Publisher, vkClient *valkey.Client) *EscalationWorker {
	return &EscalationWorker{
		escalationStore: escalationStore,
		incidentStore:   incidentStore,
		engine:          engine,
		ssePublisher:    ssePublisher,
		publisher:       publisher,
		vkClient:        vkClient,
		dispatcher: escalationDispatcher{
			publisher:     publisher,
			ssePublisher:  ssePublisher,
			incidentStore: incidentStore,
		},
	}
}

func (w *EscalationWorker) Queue() string {
	return rabbitmq.QueueEscalationProcess
}

func (w *EscalationWorker) SetCancelSet(cs *valkey.CancelSet) { w.cancelSet = cs }

func (w *EscalationWorker) PrefetchCount() int {
	return 5
}

// escalationRetryBudget clamps the message's MaxRetries to the retry ladder:
// unset or out-of-range values fall back to the constant, so a publisher can
// lower the budget for a message but never exceed the wired retry queues.
func escalationRetryBudget(maxRetries int) int {
	if maxRetries <= 0 || maxRetries > rabbitmq.MaxEscalationRetries {
		return rabbitmq.MaxEscalationRetries
	}
	return maxRetries
}

func (w *EscalationWorker) Handle(ctx context.Context, d amqp.Delivery) {
	var msg rabbitmq.EscalationMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Failed to unmarshal escalation message", "component", "escalation-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	if msg.Level <= 0 {
		logger.Error("Rejecting escalation message with non-positive level", "component", "escalation-worker", "incident_number", msg.IncidentNumber, "level", msg.Level)
		_ = d.Nack(false, false)
		return
	}

	if cancellation.IncidentCancelled(ctx, w.cancelSet, w.incidentStore, msg.IncidentNumber) {
		logger.Info("Dropping escalation; incident deleted", "component", "escalation-worker", "incident_number", msg.IncidentNumber)
		_ = d.Ack(false)
		return
	}

	// Terminal incidents must not page anyone: the sweep only reads Valkey
	// ack state, so this DB check is the backstop for entries that were
	// scheduled before the incident hit a terminal status.
	if w.incidentStore != nil && msg.IncidentNumber > 0 {
		if inc, err := w.incidentStore.GetIncident(ctx, msg.IncidentNumber); err == nil && inc != nil && escalation.IsTerminalIncidentStatus(inc.Status) {
			logger.Info("Dropping escalation; incident in terminal status", "component", "escalation-worker", "incident_number", msg.IncidentNumber, "status", inc.Status)
			_ = d.Ack(false)
			return
		}
	}

	logger.Info("Processing escalation", "component", "escalation-worker", "incident_number", msg.IncidentNumber, "policy_id", msg.PolicyID, "level", msg.Level, "retry", msg.RetryCount)

	userIDs, channels, forcedChannels, err := w.engine.EvaluatePolicy(ctx, msg.PolicyID, msg.Level)
	if err != nil {
		logger.Error("Failed to evaluate escalation policy", "component", "escalation-worker", "policy_id", msg.PolicyID, "error", err)
		if w.publisher != nil {
			msg.RetryCount++
			if msg.RetryCount <= escalationRetryBudget(msg.MaxRetries) {
				if pubErr := w.publisher.PublishEscalationRetry(ctx, msg); pubErr != nil {
					logger.Error("Failed to publish escalation retry", "component", "escalation-worker", "error", pubErr)
				} else {
					_ = d.Ack(false)
					return
				}
			}
		}
		_ = d.Nack(false, false)
		return
	}

	w.dispatcher.dispatchEscalation(ctx, msg.IncidentNumber, msg.PolicyID, msg.Level, userIDs, channels, forcedChannels)

	// Write Valkey state for level 1 to enable sweep worker escalation. The
	// schedule (max_level + per-level delay minutes) is captured in the hash
	// so the sweep worker can advance without any database lookups.
	if w.vkClient != nil && msg.Level == 1 {
		w.seedLevel1State(ctx, msg)
	}

	metrics.EscalationsFired.Add(1)
	logger.Info("Escalation complete", "component", "escalation-worker", "incident_number", msg.IncidentNumber, "level", msg.Level, "users_notified", len(userIDs))
	_ = d.Ack(false)
}

// seedLevel1State writes the per-incident escalation state to the Valkey
// hash so the sweep worker can advance the level on subsequent ticks. The
// level schedule (max_level + per-level delay minutes) is captured in the
// same hash so the sweep worker can stay Valkey-only on the hot path.
func (w *EscalationWorker) seedLevel1State(ctx context.Context, msg rabbitmq.EscalationMessage) {
	if w.vkClient == nil {
		return
	}
	incidentNumber := strconv.FormatInt(msg.IncidentNumber, 10)
	hashKey := escHashPrefix + incidentNumber
	repeatRemaining := "0"
	scheduleJSON := ""
	nextDelay := 15 * time.Minute
	policy, _ := w.escalationStore.GetPolicy(ctx, msg.PolicyID)
	if policy != nil {
		repeatRemaining = strconv.Itoa(policy.RepeatCount)
		if encoded, encErr := encodeEscalationSchedule(policy.Levels); encErr == nil {
			scheduleJSON = encoded
		} else {
			logger.Warn("Failed to encode escalation schedule", "component", "escalation-worker", "policy_id", msg.PolicyID, "error", encErr)
		}
		nextDelay = delayForPolicyLevel(policy, msg.Level)
	}
	fields := map[string]string{
		"current_level":    strconv.Itoa(msg.Level),
		"policy_id":        msg.PolicyID.String(),
		"started_at":       time.Now().UTC().Format(time.RFC3339),
		"acknowledged":     "0",
		"repeat_remaining": repeatRemaining,
	}
	if scheduleJSON != "" {
		fields["level_schedule"] = scheduleJSON
	}
	if err := w.vkClient.HMSet(ctx, hashKey, fields); err != nil {
		logger.Warn("Failed to set escalation state in Valkey", "component", "escalation-worker", "key", hashKey, "error", err)
	}
	if err := w.vkClient.Do(ctx, w.vkClient.Builder().Expire().Key(hashKey).Seconds(86400).Build()).Error(); err != nil {
		logger.Warn("Failed to set TTL on escalation state in Valkey", "component", "escalation-worker", "key", hashKey, "error", err)
	}
	nextFire := float64(time.Now().Add(nextDelay).Unix())
	if err := w.vkClient.ZAdd(ctx, escSortedSet, nextFire, incidentNumber); err != nil {
		logger.Warn("Failed to add escalation to sorted set in Valkey", "component", "escalation-worker", "key", escSortedSet, "error", err)
	}
}
