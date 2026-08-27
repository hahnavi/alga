//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/db"
	"alga/db/models"
	"alga/ics"
)

// TestUpdateRoleScopeInPlace pins the WP-A4 contract: PATCH-scope edits the
// ACTIVE assignment in place — exactly one active row remains for the role
// type (no insert collision with the partial unique index), the scope sticks,
// and ended or unknown assignments resolve to ErrICSRoleNotFound.
func TestUpdateRoleScopeInPlace(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	ctx := context.Background()
	user, err := stores.User.CreateUser(
		fmt.Sprintf("ics-scope-%s@example.com", uuid.NewString()[:8]),
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	inc, err := stores.Incident.CreateIncident(ctx, &IncidentRecord{
		Title:   "WP-A4 scope fixture",
		Summary: "in-place ICS role scope update",
	})
	if err != nil {
		t.Fatalf("create fixture incident: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.ICSRoleAssignment)(nil)).Where("incident_id = ?", inc.ID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.Incident)(nil)).Where("id = ?", inc.ID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.User)(nil)).Where("id = ?", user.ID).Exec(context.Background())
	})

	assigned, err := stores.ICSRole.AssignRole(ctx, inc.IncidentNumber, ics.RoleIncidentCommander, user.ID, nil, nil)
	if err != nil {
		t.Fatalf("assign commander role: %v", err)
	}

	newScope := "contain the blast radius in us-east-1"
	if err := stores.ICSRole.UpdateRoleScope(ctx, assigned.ID, &newScope); err != nil {
		t.Fatalf("UpdateRoleScope: %v", err)
	}

	active, err := stores.ICSRole.GetActiveRoles(ctx, inc.IncidentNumber)
	if err != nil {
		t.Fatalf("get active roles: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("active commander rows = %d, want 1 (in-place update, no re-insert)", len(active))
	}
	if active[0].ScopeDescription == nil || *active[0].ScopeDescription != newScope {
		t.Errorf("scope = %v, want %q", active[0].ScopeDescription, newScope)
	}

	// Ending the role removes it from the active set; further scope updates
	// must fail with the not-found sentinel (the handler maps this to 404).
	if err := stores.ICSRole.EndRole(ctx, assigned.ID, ics.EndReasonReplaced); err != nil {
		t.Fatalf("EndRole: %v", err)
	}
	if err := stores.ICSRole.UpdateRoleScope(ctx, assigned.ID, &newScope); !errors.Is(err, ErrICSRoleNotFound) {
		t.Errorf("UpdateRoleScope on ended role = %v, want ErrICSRoleNotFound", err)
	}
	if err := stores.ICSRole.UpdateRoleScope(ctx, uuid.New(), &newScope); !errors.Is(err, ErrICSRoleNotFound) {
		t.Errorf("UpdateRoleScope on unknown id = %v, want ErrICSRoleNotFound", err)
	}
}
