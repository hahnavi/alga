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

type MaintenanceWindowRecord struct {
	ID            uuid.UUID         `json:"id"`
	Name          string            `json:"name"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	LabelMatchers map[string]string `json:"label_matchers"`
	CreatedBy     string            `json:"created_by"`
	Enabled       bool              `json:"enabled"`
	EnabledSet    bool              `json:"-"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type MaintenanceWindowQuery struct {
	Enabled *bool
	Limit   int
	Skip    int
}

type MaintenanceWindowStore interface {
	Create(ctx context.Context, record *MaintenanceWindowRecord) (*MaintenanceWindowRecord, error)
	Update(ctx context.Context, id string, patch *MaintenanceWindowRecord) (*MaintenanceWindowRecord, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (*MaintenanceWindowRecord, error)
	List(ctx context.Context, q MaintenanceWindowQuery) ([]MaintenanceWindowRecord, int64, error)
	ListActive(ctx context.Context) ([]MaintenanceWindowRecord, error)
}

type pgMaintenanceWindowStore struct {
	pgStoreBase
}

func newPGMaintenanceWindowStore(db *bun.DB) MaintenanceWindowStore {
	return &pgMaintenanceWindowStore{pgStoreBase{db: db}}
}

func (s *pgMaintenanceWindowStore) Create(ctx context.Context, record *MaintenanceWindowRecord) (*MaintenanceWindowRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.LabelMatchers == nil {
		record.LabelMatchers = map[string]string{}
	}

	m := &models.MaintenanceWindow{
		BaseModel:     models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		Name:          record.Name,
		StartTime:     record.StartTime,
		EndTime:       record.EndTime,
		LabelMatchers: record.LabelMatchers,
		CreatedBy:     record.CreatedBy,
		Enabled:       record.Enabled,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert maintenance window: %w", err)
	}
	record.ID = m.ID
	return record, nil
}

func (s *pgMaintenanceWindowStore) Update(ctx context.Context, id string, patch *MaintenanceWindowRecord) (*MaintenanceWindowRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.MaintenanceWindow)(nil)).
		Set("updated_at = ?", now).
		Where("id = ?", uid)

	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if !patch.StartTime.IsZero() {
		q = q.Set("start_time = ?", patch.StartTime)
	}
	if !patch.EndTime.IsZero() {
		q = q.Set("end_time = ?", patch.EndTime)
	}
	if patch.LabelMatchers != nil {
		q = q.Set("label_matchers = ?", patch.LabelMatchers)
	}
	if patch.CreatedBy != "" {
		q = q.Set("created_by = ?", patch.CreatedBy)
	}
	if patch.EnabledSet {
		q = q.Set("enabled = ?", patch.Enabled)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update maintenance window: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update maintenance window: %w", err)
	}
	if n == 0 {
		return nil, errors.New("maintenance window not found")
	}

	// Reload the updated record.
	updated := new(models.MaintenanceWindow)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", uid).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload maintenance window: %w", err)
	}
	return pgMaintenanceWindowToRecord(updated), nil
}

func (s *pgMaintenanceWindowStore) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	res, err := s.db.NewDelete().Model((*models.MaintenanceWindow)(nil)).Where("id = ?", uid).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete maintenance window: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete maintenance window: %w", err)
	}
	if n == 0 {
		return errors.New("maintenance window not found")
	}
	return nil
}

func (s *pgMaintenanceWindowStore) Get(ctx context.Context, id string) (*MaintenanceWindowRecord, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}
	mw := new(models.MaintenanceWindow)
	err = s.db.NewSelect().Model(mw).Where("id = ?", uid).Scan(ctx)
	if err != nil {
		return handleQueryErr[*MaintenanceWindowRecord](err, "maintenance window")
	}
	return pgMaintenanceWindowToRecord(mw), nil
}

func (s *pgMaintenanceWindowStore) List(ctx context.Context, q MaintenanceWindowQuery) ([]MaintenanceWindowRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.MaintenanceWindow)(nil))
	listQ := s.db.NewSelect().Model((*models.MaintenanceWindow)(nil))

	if q.Enabled != nil {
		countQ = countQ.Where("enabled = ?", *q.Enabled)
		listQ = listQ.Where("enabled = ?", *q.Enabled)
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count maintenance windows: %w", err)
	}

	listQ = listQ.Order("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var items []models.MaintenanceWindow
	err = listQ.Scan(ctx, &items)
	if err != nil {
		return nil, 0, fmt.Errorf("list maintenance windows: %w", err)
	}

	out := make([]MaintenanceWindowRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgMaintenanceWindowToRecord(&items[i]))
	}
	return out, int64(total), nil
}

func (s *pgMaintenanceWindowStore) ListActive(ctx context.Context) ([]MaintenanceWindowRecord, error) {
	now := time.Now().UTC()
	var items []models.MaintenanceWindow
	err := s.db.NewSelect().Model(&items).
		Where("enabled = ?", true).
		Where("start_time <= ?", now).
		Where("end_time >= ?", now).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active maintenance windows: %w", err)
	}
	out := make([]MaintenanceWindowRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgMaintenanceWindowToRecord(&items[i]))
	}
	return out, nil
}

func pgMaintenanceWindowToRecord(mw *models.MaintenanceWindow) *MaintenanceWindowRecord {
	var labelMatchers map[string]string
	if mw.LabelMatchers != nil {
		labelMatchers = mw.LabelMatchers
	} else {
		labelMatchers = map[string]string{}
	}
	return &MaintenanceWindowRecord{
		ID:            mw.ID,
		Name:          mw.Name,
		StartTime:     mw.StartTime,
		EndTime:       mw.EndTime,
		LabelMatchers: labelMatchers,
		CreatedBy:     mw.CreatedBy,
		Enabled:       mw.Enabled,
		CreatedAt:     mw.CreatedAt,
		UpdatedAt:     mw.UpdatedAt,
	}
}
