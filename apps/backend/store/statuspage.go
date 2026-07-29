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
	StatusComponentOperational   = "operational"
	StatusComponentDegraded      = "degraded"
	StatusComponentPartialOutage = "partial_outage"
	StatusComponentMajorOutage   = "major_outage"
	StatusComponentMaintenance   = "maintenance"

	StatusPageVisibilityInternal = "internal"
	StatusPageVisibilityPublic   = "public"
)

type StatusPageRecord struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description string     `json:"description"`
	Visibility  string     `json:"visibility"`
	Enabled     bool       `json:"enabled"`
	EnabledSet  bool       `json:"-"`
	OwnerTeamID *uuid.UUID `json:"owner_team_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type StatusPageComponentRecord struct {
	ID              uuid.UUID  `json:"id"`
	StatusPageID    uuid.UUID  `json:"status_page_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	ServiceID       *uuid.UUID `json:"service_id,omitempty"`
	DisplayOrder    int        `json:"display_order"`
	DisplayOrderSet bool       `json:"-"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type StatusPageQuery struct {
	Enabled *bool
	Search  string
	Limit   int
	Skip    int
}

type StatusPageStore interface {
	// Pages
	CreatePage(ctx context.Context, record *StatusPageRecord) (*StatusPageRecord, error)
	UpdatePage(ctx context.Context, id uuid.UUID, patch *StatusPageRecord) (*StatusPageRecord, error)
	DeletePage(ctx context.Context, id uuid.UUID) error
	GetPage(ctx context.Context, id uuid.UUID) (*StatusPageRecord, error)
	GetPageBySlug(ctx context.Context, slug string) (*StatusPageRecord, error)
	ListPages(ctx context.Context, q StatusPageQuery) ([]StatusPageRecord, int64, error)

	// Components
	ListComponents(ctx context.Context, pageID uuid.UUID) ([]StatusPageComponentRecord, error)
	CreateComponent(ctx context.Context, record *StatusPageComponentRecord) (*StatusPageComponentRecord, error)
	UpdateComponent(ctx context.Context, id uuid.UUID, patch *StatusPageComponentRecord) (*StatusPageComponentRecord, error)
	DeleteComponent(ctx context.Context, id uuid.UUID) error
	GetComponent(ctx context.Context, id uuid.UUID) (*StatusPageComponentRecord, error)
}

type pgStatusPageStore struct {
	pgStoreBase
}

func newPGStatusPageStore(db *bun.DB) StatusPageStore {
	return &pgStatusPageStore{pgStoreBase{db: db}}
}

func (s *pgStatusPageStore) CreatePage(ctx context.Context, record *StatusPageRecord) (*StatusPageRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	if record.Visibility == "" {
		record.Visibility = StatusPageVisibilityInternal
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	m := &models.StatusPage{
		ID:          models.NewUUID(),
		Name:        record.Name,
		Slug:        record.Slug,
		Description: record.Description,
		Visibility:  record.Visibility,
		Enabled:     record.Enabled,
		OwnerTeamID: record.OwnerTeamID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert status page: %w", err)
	}
	return pgStatusPageToRecord(m), nil
}

func (s *pgStatusPageStore) UpdatePage(ctx context.Context, id uuid.UUID, patch *StatusPageRecord) (*StatusPageRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.StatusPage)(nil)).
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if patch.Slug != "" {
		q = q.Set("slug = ?", patch.Slug)
	}
	if patch.Description != "" {
		q = q.Set("description = ?", patch.Description)
	}
	if patch.Visibility != "" {
		q = q.Set("visibility = ?", patch.Visibility)
	}
	if patch.OwnerTeamID != nil {
		q = q.Set("owner_team_id = ?", *patch.OwnerTeamID)
	}
	if patch.EnabledSet {
		q = q.Set("enabled = ?", patch.Enabled)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update status page: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update status page: %w", err)
	}
	if n == 0 {
		return nil, ErrStatusPageNotFound
	}

	// Reload the updated record.
	updated := new(models.StatusPage)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload status page: %w", err)
	}
	return pgStatusPageToRecord(updated), nil
}

func (s *pgStatusPageStore) DeletePage(ctx context.Context, id uuid.UUID) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*models.StatusPageComponent)(nil)).
			Where("status_page_id = ?", id).Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete status page components: %w", err)
		}
		res, err := tx.NewDelete().Model((*models.StatusPage)(nil)).
			Where("id = ?", id).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete status page: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to delete status page: %w", err)
		}
		if n == 0 {
			return ErrStatusPageNotFound
		}
		return nil
	})
}

func (s *pgStatusPageStore) GetPage(ctx context.Context, id uuid.UUID) (*StatusPageRecord, error) {
	p := new(models.StatusPage)
	err := s.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*StatusPageRecord](err, "status page")
	}
	return pgStatusPageToRecord(p), nil
}

func (s *pgStatusPageStore) GetPageBySlug(ctx context.Context, slug string) (*StatusPageRecord, error) {
	p := new(models.StatusPage)
	err := s.db.NewSelect().Model(p).Where("slug = ?", slug).Scan(ctx)
	if err != nil {
		return handleQueryErr[*StatusPageRecord](err, "status page")
	}
	return pgStatusPageToRecord(p), nil
}

func (s *pgStatusPageStore) ListPages(ctx context.Context, q StatusPageQuery) ([]StatusPageRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.StatusPage)(nil))
	listQ := s.db.NewSelect().Model((*models.StatusPage)(nil))

	if q.Enabled != nil {
		countQ = countQ.Where("enabled = ?", *q.Enabled)
		listQ = listQ.Where("enabled = ?", *q.Enabled)
	}
	if q.Search != "" {
		countQ = countQ.Where("name LIKE ?", "%"+q.Search+"%")
		listQ = listQ.Where("name LIKE ?", "%"+q.Search+"%")
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count status pages: %w", err)
	}

	listQ = listQ.Order("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var items []models.StatusPage
	err = listQ.Scan(ctx, &items)
	if err != nil {
		return nil, 0, fmt.Errorf("list status pages: %w", err)
	}

	out := make([]StatusPageRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgStatusPageToRecord(&items[i]))
	}
	return out, int64(total), nil
}

func (s *pgStatusPageStore) ListComponents(ctx context.Context, pageID uuid.UUID) ([]StatusPageComponentRecord, error) {
	var items []models.StatusPageComponent
	err := s.db.NewSelect().Model(&items).
		Where("status_page_id = ?", pageID).
		Order("display_order ASC", "created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list status page components: %w", err)
	}
	out := make([]StatusPageComponentRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgStatusPageComponentToRecord(&items[i]))
	}
	return out, nil
}

func (s *pgStatusPageStore) CreateComponent(ctx context.Context, record *StatusPageComponentRecord) (*StatusPageComponentRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	if record.Status == "" {
		record.Status = StatusComponentOperational
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	m := &models.StatusPageComponent{
		ID:           models.NewUUID(),
		StatusPageID: record.StatusPageID,
		Name:         record.Name,
		Description:  record.Description,
		ServiceID:    record.ServiceID,
		DisplayOrder: record.DisplayOrder,
		Status:       record.Status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert status page component: %w", err)
	}
	return pgStatusPageComponentToRecord(m), nil
}

func (s *pgStatusPageStore) UpdateComponent(ctx context.Context, id uuid.UUID, patch *StatusPageComponentRecord) (*StatusPageComponentRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.StatusPageComponent)(nil)).
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if patch.Description != "" {
		q = q.Set("description = ?", patch.Description)
	}
	if patch.Status != "" {
		q = q.Set("status = ?", patch.Status)
	}
	if patch.DisplayOrderSet {
		q = q.Set("display_order = ?", patch.DisplayOrder)
	}
	if patch.ServiceID != nil {
		q = q.Set("service_id = ?", *patch.ServiceID)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update status page component: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update status page component: %w", err)
	}
	if n == 0 {
		return nil, ErrStatusPageComponentNotFound
	}

	// Reload the updated record.
	updated := new(models.StatusPageComponent)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload status page component: %w", err)
	}
	return pgStatusPageComponentToRecord(updated), nil
}

func (s *pgStatusPageStore) DeleteComponent(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.StatusPageComponent)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete status page component: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete status page component: %w", err)
	}
	if n == 0 {
		return ErrStatusPageComponentNotFound
	}
	return nil
}

func (s *pgStatusPageStore) GetComponent(ctx context.Context, id uuid.UUID) (*StatusPageComponentRecord, error) {
	c := new(models.StatusPageComponent)
	err := s.db.NewSelect().Model(c).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*StatusPageComponentRecord](err, "status page component")
	}
	return pgStatusPageComponentToRecord(c), nil
}

func pgStatusPageToRecord(p *models.StatusPage) *StatusPageRecord {
	return &StatusPageRecord{
		ID:          p.ID,
		Name:        p.Name,
		Slug:        p.Slug,
		Description: p.Description,
		Visibility:  p.Visibility,
		Enabled:     p.Enabled,
		OwnerTeamID: p.OwnerTeamID,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func pgStatusPageComponentToRecord(c *models.StatusPageComponent) *StatusPageComponentRecord {
	return &StatusPageComponentRecord{
		ID:           c.ID,
		StatusPageID: c.StatusPageID,
		Name:         c.Name,
		Description:  c.Description,
		ServiceID:    c.ServiceID,
		DisplayOrder: c.DisplayOrder,
		Status:       c.Status,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}
