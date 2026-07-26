package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entservice "alga/ent/service"
	entsd "alga/ent/servicedependency"
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

func newPGServiceStore(client *ent.Client) ServiceStore {
	return &pgServiceStore{pgStoreBase{client: client}}
}

func (s *pgServiceStore) CreateService(ctx context.Context, record *ServiceRecord) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	if record.Status == "" {
		record.Status = "operational"
	}

	b := s.client.Service.Create().
		SetName(record.Name).
		SetDisplayName(record.DisplayName).
		SetDescription(record.Description).
		SetSLAResponseMinutes(record.SLAResponseMinutes).
		SetSLAResolveMinutes(record.SLAResolveMinutes).
		SetStatus(entservice.Status(record.Status)).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if record.OwnerTeamID != nil {
		b.SetOwnerTeamID(*record.OwnerTeamID)
	}
	if record.EscalationPolicyID != nil {
		b.SetEscalationPolicyID(*record.EscalationPolicyID)
	}
	if record.LabelMatchers != nil {
		b.SetLabelMatchers(record.LabelMatchers)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}
	record.ID = saved.ID
	record.CreatedAt = saved.CreatedAt
	record.UpdatedAt = saved.UpdatedAt
	return record, nil
}

func (s *pgServiceStore) GetService(ctx context.Context, id string) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid service ID: %w", err)
	}

	svc, err := s.client.Service.Get(ctx, sid)
	if err != nil {
		return handleQueryErr[*ServiceRecord](err, "service")
	}
	return s.toServiceRecord(svc), nil
}

func (s *pgServiceStore) GetServiceByName(ctx context.Context, name string) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	svc, err := s.client.Service.Query().
		Where(entservice.NameEQ(name)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*ServiceRecord](err, "service by name")
	}
	return s.toServiceRecord(svc), nil
}

func (s *pgServiceStore) UpdateService(ctx context.Context, id string, record *ServiceRecord) (*ServiceRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid service ID: %w", err)
	}

	b := s.client.Service.UpdateOneID(sid).
		SetName(record.Name).
		SetDisplayName(record.DisplayName).
		SetDescription(record.Description).
		SetSLAResponseMinutes(record.SLAResponseMinutes).
		SetSLAResolveMinutes(record.SLAResolveMinutes).
		SetUpdatedAt(time.Now().UTC())

	if record.Status != "" {
		b.SetStatus(entservice.Status(record.Status))
	}
	if record.OwnerTeamID != nil {
		b.SetOwnerTeamID(*record.OwnerTeamID)
	} else {
		b.ClearOwnerTeamID()
	}
	if record.EscalationPolicyID != nil {
		b.SetEscalationPolicyID(*record.EscalationPolicyID)
	} else {
		b.ClearEscalationPolicyID()
	}
	if record.LabelMatchers != nil {
		b.SetLabelMatchers(record.LabelMatchers)
	}

	svc, err := b.Save(ctx)
	if err != nil {
		return handleQueryErr[*ServiceRecord](err, "service")
	}
	return s.toServiceRecord(svc), nil
}

func (s *pgServiceStore) DeleteService(ctx context.Context, id string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid service ID: %w", err)
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer rollbackTx(tx)

	_, err = tx.ServiceDependency.Delete().
		Where(
			entsd.Or(
				entsd.ServiceIDEQ(sid),
				entsd.DependentOnServiceIDEQ(sid),
			),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete service dependencies: %w", err)
	}

	err = tx.Service.DeleteOneID(sid).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("service not found: %w", ErrServiceNotFound)
		}
		return fmt.Errorf("failed to delete service: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit service deletion: %w", err)
	}
	return nil
}

