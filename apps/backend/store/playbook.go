package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
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

func newPGPlaybookStore(db *bun.DB) PlaybookStore {
	return &pgPlaybookStore{pgStoreBase{db: db}}
}

func playbookFromModel(p *models.Playbook) *PlaybookRecord {
	return &PlaybookRecord{
		ID:             p.ID,
		Title:          p.Title,
		Kind:           p.Kind,
		Summary:        p.Summary,
		ServiceID:      p.ServiceID,
		LabelSelectors: p.LabelSelectors,
		Tags:           p.Tags,
		CreatedBy:      p.CreatedBy,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func stepFromModel(s *models.PlaybookStep) PlaybookStepRecord {
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

func (s *pgPlaybookStore) Create(ctx context.Context, r *PlaybookRecord, steps []PlaybookStepRecord) (*PlaybookRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	p := &models.Playbook{
		ID:        models.NewUUID(),
		Title:     r.Title,
		Kind:      r.Kind,
		Summary:   r.Summary,
		ServiceID: r.ServiceID,
		CreatedBy: r.CreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if len(r.LabelSelectors) > 0 {
		p.LabelSelectors = r.LabelSelectors
	}
	if len(r.Tags) > 0 {
		p.Tags = r.Tags
	}

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(p).Exec(ctx); err != nil {
			return fmt.Errorf("failed to create playbook: %w", err)
		}
		for _, step := range steps {
			sm := &models.PlaybookStep{
				ID:               models.NewUUID(),
				PlaybookID:       p.ID,
				StepNumber:       step.StepNumber,
				Title:            step.Title,
				Description:      step.Description,
				ExpectedDuration: step.ExpectedDuration,
				Command:          step.Command,
				CreatedAt:        now,
				UpdatedAt:        now,
			}
			if _, err := tx.NewInsert().Model(sm).Exec(ctx); err != nil {
				return fmt.Errorf("failed to create playbook step: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	r.ID = p.ID
	r.CreatedAt = p.CreatedAt
	r.UpdatedAt = p.UpdatedAt
	return r, nil
}

func (s *pgPlaybookStore) Get(ctx context.Context, id uuid.UUID) (*PlaybookRecord, []PlaybookStepRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var p models.Playbook
	err := s.db.NewSelect().Model(&p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get playbook: %w", err)
	}

	var steps []models.PlaybookStep
	err = s.db.NewSelect().Model(&steps).
		Where("playbook_id = ?", id).
		OrderExpr("step_number ASC").
		Scan(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get playbook steps: %w", err)
	}

	stepRecords := make([]PlaybookStepRecord, 0, len(steps))
	for _, st := range steps {
		stepRecords = append(stepRecords, stepFromModel(&st))
	}

	return playbookFromModel(&p), stepRecords, nil
}

func (s *pgPlaybookStore) Update(ctx context.Context, id uuid.UUID, r *PlaybookRecord) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	upd := s.db.NewUpdate().Model((*models.Playbook)(nil)).
		Set("title = ?", r.Title).
		Set("kind = ?", r.Kind).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id)

	if r.Summary != "" {
		upd = upd.Set("summary = ?", r.Summary)
	} else {
		upd = upd.Set("summary = ?", "")
	}

	if r.ServiceID != nil {
		upd = upd.Set("service_id = ?", *r.ServiceID)
	} else {
		upd = upd.Set("service_id = NULL")
	}

	if len(r.LabelSelectors) > 0 {
		upd = upd.Set("label_selectors = ?", r.LabelSelectors)
	} else {
		upd = upd.Set("label_selectors = NULL")
	}

	if len(r.Tags) > 0 {
		upd = upd.Set("tags = ?", r.Tags)
	} else {
		upd = upd.Set("tags = NULL")
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update playbook: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update playbook: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("playbook not found: %w", ErrNotFound)
	}
	return nil
}

func (s *pgPlaybookStore) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.Playbook)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete playbook: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete playbook: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("playbook not found: %w", ErrNotFound)
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

	countQ := s.db.NewSelect().Model((*models.Playbook)(nil))
	listQ := s.db.NewSelect().Model((*models.Playbook)(nil))

	if filter.Kind != "" {
		countQ = countQ.Where("kind = ?", filter.Kind)
		listQ = listQ.Where("kind = ?", filter.Kind)
	}
	if filter.ServiceID != nil {
		countQ = countQ.Where("service_id = ?", *filter.ServiceID)
		listQ = listQ.Where("service_id = ?", *filter.ServiceID)
	}
	if filter.Search != "" {
		countQ = countQ.Where("(title ILIKE ? OR summary ILIKE ?)", "%"+filter.Search+"%", "%"+filter.Search+"%")
		listQ = listQ.Where("(title ILIKE ? OR summary ILIKE ?)", "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count playbooks: %w", err)
	}

	var playbooks []models.Playbook
	err = listQ.
		OrderExpr("created_at DESC").
		Limit(limit).
		Offset(skip).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list playbooks: %w", err)
	}

	records := make([]*PlaybookRecord, 0, len(playbooks))
	for _, p := range playbooks {
		rec := playbookFromModel(&p)

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

	now := time.Now().UTC()
	m := &models.PlaybookStep{
		ID:               models.NewUUID(),
		PlaybookID:       step.PlaybookID,
		StepNumber:       step.StepNumber,
		Title:            step.Title,
		Description:      step.Description,
		ExpectedDuration: step.ExpectedDuration,
		Command:          step.Command,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to add playbook step: %w", err)
	}

	record := stepFromModel(m)
	return &record, nil
}

func (s *pgPlaybookStore) UpdateStep(ctx context.Context, id uuid.UUID, step *PlaybookStepRecord) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	upd := s.db.NewUpdate().Model((*models.PlaybookStep)(nil)).
		Set("step_number = ?", step.StepNumber).
		Set("title = ?", step.Title).
		Set("description = ?", step.Description).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id)

	if step.ExpectedDuration != "" {
		upd = upd.Set("expected_duration = ?", step.ExpectedDuration)
	} else {
		upd = upd.Set("expected_duration = ?", "")
	}
	if step.Command != "" {
		upd = upd.Set("command = ?", step.Command)
	} else {
		upd = upd.Set("command = ?", "")
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update playbook step: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update playbook step: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("playbook step not found: %w", ErrNotFound)
	}
	return nil
}

func (s *pgPlaybookStore) DeleteStep(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.PlaybookStep)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete playbook step: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete playbook step: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("playbook step not found: %w", ErrNotFound)
	}
	return nil
}

func (s *pgPlaybookStore) ReorderSteps(ctx context.Context, playbookID uuid.UUID, order []StepOrder) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, o := range order {
			_, err := tx.NewUpdate().Model((*models.PlaybookStep)(nil)).
				Set("step_number = ?", o.StepNumber).
				Set("updated_at = ?", time.Now().UTC()).
				Where("id = ?", o.ID).
				Exec(ctx)
			if err != nil {
				return fmt.Errorf("failed to reorder step %s: %w", o.ID, err)
			}
		}
		return nil
	})
}

func (s *pgPlaybookStore) FindMatching(ctx context.Context, labels map[string]string) ([]*PlaybookRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var all []models.Playbook
	err := s.db.NewSelect().Model(&all).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query playbooks: %w", err)
	}

	matched := make([]*PlaybookRecord, 0, len(all))
	for _, p := range all {
		if len(p.LabelSelectors) == 0 {
			continue
		}
		if matchLabelSelectors(p.LabelSelectors, labels) {
			matched = append(matched, playbookFromModel(&p))
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
