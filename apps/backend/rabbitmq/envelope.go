package rabbitmq

import (
	"context"
	"time"

	"github.com/google/uuid"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// DefaultEventVersion is the schema version stamped onto an event when the
// producer does not set one explicitly. Bump the per-event version (via the
// message constructor) whenever a payload changes shape in a breaking way.
const DefaultEventVersion = 1

// EventEnvelope is the shared metadata carried by every RabbitMQ message Alga
// publishes. It is embedded into each message type so producers, consumers, and
// the (future) transactional outbox can rely on a single event identity and a
// consistent naming scheme.
//
// Fields:
//   - EventID:        globally unique id (UUIDv7) assigned once at publish time;
//     the idempotency key consumers/the outbox dedupe on.
//   - EventType:      the aggregate.action wire name (e.g. "alert.received").
//   - EventVersion:   payload schema version, starting at DefaultEventVersion.
//   - OccurredAt:     UTC timestamp the event was produced.
//   - TraceID:        W3C trace id propagated across the broker boundary.
//   - CorrelationKey: domain correlation grouping (e.g. alert correlation key).
//   - AggregateID:    the id of the aggregate the event is about.
//
// All fields are omitempty so embedding the envelope never changes the wire
// shape of legacy messages that leave them unset, keeping existing consumers
// backward compatible.
type EventEnvelope struct {
	EventID        string    `json:"event_id,omitempty"`
	EventType      string    `json:"event_type,omitempty"`
	EventVersion   int       `json:"event_version,omitempty"`
	OccurredAt     time.Time `json:"occurred_at,omitempty"`
	TraceID        string    `json:"trace_id,omitempty"`
	CorrelationKey string    `json:"correlation_key,omitempty"`
	AggregateID    string    `json:"aggregate_id,omitempty"`
}

// ensureEnvelope fills in identity/metadata fields that have not already been
// set by the caller. It is idempotent: existing values are preserved so that a
// message re-published onto a retry queue keeps its original EventID,
// OccurredAt, and EventType across every attempt.
func (e *EventEnvelope) ensureEnvelope(eventType string, version int, traceID string) {
	if e.EventID == "" {
		e.EventID = uuid.Must(uuid.NewV7()).String()
	}
	if e.EventType == "" {
		e.EventType = eventType
	}
	if e.EventVersion == 0 {
		e.EventVersion = version
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	if e.TraceID == "" {
		e.TraceID = traceID
	}
}

// envelopeCarrier is satisfied by *EventEnvelope and therefore by a pointer to
// any message type that embeds EventEnvelope. The publisher uses it to stamp the
// envelope uniformly regardless of the concrete message type.
type envelopeCarrier interface {
	ensureEnvelope(eventType string, version int, traceID string)
}

// traceIDFromContext returns the active OpenTelemetry trace id, or "" when no
// valid span context is present (e.g. tracing disabled). Producers use it to
// populate EventEnvelope.TraceID at publish time.
func traceIDFromContext(ctx context.Context) string {
	sc := oteltrace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

// Event type constants use the aggregate.action wire naming convention
// (gresto 33.4). The Go identifier keeps the PascalCase domain name; the string
// value is the lowercase aggregate.action form carried in EventEnvelope.EventType
// and documented in refs/plan/architecture/event-catalog.md.
const (
	// EventTypeAlertReceived is emitted when a Grafana webhook payload is
	// accepted for asynchronous processing.
	EventTypeAlertReceived = "alert.received"
	// EventTypeNotificationRequested is emitted when an alert notification
	// needs to be created or updated on a chat provider.
	EventTypeNotificationRequested = "notification.requested"
	// EventTypeAuditRecorded is emitted for asynchronous audit logging.
	EventTypeAuditRecorded = "audit.recorded"
	// EventTypeTriageRequested is emitted when a correlated alert group needs
	// enrichment before investigation.
	EventTypeTriageRequested = "triage.requested"
	// EventTypeInvestigationRequested is emitted when a correlated alert or
	// incident group needs AI investigation.
	EventTypeInvestigationRequested = "investigation.requested"
	// EventTypeEmailRequested is emitted when an email needs to be sent.
	EventTypeEmailRequested = "email.requested"
	// EventTypeIncidentPromoted is emitted when a triaged alert group is
	// promoted into an incident.
	EventTypeIncidentPromoted = "incident.promoted"
	// EventTypeEscalationTriggered is emitted when an incident escalation level
	// must be actioned.
	EventTypeEscalationTriggered = "escalation.triggered"
	// EventTypeSLASweepRequested is emitted on the periodic SLA sweep tick.
	EventTypeSLASweepRequested = "sla.sweep_requested"
	// EventTypeNotificationDispatched is emitted when a user notification must
	// be delivered across its configured channels.
	EventTypeNotificationDispatched = "notification.dispatched"
	// EventTypeICSProvisionRequested is emitted when an incident needs its
	// coordination space (ICS) provisioned.
	EventTypeICSProvisionRequested = "ics.provision_requested"
)
