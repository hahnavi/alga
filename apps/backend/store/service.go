package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type ServiceRecord struct {
	ID                  uuid.UUID                 `json:"id"`
	Name                string                    `json:"name"`
	DisplayName         string                    `json:"display_name"`
	Description         string                    `json:"description"`
	OwnerTeamID         *uuid.UUID                `json:"owner_team_id,omitempty"`
	EscalationPolicyID  *uuid.UUID                `json:"escalation_policy_id,omitempty"`
	LabelMatchers       []map[string]any          `json:"label_matchers,omitempty"`
	SLAResponseMinutes  int                       `json:"sla_response_minutes"`
	SLAResolveMinutes   int                       `json:"sla_resolve_minutes"`
	Status              string                    `json:"status"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
	ActiveIncidentCount int                       `json:"active_incident_count,omitempty"`
	Dependencies        []ServiceDependencyRecord `json:"dependencies,omitempty"`
	Dependents          []ServiceDependencyRecord `json:"dependents,omitempty"`
}

type ServiceDependencyRecord struct {
	ID                     uuid.UUID `json:"id"`
	ServiceID              uuid.UUID `json:"service_id"`
	DependentOnServiceID   uuid.UUID `json:"dependent_on_service_id"`
	DependencyType         string    `json:"dependency_type"`
	CreatedAt              time.Time `json:"created_at"`
	DependentOnServiceName string    `json:"dependent_on_service_name,omitempty"`
}

type ListServicesFilter struct {
	Status string
	Query  string
	Limit  int
	Skip   int
}

type ServiceStore interface {
	CreateService(ctx context.Context, record *ServiceRecord) (*ServiceRecord, error)
	GetService(ctx context.Context, id string) (*ServiceRecord, error)
	GetServiceByName(ctx context.Context, name string) (*ServiceRecord, error)
	UpdateService(ctx context.Context, id string, record *ServiceRecord) (*ServiceRecord, error)
	DeleteService(ctx context.Context, id string) error
	ListServices(ctx context.Context, filter ListServicesFilter) ([]ServiceRecord, int, error)
	UpdateServiceStatus(ctx context.Context, id string, status string) error
	AddDependency(ctx context.Context, serviceID, dependsOnID uuid.UUID, depType string) error
	RemoveDependency(ctx context.Context, serviceID, targetID uuid.UUID) error
	GetDependencies(ctx context.Context, serviceID uuid.UUID) ([]ServiceDependencyRecord, error)
	GetDependents(ctx context.Context, serviceID uuid.UUID) ([]ServiceDependencyRecord, error)
	HasCircularDependency(ctx context.Context, serviceID, dependsOnID uuid.UUID) (bool, error)
}

type pgServiceStore struct {
	pgStoreBase
}

func newPGServiceStore(db *bun.DB) ServiceStore {
	return &pgServiceStore{pgStoreBase{db: db}}
}

func (s *pgServiceStore) CreateService(ctx context.Context, record *ServiceRecord) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	if record.Status == "" {
		record.Status = "operational"
	}

	m := &models.Service{
		ID:                 models.NewUUID(),
		Name:               record.Name,
		DisplayName:        record.DisplayName,
		Description:        record.Description,
		OwnerTeamID:        record.OwnerTeamID,
		EscalationPolicyID: record.EscalationPolicyID,
		LabelMatchers:      record.LabelMatchers,
		SLAResponseMinutes: record.SLAResponseMinutes,
		SLAResolveMinutes:  record.SLAResolveMinutes,
		Status:             record.Status,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}
	record.ID = m.ID
	record.CreatedAt = m.CreatedAt
	record.UpdatedAt = m.UpdatedAt
	return record, nil
}

func (s *pgServiceStore) GetService(ctx context.Context, id string) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid service ID: %w", err)
	}

	var svc models.Service
	err = s.db.NewSelect().Model(&svc).Where("id = ?", sid).Scan(ctx)
	if err != nil {
		return handleQueryErr[*ServiceRecord](err, "service")
	}
	return s.toServiceRecord(&svc), nil
}

func (s *pgServiceStore) GetServiceByName(ctx context.Context, name string) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var svc models.Service
	err := s.db.NewSelect().Model(&svc).Where("name = ?", name).Scan(ctx)
	if err != nil {
		return handleQueryErr[*ServiceRecord](err, "service by name")
	}
	return s.toServiceRecord(&svc), nil
}

func (s *pgServiceStore) UpdateService(ctx context.Context, id string, record *ServiceRecord) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid service ID: %w", err)
	}

	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.Service)(nil)).
		Set("name = ?", record.Name).
		Set("display_name = ?", record.DisplayName).
		Set("description = ?", record.Description).
		Set("sla_response_minutes = ?", record.SLAResponseMinutes).
		Set("sla_resolve_minutes = ?", record.SLAResolveMinutes).
		Set("updated_at = ?", now).
		Where("id = ?", sid)

	if record.Status != "" {
		q = q.Set("status = ?", record.Status)
	}
	if record.OwnerTeamID != nil {
		q = q.Set("owner_team_id = ?", *record.OwnerTeamID)
	} else {
		q = q.Set("owner_team_id = NULL")
	}
	if record.EscalationPolicyID != nil {
		q = q.Set("escalation_policy_id = ?", *record.EscalationPolicyID)
	} else {
		q = q.Set("escalation_policy_id = NULL")
	}
	if record.LabelMatchers != nil {
		q = q.Set("label_matchers = ?", record.LabelMatchers)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return handleQueryErr[*ServiceRecord](err, "service")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("service not found: %w", ErrServiceNotFound)
	}

	// Re-fetch to return the updated record
	var svc models.Service
	if err := s.db.NewSelect().Model(&svc).Where("id = ?", sid).Scan(ctx); err != nil {
		return handleQueryErr[*ServiceRecord](err, "service")
	}
	return s.toServiceRecord(&svc), nil
}

func (s *pgServiceStore) DeleteService(ctx context.Context, id string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid service ID: %w", err)
	}

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.ServiceDependency)(nil)).
			Where("service_id = ? OR dependent_on_service_id = ?", sid, sid).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete service dependencies: %w", err)
		}

		res, err := tx.NewDelete().Model((*models.Service)(nil)).Where("id = ?", sid).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete service: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("service not found: %w", ErrServiceNotFound)
		}
		return nil
	})
}

func (s *pgServiceStore) ListServices(ctx context.Context, filter ListServicesFilter) ([]ServiceRecord, int, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	countQ := s.db.NewSelect().Model((*models.Service)(nil))

	if filter.Status != "" {
		countQ = countQ.Where("status = ?", filter.Status)
	}
	if filter.Query != "" {
		pattern := "%" + filter.Query + "%"
		countQ = countQ.Where("(name LIKE ? OR display_name LIKE ? OR description LIKE ?)", pattern, pattern, pattern)
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count services: %w", err)
	}

	var svcs []models.Service
	listQ := s.db.NewSelect().Model(&svcs).Order("name ASC")
	if filter.Status != "" {
		listQ = listQ.Where("status = ?", filter.Status)
	}
	if filter.Query != "" {
		pattern := "%" + filter.Query + "%"
		listQ = listQ.Where("(name LIKE ? OR display_name LIKE ? OR description LIKE ?)", pattern, pattern, pattern)
	}
	if filter.Limit > 0 {
		listQ = listQ.Limit(filter.Limit)
	}
	if filter.Skip > 0 {
		listQ = listQ.Offset(filter.Skip)
	}

	err = listQ.Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list services: %w", err)
	}

	records := make([]ServiceRecord, 0, len(svcs))
	for i := range svcs {
		records = append(records, *s.toServiceRecord(&svcs[i]))
	}
	return records, total, nil
}

func (s *pgServiceStore) UpdateServiceStatus(ctx context.Context, id string, status string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid service ID: %w", err)
	}

	res, err := s.db.NewUpdate().Model((*models.Service)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", sid).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update service status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("service not found: %w", ErrServiceNotFound)
	}
	return nil
}

func (s *pgServiceStore) AddDependency(ctx context.Context, serviceID, dependsOnID uuid.UUID, depType string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	m := &models.ServiceDependency{
		ID:                   models.NewUUID(),
		ServiceID:            serviceID,
		DependentOnServiceID: dependsOnID,
		DependencyType:       depType,
		CreatedAt:            time.Now().UTC(),
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to add dependency: %w", err)
	}
	return nil
}

func (s *pgServiceStore) RemoveDependency(ctx context.Context, serviceID, targetID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.db.NewDelete().Model((*models.ServiceDependency)(nil)).
		Where("service_id = ?", serviceID).
		Where("dependent_on_service_id = ?", targetID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove dependency: %w", err)
	}
	return nil
}

func (s *pgServiceStore) GetDependencies(ctx context.Context, serviceID uuid.UUID) ([]ServiceDependencyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var deps []models.ServiceDependency
	err := s.db.NewSelect().Model(&deps).Where("service_id = ?", serviceID).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependencies: %w", err)
	}

	if len(deps) == 0 {
		return nil, nil
	}

	depIDs := make([]uuid.UUID, len(deps))
	for i := range deps {
		depIDs[i] = deps[i].DependentOnServiceID
	}

	var services []models.Service
	err = s.db.NewSelect().Model(&services).Where("id IN (?)", bun.List(depIDs)).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependency names: %w", err)
	}

	nameMap := make(map[uuid.UUID]string, len(services))
	for i := range services {
		nameMap[services[i].ID] = services[i].Name
	}

	records := make([]ServiceDependencyRecord, 0, len(deps))
	for i := range deps {
		d := &deps[i]
		records = append(records, ServiceDependencyRecord{
			ID:                     d.ID,
			ServiceID:              d.ServiceID,
			DependentOnServiceID:   d.DependentOnServiceID,
			DependencyType:         d.DependencyType,
			CreatedAt:              d.CreatedAt,
			DependentOnServiceName: nameMap[d.DependentOnServiceID],
		})
	}
	return records, nil
}

func (s *pgServiceStore) GetDependents(ctx context.Context, serviceID uuid.UUID) ([]ServiceDependencyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var deps []models.ServiceDependency
	err := s.db.NewSelect().Model(&deps).Where("dependent_on_service_id = ?", serviceID).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependents: %w", err)
	}

	if len(deps) == 0 {
		return nil, nil
	}

	svcIDs := make([]uuid.UUID, len(deps))
	for i := range deps {
		svcIDs[i] = deps[i].ServiceID
	}

	var services []models.Service
	err = s.db.NewSelect().Model(&services).Where("id IN (?)", bun.List(svcIDs)).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependent names: %w", err)
	}

	nameMap := make(map[uuid.UUID]string, len(services))
	for i := range services {
		nameMap[services[i].ID] = services[i].Name
	}

	records := make([]ServiceDependencyRecord, 0, len(deps))
	for i := range deps {
		d := &deps[i]
		records = append(records, ServiceDependencyRecord{
			ID:                     d.ID,
			ServiceID:              d.ServiceID,
			DependentOnServiceID:   d.DependentOnServiceID,
			DependencyType:         d.DependencyType,
			CreatedAt:              d.CreatedAt,
			DependentOnServiceName: nameMap[d.ServiceID],
		})
	}
	return records, nil
}

func (s *pgServiceStore) HasCircularDependency(ctx context.Context, serviceID, dependsOnID uuid.UUID) (bool, error) {
	visited := map[uuid.UUID]bool{dependsOnID: true}
	queue := []uuid.UUID{dependsOnID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		deps, err := s.GetDependencies(ctx, current)
		if err != nil {
			return false, err
		}
		for _, dep := range deps {
			if dep.DependentOnServiceID == serviceID {
				return true, nil
			}
			if !visited[dep.DependentOnServiceID] {
				visited[dep.DependentOnServiceID] = true
				queue = append(queue, dep.DependentOnServiceID)
			}
		}
	}
	return false, nil
}

func (s *pgServiceStore) toServiceRecord(svc *models.Service) *ServiceRecord {
	return &ServiceRecord{
		ID:                 svc.ID,
		Name:               svc.Name,
		DisplayName:        svc.DisplayName,
		Description:        svc.Description,
		OwnerTeamID:        svc.OwnerTeamID,
		EscalationPolicyID: svc.EscalationPolicyID,
		LabelMatchers:      svc.LabelMatchers,
		SLAResponseMinutes: svc.SLAResponseMinutes,
		SLAResolveMinutes:  svc.SLAResolveMinutes,
		Status:             svc.Status,
		CreatedAt:          svc.CreatedAt,
		UpdatedAt:          svc.UpdatedAt,
	}
}
