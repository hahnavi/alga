package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entactionitem "alga/ent/actionitem"
)

type ActionItemRecord struct {
	ID           uuid.UUID  `json:"id"`
	PostMortemID uuid.UUID  `json:"post_mortem_id"`
	Description  string     `json:"description"`
	AssigneeID   *uuid.UUID `json:"assignee_id,omitempty"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	Type         string     `json:"type"`
	AssigneeName string     `json:"assignee_name,omitempty"`
	DueDate      *time.Time `json:"due_date,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type ActionItemStore interface {
	Create(ctx context.Context, record *ActionItemRecord) (*ActionItemRecord, error)
	GetByID(ctx context.Context, id uuid.UUID) (*ActionItemRecord, error)
	ListByPostMortem(ctx context.Context, postMortemID uuid.UUID) ([]ActionItemRecord, error)
	ListOpen(ctx context.Context) ([]ActionItemRecord, error)
	ListOverdue(ctx context.Context) ([]ActionItemRecord, error)
	Update(ctx context.Context, id uuid.UUID, record *ActionItemRecord) (*ActionItemRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByPostMortemID(ctx context.Context, postMortemID uuid.UUID) error
}

type pgActionItemStore struct {
	pgStoreBase
}

func newPGActionItemStore(client *ent.Client) ActionItemStore {
	return &pgActionItemStore{pgStoreBase: pgStoreBase{client: client}}
}

func (s *pgActionItemStore) Create(ctx context.Context, record *ActionItemRecord) (*ActionItemRecord, error) {
	now := time.Now().UTC()

	b := s.client.ActionItem.Create().
		SetPostMortemID(record.PostMortemID).
		SetDescription(record.Description).
		SetStatus(record.Status).
		SetPriority(record.Priority).
		SetType(record.Type).
		SetAssigneeName(record.AssigneeName).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if record.AssigneeID != nil {
		b.SetAssigneeID(*record.AssigneeID)
	}
	if record.DueDate != nil {
		b.SetDueDate(*record.DueDate)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create action item: %w", err)
	}

	return s.toRecord(saved), nil
}

func (s *pgActionItemStore) GetByID(ctx context.Context, id uuid.UUID) (*ActionItemRecord, error) {
	ai, err := s.client.ActionItem.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*ActionItemRecord](err, "action item")
	}
	return s.toRecord(ai), nil
}

func (s *pgActionItemStore) ListByPostMortem(ctx context.Context, postMortemID uuid.UUID) ([]ActionItemRecord, error) {
	items, err := s.client.ActionItem.Query().
		Where(entactionitem.PostMortemID(postMortemID)).
		Order(ent.Asc(entactionitem.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list action items: %w", err)
	}

	records := make([]ActionItemRecord, 0, len(items))
	for _, ai := range items {
		records = append(records, *s.toRecord(ai))
	}
	return records, nil
}

func (s *pgActionItemStore) ListOpen(ctx context.Context) ([]ActionItemRecord, error) {
	items, err := s.client.ActionItem.Query().
		Where(
			entactionitem.StatusNEQ("completed"),
			entactionitem.StatusNEQ("cancelled"),
		).
		Order(ent.Asc(entactionitem.FieldDueDate)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list open action items: %w", err)
	}

	records := make([]ActionItemRecord, 0, len(items))
	for _, ai := range items {
		records = append(records, *s.toRecord(ai))
	}
	return records, nil
}

func (s *pgActionItemStore) ListOverdue(ctx context.Context) ([]ActionItemRecord, error) {
	now := time.Now().UTC()
	items, err := s.client.ActionItem.Query().
		Where(
			entactionitem.StatusNEQ("completed"),
			entactionitem.StatusNEQ("cancelled"),
			entactionitem.DueDateLT(now),
			entactionitem.DueDateNotNil(),
		).
		Order(ent.Asc(entactionitem.FieldDueDate)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list overdue action items: %w", err)
	}

	records := make([]ActionItemRecord, 0, len(items))
	for _, ai := range items {
		records = append(records, *s.toRecord(ai))
	}
	return records, nil
}

func (s *pgActionItemStore) Update(ctx context.Context, id uuid.UUID, record *ActionItemRecord) (*ActionItemRecord, error) {
	b := s.client.ActionItem.UpdateOneID(id).
		SetDescription(record.Description).
		SetStatus(record.Status).
		SetPriority(record.Priority).
		SetType(record.Type).
		SetAssigneeName(record.AssigneeName).
		SetUpdatedAt(time.Now().UTC())

	if record.AssigneeID != nil {
		b.SetAssigneeID(*record.AssigneeID)
	} else {
		b.ClearAssigneeID()
	}
	if record.DueDate != nil {
		b.SetDueDate(*record.DueDate)
	} else {
		b.ClearDueDate()
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update action item: %w", err)
	}

	return s.toRecord(saved), nil
}

func (s *pgActionItemStore) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.client.ActionItem.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete action item: %w", err)
	}
	return nil
}

func (s *pgActionItemStore) DeleteByPostMortemID(ctx context.Context, postMortemID uuid.UUID) error {
	_, err := s.client.ActionItem.Delete().
		Where(entactionitem.PostMortemID(postMortemID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete action items by post-mortem: %w", err)
	}
	return nil
}

func (s *pgActionItemStore) toRecord(ai *ent.ActionItem) *ActionItemRecord {
	return &ActionItemRecord{
		ID:           ai.ID,
		PostMortemID: ai.PostMortemID,
		Description:  ai.Description,
		AssigneeID:   ai.AssigneeID,
		Status:       ai.Status,
		Priority:     ai.Priority,
		Type:         ai.Type,
		AssigneeName: ai.AssigneeName,
		DueDate:      ai.DueDate,
		CreatedAt:    ai.CreatedAt,
		UpdatedAt:    ai.UpdatedAt,
	}
}
