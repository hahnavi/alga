package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
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
	// DeleteOlderThan purges delivery-log rows older than the cutoff in
	// bounded batches (DT-E3 retention family).
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type pgNotificationDeliveryStore struct {
	pgStoreBase
}

func newPGNotificationDeliveryStore(db *bun.DB) NotificationDeliveryStore {
	return &pgNotificationDeliveryStore{pgStoreBase: pgStoreBase{db: db}}
}

func (s *pgNotificationDeliveryStore) Create(ctx context.Context, record *NotificationDeliveryRecord) (*NotificationDeliveryRecord, error) {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	m := &models.NotificationDeliveryLog{
		ID:               models.NewUUID(),
		UserID:           record.UserID,
		IncidentID:       record.IncidentID,
		NotificationType: record.NotificationType,
		Channel:          record.Channel,
		Status:           record.Status,
		ErrorMessage:     record.ErrorMessage,
		CreatedAt:        record.CreatedAt,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification delivery log: %w", err)
	}

	record.ID = m.ID
	return record, nil
}

func (s *pgNotificationDeliveryStore) ListByUser(ctx context.Context, userID uuid.UUID, limit, skip int64) ([]NotificationDeliveryRecord, error) {
	if limit <= 0 {
		limit = 20
	}

	var logs []models.NotificationDeliveryLog
	err := s.db.NewSelect().Model(&logs).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(int(limit)).
		Offset(int(skip)).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification delivery logs: %w", err)
	}

	records := make([]NotificationDeliveryRecord, 0, len(logs))
	for i := range logs {
		records = append(records, *s.toRecord(&logs[i]))
	}
	return records, nil
}

func (s *pgNotificationDeliveryStore) ListByIncident(ctx context.Context, incidentID uuid.UUID) ([]NotificationDeliveryRecord, error) {
	var logs []models.NotificationDeliveryLog
	err := s.db.NewSelect().Model(&logs).
		Where("incident_id = ?", incidentID).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification delivery logs by incident: %w", err)
	}

	records := make([]NotificationDeliveryRecord, 0, len(logs))
	for i := range logs {
		records = append(records, *s.toRecord(&logs[i]))
	}
	return records, nil
}

func (s *pgNotificationDeliveryStore) UpdateStatus(ctx context.Context, id uuid.UUID, status, errMsg string) error {
	_, err := s.db.NewUpdate().Model((*models.NotificationDeliveryLog)(nil)).
		Set("status = ?", status).
		Set("error_message = ?", errMsg).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update notification delivery status: %w", err)
	}
	return nil
}

func (s *pgNotificationDeliveryStore) toRecord(l *models.NotificationDeliveryLog) *NotificationDeliveryRecord {
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

// DeleteOlderThan hard-deletes delivery-log rows older than the cutoff in
// bounded batches (DT-E3 retention family; diagnostic append-only data riding
// DATA_RETENTION_DAYS).
func (s *pgNotificationDeliveryStore) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return deleteOlderThanBatched[models.NotificationDeliveryLog](ctx, s.db, "created_at", cutoff)
}
