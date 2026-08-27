package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"go.opentelemetry.io/otel/propagation"

	"alga/logger"
	"alga/trace"
	"alga/types"
)

type Publisher struct {
	client *Client
	ch     *amqp.Channel
	chMu   sync.Mutex
}

func NewPublisher(client *Client) (*Publisher, error) {
	if err := DeclareTopology(client); err != nil {
		return nil, fmt.Errorf("failed to declare topology: %w", err)
	}

	p := &Publisher{client: client}

	client.addOnReconnect(p.resetChannel)

	return p, nil
}

func (p *Publisher) publishChannel() *amqp.Channel {
	p.chMu.Lock()
	defer p.chMu.Unlock()

	if p.ch != nil {
		return p.ch
	}

	ch, err := p.client.Channel()
	if err != nil {
		logger.Error("failed to open publish channel", "component", "rabbitmq", "error", err)
		return nil
	}
	p.ch = ch
	return ch
}

func (p *Publisher) resetChannel() {
	p.chMu.Lock()
	defer p.chMu.Unlock()
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
}

func (p *Publisher) Close() {
	p.resetChannel()
}

// prepare stamps the shared event envelope (EventID, EventType, EventVersion,
// OccurredAt, TraceID) onto a message just before it is serialized. It is
// idempotent, so a message that already carries an envelope (e.g. one being
// re-published onto a retry queue) keeps its original identity.
func (p *Publisher) prepare(ctx context.Context, m envelopeCarrier, eventType string) {
	m.ensureEnvelope(eventType, DefaultEventVersion, traceIDFromContext(ctx))
}

// MarshalAlertMessage builds the exact alert.received AMQP body the publisher
// would send, stamps the W5 envelope (assigning a stable EventID), and returns
// the marshaled bytes plus that EventID. The outbox path uses this so the
// stored payload is byte-identical to a direct publish and consumers keep their
// original idempotency key across outbox-driven redelivery.
func MarshalAlertMessage(ctx context.Context, payload types.GrafanaAlertingPayload) ([]byte, string, error) {
	msg := AlertMessage{
		Payload:    payload,
		ReceivedAt: time.Now(),
		RetryCount: 0,
	}
	msg.ensureEnvelope(EventTypeAlertReceived, DefaultEventVersion, traceIDFromContext(ctx))
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, "", fmt.Errorf("marshal alert message: %w", err)
	}
	return body, msg.EventID, nil
}

// PublishRaw publishes already-marshaled bytes to an exchange/routing key
// without re-stamping the envelope. The outbox worker uses it to republish the
// exact payload stored in an outbox row (preserving the embedded EventID).
func (p *Publisher) PublishRaw(ctx context.Context, exchange, routingKey string, body []byte) error {
	return p.publish(ctx, exchange, routingKey, body)
}

func (p *Publisher) PublishAlert(ctx context.Context, payload types.GrafanaAlertingPayload) error {
	msg := AlertMessage{
		Payload:    payload,
		ReceivedAt: time.Now(),
		RetryCount: 0,
	}
	p.prepare(ctx, &msg, EventTypeAlertReceived)
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal alert message: %w", err)
	}
	return p.publish(ctx, ExchangeAlerts, RoutingKeyAlertProcess, body)
}

func (p *Publisher) PublishEmail(ctx context.Context, msg EmailMessage) error {
	p.prepare(ctx, &msg, EventTypeEmailRequested)
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal email message: %w", err)
	}
	return p.publish(ctx, ExchangeEmail, RoutingKeyEmailSend, body)
}

func (p *Publisher) PublishInvestigation(ctx context.Context, msg InvestigateMessage) error {
	p.prepare(ctx, &msg, EventTypeInvestigationRequested)
	logger.DebugCtx(ctx, "PublishInvestigation", "component", "rabbitmq",
		"investigation_id", msg.InvestigationID, "correlation_key", msg.CorrelationKey, "trace_id", msg.TraceID)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal investigate message: %w", err)
	}
	return p.publish(ctx, ExchangeInvestigate, RoutingKeyInvestigate, body)
}

const MaxAlertRetries = 4

var alertRetryQueues = map[int]string{
	1: QueueAlertRetry1,
	2: QueueAlertRetry2,
	3: QueueAlertRetry3,
	4: QueueAlertRetry4,
}

func (p *Publisher) PublishTriage(ctx context.Context, msg TriageMessage) error {
	p.prepare(ctx, &msg, EventTypeTriageRequested)
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal triage message: %w", err)
	}
	return p.publish(ctx, ExchangeTriage, RoutingKeyTriage, body)
}

func (p *Publisher) PublishTriageRetry(ctx context.Context, msg TriageMessage) error {
	p.prepare(ctx, &msg, EventTypeTriageRequested)
	return p.publishRetry(ctx, msg.RetryCount, MaxTriageRetries, triageRetryQueues, msg)
}

func (p *Publisher) PublishAlertRetry(ctx context.Context, msg AlertMessage) error {
	p.prepare(ctx, &msg, EventTypeAlertReceived)
	logger.InfoCtx(ctx, "Requeuing alert message", "component", "rabbitmq", "retry", msg.RetryCount)
	return p.publishRetry(ctx, msg.RetryCount, MaxAlertRetries, alertRetryQueues, msg)
}

