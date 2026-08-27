package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/capability"
	"alga/db/models"
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
	// UpdateRoleScope edits the scope of the ACTIVE assignment in place
	// (WP-A4); re-inserting would collide with the partial unique index on
	// (incident_id, role_type) WHERE status='active'. Zero rows updated ⇒
	// ErrICSRoleNotFound.
	UpdateRoleScope(ctx context.Context, assignmentID uuid.UUID, scope *string) error
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

func newPGICSRoleStore(db *bun.DB) ICSRoleStore {
	return &pgICSRoleStore{pgStoreBase{db: db}}
}

func (s *pgICSRoleStore) findIncidentByNumber(ctx context.Context, incidentNumber int64) (*models.Incident, error) {
	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", incidentNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident: %w", err)
	}
	return &inc, nil
}

func (s *pgICSRoleStore) AssignRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, userID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	m := &models.ICSRoleAssignment{
		ID:               models.NewUUID(),
		RoleType:         string(roleType),
		Status:           "active",
		AssigneeType:     "user",
		UserID:           &userID,
		IncidentID:       inc.ID,
		ParentID:         parentAssignmentID,
		ScopeDescription: scope,
		StartedAt:        time.Now().UTC(),
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to assign ICS role: %w", err)
	}

	uidCopy := userID
	return &ICSRoleRecord{
		ID:                 m.ID,
		IncidentNumber:     incidentNumber,
		RoleType:           m.RoleType,
		AssigneeType:       "user",
		UserID:             &uidCopy,
		ParentAssignmentID: parentAssignmentID,
		ScopeDescription:   m.ScopeDescription,
		Status:             m.Status,
		StartedAt:          m.StartedAt,
	}, nil
}

func (s *pgICSRoleStore) UpdateRoleScope(ctx context.Context, assignmentID uuid.UUID, scope *string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.ICSRoleAssignment)(nil)).
		Set("scope_description = ?", scope).
		Where("id = ?", assignmentID).
		Where("status = ?", "active").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update ICS role scope: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update ICS role scope: %w", err)
	}
	if n == 0 {
		return ErrICSRoleNotFound
	}
	return nil
}

func (s *pgICSRoleStore) EndRole(ctx context.Context, assignmentID uuid.UUID, reason ics.EndReason) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	reasonStr := string(reason)
	res, err := s.db.NewUpdate().Model((*models.ICSRoleAssignment)(nil)).
		Set("status = ?", "ended").
		Set("ended_reason = ?", reasonStr).
		Set("ended_at = ?", now).
		Where("id = ?", assignmentID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to end ICS role: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to end ICS role: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("ICS role assignment not found: %w", ErrICSRoleNotFound)
	}
	return nil
}

func (s *pgICSRoleStore) GetActiveRoles(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	var assignments []models.ICSRoleAssignment
	err = s.db.NewSelect().Model(&assignments).
		Where("incident_id = ?", inc.ID).
		Where("status = ?", "active").
		OrderExpr("started_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active ICS roles: %w", err)
	}

	records := make([]ICSRoleRecord, 0, len(assignments))
	for _, a := range assignments {
		rec := s.toICSRoleRecord(ctx, &a, incidentNumber)
		records = append(records, rec)
	}
	return records, nil
}

func (s *pgICSRoleStore) GetActiveIC(ctx context.Context, incidentNumber int64) (*ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	var a models.ICSRoleAssignment
	err = s.db.NewSelect().Model(&a).
		Where("incident_id = ?", inc.ID).
		Where("status = ?", "active").
		Where("role_type = ?", string(ics.RoleIncidentCommander)).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*ICSRoleRecord](err, "active incident commander")
	}

	rec := s.toICSRoleRecord(ctx, &a, incidentNumber)
	return &rec, nil
}

func (s *pgICSRoleStore) GetAllRoles(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	var assignments []models.ICSRoleAssignment
	err = s.db.NewSelect().Model(&assignments).
		Where("incident_id = ?", inc.ID).
		OrderExpr("started_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query all ICS roles: %w", err)
	}

	records := make([]ICSRoleRecord, 0, len(assignments))
	for _, a := range assignments {
		rec := s.toICSRoleRecord(ctx, &a, incidentNumber)
		records = append(records, rec)
	}
	return records, nil
}

