package worker

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"alga/cancellation"
	"alga/logger"
	"alga/notification"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

type NotificationDispatchWorker struct {
	notificationStore store.NotificationStore
	dispatcher        *notification.Dispatcher
	ssePublisher      *sse.DualPublisher
	publisher         *rabbitmq.Publisher
	incidentStore     store.IncidentStore
	cancelSet         *valkey.CancelSet
}

func NewNotificationDispatchWorker(notificationStore store.NotificationStore, dispatcher *notification.Dispatcher, ssePublisher *sse.DualPublisher, publisher *rabbitmq.Publisher) *NotificationDispatchWorker {
	return &NotificationDispatchWorker{
		notificationStore: notificationStore,
		dispatcher:        dispatcher,
		ssePublisher:      ssePublisher,
		publisher:         publisher,
	}
}

func (w *NotificationDispatchWorker) SetCancelSet(cs *valkey.CancelSet)       { w.cancelSet = cs }
func (w *NotificationDispatchWorker) SetIncidentStore(is store.IncidentStore) { w.incidentStore = is }

func (w *NotificationDispatchWorker) Queue() string      { return rabbitmq.QueueNotificationDispatchProcess }
func (w *NotificationDispatchWorker) PrefetchCount() int { return 10 }

func (w *NotificationDispatchWorker) Handle(ctx context.Context, d amqp.Delivery) {
	var msg rabbitmq.NotificationDispatchMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Notification dispatch worker unmarshal failed", "component", "notification-dispatch", "error", err)
		_ = d.Nack(false, false)
		return
	}

	logger.Info("Processing notification dispatch", "component", "notification-dispatch", "user_id", msg.UserID, "type", msg.NotificationType, "retry", msg.RetryCount)

	if msg.IncidentNumber != 0 && cancellation.IncidentCancelled(ctx, w.cancelSet, w.incidentStore, msg.IncidentNumber) {
		logger.Info("Dropping notification dispatch; incident deleted", "component", "notification-dispatch", "incident_number", msg.IncidentNumber)
		_ = d.Ack(false)
		return
	}

	var saved *store.NotificationRecord

	if msg.NotificationID != "" {
		nid, err := uuid.Parse(msg.NotificationID)
		if err != nil {
			// A non-parseable notification id is a producer-side contract
			// violation. Retrying won't fix it; dead-letter so the bad
			// message doesn't loop forever.
			logger.Error("Notification dispatch received malformed notification_id; dead-lettering", "component", "notification-dispatch", "notification_id", msg.NotificationID, "error", err)
			_ = d.Nack(false, false)
			return
		}
		saved = &store.NotificationRecord{
			ID:           nid,
			UserID:       msg.UserID,
			Type:         msg.NotificationType,
			Title:        msg.Title,
			Message:      msg.Message,
			ResourceType: msg.ResourceType,
			ResourceID:   msg.ResourceID,
			Read:         true,
		}
	} else {
		record := &store.NotificationRecord{
			UserID:       msg.UserID,
			Type:         msg.NotificationType,
			Title:        msg.Title,
			Message:      msg.Message,
			ResourceType: msg.ResourceType,
			ResourceID:   msg.ResourceID,
			Read:         false,
		}

		var err error
		saved, err = w.notificationStore.Create(ctx, record)
		if err != nil {
			logger.Error("Failed to create notification record for user", "component", "notification-dispatch", "user_id", msg.UserID, "error", err)
			w.retryOrDeadLetter(ctx, d, msg)
			return
		}
		msg.NotificationID = saved.ID.String()
	}

	if w.ssePublisher != nil && msg.NotificationID != "" && saved.ID != uuid.Nil {
		w.ssePublisher.PublishToUser(msg.UserID, sse.Event{
			Type: "notification",
			Data: map[string]any{
				"id":            saved.ID.String(),
				"type":          saved.Type,
				"title":         saved.Title,
				"message":       saved.Message,
				"resource_type": saved.ResourceType,
				"resource_id":   saved.ResourceID,
				"created_at":    saved.CreatedAt,
			},
		})
	}

	if len(msg.Channels) > 0 {
		if err := w.dispatcher.DispatchChannels(ctx, msg.UserID, msg.NotificationType, msg.Title, msg.Message, msg.ResourceType, msg.ResourceID, msg.Channels, nil, msg.IncidentNumber, msg.Level); err != nil {
			logger.Error("Failed to dispatch notification for user", "component", "notification-dispatch", "user_id", msg.UserID, "error", err)
			w.retryOrDeadLetter(ctx, d, msg)
			return
		}
	} else {
		if err := w.dispatcher.Dispatch(ctx, msg.UserID, msg.NotificationType, msg.Title, msg.Message, msg.ResourceType, msg.ResourceID, nil); err != nil {
			logger.Error("Failed to dispatch notification for user", "component", "notification-dispatch", "user_id", msg.UserID, "error", err)
			w.retryOrDeadLetter(ctx, d, msg)
			return
		}
	}

	_ = d.Ack(false)
}

func (w *NotificationDispatchWorker) retryOrDeadLetter(ctx context.Context, d amqp.Delivery, msg rabbitmq.NotificationDispatchMessage) {
	if w.publisher == nil {
		_ = d.Nack(false, false)
		return
	}
	msg.RetryCount++
	if msg.RetryCount <= rabbitmq.MaxNotificationDispatchRetries {
		if err := w.publisher.PublishNotificationDispatchRetry(ctx, msg); err != nil {
			logger.Error("Failed to publish notification dispatch retry", "component", "notification-dispatch", "error", err)
			_ = d.Nack(false, false)
			return
		}
		_ = d.Ack(false)
		return
	}
	_ = d.Nack(false, false)
}