const MaxInvestigateRetries = 4

var investigateRetryQueues = map[int]string{
	1: QueueInvestigateRetry1,
	2: QueueInvestigateRetry2,
	3: QueueInvestigateRetry3,
	4: QueueInvestigateRetry4,
}

func (p *Publisher) PublishInvestigationRetry(ctx context.Context, msg InvestigateMessage) error {
	p.prepare(ctx, &msg, EventTypeInvestigationRequested)
	logger.InfoCtx(ctx, "Requeuing investigation", "component", "rabbitmq", "investigation_id", msg.InvestigationID, "retry", msg.RetryCount)
	return p.publishRetry(ctx, msg.RetryCount, MaxInvestigateRetries, investigateRetryQueues, msg)
}

func (p *Publisher) PublishIncident(ctx context.Context, msg IncidentMessage) error {
	p.prepare(ctx, &msg, EventTypeIncidentPromoted)
	return p.publishJSON(ctx, ExchangeIncident, RoutingKeyIncident, msg)
}

func (p *Publisher) PublishIncidentRetry(ctx context.Context, msg IncidentMessage) error {
	p.prepare(ctx, &msg, EventTypeIncidentPromoted)
	return p.publishRetry(ctx, msg.RetryCount, MaxIncidentRetries, incidentRetryQueues, msg)
}

func (p *Publisher) PublishEscalation(ctx context.Context, msg EscalationMessage) error {
	p.prepare(ctx, &msg, EventTypeEscalationTriggered)
	return p.publishJSON(ctx, ExchangeEscalation, RoutingKeyEscalation, msg)
}

func (p *Publisher) PublishEscalationRetry(ctx context.Context, msg EscalationMessage) error {
	p.prepare(ctx, &msg, EventTypeEscalationTriggered)
	return p.publishRetry(ctx, msg.RetryCount, MaxEscalationRetries, escalationRetryQueues, msg)
}

func (p *Publisher) PublishSLASweep(ctx context.Context, msg SLASweepMessage) error {
	p.prepare(ctx, &msg, EventTypeSLASweepRequested)
	return p.publishJSON(ctx, ExchangeSLA, RoutingKeySLASweep, msg)
}

func (p *Publisher) PublishNotificationDispatch(ctx context.Context, msg NotificationDispatchMessage) error {
	p.prepare(ctx, &msg, EventTypeNotificationDispatched)
	return p.publishJSON(ctx, ExchangeNotificationDispatch, RoutingKeyNotificationDispatch, msg)
}

func (p *Publisher) PublishNotificationDispatchRetry(ctx context.Context, msg NotificationDispatchMessage) error {
	p.prepare(ctx, &msg, EventTypeNotificationDispatched)
	return p.publishRetry(ctx, msg.RetryCount, MaxNotificationDispatchRetries, notificationDispatchRetryQueues, msg)
}

func (p *Publisher) PublishICSProvision(ctx context.Context, msg ICSProvisionMessage) error {
	p.prepare(ctx, &msg, EventTypeICSProvisionRequested)
	return p.publishJSON(ctx, ExchangeICSProvision, RoutingKeyICSProvision, msg)
}

func (p *Publisher) publishJSON(ctx context.Context, exchange, routingKey string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	return p.publish(ctx, exchange, routingKey, body)
}

var ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")

func (p *Publisher) doPublish(ctx context.Context, exchange, routingKey string, body []byte) error {
	ch := p.publishChannel()
	if ch == nil {
		return errors.New("no available channel")
	}
	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}
	if trace.Enabled() {
		pub.Headers = injectTraceHeaders(ctx, trace.Propagator(), nil)
	}
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ch.PublishWithContext(pubCtx, exchange, routingKey, false, false, pub); err != nil {
		p.resetChannel()
		return fmt.Errorf("publish to %s/%s: %w", exchange, routingKey, err)
	}
	return nil
}

// injectTraceHeaders injects the active W3C trace context into the given AMQP
// header table using the provided propagator. The caller is responsible for the
// trace.Enabled() guard so the hot publish path pays nothing when no OTLP
// collector is configured; this helper only performs the injection. The returned
// table is safe to assign to amqp.Publishing.Headers.
func injectTraceHeaders(ctx context.Context, propagator propagation.TextMapPropagator, headers amqp.Table) amqp.Table {
	carrier := NewAMQPCarrier(headers)
	propagator.Inject(ctx, carrier)
	return amqp.Table(*carrier)
}

func (p *Publisher) publishToQueue(ctx context.Context, queue string, body []byte) error {
	return p.doPublish(ctx, "", queue, body)
}

func (p *Publisher) publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	return p.doPublish(ctx, exchange, routingKey, body)
}

func (p *Publisher) publishRetry(ctx context.Context, retryCount, maxRetries int, queues map[int]string, msg any) error {
	if retryCount > maxRetries {
		return fmt.Errorf("retry %d > max %d: %w", retryCount, maxRetries, ErrMaxRetriesExceeded)
	}
	queue, ok := queues[retryCount]
	if !ok {
		return fmt.Errorf("no retry queue for attempt %d", retryCount)
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal retry message: %w", err)
	}
	return p.publishToQueue(ctx, queue, body)
}
