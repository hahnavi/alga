package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type TriageRuleRecord struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Conditions  []map[string]any `json:"conditions"`
	MatchMode   string           `json:"match_mode"`
	Decision    string           `json:"decision"`
	Severity    string           `json:"severity"`
	Category    string           `json:"category"`
	Enrichment  map[string]any   `json:"enrichment"`
	Priority    int              `json:"priority"`
	Enabled     bool             `json:"enabled"`
	CreatedBy   *uuid.UUID       `json:"created_by,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type TriageRuleQuery struct {
	Search  string
	Enabled *bool
	Limit   int
	Skip    int
}

type TriageRuleStore interface {
	Create(ctx context.Context, record *TriageRuleRecord) (*TriageRuleRecord, error)
	Update(ctx context.Context, id string, patch *TriageRuleRecord) (*TriageRuleRecord, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*TriageRuleRecord, error)
	List(ctx context.Context, q TriageRuleQuery) ([]TriageRuleRecord, int64, error)
	ListEnabled(ctx context.Context) ([]TriageRuleRecord, error)
	Reorder(ctx context.Context, ids []string) error
}

type pgTriageRuleStore struct {
	pgStoreBase
}

func newPGTriageRuleStore(db *bun.DB) TriageRuleStore {
	return &pgTriageRuleStore{pgStoreBase{db: db}}
}

func pgTriageRuleToRecord(r *models.TriageRule) *TriageRuleRecord {
	var conditions []map[string]any
	if r.Conditions != nil {
		conditions = r.Conditions
	} else {
		conditions = []map[string]any{}
	}
	var enrichment map[string]any
	if r.Enrichment != nil {
		enrichment = r.Enrichment
	} else {
		enrichment = map[string]any{}
	}
	severity := ""
	if r.Severity != nil {
		severity = *r.Severity
	}
	ruleCategory := ""
	if r.Category != nil {
		ruleCategory = *r.Category
	}
	return &TriageRuleRecord{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Conditions:  conditions,
		MatchMode:   r.MatchMode,
		Decision:    r.Decision,
		Severity:    severity,
		Category:    ruleCategory,
		Enrichment:  enrichment,
		Priority:    r.Priority,
		Enabled:     r.Enabled,
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func (s *pgTriageRuleStore) Create(ctx context.Context, record *TriageRuleRecord) (*TriageRuleRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.Name == "" {
		return nil, errors.New("name is required")
	}
	if record.Decision == "" {
		return nil, errors.New("decision is required")
	}
	if record.MatchMode == "" {
		record.MatchMode = "all"
	}
	if record.Conditions == nil {
		record.Conditions = []map[string]any{}
	}
	if record.Enrichment == nil {
		record.Enrichment = map[string]any{}
	}

	var severity *string
	if record.Severity != "" {
		severity = &record.Severity
	}
	var category *string
	if record.Category != "" {
		category = &record.Category
	}

	m := &models.TriageRule{
		ID:          models.NewUUID(),
		Name:        record.Name,
		Description: record.Description,
		Conditions:  record.Conditions,
		MatchMode:   record.MatchMode,
		Decision:    record.Decision,
		Severity:    severity,
		Category:    category,
		Enrichment:  record.Enrichment,
		Priority:    record.Priority,
		Enabled:     record.Enabled,
		CreatedBy:   record.CreatedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("insert triage rule: %w", err)
	}
	record.ID = m.ID
	return record, nil
}

func (s *pgTriageRuleStore) Update(ctx context.Context, id string, patch *TriageRuleRecord) (*TriageRuleRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	q := s.db.NewUpdate().Model((*models.TriageRule)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", uid)

	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if patch.Description != "" {
		q = q.Set("description = ?", patch.Description)
	}
	if patch.Conditions != nil {
		q = q.Set("conditions = ?", patch.Conditions)
	}
	if patch.MatchMode != "" {
		q = q.Set("match_mode = ?", patch.MatchMode)
	}
	if patch.Decision != "" {
		q = q.Set("decision = ?", patch.Decision)
	}
	if patch.Severity != "" {
		q = q.Set("severity = ?", patch.Severity)
	}
	if patch.Category != "" {
		q = q.Set("category = ?", patch.Category)
	}
	if patch.Enrichment != nil {
		q = q.Set("enrichment = ?", patch.Enrichment)
	}
	if patch.Priority != 0 {
		q = q.Set("priority = ?", patch.Priority)
	}
	// Enabled field: always set since it's a required bool (can't meaningfully detect "not provided")
	q = q.Set("enabled = ?", patch.Enabled)

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update triage rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, errors.New("triage rule not found")
	}

	// Re-fetch to return the updated record
	var updated models.TriageRule
	if err := s.db.NewSelect().Model(&updated).Where("id = ?", uid).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to re-fetch triage rule: %w", err)
	}
	return pgTriageRuleToRecord(&updated), nil
}

func (s *pgTriageRuleStore) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	res, err := s.db.NewDelete().Model((*models.TriageRule)(nil)).Where("id = ?", uid).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete triage rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("triage rule not found")
	}
	return nil
}

func (s *pgTriageRuleStore) Get(ctx context.Context, id string) (*TriageRuleRecord, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}
	var r models.TriageRule
	err = s.db.NewSelect().Model(&r).Where("id = ?", uid).Scan(ctx)
	if err != nil {
		return handleQueryErr[*TriageRuleRecord](err, "triage rule")
	}
	return pgTriageRuleToRecord(&r), nil
}

func (s *pgTriageRuleStore) List(ctx context.Context, q TriageRuleQuery) ([]TriageRuleRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.TriageRule)(nil))
	if q.Enabled != nil {
		countQ = countQ.Where("enabled = ?", *q.Enabled)
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count triage rules: %w", err)
	}

	var items []models.TriageRule
	listQ := s.db.NewSelect().Model(&items).Order("priority ASC")
	if q.Enabled != nil {
		listQ = listQ.Where("enabled = ?", *q.Enabled)
	}
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	err = listQ.Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list triage rules: %w", err)
	}

	var out []TriageRuleRecord
	for i := range items {
		r := &items[i]
		if q.Search != "" {
			text := strings.TrimSpace(strings.ToLower(q.Search))
			if !strings.Contains(strings.ToLower(r.Name), text) &&
				!strings.Contains(strings.ToLower(r.Description), text) &&
				!strings.Contains(strings.ToLower(r.Decision), text) {
				continue
			}
		}
		out = append(out, *pgTriageRuleToRecord(r))
	}
	if out == nil {
		out = []TriageRuleRecord{}
	}
	return out, int64(total), nil
}

func (s *pgTriageRuleStore) ListEnabled(ctx context.Context) ([]TriageRuleRecord, error) {
	var items []models.TriageRule
	err := s.db.NewSelect().Model(&items).
		Where("enabled = ?", true).
		Order("priority ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled triage rules: %w", err)
	}
	var out []TriageRuleRecord
	for i := range items {
		out = append(out, *pgTriageRuleToRecord(&items[i]))
	}
	if out == nil {
		out = []TriageRuleRecord{}
	}
	return out, nil
}

func (s *pgTriageRuleStore) Reorder(ctx context.Context, ids []string) error {
	for i, id := range ids {
		uid, err := uuid.Parse(id)
		if err != nil {
			return fmt.Errorf("invalid id %q: %w", id, err)
		}
		res, err := s.db.NewUpdate().Model((*models.TriageRule)(nil)).
			Set("priority = ?", i).
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", uid).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to reorder triage rule: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("triage rule %q not found", id)
		}
	}
	return nil
}
