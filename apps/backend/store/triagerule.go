package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/triagerule"
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

func newPGTriageRuleStore(client *ent.Client) TriageRuleStore {
	return &pgTriageRuleStore{pgStoreBase{client: client}}
}

func pgTriageRuleToRecord(r *ent.TriageRule) *TriageRuleRecord {
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
	var createdBy *uuid.UUID
	if r.CreatedBy != uuid.Nil {
		createdBy = &r.CreatedBy
	}
	return &TriageRuleRecord{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Conditions:  conditions,
		MatchMode:   r.MatchMode,
		Decision:    r.Decision,
		Severity:    r.Severity,
		Category:    r.Category,
		Enrichment:  enrichment,
		Priority:    r.Priority,
		Enabled:     r.Enabled,
		CreatedBy:   createdBy,
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
	if record.Conditions == nil {
		record.Conditions = []map[string]any{}
	}
	if record.Enrichment == nil {
		record.Enrichment = map[string]any{}
	}

	b := s.client.TriageRule.Create().
		SetName(record.Name).
		SetDescription(record.Description).
		SetConditions(record.Conditions).
		SetMatchMode(record.MatchMode).
		SetDecision(record.Decision).
		SetSeverity(record.Severity).
		SetCategory(record.Category).
		SetEnrichment(record.Enrichment).
		SetPriority(record.Priority).
		SetEnabled(record.Enabled).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if record.CreatedBy != nil {
		b.SetNillableCreatedBy(record.CreatedBy)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("insert triage rule: %w", err)
	}
	record.ID = saved.ID
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

	b := s.client.TriageRule.UpdateOneID(uid).SetUpdatedAt(time.Now().UTC())

	if patch.Name != "" {
		b.SetName(patch.Name)
	}
	if patch.Description != "" {
		b.SetDescription(patch.Description)
	}
	if patch.Conditions != nil {
		b.SetConditions(patch.Conditions)
	}
	if patch.MatchMode != "" {
		b.SetMatchMode(patch.MatchMode)
	}
	if patch.Decision != "" {
		b.SetDecision(patch.Decision)
	}
	if patch.Severity != "" {
		b.SetSeverity(patch.Severity)
	}
	if patch.Category != "" {
		b.SetCategory(patch.Category)
	}
	if patch.Enrichment != nil {
		b.SetEnrichment(patch.Enrichment)
	}
	if patch.Priority != 0 {
		b.SetPriority(patch.Priority)
	}
	// Enabled field: always set since it's a required bool (can't meaningfully detect "not provided")
	b.SetEnabled(patch.Enabled)

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.New("triage rule not found")
		}
		return nil, fmt.Errorf("failed to update triage rule: %w", err)
	}
	return pgTriageRuleToRecord(saved), nil
}

func (s *pgTriageRuleStore) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	err = s.client.TriageRule.DeleteOneID(uid).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.New("triage rule not found")
		}
		return fmt.Errorf("failed to delete triage rule: %w", err)
	}
	return nil
}

func (s *pgTriageRuleStore) Get(ctx context.Context, id string) (*TriageRuleRecord, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}
	r, err := s.client.TriageRule.Get(ctx, uid)
	if err != nil {
		return handleQueryErr[*TriageRuleRecord](err, "triage rule")
	}
	return pgTriageRuleToRecord(r), nil
}

func (s *pgTriageRuleStore) List(ctx context.Context, q TriageRuleQuery) ([]TriageRuleRecord, int64, error) {
	query := s.client.TriageRule.Query()

	if q.Enabled != nil {
		query = query.Where(triagerule.Enabled(*q.Enabled))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count triage rules: %w", err)
	}

	query = query.Order(ent.Asc(triagerule.FieldPriority))

	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}

	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list triage rules: %w", err)
	}

	var out []TriageRuleRecord
	for _, r := range items {
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
	items, err := s.client.TriageRule.Query().
		Where(triagerule.Enabled(true)).
		Order(ent.Asc(triagerule.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled triage rules: %w", err)
	}
	var out []TriageRuleRecord
	for _, r := range items {
		out = append(out, *pgTriageRuleToRecord(r))
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
		_, err = s.client.TriageRule.UpdateOneID(uid).
			SetPriority(i).
			SetUpdatedAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return fmt.Errorf("triage rule %q not found", id)
			}
			return fmt.Errorf("failed to reorder triage rule: %w", err)
		}
	}
	return nil
}