func (s *pgICSRoleStore) GetDelegationTree(ctx context.Context, incidentNumber int64) ([]ICSRoleRecord, error) {
	return s.GetActiveRoles(ctx, incidentNumber)
}

func (s *pgICSRoleStore) EndAllRolesForIncident(ctx context.Context, incidentNumber int64, reason ics.EndReason) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	reasonStr := string(reason)
	_, err = s.db.NewUpdate().Model((*models.ICSRoleAssignment)(nil)).
		Set("status = ?", "ended").
		Set("ended_reason = ?", reasonStr).
		Set("ended_at = ?", now).
		Where("incident_id = ?", inc.ID).
		Where("status = ?", "active").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to end all ICS roles for incident: %w", err)
	}
	return nil
}

func (s *pgICSRoleStore) AssignAgentRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, agentTokenID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	var agentToken models.AgentToken
	err = s.db.NewSelect().Model(&agentToken).
		Where("id = ?", agentTokenID).
		Where("revoked = ?", false).
		Where("enabled = ?", true).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
		}
		return nil, fmt.Errorf("failed to find agent: %w", err)
	}
	requiredCap := ics.RoleRequiredCapability(roleType)
	if requiredCap != "" && !capability.Has(agentToken.Capabilities, requiredCap) {
		return nil, fmt.Errorf("%w: role %s requires %s", ErrAgentCapabilityMismatch, roleType, requiredCap)
	}

	m := &models.ICSRoleAssignment{
		ID:               models.NewUUID(),
		RoleType:         string(roleType),
		Status:           "active",
		AssigneeType:     "agent",
		AgentTokenID:     &agentTokenID,
		IncidentID:       inc.ID,
		ParentID:         parentAssignmentID,
		ScopeDescription: scope,
		StartedAt:        time.Now().UTC(),
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to assign agent ICS role: %w", err)
	}

	rec := s.toICSRoleRecord(ctx, m, incidentNumber)
	return &rec, nil
}

func (s *pgICSRoleStore) GetActiveRolesForAgent(ctx context.Context, agentTokenID uuid.UUID) ([]ICSRoleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var results []models.ICSRoleAssignment
	err := s.db.NewSelect().Model(&results).
		Where("agent_token_id = ?", agentTokenID).
		Where("status = ?", "active").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent ICS roles: %w", err)
	}

	records := make([]ICSRoleRecord, 0, len(results))
	for _, r := range results {
		incNumber := int64(0)
		var inc models.Incident
		if err := s.db.NewSelect().Model(&inc).Where("id = ?", r.IncidentID).Scan(ctx); err == nil {
			incNumber = inc.IncidentNumber
		}
		rec := s.toICSRoleRecord(ctx, &r, incNumber)
		records = append(records, rec)
	}
	return records, nil
}

func (s *pgICSRoleStore) EndRolesForAgent(ctx context.Context, agentTokenID uuid.UUID, reason ics.EndReason) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	reasonStr := string(reason)
	_, err := s.db.NewUpdate().Model((*models.ICSRoleAssignment)(nil)).
		Set("status = ?", "ended").
		Set("ended_reason = ?", reasonStr).
		Set("ended_at = ?", now).
		Where("agent_token_id = ?", agentTokenID).
		Where("status = ?", "active").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to end agent ICS roles: %w", err)
	}
	return nil
}

func (s *pgICSRoleStore) toICSRoleRecord(ctx context.Context, a *models.ICSRoleAssignment, incidentNumber int64) ICSRoleRecord {
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

	if a.UserID != nil {
		var u models.User
		if err := s.db.NewSelect().Model(&u).Where("id = ?", *a.UserID).Scan(ctx); err == nil {
			uid := u.ID
			rec.UserID = &uid
			rec.UserName = u.FullName
			rec.UserEmail = u.Email
		}
	}

	if a.AgentTokenID != nil {
		var at models.AgentToken
		if err := s.db.NewSelect().Model(&at).Where("id = ?", *a.AgentTokenID).Scan(ctx); err == nil {
			atid := at.ID
			rec.AgentTokenID = &atid
			rec.AgentName = at.Name
			rec.AgentType = at.AgentType
			rec.AgentRevoked = at.Revoked
		}
	}

	return rec
}
