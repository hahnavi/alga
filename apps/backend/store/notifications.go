package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type NotificationRecord struct {
	ID                     uuid.UUID `json:"id"`
	UserID                 string    `json:"user_id"`
	Type                   string    `json:"type"`
	Title                  string    `json:"title"`
	Message                string    `json:"message"`
	Read                   bool      `json:"read"`
	ResourceType           string    `json:"resource_type"`
	ResourceID             string    `json:"resource_id"`
	TriggeredByUserID      string    `json:"triggered_by_user_id,omitempty"`
	TriggeredByDisplayName string    `json:"triggered_by_display_name,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

type NotificationStore interface {
	Create(ctx context.Context, n *NotificationRecord) (*NotificationRecord, error)
	ListByUser(ctx context.Context, userID string, limit, skip int64) ([]NotificationRecord, error)
	GetUnreadCount(ctx context.Context, userID string) (int64, error)
	MarkRead(ctx context.Context, userID, id string) error
	MarkAllRead(ctx context.Context, userID string) error
}

type pgNotificationStore struct {
	pgStoreBase
}

func newPGNotificationStore(db *bun.DB) NotificationStore {
	return &pgNotificationStore{pgStoreBase{db: db}}
}

func (s *pgNotificationStore) Create(ctx context.Context, n *NotificationRecord) (*NotificationRecord, error) {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	var resourceType *string
	if n.ResourceType != "" {
		resourceType = &n.ResourceType
	}

	uid, err := uuid.Parse(n.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %s", n.UserID)
	}

	var triggeredBy *uuid.UUID
	if n.TriggeredByUserID != "" {
		tid, err := uuid.Parse(n.TriggeredByUserID)
		if err != nil {
			return nil, fmt.Errorf("invalid triggered_by user id: %s", n.TriggeredByUserID)
		}
		triggeredBy = &tid
	}

	m := &models.Notification{
		ID:                     models.NewUUID(),
		UserID:                 uid,
		Type:                   n.Type,
		Title:                  n.Title,
		Message:                n.Message,
		Read:                   n.Read,
		ResourceType:           resourceType,
		ResourceID:             n.ResourceID,
		TriggeredByUserID:      triggeredBy,
		TriggeredByDisplayName: n.TriggeredByDisplayName,
		CreatedAt:              n.CreatedAt,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	n.ID = m.ID
	return n, nil
}

func (s *pgNotificationStore) ListByUser(ctx context.Context, userID string, limit, skip int64) ([]NotificationRecord, error) {
	if limit <= 0 {
		limit = 20
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %s", userID)
	}

	var nfns []models.Notification
	err = s.db.NewSelect().Model(&nfns).
		Where("user_id = ?", uid).
		Order("created_at DESC").
		Limit(int(limit)).
		Offset(int(skip)).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}

	var records []NotificationRecord
	for i := range nfns {
		n := &nfns[i]
		resourceType := ""
		if n.ResourceType != nil {
			resourceType = *n.ResourceType
		}
		triggeredBy := ""
		if n.TriggeredByUserID != nil {
			triggeredBy = n.TriggeredByUserID.String()
		}
		records = append(records, NotificationRecord{
			ID:                     n.ID,
			UserID:                 n.UserID.String(),
			Type:                   n.Type,
			Title:                  n.Title,
			Message:                n.Message,
			Read:                   n.Read,
			ResourceType:           resourceType,
			ResourceID:             n.ResourceID,
			TriggeredByUserID:      triggeredBy,
			TriggeredByDisplayName: n.TriggeredByDisplayName,
			CreatedAt:              n.CreatedAt,
		})
	}
	if records == nil {
		records = []NotificationRecord{}
	}
	return records, nil
}

func (s *pgNotificationStore) GetUnreadCount(ctx context.Context, userID string) (int64, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return 0, fmt.Errorf("invalid user id: %s", userID)
	}

	count, err := s.db.NewSelect().Model((*models.Notification)(nil)).
		Where("user_id = ?", uid).
		Where("\"read\" = ?", false).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}
	return int64(count), nil
}

func (s *pgNotificationStore) MarkRead(ctx context.Context, userID, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid notification id: %s", id)
	}

	userUID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user id: %s", userID)
	}

	res, err := s.db.NewUpdate().Model((*models.Notification)(nil)).
		Set("\"read\" = ?", true).
		Where("id = ?", uid).
		Where("user_id = ?", userUID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark notification read: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to mark notification read: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("notification not found: %w", ErrNotificationNotFound)
	}
	return nil
}

func (s *pgNotificationStore) MarkAllRead(ctx context.Context, userID string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user id: %s", userID)
	}

	_, err = s.db.NewUpdate().Model((*models.Notification)(nil)).
		Set("\"read\" = ?", true).
		Where("user_id = ?", uid).
		Where("\"read\" = ?", false).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark all notifications read: %w", err)
	}
	return nil
}
