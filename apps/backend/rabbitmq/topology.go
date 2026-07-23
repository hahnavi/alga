package rabbitmq

import (
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/logger"
)

const (
	ExchangeAlerts        = "alga.alerts"
	ExchangeNotifications = "alga.notifications"
	ExchangeAudit         = "alga.audit"
	ExchangeDLX           = "alga.dlx"

	QueueAlertProcess     = "alga.alert.process"
	QueueNotificationSend = "alga.notification.send"
	QueueAuditLog         = "alga.audit.log"
	QueueDeadLetter       = "alga.dead_letter"

	QueueAlertRetry1 = "alga.alert.retry.1"
	QueueAlertRetry2 = "alga.alert.retry.2"
	QueueAlertRetry3 = "alga.alert.retry.3"
	QueueAlertRetry4 = "alga.alert.retry.4"

	RoutingKeyAlertProcess     = "process"
	RoutingKeyNotificationSend = "send"

	ExchangeEmail       = "alga.email"
	QueueEmailSend      = "alga.email.send"
	RoutingKeyEmailSend = "send"

	ExchangeInvestigate     = "alga.investigate"
	QueueInvestigateProcess = "alga.investigate.process"
	QueueInvestigateRetry1  = "alga.investigate.retry.1"
	QueueInvestigateRetry2  = "alga.investigate.retry.2"
	QueueInvestigateRetry3  = "alga.investigate.retry.3"
	QueueInvestigateRetry4  = "alga.investigate.retry.4"
	RoutingKeyInvestigate   = "process"

	ExchangeTriage     = "alga.triage"
	QueueTriageProcess = "alga.triage.process"
	QueueTriageRetry1  = "alga.triage.retry.1"
	QueueTriageRetry2  = "alga.triage.retry.2"
	QueueTriageRetry3  = "alga.triage.retry.3"
	QueueTriageRetry4  = "alga.triage.retry.4"
	RoutingKeyTriage   = "process"

	ExchangeIncident     = "alga.incidents"
	QueueIncidentProcess = "alga.incident.process"
	QueueIncidentRetry1  = "alga.incident.retry.1"
	QueueIncidentRetry2  = "alga.incident.retry.2"
	QueueIncidentRetry3  = "alga.incident.retry.3"
	QueueIncidentRetry4  = "alga.incident.retry.4"
	RoutingKeyIncident   = "incident.process"

	ExchangeEscalation     = "alga.escalation"
	QueueEscalationProcess = "alga.escalation.process"
	QueueEscalationRetry1  = "alga.escalation.retry.1"
	QueueEscalationRetry2  = "alga.escalation.retry.2"
	QueueEscalationRetry3  = "alga.escalation.retry.3"
	QueueEscalationRetry4  = "alga.escalation.retry.4"
	RoutingKeyEscalation   = "escalation.process"

	ExchangeSLA        = "alga.sla"
	QueueSLASweep      = "alga.sla.sweep"
	RoutingKeySLASweep = "sla.sweep"

	ExchangeNotificationDispatch     = "alga.notification-dispatch"
	QueueNotificationDispatchProcess = "alga.notification-dispatch.process"
	QueueNotificationDispatchRetry1  = "alga.notification-dispatch.retry.1"
	QueueNotificationDispatchRetry2  = "alga.notification-dispatch.retry.2"
	QueueNotificationDispatchRetry3  = "alga.notification-dispatch.retry.3"
	QueueNotificationDispatchRetry4  = "alga.notification-dispatch.retry.4"
	RoutingKeyNotificationDispatch   = "notification-dispatch.process"

	ExchangeICSProvision   = "alga.ics-provision"
	QueueICSProvision      = "alga.ics-provision.process"
	RoutingKeyICSProvision = "ics-provision.process"
)

