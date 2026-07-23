package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entndlog "alga/ent/notificationdeliverylog"
)

type NotificationDeliveryRecord struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	IncidentID       *uuid.UUID `json:"incident_id,omitempty"`
	NotificationType string     `json:"notification_type"`
	Channel          string     `json:"channel"`
	Status           string     `json:"status"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type NotificationDeliveryStore interface {
	Create(ctx context.Context, record *NotificationDeliveryRecord) (*NotificationDeliveryRecord, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, skip int64) ([]NotificationDeliveryRecord, error)
	ListByIncident(ctx context.Context, incidentID uuid.UUID) ([]NotificationDeliveryRecord, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status, errMsg string) error
}

type pgNotificationDeliveryStore struct {
	pgStoreBase
}

func newPGNotificationDeliveryStore(client *ent.Client) NotificationDeliveryStore {
	return &pgNotificationDeliveryStore{pgStoreBase: pgStoreBase{client: client}}
}

func (s *pgNotificationDeliveryStore) Create(ctx context.Context, record *NotificationDeliveryRecord) (*NotificationDeliveryRecord, error) {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	b := s.client.NotificationDeliveryLog.Create().
		SetUserID(record.UserID).
		SetNotificationType(record.NotificationType).
		SetChannel(record.Channel).
		SetStatus(record.Status).
		SetErrorMessage(record.ErrorMessage).
		SetCreatedAt(record.CreatedAt)

	if record.IncidentID != nil {
		b.SetIncidentID(*record.IncidentID)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification delivery log: %w", err)
	}

	record.ID = saved.ID
	return record, nil
}

func (s *pgNotificationDeliveryStore) ListByUser(ctx context.Context, userID uuid.UUID, limit, skip int64) ([]NotificationDeliveryRecord, error) {
	if limit <= 0 {
		limit = 20
	}

	logs, err := s.client.NotificationDeliveryLog.Query().
		Where(entndlog.UserID(userID)).
		Order(ent.Desc(entndlog.FieldCreatedAt)).
		Limit(int(limit)).
		Offset(int(skip)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification delivery logs: %w", err)
	}

	records := make([]NotificationDeliveryRecord, 0, len(logs))
	for _, l := range logs {
		records = append(records, *s.toRecord(l))
	}
	return records, nil
}

func (s *pgNotificationDeliveryStore) ListByIncident(ctx context.Context, incidentID uuid.UUID) ([]NotificationDeliveryRecord, error) {
	logs, err := s.client.NotificationDeliveryLog.Query().
		Where(entndlog.IncidentID(incidentID)).
		Order(ent.Desc(entndlog.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification delivery logs by incident: %w", err)
	}

	records := make([]NotificationDeliveryRecord, 0, len(logs))
	for _, l := range logs {
		records = append(records, *s.toRecord(l))
	}
	return records, nil
}

func (s *pgNotificationDeliveryStore) UpdateStatus(ctx context.Context, id uuid.UUID, status, errMsg string) error {
	b := s.client.NotificationDeliveryLog.UpdateOneID(id).
		SetStatus(status).
		SetErrorMessage(errMsg)

	_, err := b.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update notification delivery status: %w", err)
	}
	return nil
}

func (s *pgNotificationDeliveryStore) toRecord(l *ent.NotificationDeliveryLog) *NotificationDeliveryRecord {
	return &NotificationDeliveryRecord{
		ID:               l.ID,
		UserID:           l.UserID,
		IncidentID:       l.IncidentID,
		NotificationType: l.NotificationType,
		Channel:          l.Channel,
		Status:           l.Status,
		ErrorMessage:     l.ErrorMessage,
		CreatedAt:        l.CreatedAt,
	}
}
