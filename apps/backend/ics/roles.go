package ics

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrRoleNotAssignable = errors.New("role type is not assignable")

// ErrActiveICExists is returned when an incident already has an active
// Incident Commander and cannot accept another IC assignment.
var ErrActiveICExists = errors.New("incident already has an active IC")

type RoleManager struct {
	roleStore RoleStore
}

func NewRoleManager(roleStore RoleStore) *RoleManager {
	return &RoleManager{roleStore: roleStore}
}

func (m *RoleManager) AssignRole(ctx context.Context, incidentNumber int64, roleType RoleType, userID uuid.UUID, delegatedBy *uuid.UUID, scope *string) (*RoleRecord, error) {
	if !ValidRoleType(roleType) {
		return nil, fmt.Errorf("invalid role type: %s", roleType)
	}
	if !AssignableRoleType(roleType) {
		return nil, fmt.Errorf("%w: %s", ErrRoleNotAssignable, roleType)
	}
	if roleType == RoleIncidentCommander {
		activeIC, err := m.roleStore.GetActiveIC(ctx, incidentNumber)
		if err != nil {
			return nil, fmt.Errorf("check active IC: %w", err)
		}
		if activeIC != nil {
			return nil, fmt.Errorf("%w (assignment %s); hand off or end the current IC first", ErrActiveICExists, activeIC.ID)
		}
	}
	return m.roleStore.AssignRole(ctx, incidentNumber, roleType, userID, delegatedBy, scope)
}

func (m *RoleManager) AssignAgentRole(ctx context.Context, incidentNumber int64, roleType RoleType, agentTokenID uuid.UUID, delegatedBy *uuid.UUID, scope *string) (*RoleRecord, error) {
	if !ValidRoleType(roleType) {
		return nil, fmt.Errorf("invalid role type: %s", roleType)
	}
	if !AssignableRoleType(roleType) {
		return nil, fmt.Errorf("%w: %s", ErrRoleNotAssignable, roleType)
	}
	if roleType == RoleIncidentCommander {
		activeIC, err := m.roleStore.GetActiveIC(ctx, incidentNumber)
		if err != nil {
			return nil, fmt.Errorf("check active IC: %w", err)
		}
		if activeIC != nil {
			return nil, fmt.Errorf("%w (assignment %s); hand off or end the current IC first", ErrActiveICExists, activeIC.ID)
		}
	}
	return m.roleStore.AssignAgentRole(ctx, incidentNumber, roleType, agentTokenID, delegatedBy, scope)
}

func (m *RoleManager) EndRole(ctx context.Context, assignmentID uuid.UUID, reason EndReason) error {
	return m.roleStore.EndRole(ctx, assignmentID, reason)
}

func (m *RoleManager) GetActiveRoles(ctx context.Context, incidentNumber int64) ([]RoleRecord, error) {
	return m.roleStore.GetActiveRoles(ctx, incidentNumber)
}

func (m *RoleManager) GetActiveIC(ctx context.Context, incidentNumber int64) (*RoleRecord, error) {
	return m.roleStore.GetActiveIC(ctx, incidentNumber)
}

func (m *RoleManager) EndAllRoles(ctx context.Context, incidentNumber int64, reason EndReason) error {
	return m.roleStore.EndAllRolesForIncident(ctx, incidentNumber, reason)
}
