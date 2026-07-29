package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

const (
	ThreadOwnerAlert                 = "alert"
	ThreadOwnerIncidentInvestigation = "incident_inv"
	ThreadOwnerIncidentCoordination  = "incident_coord"
)

// IsIncidentThreadOwner reports whether ownerType is any incident-scoped thread
// (investigation working thread or coordination thread).
func IsIncidentThreadOwner(ownerType string) bool {
	return ownerType == ThreadOwnerIncidentInvestigation || ownerType == ThreadOwnerIncidentCoordination
}

type InvestigationThreadRecord struct {
	ID        uuid.UUID                    `json:"id"`
	ThreadID  string                       `json:"thread_id"`
	OwnerType string                       `json:"owner_type"`
	OwnerID   string                       `json:"owner_id"`
	Messages  []InvestigationThreadMessage `json:"messages,omitempty"`
	CreatedAt time.Time                    `json:"created_at"`
	UpdatedAt time.Time                    `json:"updated_at"`
}

type InvestigationThreadMessage struct {
	ID               uuid.UUID `json:"id"`
	ThreadID         string    `json:"thread_id,omitempty"`
	Type             string    `json:"type"`
	Source           string    `json:"source"`
	Message          string    `json:"message"`
	Internal         bool      `json:"internal"`
	Edited           bool      `json:"edited"`
	UserID           string    `json:"user_id,omitempty"`
	Username         string    `json:"username,omitempty"`
	AgentType        string    `json:"agent_type,omitempty"`
	MMPostID         string    `json:"mm_post_id,omitempty"`
	SlackMessageTS   string    `json:"slack_message_ts,omitempty"`
	ReplyToMessageID string    `json:"reply_to_message_id,omitempty"`
	Mentions         []string  `json:"mentions,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type InvestigationThreadStore interface {
	EnsureThread(ctx context.Context, ownerType string, ownerID string) (*InvestigationThreadRecord, error)
	GetThreadByOwner(ctx context.Context, ownerType string, ownerID string, limit int64, skip int64) (*InvestigationThreadRecord, int64, error)
	AddMessage(ctx context.Context, threadID string, message InvestigationThreadMessage) (*InvestigationThreadMessage, error)
	UpdateMessage(ctx context.Context, ownerType string, ownerID string, messageID string, message string, markEdited bool) (*InvestigationThreadMessage, error)
	DeleteMessage(ctx context.Context, ownerType string, ownerID string, messageID string) error
}

type pgInvestigationThreadStore struct {
	pgStoreBase
}

func newPGInvestigationThreadStore(db *bun.DB) InvestigationThreadStore {
	return &pgInvestigationThreadStore{pgStoreBase{db: db}}
}

func (s *pgInvestigationThreadStore) EnsureThread(ctx context.Context, ownerType string, ownerID string) (*InvestigationThreadRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	existing, _, err := s.getThreadByOwner(ctx, ownerType, ownerID, 0, 0)
	if err == nil {
		return existing, nil
	}
	if !isInvestigationThreadNotFound(err) {
		return nil, err
	}

	m := &models.InvestigationThread{
		ThreadID:  uuid.NewString(),
		OwnerType: ownerType,
		OwnerID:   ownerID,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = time.Now().UTC()
	m.UpdatedAt = time.Now().UTC()

	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		if pgIsDuplicateKey(err) {
			record, _, err := s.getThreadByOwner(ctx, ownerType, ownerID, 0, 0)
			return record, err
		}
		return nil, fmt.Errorf("failed to create investigation thread: %w", err)
	}
	return investigationThreadFromModel(m), nil
}

func (s *pgInvestigationThreadStore) GetThreadByOwner(ctx context.Context, ownerType string, ownerID string, limit int64, skip int64) (*InvestigationThreadRecord, int64, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.getThreadByOwner(ctx, ownerType, ownerID, limit, skip)
}

func (s *pgInvestigationThreadStore) AddMessage(ctx context.Context, threadID string, message InvestigationThreadMessage) (*InvestigationThreadMessage, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var result *InvestigationThreadMessage
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var thread models.InvestigationThread
		if err := tx.NewSelect().Model(&thread).Where("thread_id = ?", threadID).Scan(ctx); err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("failed to query investigation thread: %w", err)
		}

		mentions := message.Mentions
		if mentions == nil {
			mentions = []string{}
		}

		m := &models.InvestigationThreadMessage{
			ThreadID:         thread.ID,
			Message:          message.Message,
			Internal:         message.Internal,
			Edited:           message.Edited,
			UserID:           message.UserID,
			Username:         message.Username,
			AgentType:        message.AgentType,
			MMPostID:         message.MMPostID,
			SlackMessageTs:   message.SlackMessageTS,
			ReplyToMessageID: message.ReplyToMessageID,
			Mentions:         mentions,
		}
		m.ID = models.NewUUID()
		if message.Type != "" {
			m.Type = message.Type
		} else {
			m.Type = "comment"
		}
		if message.Source != "" {
			m.Source = message.Source
		} else {
			m.Source = "user"
		}
		if !message.CreatedAt.IsZero() {
			m.CreatedAt = message.CreatedAt
		} else {
			m.CreatedAt = time.Now().UTC()
		}
		if !message.UpdatedAt.IsZero() {
			m.UpdatedAt = message.UpdatedAt
		} else {
			m.UpdatedAt = time.Now().UTC()
		}

		if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
			return fmt.Errorf("failed to create investigation thread message: %w", err)
		}

		if _, err := tx.NewUpdate().Model((*models.InvestigationThread)(nil)).
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", thread.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to update investigation thread timestamp: %w", err)
		}

		record := investigationThreadMessageFromModel(m)
		record.ThreadID = thread.ThreadID
		result = record
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *pgInvestigationThreadStore) UpdateMessage(ctx context.Context, ownerType string, ownerID string, messageID string, message string, markEdited bool) (*InvestigationThreadMessage, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	id, err := uuid.Parse(messageID)
	if err != nil {
		return nil, fmt.Errorf("invalid investigation thread message id: %w", err)
	}

	res, err := s.db.NewUpdate().Model((*models.InvestigationThreadMessage)(nil)).
		Set("message = ?", message).
		Set("edited = ?", markEdited).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Where("thread_id IN (SELECT id FROM investigation_threads WHERE owner_type = ? AND owner_id = ?)", ownerType, ownerID).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update investigation thread message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update investigation thread message: %w", err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}

	var saved models.InvestigationThreadMessage
	if err := s.db.NewSelect().Model(&saved).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload investigation thread message: %w", err)
	}

	record := investigationThreadMessageFromModel(&saved)
	if thread, _, err := s.getThreadByOwner(ctx, ownerType, ownerID, 0, 0); err == nil && thread != nil {
		record.ThreadID = thread.ThreadID
	}
	return record, nil
}

func (s *pgInvestigationThreadStore) DeleteMessage(ctx context.Context, ownerType string, ownerID string, messageID string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	id, err := uuid.Parse(messageID)
	if err != nil {
		return fmt.Errorf("invalid investigation thread message id: %w", err)
	}

	res, err := s.db.NewDelete().Model((*models.InvestigationThreadMessage)(nil)).
		Where("id = ?", id).
		Where("thread_id IN (SELECT id FROM investigation_threads WHERE owner_type = ? AND owner_id = ?)", ownerType, ownerID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete investigation thread message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete investigation thread message: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *pgInvestigationThreadStore) getThreadByOwner(ctx context.Context, ownerType string, ownerID string, limit int64, skip int64) (*InvestigationThreadRecord, int64, error) {
	var thread models.InvestigationThread
	err := s.db.NewSelect().Model(&thread).
		Where("owner_type = ?", ownerType).
		Where("owner_id = ?", ownerID).
		Scan(ctx)
	if err != nil {
		record, qerr := handleQueryErr[*InvestigationThreadRecord](err, "investigation thread")
		if qerr != nil {
			return nil, 0, qerr
		}
		if record == nil {
			return nil, 0, ErrNotFound
		}
	}

	// Count total messages.
	total, err := s.db.NewSelect().Model((*models.InvestigationThreadMessage)(nil)).
		Where("thread_id = ?", thread.ID).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count investigation thread messages: %w", err)
	}

	record := investigationThreadFromModel(&thread)

	// Load messages with pagination.
	msgQuery := s.db.NewSelect().Model((*models.InvestigationThreadMessage)(nil)).
		Where("thread_id = ?", thread.ID).
		Order("created_at ASC")
	if limit > 0 {
		msgQuery = msgQuery.Limit(int(limit))
	}
	if skip > 0 {
		msgQuery = msgQuery.Offset(int(skip))
	}
	var messages []models.InvestigationThreadMessage
	if err := msgQuery.Scan(ctx, &messages); err != nil {
		return nil, 0, fmt.Errorf("failed to query investigation thread messages: %w", err)
	}
	if len(messages) > 0 {
		record.Messages = make([]InvestigationThreadMessage, 0, len(messages))
		for i := range messages {
			msg := investigationThreadMessageFromModel(&messages[i])
			msg.ThreadID = thread.ThreadID
			record.Messages = append(record.Messages, *msg)
		}
	}

	return record, int64(total), nil
}

func isInvestigationThreadNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func investigationThreadFromModel(m *models.InvestigationThread) *InvestigationThreadRecord {
	if m == nil {
		return nil
	}
	return &InvestigationThreadRecord{
		ID:        m.ID,
		ThreadID:  m.ThreadID,
		OwnerType: m.OwnerType,
		OwnerID:   m.OwnerID,
		Messages:  []InvestigationThreadMessage{},
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func investigationThreadMessageFromModel(m *models.InvestigationThreadMessage) *InvestigationThreadMessage {
	if m == nil {
		return nil
	}
	mentions := m.Mentions
	if mentions == nil {
		mentions = []string{}
	}
	return &InvestigationThreadMessage{
		ID:               m.ID,
		Type:             m.Type,
		Source:           m.Source,
		Message:          m.Message,
		Internal:         m.Internal,
		Edited:           m.Edited,
		UserID:           m.UserID,
		Username:         m.Username,
		AgentType:        m.AgentType,
		MMPostID:         m.MMPostID,
		SlackMessageTS:   m.SlackMessageTs,
		ReplyToMessageID: m.ReplyToMessageID,
		Mentions:         mentions,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}
