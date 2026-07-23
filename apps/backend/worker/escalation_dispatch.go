package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
)

const (
	forcedChannelsVoice = "voice"
)

type escalationDispatcher struct {
	publisher     *rabbitmq.Publisher
	ssePublisher  *sse.DualPublisher
	incidentStore store.IncidentStore
}

// escalationLevelSchedule is the on-the-wire schedule captured in the
// escalation hash so the sweep worker can advance without database lookups.
// Keys are the level numbers and values are the per-level delay in minutes.
// levelScheduleMaxLevel lets the sweep worker know when to wrap to level 1
// without scanning the map.
type escalationLevelSchedule struct {
	MaxLevel int         `json:"max_level"`
	Delays   map[int]int `json:"delays_minutes"`
}

func encodeEscalationSchedule(levels []store.EscalationLevelRecord) (string, error) {
	if len(levels) == 0 {
		return "", nil
	}
	sched := escalationLevelSchedule{
		MaxLevel: 0,
		Delays:   make(map[int]int, len(levels)),
	}
	for _, lvl := range levels {
		if lvl.LevelNumber <= 0 {
			continue
		}
		if lvl.LevelNumber > sched.MaxLevel {
			sched.MaxLevel = lvl.LevelNumber
		}
		sched.Delays[lvl.LevelNumber] = lvl.DelayMinutes
	}
	if sched.MaxLevel == 0 {
		return "", nil
	}
	out, err := json.Marshal(sched)
	if err != nil {
		return "", fmt.Errorf("marshal escalation schedule: %w", err)
	}
	return string(out), nil
}

func decodeEscalationSchedule(raw string) (escalationLevelSchedule, error) {
	var sched escalationLevelSchedule
	if raw == "" {
		return sched, nil
	}
	if err := json.Unmarshal([]byte(raw), &sched); err != nil {
		return sched, fmt.Errorf("unmarshal escalation schedule: %w", err)
	}
	return sched, nil
}

// cacheScheduleFromPolicy encodes a level list and writes it to the
// incident's escalation hash so the sweep worker can advance without a
// database read on subsequent ticks. Failures are logged but non-fatal — the
// sweep worker will simply re-fetch from the store on the next tick.
func cacheScheduleFromPolicy(ctx context.Context, vk hashWriter, hashKey string, levels []store.EscalationLevelRecord) escalationLevelSchedule {
	encoded, err := encodeEscalationSchedule(levels)
	if err != nil || encoded == "" {
		return escalationLevelSchedule{}
	}
	if vk != nil {
		if err := vk.HSet(ctx, hashKey, "level_schedule", encoded); err != nil {
			logger.Warn("Failed to recache escalation schedule", "component", "escalation-sweep", "key", hashKey, "error", err)
		}
	}
	sched, _ := decodeEscalationSchedule(encoded)
	return sched
}

// hashWriter is the minimal Valkey surface the schedule cache needs. It is
// satisfied by *valkey.Client and by test fakes.
type hashWriter interface {
	HSet(ctx context.Context, key, field, value string) error
}

func (d *escalationDispatcher) dispatchEscalation(
	ctx context.Context,
	incidentNumber int64,
	policyID uuid.UUID,
	level int,
	userIDs []uuid.UUID,
	channels []string,
	forcedChannels bool,
) {
	if forcedChannels {
		channels = []string{forcedChannelsVoice}
	}

	incidentNumberText := strconv.FormatInt(incidentNumber, 10)
	for _, uid := range userIDs {
		if d.publisher != nil {
			if err := d.publisher.PublishNotificationDispatch(ctx, rabbitmq.NotificationDispatchMessage{
				UserID:           uid.String(),
				NotificationType: "escalation",
				Title:            fmt.Sprintf("Incident %d escalated to level %d", incidentNumber, level),
				Message:          fmt.Sprintf("Incident %d has been escalated to level %d.", incidentNumber, level),
				ResourceType:     "incident",
				ResourceID:       incidentNumberText,
				IncidentNumber:   incidentNumber,
				Channels:         channels,
				Level:            level,
			}); err != nil {
				logger.Error("Failed to publish notification dispatch for escalation", "component", "escalation-dispatch", "error", err)
			}
		}

		if d.ssePublisher != nil {
			d.ssePublisher.PublishToUser(uid.String(), sse.Event{
				Type: "escalation_notification",
				Data: map[string]any{
					"incident_number": incidentNumber,
					"policy_id":       policyID.String(),
					"level":           level,
					"channels":        channels,
				},
			})
		}
	}

	if d.incidentStore != nil && incidentNumber > 0 {
		message := fmt.Sprintf("Escalated to level %d: notified %d users", level, len(userIDs))
		entry := &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "escalation",
			ActorType:      "system",
			Message:        message,
		}
		if forcedChannels {
			entry.Metadata = map[string]any{
				"forced_channels": true,
				"channels":        channels,
			}
		}
		if err := d.incidentStore.AddTimelineEntry(ctx, entry); err != nil {
			logger.Warn("Failed to add escalation timeline entry", "component", "escalation", "incident_number", incidentNumber, "error", err)
		}
	}
}

func delayForPolicyLevel(policy *store.EscalationPolicyRecord, level int) time.Duration {
	const minLevelDelay = 1 * time.Minute
	for _, lvl := range policy.Levels {
		if lvl.LevelNumber == level {
			d := time.Duration(lvl.DelayMinutes) * time.Minute
			// Clamp to a reasonable minimum so a misconfigured 0-minute level
			// cannot pair with the 10s sweep tick to page at 10s intervals.
			// Real incidents should never need sub-minute level transitions.
			if d < minLevelDelay {
				return minLevelDelay
			}
			return d
		}
	}
	return 5 * time.Minute
}
