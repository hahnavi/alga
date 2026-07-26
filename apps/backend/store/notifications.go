package store

import (
	"context"
	"fmt"
	"time"

	"alga/ent"
	"alga/ent/notification"

	"github.com/google/uuid"
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

func newPGNotificationStore(client *ent.Client) NotificationStore {
	return &pgNotificationStore{pgStoreBase{client: client}}
}

func (s *pgNotificationStore) Create(ctx context.Context, n *NotificationRecord) (*NotificationRecord, error) {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	saved, err := s.client.Notification.Create().
		SetUserID(n.UserID).
		SetType(notification.Type(n.Type)).
		SetTitle(n.Title).
		SetMessage(n.Message).
		SetRead(n.Read).
		SetResourceType(notification.ResourceType(n.ResourceType)).
		SetResourceID(n.ResourceID).
		SetTriggeredByUserID(n.TriggeredByUserID).
		SetTriggeredByDisplayName(n.TriggeredByDisplayName).
		SetCreatedAt(n.CreatedAt).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	n.ID = saved.ID
	return n, nil
}

func (s *pgNotificationStore) ListByUser(ctx context.Context, userID string, limit, skip int64) ([]NotificationRecord, error) {
	if limit <= 0 {
		limit = 20
	}

	query := s.client.Notification.Query().
		Where(notification.UserID(userID)).
		Order(ent.Desc(notification.FieldCreatedAt)).
		Limit(int(limit)).
		Offset(int(skip))

	nfns, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}

	var records []NotificationRecord
	for _, n := range nfns {
		resourceType := ""
		if n.ResourceType != nil {
			resourceType = string(*n.ResourceType)
		}
		records = append(records, NotificationRecord{
			ID:                     n.ID,
			UserID:                 n.UserID,
			Type:                   string(n.Type),
			Title:                  n.Title,
			Message:                n.Message,
			Read:                   n.Read,
			ResourceType:           resourceType,
			ResourceID:             n.ResourceID,
			TriggeredByUserID:      n.TriggeredByUserID,
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
	count, err := s.client.Notification.Query().
		Where(notification.UserID(userID), notification.Read(false)).
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

	_, err = s.client.Notification.UpdateOneID(uid).
		Where(notification.UserID(userID)).
		SetRead(true).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("notification not found: %w", ErrNotificationNotFound)
		}
		return fmt.Errorf("failed to mark notification read: %w", err)
	}
	return nil
}

func (s *pgNotificationStore) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.client.Notification.Update().
		Where(notification.UserID(userID), notification.Read(false)).
		SetRead(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark all notifications read: %w", err)
	}
	return nil
}
