package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/maintenancewindow"
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

func newPGMaintenanceWindowStore(client *ent.Client) MaintenanceWindowStore {
	return &pgMaintenanceWindowStore{pgStoreBase{client: client}}
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

	saved, err := s.client.MaintenanceWindow.Create().
		SetName(record.Name).
		SetStartTime(record.StartTime).
		SetEndTime(record.EndTime).
		SetLabelMatchers(record.LabelMatchers).
		SetCreatedBy(record.CreatedBy).
		SetEnabled(record.Enabled).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert maintenance window: %w", err)
	}
	record.ID = saved.ID
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

	b := s.client.MaintenanceWindow.UpdateOneID(uid).SetUpdatedAt(time.Now().UTC())

	if patch.Name != "" {
		b.SetName(patch.Name)
	}
	if !patch.StartTime.IsZero() {
		b.SetStartTime(patch.StartTime)
	}
	if !patch.EndTime.IsZero() {
		b.SetEndTime(patch.EndTime)
	}
	if patch.LabelMatchers != nil {
		b.SetLabelMatchers(patch.LabelMatchers)
	}
	if patch.CreatedBy != "" {
		b.SetCreatedBy(patch.CreatedBy)
	}
	if patch.EnabledSet {
		b.SetEnabled(patch.Enabled)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.New("maintenance window not found")
		}
		return nil, fmt.Errorf("failed to update maintenance window: %w", err)
	}
	return pgMaintenanceWindowToRecord(saved), nil
}

func (s *pgMaintenanceWindowStore) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	err = s.client.MaintenanceWindow.DeleteOneID(uid).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.New("maintenance window not found")
		}
		return fmt.Errorf("failed to delete maintenance window: %w", err)
	}
	return nil
}

func (s *pgMaintenanceWindowStore) Get(ctx context.Context, id string) (*MaintenanceWindowRecord, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}
	mw, err := s.client.MaintenanceWindow.Get(ctx, uid)
	if err != nil {
		return handleQueryErr[*MaintenanceWindowRecord](err, "maintenance window")
	}
	return pgMaintenanceWindowToRecord(mw), nil
}

func (s *pgMaintenanceWindowStore) List(ctx context.Context, q MaintenanceWindowQuery) ([]MaintenanceWindowRecord, int64, error) {
	query := s.client.MaintenanceWindow.Query()

	if q.Enabled != nil {
		query = query.Where(maintenancewindow.EnabledEQ(*q.Enabled))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count maintenance windows: %w", err)
	}

	query = query.Order(ent.Desc(maintenancewindow.FieldCreatedAt))

	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}

	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list maintenance windows: %w", err)
	}

	out := make([]MaintenanceWindowRecord, 0, len(items))
	for _, mw := range items {
		out = append(out, *pgMaintenanceWindowToRecord(mw))
	}
	return out, int64(total), nil
}

func (s *pgMaintenanceWindowStore) ListActive(ctx context.Context) ([]MaintenanceWindowRecord, error) {
	now := time.Now().UTC()
	items, err := s.client.MaintenanceWindow.Query().
		Where(
			maintenancewindow.EnabledEQ(true),
			maintenancewindow.StartTimeLTE(now),
			maintenancewindow.EndTimeGTE(now),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active maintenance windows: %w", err)
	}
	out := make([]MaintenanceWindowRecord, 0, len(items))
	for _, mw := range items {
		out = append(out, *pgMaintenanceWindowToRecord(mw))
	}
	return out, nil
}

func pgMaintenanceWindowToRecord(mw *ent.MaintenanceWindow) *MaintenanceWindowRecord {
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
