package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/capability"
	"alga/ent"
	entagenttoken "alga/ent/agenttoken"
	enticsrole "alga/ent/icsroleassignment"
	entincident "alga/ent/incident"

	"alga/ics"
)

type ICSRoleRecord struct {
	ID                 uuid.UUID  `json:"id"`
	IncidentNumber     int64      `json:"incident_number"`
	RoleType           string     `json:"role_type"`
	AssigneeType       string     `json:"assignee_type"`
	UserID             *uuid.UUID `json:"user_id,omitempty"`
	UserName           string     `json:"user_name,omitempty"`
	UserEmail          string     `json:"user_email,omitempty"`
	AgentTokenID       *uuid.UUID `json:"agent_token_id,omitempty"`
	AgentName          string     `json:"agent_name,omitempty"`
	AgentType          string     `json:"agent_type,omitempty"`
	AgentRevoked       bool       `json:"agent_revoked,omitempty"`
	ParentAssignmentID *uuid.UUID `json:"parent_assignment_id,omitempty"`
	ScopeDescription   *string    `json:"scope_description,omitempty"`
	Status             string     `json:"status"`
	EndedReason        *string    `json:"ended_reason,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
}

type ICSRoleStore interface {
	AssignRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, userID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ICSRoleRecord, error)
	AssignAgentRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, agentTokenID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ICSRoleRecord, error)
	EndRole(ctx context.Context, assignmentID uuid.UUID, reason ics.EndReason) error
	GetActiveRoles(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error)
	GetActiveIC(ctx context.Context, incidentNumber int64) (*ICSRoleRecord, error)
	GetAllRoles(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error)
	GetDelegationTree(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error)
	GetActiveRolesForAgent(ctx context.Context, agentTokenID uuid.UUID) ([]ICSRoleRecord, error)
	EndAllRolesForIncident(ctx context.Context, incidentNumber int64, reason ics.EndReason) error
	EndRolesForAgent(ctx context.Context, agentTokenID uuid.UUID, reason ics.EndReason) error
}

type pgICSRoleStore struct {
	pgStoreBase
}

func newPGICSRoleStore(client *ent.Client) ICSRoleStore {
	return &pgICSRoleStore{pgStoreBase{client: client}}
}

func (s *pgICSRoleStore) AssignRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, userID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident: %w", err)
	}

	b := s.client.ICSRoleAssignment.Create().
		SetRoleType(string(roleType)).
		SetStatus(string(ics.RoleStatusActive)).
		SetAssigneeType("user").
		SetUserID(userID).
		SetIncidentID(inc.ID).
		SetStartedAt(time.Now().UTC())

	if parentAssignmentID != nil {
		b.SetParentID(*parentAssignmentID)
	}
	if scope != nil {
		b.SetScopeDescription(*scope)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to assign ICS role: %w", err)
	}

	uidCopy := userID
	return &ICSRoleRecord{
		ID:                 saved.ID,
		IncidentNumber:     incidentNumber,
		RoleType:           saved.RoleType,
		AssigneeType:       "user",
		UserID:             &uidCopy,
		ParentAssignmentID: parentAssignmentID,
		ScopeDescription:   saved.ScopeDescription,
		Status:             saved.Status,
		StartedAt:          saved.StartedAt,
	}, nil
}

func (s *pgICSRoleStore) EndRole(ctx context.Context, assignmentID uuid.UUID, reason ics.EndReason) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.client.ICSRoleAssignment.UpdateOneID(assignmentID).
		SetStatus(string(ics.RoleStatusEnded)).
		SetEndedReason(string(reason)).
		SetEndedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("ICS role assignment not found: %w", ErrICSRoleNotFound)
		}
		return fmt.Errorf("failed to end ICS role: %w", err)
	}
	return nil
}

func (s *pgICSRoleStore) GetActiveRoles(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	assignments, err := s.client.ICSRoleAssignment.Query().
		Where(
			enticsrole.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
			enticsrole.StatusEQ(string(ics.RoleStatusActive)),
		).
		WithUser().
		WithAgentToken().
		Order(ent.Asc(enticsrole.FieldStartedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active ICS roles: %w", err)
	}

	records := make([]ICSRoleRecord, 0, len(assignments))
	for _, a := range assignments {
		records = append(records, s.toICSRoleRecord(a, incidentNumber))
	}
	return records, nil
}

func (s *pgICSRoleStore) GetActiveIC(ctx context.Context, incidentNumber int64) (*ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	a, err := s.client.ICSRoleAssignment.Query().
		Where(
			enticsrole.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
			enticsrole.StatusEQ(string(ics.RoleStatusActive)),
			enticsrole.RoleTypeEQ(string(ics.RoleIncidentCommander)),
		).
		WithUser().
		WithAgentToken().
		Only(ctx)
	if err != nil {
		return handleQueryErr[*ICSRoleRecord](err, "active incident commander")
	}

	rec := s.toICSRoleRecord(a, incidentNumber)
	return &rec, nil
}

func (s *pgICSRoleStore) GetAllRoles(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	assignments, err := s.client.ICSRoleAssignment.Query().
		Where(
			enticsrole.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
		).
		WithUser().
		WithAgentToken().
		Order(ent.Asc(enticsrole.FieldStartedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query all ICS roles: %w", err)
	}

	records := make([]ICSRoleRecord, 0, len(assignments))
	for _, a := range assignments {
		records = append(records, s.toICSRoleRecord(a, incidentNumber))
	}
	return records, nil
}

func (s *pgICSRoleStore) GetDelegationTree(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error) {
	return s.GetActiveRoles(ctx, incidentNumber)
}

func (s *pgICSRoleStore) EndAllRolesForIncident(ctx context.Context, incidentNumber int64, reason ics.EndReason) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.client.ICSRoleAssignment.Update().
		Where(
			enticsrole.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
			enticsrole.StatusEQ(string(ics.RoleStatusActive)),
		).
		SetStatus(string(ics.RoleStatusEnded)).
		SetEndedReason(string(reason)).
		SetEndedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to end all ICS roles for incident: %w", err)
	}
	return nil
}

func (s *pgICSRoleStore) AssignAgentRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, agentTokenID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident: %w", err)
	}

	agentToken, err := s.client.AgentToken.Query().
		Where(
			entagenttoken.ID(agentTokenID),
			entagenttoken.Revoked(false),
			entagenttoken.Enabled(true),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
		}
		return nil, fmt.Errorf("failed to find agent: %w", err)
	}
	requiredCap := ics.RoleRequiredCapability(roleType)
	if requiredCap != "" && !capability.Has(agentToken.Capabilities, requiredCap) {
		return nil, fmt.Errorf("%w: role %s requires %s", ErrAgentCapabilityMismatch, roleType, requiredCap)
	}

	b := s.client.ICSRoleAssignment.Create().
		SetRoleType(string(roleType)).
		SetStatus(string(ics.RoleStatusActive)).
		SetAssigneeType("agent").
		SetAgentTokenID(agentTokenID).
		SetIncidentID(inc.ID).
		SetStartedAt(time.Now().UTC())

	if parentAssignmentID != nil {
		b.SetParentID(*parentAssignmentID)
	}
	if scope != nil {
		b.SetScopeDescription(*scope)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to assign agent ICS role: %w", err)
	}

	savedWithAgent, err := s.client.ICSRoleAssignment.Query().
		Where(enticsrole.ID(saved.ID)).
		WithAgentToken().
		Only(ctx)
	if err != nil {
		atid := agentTokenID
		return &ICSRoleRecord{
			ID: saved.ID, IncidentNumber: incidentNumber, RoleType: saved.RoleType,
			AssigneeType: "agent", AgentTokenID: &atid,
			Status: saved.Status, StartedAt: saved.StartedAt,
		}, nil
	}

	rec := s.toICSRoleRecord(savedWithAgent, incidentNumber)
	return &rec, nil
}

func (s *pgICSRoleStore) GetActiveRolesForAgent(ctx context.Context, agentTokenID uuid.UUID) ([]ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	results, err := s.client.ICSRoleAssignment.Query().
		Where(
			enticsrole.HasAgentTokenWith(entagenttoken.ID(agentTokenID)),
			enticsrole.StatusEQ(string(ics.RoleStatusActive)),
		).
		WithIncident().
		WithAgentToken().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent ICS roles: %w", err)
	}

	records := make([]ICSRoleRecord, 0, len(results))
	for _, r := range results {
		incNumber := int64(0)
		if inc := r.Edges.Incident; inc != nil {
			incNumber = inc.IncidentNumber
		}
		records = append(records, s.toICSRoleRecord(r, incNumber))
	}
	return records, nil
}

func (s *pgICSRoleStore) EndRolesForAgent(ctx context.Context, agentTokenID uuid.UUID, reason ics.EndReason) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.client.ICSRoleAssignment.Update().
		Where(
			enticsrole.HasAgentTokenWith(entagenttoken.ID(agentTokenID)),
			enticsrole.StatusEQ(string(ics.RoleStatusActive)),
		).
		SetStatus(string(ics.RoleStatusEnded)).
		SetEndedReason(string(reason)).
		SetEndedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to end agent ICS roles: %w", err)
	}
	return nil
}

func (s *pgICSRoleStore) toICSRoleRecord(a *ent.ICSRoleAssignment, incidentNumber int64) ICSRoleRecord {
	rec := ICSRoleRecord{
		ID:               a.ID,
		IncidentNumber:   incidentNumber,
		RoleType:         a.RoleType,
		AssigneeType:     a.AssigneeType,
		Status:           a.Status,
		ScopeDescription: a.ScopeDescription,
		EndedReason:      a.EndedReason,
		StartedAt:        a.StartedAt,
		EndedAt:          a.EndedAt,
	}

	if u := a.Edges.User; u != nil {
		uid := u.ID
		rec.UserID = &uid
		rec.UserName = u.FullName
		rec.UserEmail = u.Email
	}

	if at := a.Edges.AgentToken; at != nil {
		atid := at.ID
		rec.AgentTokenID = &atid
		rec.AgentName = at.Name
		rec.AgentType = at.AgentType
		rec.AgentRevoked = at.Revoked
	}

	return rec
}
