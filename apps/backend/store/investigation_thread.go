package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/investigationthread"
	"alga/ent/investigationthreadmessage"
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

func newPGInvestigationThreadStore(client *ent.Client) InvestigationThreadStore {
	return &pgInvestigationThreadStore{pgStoreBase{client: client}}
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

	created, err := s.client.InvestigationThread.Create().
		SetThreadID(uuid.NewString()).
		SetOwnerType(ownerType).
		SetOwnerID(ownerID).
		Save(ctx)
	if err != nil {
		if pgIsDuplicateKey(err) {
			record, _, err := s.getThreadByOwner(ctx, ownerType, ownerID, 0, 0)
			return record, err
		}
		return nil, fmt.Errorf("failed to create investigation thread: %w", err)
	}
	return investigationThreadFromEnt(created), nil
}

func (s *pgInvestigationThreadStore) GetThreadByOwner(ctx context.Context, ownerType string, ownerID string, limit int64, skip int64) (*InvestigationThreadRecord, int64, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.getThreadByOwner(ctx, ownerType, ownerID, limit, skip)
}

func (s *pgInvestigationThreadStore) AddMessage(ctx context.Context, threadID string, message InvestigationThreadMessage) (*InvestigationThreadMessage, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin investigation thread message transaction: %w", err)
	}
	defer rollbackTx(tx)

	thread, err := tx.Client().InvestigationThread.Query().
		Where(investigationthread.ThreadID(threadID)).
		Only(ctx)
	if err != nil {
		found, qerr := handleQueryErr[*ent.InvestigationThread](err, "investigation thread")
		if qerr != nil {
			return nil, qerr
		}
		if found == nil {
			return nil, ErrNotFound
		}
		thread = found
	}

	mentions := message.Mentions
	if mentions == nil {
		mentions = []string{}
	}

	create := tx.Client().InvestigationThreadMessage.Create().
		SetThreadID(thread.ID).
		SetMessage(message.Message).
		SetInternal(message.Internal).
		SetEdited(message.Edited).
		SetUserID(message.UserID).
		SetUsername(message.Username).
		SetAgentType(message.AgentType).
		SetMmPostID(message.MMPostID).
		SetSlackMessageTs(message.SlackMessageTS).
		SetReplyToMessageID(message.ReplyToMessageID).
		SetMentions(mentions)
	if message.Type != "" {
		create.SetType(message.Type)
	}
	if message.Source != "" {
		create.SetSource(message.Source)
	}
	if !message.CreatedAt.IsZero() {
		create.SetCreatedAt(message.CreatedAt)
	}
	if !message.UpdatedAt.IsZero() {
		create.SetUpdatedAt(message.UpdatedAt)
	}

	created, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create investigation thread message: %w", err)
	}

	if _, err := tx.Client().InvestigationThread.UpdateOneID(thread.ID).SetUpdatedAt(time.Now().UTC()).Save(ctx); err != nil {
		return nil, fmt.Errorf("failed to update investigation thread timestamp: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit investigation thread message transaction: %w", err)
	}

	record := investigationThreadMessageFromEnt(created)
	record.ThreadID = thread.ThreadID
	return record, nil
}

func (s *pgInvestigationThreadStore) UpdateMessage(ctx context.Context, ownerType string, ownerID string, messageID string, message string, markEdited bool) (*InvestigationThreadMessage, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	id, err := uuid.Parse(messageID)
	if err != nil {
		return nil, fmt.Errorf("invalid investigation thread message id: %w", err)
	}

	saved, err := s.client.InvestigationThreadMessage.UpdateOneID(id).
		Where(investigationthreadmessage.HasThreadWith(
			investigationthread.OwnerType(ownerType),
			investigationthread.OwnerID(ownerID),
		)).
		SetMessage(message).
		SetEdited(markEdited).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to update investigation thread message: %w", err)
	}

	record := investigationThreadMessageFromEnt(saved)
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

	n, err := s.client.InvestigationThreadMessage.Delete().
		Where(
			investigationthreadmessage.ID(id),
			investigationthreadmessage.HasThreadWith(
				investigationthread.OwnerType(ownerType),
				investigationthread.OwnerID(ownerID),
			),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete investigation thread message: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *pgInvestigationThreadStore) getThreadByOwner(ctx context.Context, ownerType string, ownerID string, limit int64, skip int64) (*InvestigationThreadRecord, int64, error) {
	thread, err := s.client.InvestigationThread.Query().
		Where(
			investigationthread.OwnerType(ownerType),
			investigationthread.OwnerID(ownerID),
		).
		WithMessages(func(q *ent.InvestigationThreadMessageQuery) {
			q.Order(ent.Asc(investigationthreadmessage.FieldCreatedAt))
			if limit > 0 {
				q.Limit(int(limit))
			}
			if skip > 0 {
				q.Offset(int(skip))
			}
		}).
		Only(ctx)
	if err != nil {
		record, qerr := handleQueryErr[*InvestigationThreadRecord](err, "investigation thread")
		if qerr != nil {
			return nil, 0, qerr
		}
		if record == nil {
			return nil, 0, ErrNotFound
		}
	}

	total, err := s.client.InvestigationThreadMessage.Query().
		Where(investigationthreadmessage.HasThreadWith(
			investigationthread.OwnerType(ownerType),
			investigationthread.OwnerID(ownerID),
		)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count investigation thread messages: %w", err)
	}
	return investigationThreadFromEnt(thread), int64(total), nil
}

func isInvestigationThreadNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func investigationThreadFromEnt(e *ent.InvestigationThread) *InvestigationThreadRecord {
	if e == nil {
		return nil
	}
	record := &InvestigationThreadRecord{
		ID:        e.ID,
		ThreadID:  e.ThreadID,
		OwnerType: e.OwnerType,
		OwnerID:   e.OwnerID,
		Messages:  []InvestigationThreadMessage{},
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if messages := e.Edges.Messages; len(messages) > 0 {
		record.Messages = make([]InvestigationThreadMessage, 0, len(messages))
		for _, message := range messages {
			msg := investigationThreadMessageFromEnt(message)
			msg.ThreadID = e.ThreadID
			record.Messages = append(record.Messages, *msg)
		}
	}
	return record
}

func investigationThreadMessageFromEnt(e *ent.InvestigationThreadMessage) *InvestigationThreadMessage {
	if e == nil {
		return nil
	}
	mentions := e.Mentions
	if mentions == nil {
		mentions = []string{}
	}
	return &InvestigationThreadMessage{
		ID:               e.ID,
		Type:             e.Type,
		Source:           e.Source,
		Message:          e.Message,
		Internal:         e.Internal,
		Edited:           e.Edited,
		UserID:           e.UserID,
		Username:         e.Username,
		AgentType:        e.AgentType,
		MMPostID:         e.MmPostID,
		SlackMessageTS:   e.SlackMessageTs,
		ReplyToMessageID: e.ReplyToMessageID,
		Mentions:         mentions,
		CreatedAt:        e.CreatedAt,
		UpdatedAt:        e.UpdatedAt,
	}
}
