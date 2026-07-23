package worker

import (
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/logger"
	"alga/rabbitmq"
	"alga/store"
	"alga/webhook"
)

type AlertWorker struct {
	receiver   *webhook.Receiver
	alertStore store.Store
	publisher  *rabbitmq.Publisher
}

func NewAlertWorker(receiver *webhook.Receiver, alertStore store.Store, publisher *rabbitmq.Publisher) *AlertWorker {
	return &AlertWorker{
		receiver:   receiver,
		alertStore: alertStore,
		publisher:  publisher,
	}
}

func (w *AlertWorker) Queue() string {
	return rabbitmq.QueueAlertProcess
}

func (w *AlertWorker) Handle(ctx context.Context, d amqp.Delivery) {
	var msg rabbitmq.AlertMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Failed to unmarshal alert message", "component", "alert-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	if err := w.receiver.ProcessAlerts(ctx, msg.Payload); err != nil {
		w.handleRetry(ctx, d, msg, err)
		return
	}

	_ = d.Ack(false)
}

func (w *AlertWorker) PrefetchCount() int {
	return 10
}

func (w *AlertWorker) handleRetry(ctx context.Context, d amqp.Delivery, msg rabbitmq.AlertMessage, processErr error) {
	if w.publisher == nil {
		logger.Warn("No publisher available for alert retry; dead-lettering", "component", "alert-worker", "fingerprint_count", len(msg.Payload.Alerts))
		_ = d.Nack(false, false)
		return
	}

	msg.RetryCount++
	if msg.RetryCount > rabbitmq.MaxAlertRetries {
		logger.Error("Alert processing failed after max retries; dead-lettering", "component", "alert-worker", "max_retries", rabbitmq.MaxAlertRetries, "alert_count", len(msg.Payload.Alerts), "error", processErr)
		_ = d.Nack(false, false)
		return
	}

	retryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := w.publisher.PublishAlertRetry(retryCtx, msg); err != nil {
		logger.Error("Failed to publish alert to retry queue; requeuing immediately", "component", "alert-worker", "retry", msg.RetryCount, "error", err)
		_ = d.Nack(false, true)
		return
	}

	logger.Info("Alert processing failed; queued for retry", "component", "alert-worker", "retry", msg.RetryCount, "max_retries", rabbitmq.MaxAlertRetries, "error", processErr)
	_ = d.Ack(false)
}

type AuditWorker struct {
	auditStore store.AuditStore
}

func NewAuditWorker(auditStore store.AuditStore) *AuditWorker {
	return &AuditWorker{
		auditStore: auditStore,
	}
}

func (w *AuditWorker) Queue() string {
	return rabbitmq.QueueAuditLog
}

func (w *AuditWorker) Handle(ctx context.Context, d amqp.Delivery) {
	var msg AuditDetails
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Failed to unmarshal audit message", "component", "audit-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	w.auditStore.Log(store.AuditEvent(msg.Event), nil, msg.Username, msg.IP, msg.UserAgent, msg.Success, msg.Details)

	_ = d.Ack(false)
}

func (w *AuditWorker) PrefetchCount() int {
	return 10
}

type AuditDetails struct {
	Event     string         `json:"event"`
	UserID    *string        `json:"user_id,omitempty"`
	Username  string         `json:"username,omitempty"`
	IP        string         `json:"ip,omitempty"`
	UserAgent string         `json:"user_agent,omitempty"`
	Success   bool           `json:"success"`
	Details   map[string]any `json:"details,omitempty"`
}

type NotificationWorker struct {
	receiver *webhook.Receiver
}

func NewNotificationWorker(receiver *webhook.Receiver) *NotificationWorker {
	return &NotificationWorker{
		receiver: receiver,
	}
}

func (w *NotificationWorker) Queue() string {
	return rabbitmq.QueueNotificationSend
}

func (w *NotificationWorker) Handle(ctx context.Context, d amqp.Delivery) {
	logger.DebugCtx(ctx, "notification worker: message consumed (handled by notification_dispatch worker)")
	_ = d.Ack(false)
}

func (w *NotificationWorker) PrefetchCount() int {
	return 10
}
