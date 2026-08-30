package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
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
	// ListByPostMortemIDs batch-loads action items for many post-mortems in
	// one query, grouped by post-mortem ID. Used by list endpoints to avoid a
	// per-row action-item query.
	ListByPostMortemIDs(ctx context.Context, postMortemIDs []uuid.UUID) (map[uuid.UUID][]ActionItemRecord, error)
	ListOpen(ctx context.Context) ([]ActionItemRecord, error)
	ListOverdue(ctx context.Context) ([]ActionItemRecord, error)
	Update(ctx context.Context, id uuid.UUID, record *ActionItemRecord) (*ActionItemRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByPostMortemID(ctx context.Context, postMortemID uuid.UUID) error
}

type pgActionItemStore struct {
	pgStoreBase
}

func newPGActionItemStore(db *bun.DB) ActionItemStore {
	return &pgActionItemStore{pgStoreBase: pgStoreBase{db: db}}
}

func (s *pgActionItemStore) Create(ctx context.Context, record *ActionItemRecord) (*ActionItemRecord, error) {
	now := time.Now().UTC()

	if record.Status == "" {
		record.Status = "open"
	}
	if record.Priority == "" {
		record.Priority = "medium"
	}
	if record.Type == "" {
		record.Type = "investigate"
	}

	m := &models.ActionItem{
		PostMortemID: record.PostMortemID,
		Description:  record.Description,
		Status:       record.Status,
		Priority:     record.Priority,
		Type:         record.Type,
		AssigneeName: record.AssigneeName,
		AssigneeID:   record.AssigneeID,
		DueDate:      record.DueDate,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create action item: %w", err)
	}

	return s.toRecord(m), nil
}

func (s *pgActionItemStore) GetByID(ctx context.Context, id uuid.UUID) (*ActionItemRecord, error) {
	var ai models.ActionItem
	err := s.db.NewSelect().Model(&ai).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*ActionItemRecord](err, "action item")
	}
	return s.toRecord(&ai), nil
}

func (s *pgActionItemStore) ListByPostMortem(ctx context.Context, postMortemID uuid.UUID) ([]ActionItemRecord, error) {
	var items []models.ActionItem
	err := s.db.NewSelect().Model(&items).
		Where("post_mortem_id = ?", postMortemID).
		OrderExpr("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list action items: %w", err)
	}

	records := make([]ActionItemRecord, 0, len(items))
	for _, ai := range items {
		records = append(records, *s.toRecord(&ai))
	}
	return records, nil
}

func (s *pgActionItemStore) ListByPostMortemIDs(ctx context.Context, postMortemIDs []uuid.UUID) (map[uuid.UUID][]ActionItemRecord, error) {
	out := make(map[uuid.UUID][]ActionItemRecord, len(postMortemIDs))
	if len(postMortemIDs) == 0 {
		return out, nil
	}
	var items []models.ActionItem
	err := s.db.NewSelect().Model(&items).
		Where("post_mortem_id IN (?)", bun.In(postMortemIDs)).
		OrderExpr("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list action items by post-mortems: %w", err)
	}
	for i := range items {
		out[items[i].PostMortemID] = append(out[items[i].PostMortemID], *s.toRecord(&items[i]))
	}
	return out, nil
}

func (s *pgActionItemStore) ListOpen(ctx context.Context) ([]ActionItemRecord, error) {
	var items []models.ActionItem
	err := s.db.NewSelect().Model(&items).
		Where("status != ?", "completed").
		Where("status != ?", "cancelled").
		// NULLS LAST keeps undated items from crowding out everything with a
		// due date; Postgres ASC otherwise sorts NULLs first.
		OrderExpr("due_date ASC NULLS LAST").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list open action items: %w", err)
	}

	records := make([]ActionItemRecord, 0, len(items))
	for _, ai := range items {
		records = append(records, *s.toRecord(&ai))
	}
	return records, nil
}

func (s *pgActionItemStore) ListOverdue(ctx context.Context) ([]ActionItemRecord, error) {
	now := time.Now().UTC()
	var items []models.ActionItem
	err := s.db.NewSelect().Model(&items).
		Where("status != ?", "completed").
		Where("status != ?", "cancelled").
		Where("due_date < ?", now).
		Where("due_date IS NOT NULL").
		OrderExpr("due_date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list overdue action items: %w", err)
	}

	records := make([]ActionItemRecord, 0, len(items))
	for _, ai := range items {
		records = append(records, *s.toRecord(&ai))
	}
	return records, nil
}

func (s *pgActionItemStore) Update(ctx context.Context, id uuid.UUID, record *ActionItemRecord) (*ActionItemRecord, error) {
	upd := s.db.NewUpdate().Model((*models.ActionItem)(nil)).
		Set("description = ?", record.Description).
		Set("status = ?", record.Status).
		Set("priority = ?", record.Priority).
		Set("type = ?", record.Type).
		Set("assignee_name = ?", record.AssigneeName).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id)

	if record.AssigneeID != nil {
		upd = upd.Set("assignee_id = ?", *record.AssigneeID)
	} else {
		upd = upd.Set("assignee_id = NULL")
	}
	if record.DueDate != nil {
		upd = upd.Set("due_date = ?", *record.DueDate)
	} else {
		upd = upd.Set("due_date = NULL")
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update action item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update action item: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("action item not found: %w", ErrNotFound)
	}

	var ai models.ActionItem
	err = s.db.NewSelect().Model(&ai).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reload action item: %w", err)
	}
	return s.toRecord(&ai), nil
}

func (s *pgActionItemStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.NewDelete().Model((*models.ActionItem)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete action item: %w", err)
	}
	return nil
}

func (s *pgActionItemStore) DeleteByPostMortemID(ctx context.Context, postMortemID uuid.UUID) error {
	_, err := s.db.NewDelete().Model((*models.ActionItem)(nil)).
		Where("post_mortem_id = ?", postMortemID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete action items by post-mortem: %w", err)
	}
	return nil
}

func (s *pgActionItemStore) toRecord(ai *models.ActionItem) *ActionItemRecord {
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
