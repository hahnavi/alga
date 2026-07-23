package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/playbook"
	"alga/ent/playbookstep"
	"alga/matching"
)

type PlaybookRecord struct {
	ID             uuid.UUID        `json:"id"`
	Title          string           `json:"title"`
	Kind           string           `json:"kind"`
	Summary        string           `json:"summary"`
	ServiceID      *uuid.UUID       `json:"service_id,omitempty"`
	LabelSelectors []map[string]any `json:"label_selectors,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
	CreatedBy      uuid.UUID        `json:"created_by"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type PlaybookStepRecord struct {
	ID               uuid.UUID `json:"id"`
	PlaybookID       uuid.UUID `json:"playbook_id"`
	StepNumber       int       `json:"step_number"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	ExpectedDuration string    `json:"expected_duration,omitempty"`
	Command          string    `json:"command,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PlaybookFilter struct {
	Kind      string
	ServiceID *uuid.UUID
	Tag       string
	Search    string
}

type StepOrder struct {
	ID         uuid.UUID
	StepNumber int
}

type PlaybookStore interface {
	Create(ctx context.Context, r *PlaybookRecord, steps []PlaybookStepRecord) (*PlaybookRecord, error)
	Get(ctx context.Context, id uuid.UUID) (*PlaybookRecord, []PlaybookStepRecord, error)
	Update(ctx context.Context, id uuid.UUID, r *PlaybookRecord) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter PlaybookFilter, limit, skip int) ([]*PlaybookRecord, int64, error)
	AddStep(ctx context.Context, step *PlaybookStepRecord) (*PlaybookStepRecord, error)
	UpdateStep(ctx context.Context, id uuid.UUID, step *PlaybookStepRecord) error
	DeleteStep(ctx context.Context, id uuid.UUID) error
	ReorderSteps(ctx context.Context, playbookID uuid.UUID, order []StepOrder) error
	FindMatching(ctx context.Context, labels map[string]string) ([]*PlaybookRecord, error)
}

type pgPlaybookStore struct {
	pgStoreBase
}

func newPGPlaybookStore(client *ent.Client) PlaybookStore {
	return &pgPlaybookStore{pgStoreBase{client: client}}
}

func playbookFromEnt(p *ent.Playbook) *PlaybookRecord {
	return &PlaybookRecord{
		ID:             p.ID,
		Title:          p.Title,
		Kind:           p.Kind.String(),
		Summary:        p.Summary,
		ServiceID:      p.ServiceID,
		LabelSelectors: p.LabelSelectors,
		Tags:           p.Tags,
		CreatedBy:      p.CreatedBy,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func stepFromEnt(s *ent.PlaybookStep) PlaybookStepRecord {
	return PlaybookStepRecord{
		ID:               s.ID,
		PlaybookID:       s.PlaybookID,
		StepNumber:       s.StepNumber,
		Title:            s.Title,
		Description:      s.Description,
		ExpectedDuration: s.ExpectedDuration,
		Command:          s.Command,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}

func createSteps(ctx context.Context, tx *ent.Tx, playbookID uuid.UUID, steps []PlaybookStepRecord) error {
	for _, step := range steps {
		b := tx.PlaybookStep.Create().
			SetPlaybookID(playbookID).
			SetStepNumber(step.StepNumber).
			SetTitle(step.Title).
			SetCreatedAt(time.Now().UTC()).
			SetUpdatedAt(time.Now().UTC())

		if step.Description != "" {
			b.SetDescription(step.Description)
		}
		if step.ExpectedDuration != "" {
			b.SetExpectedDuration(step.ExpectedDuration)
		}
		if step.Command != "" {
			b.SetCommand(step.Command)
		}

		if _, err := b.Save(ctx); err != nil {
			return fmt.Errorf("failed to create playbook step: %w", err)
		}
	}
	return nil
}

func (s *pgPlaybookStore) Create(ctx context.Context, r *PlaybookRecord, steps []PlaybookStepRecord) (*PlaybookRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer rollbackTx(tx)

	b := tx.Playbook.Create().
		SetTitle(r.Title).
		SetKind(playbook.Kind(r.Kind)).
		SetCreatedBy(r.CreatedBy).
		SetCreatedAt(time.Now().UTC()).
		SetUpdatedAt(time.Now().UTC())

	if r.Summary != "" {
		b.SetSummary(r.Summary)
	}
	if r.ServiceID != nil {
		b.SetServiceID(*r.ServiceID)
	}
	if len(r.LabelSelectors) > 0 {
		b.SetLabelSelectors(r.LabelSelectors)
	}
	if len(r.Tags) > 0 {
		b.SetTags(r.Tags)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create playbook: %w", err)
	}

	if err := createSteps(ctx, tx, saved.ID, steps); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.ID = saved.ID
	r.CreatedAt = saved.CreatedAt
	r.UpdatedAt = saved.UpdatedAt
	return r, nil
}

func (s *pgPlaybookStore) Get(ctx context.Context, id uuid.UUID) (*PlaybookRecord, []PlaybookStepRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	p, err := s.client.Playbook.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get playbook: %w", err)
	}

	steps, err := s.client.PlaybookStep.Query().
		Where(playbookstep.PlaybookIDEQ(id)).
		Order(ent.Asc(playbookstep.FieldStepNumber)).
		All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get playbook steps: %w", err)
	}

	stepRecords := make([]PlaybookStepRecord, 0, len(steps))
	for _, s := range steps {
		stepRecords = append(stepRecords, stepFromEnt(s))
	}

	return playbookFromEnt(p), stepRecords, nil
}

func (s *pgPlaybookStore) Update(ctx context.Context, id uuid.UUID, r *PlaybookRecord) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	ub := s.client.Playbook.UpdateOneID(id).
		SetTitle(r.Title).
		SetKind(playbook.Kind(r.Kind)).
		SetUpdatedAt(time.Now().UTC())

	if r.Summary != "" {
		ub.SetSummary(r.Summary)
	} else {
		ub.ClearSummary()
	}

	if r.ServiceID != nil {
		ub.SetServiceID(*r.ServiceID)
	} else {
		ub.ClearServiceID()
	}

	if len(r.LabelSelectors) > 0 {
		ub.SetLabelSelectors(r.LabelSelectors)
	} else {
		ub.ClearLabelSelectors()
	}

	if len(r.Tags) > 0 {
		ub.SetTags(r.Tags)
	} else {
		ub.ClearTags()
	}

	_, err := ub.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("playbook not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to update playbook: %w", err)
	}
	return nil
}