func (s *pgServiceStore) ListServices(ctx context.Context, filter ListServicesFilter) ([]ServiceRecord, int, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	query := s.client.Service.Query().Order(ent.Asc(entservice.FieldName))

	if filter.Status != "" {
		query = query.Where(entservice.StatusEQ(entservice.Status(filter.Status)))
	}
	if filter.Query != "" {
		query = query.Where(
			entservice.Or(
				entservice.NameContains(filter.Query),
				entservice.DisplayNameContains(filter.Query),
				entservice.DescriptionContains(filter.Query),
			),
		)
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count services: %w", err)
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Skip > 0 {
		query = query.Offset(filter.Skip)
	}

	svcs, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list services: %w", err)
	}

	records := make([]ServiceRecord, 0, len(svcs))
	for _, svc := range svcs {
		records = append(records, *s.toServiceRecord(svc))
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

	_, err = s.client.Service.UpdateOneID(sid).
		SetStatus(entservice.Status(status)).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("service not found: %w", ErrServiceNotFound)
		}
		return fmt.Errorf("failed to update service status: %w", err)
	}
	return nil
}

func (s *pgServiceStore) AddDependency(ctx context.Context, serviceID, dependsOnID uuid.UUID, depType string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.client.ServiceDependency.Create().
		SetServiceID(serviceID).
		SetDependentOnServiceID(dependsOnID).
		SetDependencyType(entsd.DependencyType(depType)).
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to add dependency: %w", err)
	}
	return nil
}

func (s *pgServiceStore) RemoveDependency(ctx context.Context, serviceID, targetID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.client.ServiceDependency.Delete().
		Where(
			entsd.ServiceIDEQ(serviceID),
			entsd.DependentOnServiceIDEQ(targetID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove dependency: %w", err)
	}
	return nil
}

func (s *pgServiceStore) GetDependencies(ctx context.Context, serviceID uuid.UUID) ([]ServiceDependencyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	deps, err := s.client.ServiceDependency.Query().
		Where(entsd.ServiceIDEQ(serviceID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependencies: %w", err)
	}

	if len(deps) == 0 {
		return nil, nil
	}

	depIDs := make([]uuid.UUID, len(deps))
	for i, d := range deps {
		depIDs[i] = d.DependentOnServiceID
	}

	services, err := s.client.Service.Query().
		Where(entservice.IDIn(depIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependency names: %w", err)
	}

	nameMap := make(map[uuid.UUID]string, len(services))
	for _, svc := range services {
		nameMap[svc.ID] = svc.Name
	}

	records := make([]ServiceDependencyRecord, 0, len(deps))
	for _, d := range deps {
		rec := ServiceDependencyRecord{
			ID:                     d.ID,
			ServiceID:              d.ServiceID,
			DependentOnServiceID:   d.DependentOnServiceID,
			DependencyType:         string(d.DependencyType),
			CreatedAt:              d.CreatedAt,
			DependentOnServiceName: nameMap[d.DependentOnServiceID],
		}
		records = append(records, rec)
	}
	return records, nil
}

func (s *pgServiceStore) GetDependents(ctx context.Context, serviceID uuid.UUID) ([]ServiceDependencyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	deps, err := s.client.ServiceDependency.Query().
		Where(entsd.DependentOnServiceIDEQ(serviceID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependents: %w", err)
	}

	if len(deps) == 0 {
		return nil, nil
	}

	svcIDs := make([]uuid.UUID, len(deps))
	for i, d := range deps {
		svcIDs[i] = d.ServiceID
	}

	services, err := s.client.Service.Query().
		Where(entservice.IDIn(svcIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependent names: %w", err)
	}

	nameMap := make(map[uuid.UUID]string, len(services))
	for _, svc := range services {
		nameMap[svc.ID] = svc.Name
	}

	records := make([]ServiceDependencyRecord, 0, len(deps))
	for _, d := range deps {
		rec := ServiceDependencyRecord{
			ID:                     d.ID,
			ServiceID:              d.ServiceID,
			DependentOnServiceID:   d.DependentOnServiceID,
			DependencyType:         string(d.DependencyType),
			CreatedAt:              d.CreatedAt,
			DependentOnServiceName: nameMap[d.ServiceID],
		}
		records = append(records, rec)
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

func (s *pgServiceStore) toServiceRecord(svc *ent.Service) *ServiceRecord {
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
		Status:             string(svc.Status),
		CreatedAt:          svc.CreatedAt,
		UpdatedAt:          svc.UpdatedAt,
	}
}
