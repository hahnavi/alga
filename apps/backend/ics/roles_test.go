package ics

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAssignRoleInvalidType(t *testing.T) {
	rm := NewRoleManager(newStubRoleStore())
	_, err := rm.AssignRole(context.Background(), 1, RoleType("invalid"), uuid.New(), nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid role type")
	}
	if got, want := err.Error(), "invalid role type: invalid"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestRoleRequiredCapability(t *testing.T) {
	t.Parallel()
	cases := []struct {
		role RoleType
		want string
	}{
		{RoleIncidentCommander, "command"},
		{RoleCommunicationsLead, "communicate"},
		{RoleResponder, "investigate"},
	}
	for _, tt := range cases {
		if got := RoleRequiredCapability(tt.role); got != tt.want {
			t.Fatalf("RoleRequiredCapability(%s) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

func TestAssignICSuccess(t *testing.T) {
	stub := newStubRoleStore()
	rm := NewRoleManager(stub)
	userID := uuid.New()

	rec, err := rm.AssignRole(context.Background(), 1, RoleIncidentCommander, userID, nil, nil)
	if err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if rec.RoleType != string(RoleIncidentCommander) {
		t.Errorf("RoleType = %q, want %q", rec.RoleType, RoleIncidentCommander)
	}
	if rec.UserID == nil || *rec.UserID != userID {
		t.Errorf("UserID = %v, want %s", rec.UserID, userID)
	}
	if !stub.hasActiveIC(1) {
		t.Error("expected active IC in stub")
	}
}

func TestAssignDuplicateICFails(t *testing.T) {
	stub := newStubRoleStore()
	rm := NewRoleManager(stub)

	_, err := rm.AssignRole(context.Background(), 1, RoleIncidentCommander, uuid.New(), nil, nil)
	if err != nil {
		t.Fatalf("first AssignRole: %v", err)
	}

	_, err = rm.AssignRole(context.Background(), 1, RoleIncidentCommander, uuid.New(), nil, nil)
	if err == nil {
		t.Fatal("expected error for duplicate IC")
	}
}

func TestAssignCommunicatorWithExistingICSucceeds(t *testing.T) {
	stub := newStubRoleStore()
	rm := NewRoleManager(stub)

	_, err := rm.AssignRole(context.Background(), 1, RoleIncidentCommander, uuid.New(), nil, nil)
	if err != nil {
		t.Fatalf("assign IC: %v", err)
	}

	rec, err := rm.AssignRole(context.Background(), 1, RoleCommunicationsLead, uuid.New(), nil, nil)
	if err != nil {
		t.Fatalf("assign communicator: %v", err)
	}
	if rec.RoleType != string(RoleCommunicationsLead) {
		t.Errorf("RoleType = %q, want %q", rec.RoleType, RoleCommunicationsLead)
	}
}

func TestAssignUnassignableRoleFails(t *testing.T) {
	stub := newStubRoleStore()
	rm := NewRoleManager(stub)

	_, err := rm.AssignRole(context.Background(), 1, RoleType("invalid_role"), uuid.New(), nil, nil)
	if err == nil {
		t.Fatal("expected unassignable role assignment to fail")
	}
}

func TestEndRole(t *testing.T) {
	stub := newStubRoleStore()
	rm := NewRoleManager(stub)

	rec, err := rm.AssignRole(context.Background(), 1, RoleIncidentCommander, uuid.New(), nil, nil)
	if err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	err = rm.EndRole(context.Background(), rec.ID, EndReasonReplaced)
	if err != nil {
		t.Fatalf("EndRole: %v", err)
	}

	activeIC, err := rm.GetActiveIC(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetActiveIC: %v", err)
	}
	if activeIC != nil {
		t.Error("expected no active IC after end")
	}
}

func TestGetActiveRoles(t *testing.T) {
	stub := newStubRoleStore()
	rm := NewRoleManager(stub)

	_, _ = rm.AssignRole(context.Background(), 1, RoleIncidentCommander, uuid.New(), nil, nil)
	_, _ = rm.AssignRole(context.Background(), 1, RoleResponder, uuid.New(), nil, nil)

	roles, err := rm.GetActiveRoles(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetActiveRoles: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("len(roles) = %d, want 2", len(roles))
	}
}

func TestEndAllRoles(t *testing.T) {
	stub := newStubRoleStore()
	rm := NewRoleManager(stub)

	_, _ = rm.AssignRole(context.Background(), 1, RoleIncidentCommander, uuid.New(), nil, nil)
	_, _ = rm.AssignRole(context.Background(), 1, RoleResponder, uuid.New(), nil, nil)

	err := rm.EndAllRoles(context.Background(), 1, EndReasonIncidentResolved)
	if err != nil {
		t.Fatalf("EndAllRoles: %v", err)
	}

	roles, err := rm.GetActiveRoles(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetActiveRoles: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("len(roles) = %d, want 0", len(roles))
	}
}