func (s *pgPlaybookStore) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	err := s.client.Playbook.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("playbook not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete playbook: %w", err)
	}
	return nil
}

func (s *pgPlaybookStore) List(ctx context.Context, filter PlaybookFilter, limit, skip int) ([]*PlaybookRecord, int64, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)

	q := s.client.Playbook.Query()

	if filter.Kind != "" {
		q.Where(playbook.KindEQ(playbook.Kind(filter.Kind)))
	}
	if filter.ServiceID != nil {
		q.Where(playbook.ServiceIDEQ(*filter.ServiceID))
	}
	if filter.Search != "" {
		q.Where(playbook.Or(
			playbook.TitleContainsFold(filter.Search),
			playbook.SummaryContainsFold(filter.Search),
		))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count playbooks: %w", err)
	}

	playbooks, err := q.
		Order(ent.Desc(playbook.FieldCreatedAt)).
		Limit(limit).
		Offset(skip).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list playbooks: %w", err)
	}

	records := make([]*PlaybookRecord, 0, len(playbooks))
	for _, p := range playbooks {
		rec := playbookFromEnt(p)

		if filter.Tag != "" {
			found := false
			for _, t := range p.Tags {
				if t == filter.Tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		records = append(records, rec)
	}

	if filter.Tag != "" {
		total = len(records)
	}

	return records, int64(total), nil
}

func (s *pgPlaybookStore) AddStep(ctx context.Context, step *PlaybookStepRecord) (*PlaybookStepRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	b := s.client.PlaybookStep.Create().
		SetPlaybookID(step.PlaybookID).
		SetStepNumber(step.StepNumber).
		SetTitle(step.Title).
		SetCreatedAt(time.Now().UTC()).
		SetUpdatedAt(time.Now().UTC())

	if step.Description != "" {
		b.SetDescription(step.Description)
	}
	if step.ExpectedDuration != "" {
		b.SetExpectedDuration(step.ExpectedDuration)
	}
	if step.Command != "" {
		b.SetCommand(step.Command)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to add playbook step: %w", err)
	}

	record := stepFromEnt(saved)
	return &record, nil
}

func (s *pgPlaybookStore) UpdateStep(ctx context.Context, id uuid.UUID, step *PlaybookStepRecord) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	ub := s.client.PlaybookStep.UpdateOneID(id).
		SetStepNumber(step.StepNumber).
		SetTitle(step.Title).
		SetDescription(step.Description).
		SetUpdatedAt(time.Now().UTC())

	if step.ExpectedDuration != "" {
		ub.SetExpectedDuration(step.ExpectedDuration)
	} else {
		ub.ClearExpectedDuration()
	}
	if step.Command != "" {
		ub.SetCommand(step.Command)
	} else {
		ub.ClearCommand()
	}

	_, err := ub.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("playbook step not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to update playbook step: %w", err)
	}
	return nil
}

func (s *pgPlaybookStore) DeleteStep(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	err := s.client.PlaybookStep.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("playbook step not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete playbook step: %w", err)
	}
	return nil
}

func (s *pgPlaybookStore) ReorderSteps(ctx context.Context, playbookID uuid.UUID, order []StepOrder) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer rollbackTx(tx)

	for _, o := range order {
		_, err := tx.PlaybookStep.UpdateOneID(o.ID).
			SetStepNumber(o.StepNumber).
			SetUpdatedAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to reorder step %s: %w", o.ID, err)
		}
	}

	return tx.Commit()
}

func (s *pgPlaybookStore) FindMatching(ctx context.Context, labels map[string]string) ([]*PlaybookRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	all, err := s.client.Playbook.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query playbooks: %w", err)
	}

	matched := make([]*PlaybookRecord, 0, len(all))
	for _, p := range all {
		if len(p.LabelSelectors) == 0 {
			continue
		}
		if matchLabelSelectors(p.LabelSelectors, labels) {
			matched = append(matched, playbookFromEnt(p))
		}
	}

	return matched, nil
}

func matchLabelSelectors(selectors []map[string]any, labels map[string]string) bool {
	for _, selector := range selectors {
		if matchSingleSelector(selector, labels) {
			return true
		}
	}
	return false
}

func matchSingleSelector(selector map[string]any, labels map[string]string) bool {
	for key, val := range selector {
		labelVal, ok := labels[key]
		if !ok {
			return false
		}
		strVal, ok := val.(string)
		if !ok {
			return false
		}
		if !matchLabelValue(strVal, labelVal) {
			return false
		}
	}
	return true
}

func matchLabelValue(pattern, value string) bool {
	if strings.HasPrefix(pattern, "~") {
		re, err := matching.GetCompiledRegex(strings.TrimPrefix(pattern, "~"))
		if err != nil {
			return false
		}
		return re.MatchString(value)
	}
	return pattern == value
}