// declareTopology sets up all exchanges, queues, and bindings.
func declareTopology(c *Client) error {
	logger.Info("declaring RabbitMQ topology", "component", "rabbitmq")

	ch, err := c.Channel()
	if err != nil {
		return fmt.Errorf("open channel for topology: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.ExchangeDeclare(ExchangeDLX, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare DLX exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare DLX exchange: %w", err)
	}

	// Main exchanges
	for name, kind := range map[string]string{
		ExchangeAlerts:        "direct",
		ExchangeNotifications: "direct",
		ExchangeAudit:         "fanout",
	} {
		if err := ch.ExchangeDeclare(name, kind, true, false, false, false, nil); err != nil {
			logger.Error("failed to declare exchange", "component", "rabbitmq", "exchange", name, "error", err)
			return fmt.Errorf("declare exchange %s: %w", name, err)
		}
	}

	if _, err := ch.QueueDeclare(QueueDeadLetter, true, false, false, false, nil); err != nil {
		logger.Error("failed to declare dead letter queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare dead letter queue: %w", err)
	}
	if err := ch.QueueBind(QueueDeadLetter, "#", ExchangeDLX, false, nil); err != nil {
		logger.Error("failed to bind dead letter queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind dead letter queue: %w", err)
	}

	// Main queues with DLX
	mainQueues := map[string]struct {
		exchange   string
		routingKey string
	}{
		QueueAlertProcess:     {ExchangeAlerts, RoutingKeyAlertProcess},
		QueueNotificationSend: {ExchangeNotifications, RoutingKeyNotificationSend},
		QueueAuditLog:         {ExchangeAudit, ""},
	}

	for name, cfg := range mainQueues {
		args := amqp.Table{
			"x-dead-letter-exchange": ExchangeDLX,
		}
		if _, err := ch.QueueDeclare(name, true, false, false, false, args); err != nil {
			logger.Error("failed to declare queue", "component", "rabbitmq", "queue", name, "error", err)
			return fmt.Errorf("declare queue %s: %w", name, err)
		}
		if cfg.routingKey != "" {
			if err := ch.QueueBind(name, cfg.routingKey, cfg.exchange, false, nil); err != nil {
				logger.Error("failed to bind queue", "component", "rabbitmq", "queue", name, "error", err)
				return fmt.Errorf("bind queue %s: %w", name, err)
			}
		} else {
			if err := ch.QueueBind(name, "", cfg.exchange, false, nil); err != nil {
				logger.Error("failed to bind queue", "component", "rabbitmq", "queue", name, "error", err)
				return fmt.Errorf("bind queue %s: %w", name, err)
			}
		}
	}

	// Retry queues with TTL
	retries := []struct {
		name string
		ttl  int32 // milliseconds
	}{
		{QueueAlertRetry1, retryTTLms(0)},
		{QueueAlertRetry2, retryTTLms(1)},
		{QueueAlertRetry3, retryTTLms(2)},
		{QueueAlertRetry4, retryTTLms(3)},
	}

	for _, r := range retries {
		ch, err = declareRetryQueue(c, ch, r.name, r.ttl, ExchangeAlerts, RoutingKeyAlertProcess)
		if err != nil {
			return err
		}
		// Note: do NOT bind retry queues to the exchange. They receive messages
		// only via DLX when the main queue nacks. Binding them causes every
		// published message to also land in the retry queue, where TTL expiry
		// re-delivers it as a duplicate.
	}

	// Email exchange
	if err := ch.ExchangeDeclare(ExchangeEmail, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare email exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare email exchange: %w", err)
	}
	emailArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueEmailSend, true, false, false, false, emailArgs); err != nil {
		logger.Error("failed to declare email queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare email queue: %w", err)
	}
	if err := ch.QueueBind(QueueEmailSend, RoutingKeyEmailSend, ExchangeEmail, false, nil); err != nil {
		logger.Error("failed to bind email queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind email queue: %w", err)
	}

	// Investigate exchange
	if err := ch.ExchangeDeclare(ExchangeInvestigate, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare investigate exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare investigate exchange: %w", err)
	}

	// Investigate main queue
	invArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueInvestigateProcess, true, false, false, false, invArgs); err != nil {
		logger.Error("failed to declare investigate queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare investigate queue: %w", err)
	}
	if err := ch.QueueBind(QueueInvestigateProcess, RoutingKeyInvestigate, ExchangeInvestigate, false, nil); err != nil {
		logger.Error("failed to bind investigate queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind investigate queue: %w", err)
	}

	// Investigate retry queues
	invRetries := []struct {
		name string
		ttl  int32
	}{
		{QueueInvestigateRetry1, retryTTLms(0)},
		{QueueInvestigateRetry2, retryTTLms(1)},
		{QueueInvestigateRetry3, retryTTLms(2)},
		{QueueInvestigateRetry4, retryTTLms(3)},
	}

	for _, r := range invRetries {
		ch, err = declareRetryQueue(c, ch, r.name, r.ttl, ExchangeInvestigate, RoutingKeyInvestigate)
		if err != nil {
			return err
		}
		// Note: do NOT bind retry queues to the exchange. They receive messages
		// only via DLX when the main queue nacks. Binding them causes every
		// published message to also land in the retry queue, where TTL expiry
		// re-delivers it as a duplicate.
	}

	// Triage exchange
	if err := ch.ExchangeDeclare(ExchangeTriage, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare triage exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare triage exchange: %w", err)
	}

	// Triage main queue
	triageArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueTriageProcess, true, false, false, false, triageArgs); err != nil {
		logger.Error("failed to declare triage queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare triage queue: %w", err)
	}
	if err := ch.QueueBind(QueueTriageProcess, RoutingKeyTriage, ExchangeTriage, false, nil); err != nil {
		logger.Error("failed to bind triage queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind triage queue: %w", err)
	}

	// Triage retry queues
	triageRetries := []struct {
		name string
		ttl  int32
	}{
		{QueueTriageRetry1, retryTTLms(0)},
		{QueueTriageRetry2, retryTTLms(1)},
		{QueueTriageRetry3, retryTTLms(2)},
		{QueueTriageRetry4, retryTTLms(3)},
	}

	for _, r := range triageRetries {
		ch, err = declareRetryQueue(c, ch, r.name, r.ttl, ExchangeTriage, RoutingKeyTriage)
		if err != nil {
			return err
		}
		// Note: do NOT bind retry queues to the exchange. They receive messages
		// only via DLX when the main queue nacks. Binding them causes every
		// published message to also land in the retry queue, where TTL expiry
		// re-delivers it as a duplicate.
	}

	// Incident exchange
	if err := ch.ExchangeDeclare(ExchangeIncident, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare incident exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare incident exchange: %w", err)
	}

	incidentArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueIncidentProcess, true, false, false, false, incidentArgs); err != nil {
		logger.Error("failed to declare incident queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare incident queue: %w", err)
	}
	if err := ch.QueueBind(QueueIncidentProcess, RoutingKeyIncident, ExchangeIncident, false, nil); err != nil {
		logger.Error("failed to bind incident queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind incident queue: %w", err)
	}

	incidentRetries := []struct {
		name string
		ttl  int32
	}{
		{QueueIncidentRetry1, retryTTLms(0)},
		{QueueIncidentRetry2, retryTTLms(1)},
		{QueueIncidentRetry3, retryTTLms(2)},
		{QueueIncidentRetry4, retryTTLms(3)},
	}

	for _, r := range incidentRetries {
		ch, err = declareRetryQueue(c, ch, r.name, r.ttl, ExchangeIncident, RoutingKeyIncident)
		if err != nil {
			return err
		}
	}

	// Escalation exchange
	if err := ch.ExchangeDeclare(ExchangeEscalation, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare escalation exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare escalation exchange: %w", err)
	}

	escArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueEscalationProcess, true, false, false, false, escArgs); err != nil {
		logger.Error("failed to declare escalation queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare escalation queue: %w", err)
	}
	if err := ch.QueueBind(QueueEscalationProcess, RoutingKeyEscalation, ExchangeEscalation, false, nil); err != nil {
		logger.Error("failed to bind escalation queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind escalation queue: %w", err)
	}

	escRetries := []struct {
		name string
		ttl  int32
	}{
		{QueueEscalationRetry1, retryTTLms(0)},
		{QueueEscalationRetry2, retryTTLms(1)},
		{QueueEscalationRetry3, retryTTLms(2)},
		{QueueEscalationRetry4, retryTTLms(3)},
	}

	for _, r := range escRetries {
		ch, err = declareRetryQueue(c, ch, r.name, r.ttl, ExchangeEscalation, RoutingKeyEscalation)
		if err != nil {
			return err
		}
	}

	// SLA exchange
	if err := ch.ExchangeDeclare(ExchangeSLA, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare SLA exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare SLA exchange: %w", err)
	}

	slaArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueSLASweep, true, false, false, false, slaArgs); err != nil {
		logger.Error("failed to declare SLA sweep queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare SLA sweep queue: %w", err)
	}
	if err := ch.QueueBind(QueueSLASweep, RoutingKeySLASweep, ExchangeSLA, false, nil); err != nil {
		logger.Error("failed to bind SLA sweep queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind SLA sweep queue: %w", err)
	}

	// Notification dispatch exchange
	if err := ch.ExchangeDeclare(ExchangeNotificationDispatch, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare notification-dispatch exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare notification-dispatch exchange: %w", err)
	}

	ndArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueNotificationDispatchProcess, true, false, false, false, ndArgs); err != nil {
		logger.Error("failed to declare notification-dispatch queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare notification-dispatch queue: %w", err)
	}
	if err := ch.QueueBind(QueueNotificationDispatchProcess, RoutingKeyNotificationDispatch, ExchangeNotificationDispatch, false, nil); err != nil {
		logger.Error("failed to bind notification-dispatch queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind notification-dispatch queue: %w", err)
	}

	ndRetries := []struct {
		name string
		ttl  int32
	}{
		{QueueNotificationDispatchRetry1, retryTTLms(0)},
		{QueueNotificationDispatchRetry2, retryTTLms(1)},
		{QueueNotificationDispatchRetry3, retryTTLms(2)},
		{QueueNotificationDispatchRetry4, retryTTLms(3)},
	}

	for _, r := range ndRetries {
		ch, err = declareRetryQueue(c, ch, r.name, r.ttl, ExchangeNotificationDispatch, RoutingKeyNotificationDispatch)
		if err != nil {
			return err
		}
	}

	if err := declareICSProvision(ch); err != nil {
		return err
	}

	logger.Info("RabbitMQ topology declared successfully", "component", "rabbitmq")
	return nil
}

