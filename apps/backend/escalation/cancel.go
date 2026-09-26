package escalation

import (
	"context"
	"strconv"

	"alga/logger"
	"alga/store"
)

// State keys shared by every escalation-cancel call site (API acknowledge,
// phone callbacks, Slack interactive buttons) and by the sweep worker, which
// reads the same hash and pending set.
const (
	// StateHashPrefix is the per-incident escalation state hash prefix; the
	// full key is StateHashPrefix + incident number.
	StateHashPrefix = "alga:esc:"
	// PendingSetKey is the sorted set of incident numbers with a pending
	// escalation advance.
	PendingSetKey = "alga:esc:pending"
)

// StateClient is the minimal Valkey surface cancellation needs. Satisfied by
// *valkey.Client and by test fakes.
type StateClient interface {
	ZScore(ctx context.Context, key, member string) (float64, error)
	HSet(ctx context.Context, key, field, value string) error
	ZRem(ctx context.Context, key, member string) error
}

// TimelineWriter is the minimal incident-store surface cancellation needs to
// record an escalation_cancelled timeline entry.
type TimelineWriter interface {
	AddTimelineEntry(ctx context.Context, record *store.IncidentTimelineEntryRecord) error
}

// CancelForIncident marks the incident's escalation state as acknowledged and
// removes it from the pending sweep set so no further levels fire, then writes
// a system timeline entry describing why. All storage errors are logged and
// swallowed: cancellation is a best-effort side effect and must never fail its
// caller (an ack handler or webhook button response).
func CancelForIncident(ctx context.Context, vkClient StateClient, timeline TimelineWriter, incidentID, reason string) {
	if vkClient == nil || incidentID == "" {
		return
	}
	// The timeline entry is only honest when an escalation was actually
	// pending: the escalation worker adds every fired escalation to the
	// pending set, so its absence means there is nothing to cancel and
	// writing "Escalation stopped" would fabricate history.
	wasPending, pendingErr := vkClient.ZScore(ctx, PendingSetKey, incidentID)
	hashKey := StateHashPrefix + incidentID
	if err := vkClient.HSet(ctx, hashKey, "acknowledged", "1"); err != nil {
		logger.WarnCtx(ctx, "Failed to mark escalation acknowledged in Valkey", "component", "escalation", "incident_id", incidentID, "error", err)
	}
	if err := vkClient.ZRem(ctx, PendingSetKey, incidentID); err != nil {
		logger.WarnCtx(ctx, "Failed to remove escalation from pending set in Valkey", "component", "escalation", "incident_id", incidentID, "error", err)
	}
	if timeline == nil || pendingErr != nil || wasPending == 0 {
		return
	}
	incidentNumber, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil || incidentNumber <= 0 {
		return
	}
	if err := timeline.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "escalation_cancelled",
		ActorType:      "system",
		Message:        "Escalation stopped — " + reason,
	}); err != nil {
		logger.WarnCtx(ctx, "Failed to add escalation_cancelled timeline entry", "component", "escalation", "incident_id", incidentID, "error", err)
	}
}

// terminalIncidentStatuses are the statuses that make escalation cancellation
// pointless: no further level can fire for them.
var terminalIncidentStatuses = map[string]struct{}{
	"resolved":  {},
	"closed":    {},
	"cancelled": {},
}

// IsTerminalIncidentStatus reports whether an incident status means any
// pending escalation is already inert.
func IsTerminalIncidentStatus(status string) bool {
	_, ok := terminalIncidentStatuses[status]
	return ok
}
