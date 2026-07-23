package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/statuspage"
	"alga/ent/statuspagecomponent"
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

func newPGStatusPageStore(client *ent.Client) StatusPageStore {
	return &pgStatusPageStore{pgStoreBase{client: client}}
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

	saved, err := s.client.StatusPage.Create().
		SetName(record.Name).
		SetSlug(record.Slug).
		SetDescription(record.Description).
		SetVisibility(record.Visibility).
		SetEnabled(record.Enabled).
		SetNillableOwnerTeamID(record.OwnerTeamID).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert status page: %w", err)
	}
	return pgStatusPageToRecord(saved), nil
}

func (s *pgStatusPageStore) UpdatePage(ctx context.Context, id uuid.UUID, patch *StatusPageRecord) (*StatusPageRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	b := s.client.StatusPage.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if patch.Name != "" {
		b.SetName(patch.Name)
	}
	if patch.Slug != "" {
		b.SetSlug(patch.Slug)
	}
	if patch.Description != "" {
		b.SetDescription(patch.Description)
	}
	if patch.Visibility != "" {
		b.SetVisibility(patch.Visibility)
	}
	if patch.OwnerTeamID != nil {
		b.SetOwnerTeamID(*patch.OwnerTeamID)
	}
	if patch.EnabledSet {
		b.SetEnabled(patch.Enabled)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrStatusPageNotFound
		}
		return nil, fmt.Errorf("failed to update status page: %w", err)
	}
	return pgStatusPageToRecord(saved), nil
}

func (s *pgStatusPageStore) DeletePage(ctx context.Context, id uuid.UUID) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if _, err := tx.StatusPageComponent.Delete().
		Where(statuspagecomponent.StatusPageIDEQ(id)).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete status page components: %w", err)
	}
	if err := tx.StatusPage.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return ErrStatusPageNotFound
		}
		return fmt.Errorf("failed to delete status page: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *pgStatusPageStore) GetPage(ctx context.Context, id uuid.UUID) (*StatusPageRecord, error) {
	p, err := s.client.StatusPage.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*StatusPageRecord](err, "status page")
	}
	return pgStatusPageToRecord(p), nil
}

func (s *pgStatusPageStore) GetPageBySlug(ctx context.Context, slug string) (*StatusPageRecord, error) {
	p, err := s.client.StatusPage.Query().
		Where(statuspage.SlugEQ(slug)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*StatusPageRecord](err, "status page")
	}
	return pgStatusPageToRecord(p), nil
}

func (s *pgStatusPageStore) ListPages(ctx context.Context, q StatusPageQuery) ([]StatusPageRecord, int64, error) {
	query := s.client.StatusPage.Query()
	if q.Enabled != nil {
		query = query.Where(statuspage.EnabledEQ(*q.Enabled))
	}
	if q.Search != "" {
		query = query.Where(statuspage.NameContains(q.Search))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count status pages: %w", err)
	}
	query = query.Order(ent.Desc(statuspage.FieldCreatedAt))
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}
	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list status pages: %w", err)
	}
	out := make([]StatusPageRecord, 0, len(items))
	for _, p := range items {
		out = append(out, *pgStatusPageToRecord(p))
	}
	return out, int64(total), nil
}

func (s *pgStatusPageStore) ListComponents(ctx context.Context, pageID uuid.UUID) ([]StatusPageComponentRecord, error) {
	items, err := s.client.StatusPageComponent.Query().
		Where(statuspagecomponent.StatusPageIDEQ(pageID)).
		Order(ent.Asc(statuspagecomponent.FieldDisplayOrder), ent.Asc(statuspagecomponent.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list status page components: %w", err)
	}
	out := make([]StatusPageComponentRecord, 0, len(items))
	for _, c := range items {
		out = append(out, *pgStatusPageComponentToRecord(c))
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

	b := s.client.StatusPageComponent.Create().
		SetStatusPageID(record.StatusPageID).
		SetName(record.Name).
		SetDescription(record.Description).
		SetDisplayOrder(record.DisplayOrder).
		SetStatus(record.Status).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if record.ServiceID != nil {
		b.SetServiceID(*record.ServiceID)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert status page component: %w", err)
	}
	return pgStatusPageComponentToRecord(saved), nil
}

func (s *pgStatusPageStore) UpdateComponent(ctx context.Context, id uuid.UUID, patch *StatusPageComponentRecord) (*StatusPageComponentRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	b := s.client.StatusPageComponent.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if patch.Name != "" {
		b.SetName(patch.Name)
	}
	if patch.Description != "" {
		b.SetDescription(patch.Description)
	}
	if patch.Status != "" {
		b.SetStatus(patch.Status)
	}
	if patch.DisplayOrderSet {
		b.SetDisplayOrder(patch.DisplayOrder)
	}
	if patch.ServiceID != nil {
		b.SetServiceID(*patch.ServiceID)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrStatusPageComponentNotFound
		}
		return nil, fmt.Errorf("failed to update status page component: %w", err)
	}
	return pgStatusPageComponentToRecord(saved), nil
}

func (s *pgStatusPageStore) DeleteComponent(ctx context.Context, id uuid.UUID) error {
	err := s.client.StatusPageComponent.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrStatusPageComponentNotFound
		}
		return fmt.Errorf("failed to delete status page component: %w", err)
	}
	return nil
}

func (s *pgStatusPageStore) GetComponent(ctx context.Context, id uuid.UUID) (*StatusPageComponentRecord, error) {
	c, err := s.client.StatusPageComponent.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*StatusPageComponentRecord](err, "status page component")
	}
	return pgStatusPageComponentToRecord(c), nil
}

func pgStatusPageToRecord(p *ent.StatusPage) *StatusPageRecord {
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

func pgStatusPageComponentToRecord(c *ent.StatusPageComponent) *StatusPageComponentRecord {
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
