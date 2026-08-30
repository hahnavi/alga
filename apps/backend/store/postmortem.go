package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type PostMortemRecord struct {
	ID                  uuid.UUID          `json:"id"`
	IncidentID          uuid.UUID          `json:"incident_id"`
	Title               string             `json:"title"`
	Status              string             `json:"status"`
	Summary             string             `json:"summary"`
	Timeline            []map[string]any   `json:"timeline,omitempty"`
	RootCause           string             `json:"root_cause"`
	ContributingFactors []string           `json:"contributing_factors,omitempty"`
	Impact              string             `json:"impact"`
	LessonsLearned      string             `json:"lessons_learned"`
	WhatWentWell        string             `json:"what_went_well"`
	WhatWentWrong       string             `json:"what_went_wrong"`
	BlamelessConfirmed  bool               `json:"blameless_confirmed"`
	BlamelessNotes      string             `json:"blameless_notes"`
	ApprovedByID        *uuid.UUID         `json:"approved_by_id,omitempty"`
	PublishedAt         *time.Time         `json:"published_at,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
	ActionItems         []ActionItemRecord `json:"action_items,omitempty"`
	IncidentTitle       string             `json:"incident_title,omitempty"`
	IncidentNumber      int64              `json:"incident_number,omitempty"`
	IncidentSeverity    string             `json:"incident_severity,omitempty"`
}

type PostMortemListFilter struct {
	Status string
	Limit  int
	Skip   int
}

type PostMortemStore interface {
	Create(ctx context.Context, record *PostMortemRecord) (*PostMortemRecord, error)
	GetByIncidentID(ctx context.Context, incidentID uuid.UUID) (*PostMortemRecord, error)
	// ExistsByIncidentID answers "does a post-mortem exist for this incident"
	// without paying for action-item loading — used by the auto-draft guard.
	ExistsByIncidentID(ctx context.Context, incidentID uuid.UUID) (bool, error)
	GetByID(ctx context.Context, id uuid.UUID) (*PostMortemRecord, error)
	Update(ctx context.Context, id uuid.UUID, record *PostMortemRecord) (*PostMortemRecord, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) (*PostMortemRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter PostMortemListFilter) ([]PostMortemRecord, int, error)
}

type pgPostMortemStore struct {
	pgStoreBase
	actionItemStore ActionItemStore
}

func newPGPostMortemStore(db *bun.DB, actionItemStore ActionItemStore) PostMortemStore {
	return &pgPostMortemStore{pgStoreBase: pgStoreBase{db: db}, actionItemStore: actionItemStore}
}

func (s *pgPostMortemStore) Create(ctx context.Context, record *PostMortemRecord) (*PostMortemRecord, error) {
	now := time.Now().UTC()

	if record.Status == "" {
		record.Status = "draft"
	}

	m := &models.PostMortem{
		ID:                  models.NewUUID(),
		IncidentID:          record.IncidentID,
		Title:               record.Title,
		Status:              record.Status,
		Summary:             record.Summary,
		Timeline:            record.Timeline,
		RootCause:           record.RootCause,
		ContributingFactors: record.ContributingFactors,
		Impact:              record.Impact,
		LessonsLearned:      record.LessonsLearned,
		WhatWentWell:        record.WhatWentWell,
		WhatWentWrong:       record.WhatWentWrong,
		BlamelessConfirmed:  record.BlamelessConfirmed,
		BlamelessNotes:      record.BlamelessNotes,
		ApprovedByID:        record.ApprovedByID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create post-mortem: %w", err)
	}

	return s.toRecord(ctx, m)
}

func (s *pgPostMortemStore) GetByIncidentID(ctx context.Context, incidentID uuid.UUID) (*PostMortemRecord, error) {
	var pm models.PostMortem
	err := s.db.NewSelect().Model(&pm).Where("incident_id = ?", incidentID).Scan(ctx)
	if err != nil {
		return handleQueryErr[*PostMortemRecord](err, "post-mortem by incident id")
	}
	return s.toRecord(ctx, &pm)
}

func (s *pgPostMortemStore) ExistsByIncidentID(ctx context.Context, incidentID uuid.UUID) (bool, error) {
	exists, err := s.db.NewSelect().Model((*models.PostMortem)(nil)).
		Where("incident_id = ?", incidentID).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check post-mortem existence: %w", err)
	}
	return exists, nil
}

func (s *pgPostMortemStore) GetByID(ctx context.Context, id uuid.UUID) (*PostMortemRecord, error) {
	var pm models.PostMortem
	err := s.db.NewSelect().Model(&pm).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*PostMortemRecord](err, "post-mortem")
	}
	return s.toRecord(ctx, &pm)
}

func (s *pgPostMortemStore) Update(ctx context.Context, id uuid.UUID, record *PostMortemRecord) (*PostMortemRecord, error) {
	upd := s.db.NewUpdate().Model((*models.PostMortem)(nil)).
		Set("title = ?", record.Title).
		Set("status = ?", record.Status).
		Set("summary = ?", record.Summary).
		Set("root_cause = ?", record.RootCause).
		Set("impact = ?", record.Impact).
		Set("lessons_learned = ?", record.LessonsLearned).
		Set("what_went_well = ?", record.WhatWentWell).
		Set("what_went_wrong = ?", record.WhatWentWrong).
		Set("blameless_confirmed = ?", record.BlamelessConfirmed).
		Set("blameless_notes = ?", record.BlamelessNotes).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id)

	if record.Timeline != nil {
		upd = upd.Set("timeline = ?", record.Timeline)
	}
	if record.ContributingFactors != nil {
		upd = upd.Set("contributing_factors = ?", record.ContributingFactors)
	}
	if record.ApprovedByID != nil {
		upd = upd.Set("approved_by_id = ?", *record.ApprovedByID)
	} else {
		upd = upd.Set("approved_by_id = NULL")
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update post-mortem: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update post-mortem: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("post-mortem not found: %w", ErrNotFound)
	}

	var pm models.PostMortem
	err = s.db.NewSelect().Model(&pm).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reload post-mortem: %w", err)
	}
	return s.toRecord(ctx, &pm)
}

func (s *pgPostMortemStore) UpdateStatus(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) (*PostMortemRecord, error) {
	now := time.Now().UTC()
	upd := s.db.NewUpdate().Model((*models.PostMortem)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if approvedBy != nil {
		upd = upd.Set("approved_by_id = ?", *approvedBy)
	}
	if status == "published" {
		upd = upd.Set("published_at = ?", now)
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update post-mortem status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update post-mortem status: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("post-mortem not found: %w", ErrNotFound)
	}

	var pm models.PostMortem
	err = s.db.NewSelect().Model(&pm).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reload post-mortem: %w", err)
	}
	return s.toRecord(ctx, &pm)
}

func (s *pgPostMortemStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.NewDelete().Model((*models.PostMortem)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete post-mortem: %w", err)
	}
	return nil
}

func (s *pgPostMortemStore) List(ctx context.Context, filter PostMortemListFilter) ([]PostMortemRecord, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	countQ := s.db.NewSelect().Model((*models.PostMortem)(nil))
	if filter.Status != "" {
		countQ = countQ.Where("status = ?", filter.Status)
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count post-mortems: %w", err)
	}

	listQ := s.db.NewSelect().Model((*models.PostMortem)(nil))
	if filter.Status != "" {
		listQ = listQ.Where("status = ?", filter.Status)
	}
	listQ = listQ.OrderExpr("created_at DESC")

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)
	listQ = listQ.Limit(limit)
	if filter.Skip > 0 {
		listQ = listQ.Offset(filter.Skip)
	}

	var items []models.PostMortem
	err = listQ.Scan(ctx, &items)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list post-mortems: %w", err)
	}

	records := make([]PostMortemRecord, 0, len(items))
	for _, pm := range items {
		rec := postMortemModelToRecord(&pm)
		records = append(records, *rec)
	}

	// Batch-load action items for the page in one query instead of one
	// ListByPostMortem per row.
	if s.actionItemStore != nil && len(records) > 0 {
		ids := make([]uuid.UUID, 0, len(records))
		for i := range records {
			ids = append(ids, records[i].ID)
		}
		byPM, aiErr := s.actionItemStore.ListByPostMortemIDs(ctx, ids)
		if aiErr == nil {
			for i := range records {
				if items, ok := byPM[records[i].ID]; ok {
					records[i].ActionItems = items
				}
			}
		}
	}

	return records, total, nil
}

func (s *pgPostMortemStore) toRecord(ctx context.Context, pm *models.PostMortem) (*PostMortemRecord, error) {
	rec := postMortemModelToRecord(pm)

	if s.actionItemStore != nil {
		items, err := s.actionItemStore.ListByPostMortem(ctx, pm.ID)
		if err == nil {
			rec.ActionItems = items
		}
	}

	return rec, nil
}

// postMortemModelToRecord maps the persistence model to the API record
// without side-effect loads.
func postMortemModelToRecord(pm *models.PostMortem) *PostMortemRecord {
	return &PostMortemRecord{
		ID:                  pm.ID,
		IncidentID:          pm.IncidentID,
		Title:               pm.Title,
		Status:              pm.Status,
		Summary:             pm.Summary,
		Timeline:            pm.Timeline,
		RootCause:           pm.RootCause,
		ContributingFactors: pm.ContributingFactors,
		Impact:              pm.Impact,
		LessonsLearned:      pm.LessonsLearned,
		WhatWentWell:        pm.WhatWentWell,
		WhatWentWrong:       pm.WhatWentWrong,
		BlamelessConfirmed:  pm.BlamelessConfirmed,
		BlamelessNotes:      pm.BlamelessNotes,
		ApprovedByID:        pm.ApprovedByID,
		PublishedAt:         pm.PublishedAt,
		CreatedAt:           pm.CreatedAt,
		UpdatedAt:           pm.UpdatedAt,
	}
}
