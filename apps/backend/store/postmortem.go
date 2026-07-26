package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entpostmortem "alga/ent/postmortem"
	"alga/ent/predicate"
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

func newPGPostMortemStore(client *ent.Client, actionItemStore ActionItemStore) PostMortemStore {
	return &pgPostMortemStore{pgStoreBase: pgStoreBase{client: client}, actionItemStore: actionItemStore}
}

func (s *pgPostMortemStore) Create(ctx context.Context, record *PostMortemRecord) (*PostMortemRecord, error) {
	now := time.Now().UTC()

	if record.Status == "" {
		record.Status = "draft"
	}

	b := s.client.PostMortem.Create().
		SetIncidentID(record.IncidentID).
		SetTitle(record.Title).
		SetStatus(entpostmortem.Status(record.Status)).
		SetSummary(record.Summary).
		SetRootCause(record.RootCause).
		SetImpact(record.Impact).
		SetLessonsLearned(record.LessonsLearned).
		SetWhatWentWell(record.WhatWentWell).
		SetWhatWentWrong(record.WhatWentWrong).
		SetBlamelessConfirmed(record.BlamelessConfirmed).
		SetBlamelessNotes(record.BlamelessNotes).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if record.Timeline != nil {
		b.SetTimeline(record.Timeline)
	}
	if record.ContributingFactors != nil {
		b.SetContributingFactors(record.ContributingFactors)
	}
	if record.ApprovedByID != nil {
		b.SetApprovedByID(*record.ApprovedByID)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create post-mortem: %w", err)
	}

	return s.toRecord(ctx, saved)
}

func (s *pgPostMortemStore) GetByIncidentID(ctx context.Context, incidentID uuid.UUID) (*PostMortemRecord, error) {
	pm, err := s.client.PostMortem.Query().
		Where(entpostmortem.IncidentID(incidentID)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*PostMortemRecord](err, "post-mortem by incident id")
	}
	return s.toRecord(ctx, pm)
}

func (s *pgPostMortemStore) GetByID(ctx context.Context, id uuid.UUID) (*PostMortemRecord, error) {
	pm, err := s.client.PostMortem.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*PostMortemRecord](err, "post-mortem")
	}
	return s.toRecord(ctx, pm)
}

func (s *pgPostMortemStore) Update(ctx context.Context, id uuid.UUID, record *PostMortemRecord) (*PostMortemRecord, error) {
	b := s.client.PostMortem.UpdateOneID(id).
		SetTitle(record.Title).
		SetStatus(entpostmortem.Status(record.Status)).
		SetSummary(record.Summary).
		SetRootCause(record.RootCause).
		SetImpact(record.Impact).
		SetLessonsLearned(record.LessonsLearned).
		SetWhatWentWell(record.WhatWentWell).
		SetWhatWentWrong(record.WhatWentWrong).
		SetBlamelessConfirmed(record.BlamelessConfirmed).
		SetBlamelessNotes(record.BlamelessNotes).
		SetUpdatedAt(time.Now().UTC())

	if record.Timeline != nil {
		b.SetTimeline(record.Timeline)
	}
	if record.ContributingFactors != nil {
		b.SetContributingFactors(record.ContributingFactors)
	}
	if record.ApprovedByID != nil {
		b.SetApprovedByID(*record.ApprovedByID)
	} else {
		b.ClearApprovedByID()
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update post-mortem: %w", err)
	}

	return s.toRecord(ctx, saved)
}

func (s *pgPostMortemStore) UpdateStatus(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) (*PostMortemRecord, error) {
	b := s.client.PostMortem.UpdateOneID(id).
		SetStatus(entpostmortem.Status(status)).
		SetUpdatedAt(time.Now().UTC())

	if approvedBy != nil {
		b.SetApprovedByID(*approvedBy)
	}
	if status == "published" {
		now := time.Now().UTC()
		b.SetPublishedAt(now)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update post-mortem status: %w", err)
	}

	return s.toRecord(ctx, saved)
}

func (s *pgPostMortemStore) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.client.PostMortem.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete post-mortem: %w", err)
	}
	return nil
}

func (s *pgPostMortemStore) List(ctx context.Context, filter PostMortemListFilter) ([]PostMortemRecord, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var preds []predicate.PostMortem
	if filter.Status != "" {
		preds = append(preds, entpostmortem.StatusEQ(entpostmortem.Status(filter.Status)))
	}

	total, err := s.client.PostMortem.Query().Where(preds...).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count post-mortems: %w", err)
	}

	query := s.client.PostMortem.Query().Where(preds...).
		Order(ent.Desc(entpostmortem.FieldCreatedAt))

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)
	query = query.Limit(limit)
	if filter.Skip > 0 {
		query = query.Offset(filter.Skip)
	}

	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list post-mortems: %w", err)
	}

	records := make([]PostMortemRecord, 0, len(items))
	for _, pm := range items {
		rec, recErr := s.toRecord(ctx, pm)
		if recErr != nil {
			return nil, 0, recErr
		}
		records = append(records, *rec)
	}

	return records, total, nil
}

func (s *pgPostMortemStore) toRecord(ctx context.Context, pm *ent.PostMortem) (*PostMortemRecord, error) {
	rec := &PostMortemRecord{
		ID:                  pm.ID,
		IncidentID:          pm.IncidentID,
		Title:               pm.Title,
		Status:              string(pm.Status),
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

	if s.actionItemStore != nil {
		items, err := s.actionItemStore.ListByPostMortem(ctx, pm.ID)
		if err == nil {
			rec.ActionItems = items
		}
	}

	return rec, nil
}