// declareRetryQueue declares a retry queue carrying the given message TTL and
// dead-letter wiring. It is resilient to an already-declared queue whose
// arguments (e.g. x-message-ttl) now differ from what the unified RetrySchedule
// dictates: RabbitMQ rejects such a redeclare with PRECONDITION_FAILED (406),
// which previously took the worker down at startup on clusters that had the
// queue declared under the old literals. We detect that specific condition,
// delete the stale queue (its bindings go with it — retry queues are never bound
// to an exchange), and redeclare with the intended args. Any other error, or a
// second PreconditionFailed after the delete, is returned unchanged. A healthy
// queue whose args already match is left untouched.
//
// A PRECONDITION_FAILED response also closes the AMQP channel, so the delete and
// redeclare must happen on a fresh channel; ch is returned so the caller can
// continue using the live channel for subsequent declarations.
func declareRetryQueue(c *Client, ch *amqp.Channel, name string, ttlMs int32, dlx, dlxRoutingKey string) (*amqp.Channel, error) {
	args := amqp.Table{
		"x-dead-letter-exchange":    dlx,
		"x-dead-letter-routing-key": dlxRoutingKey,
		"x-message-ttl":             ttlMs,
	}
	if _, err := ch.QueueDeclare(name, true, false, false, false, args); err != nil {
		if !isPreconditionFailed(err) {
			return ch, fmt.Errorf("declare retry queue %s: %w", name, err)
		}
		logger.Warn("retry queue args changed; deleting and redeclaring",
			"component", "rabbitmq", "queue", name, "error", err)

		// The broker has closed ch with the 406; open a fresh channel to recover.
		_ = ch.Close()
		reCh, dErr := c.Channel()
		if dErr != nil {
			return ch, fmt.Errorf("open recovery channel for retry queue %s: %w", name, dErr)
		}
		if _, dErr := reCh.QueueDelete(name, false, false, false); dErr != nil {
			_ = reCh.Close()
			return ch, fmt.Errorf("delete stale retry queue %s: %w", name, dErr)
		}
		if _, dErr := reCh.QueueDeclare(name, true, false, false, false, args); dErr != nil {
			_ = reCh.Close()
			return ch, fmt.Errorf("redeclare retry queue %s: %w", name, dErr)
		}
		logger.Info("retry queue redeclared with updated args", "component", "rabbitmq", "queue", name)
		return reCh, nil
	}
	return ch, nil
}

// isPreconditionFailed reports whether err is an AMQP PRECONDITION_FAILED (406),
// the code RabbitMQ returns when a queue/exchange is redeclared with arguments
// that conflict with its existing definition.
func isPreconditionFailed(err error) bool {
	var amqpErr *amqp.Error
	if errors.As(err, &amqpErr) {
		return amqpErr.Code == amqp.PreconditionFailed
	}
	return false
}

func declareICSProvision(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(ExchangeICSProvision, "direct", true, false, false, false, nil); err != nil {
		logger.Error("failed to declare ICS provision exchange", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare ICS provision exchange: %w", err)
	}

	icsArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	if _, err := ch.QueueDeclare(QueueICSProvision, true, false, false, false, icsArgs); err != nil {
		logger.Error("failed to declare ICS provision queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("declare ICS provision queue: %w", err)
	}
	if err := ch.QueueBind(QueueICSProvision, RoutingKeyICSProvision, ExchangeICSProvision, false, nil); err != nil {
		logger.Error("failed to bind ICS provision queue", "component", "rabbitmq", "error", err)
		return fmt.Errorf("bind ICS provision queue: %w", err)
	}
	return nil
}
