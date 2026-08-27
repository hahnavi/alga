//go:build integration

package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/db"
	"alga/db/models"
	"alga/ics"
)

// TestICSCloseTeardownAndReopenReset pins the store contract: the
// close-time teardown persists roles with the migration-00020 'incident_closed'
// end reason (CHECK-accepted), and the reopen reset nulls sla_resolved_at so
// resolve-breach detection can fire again.
func TestICSCloseTeardownAndReopenReset(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	ctx := context.Background()
	user, err := stores.User.CreateUser(
		fmt.Sprintf("ics-teardown-%s@example.com", uuid.NewString()[:8]),
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	userID := user.ID

	inc, err := stores.Incident.CreateIncident(ctx, &IncidentRecord{
		Title:   "teardown fixture",
		Summary: "close-time ICS role teardown",
	})
	if err != nil {
		t.Fatalf("create fixture incident: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.ICSRoleAssignment)(nil)).Where("incident_id = ?", inc.ID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.Incident)(nil)).Where("id = ?", inc.ID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.User)(nil)).Where("id = ?", userID).Exec(context.Background())
	})

	if _, err := stores.ICSRole.AssignRole(ctx, inc.IncidentNumber, ics.RoleIncidentCommander, userID, nil, nil); err != nil {
		t.Fatalf("assign commander role: %v", err)
	}

	// Resolving stamps sla_resolved_at via applyStatusTimestampsBun.
	if err := stores.Incident.TransitionIncidentStatus(ctx, inc.IncidentNumber, []string{"detected", "active"}, "resolved"); err != nil {
		t.Fatalf("resolve incident: %v", err)
	}
	resolved, err := stores.Incident.GetIncident(ctx, inc.IncidentNumber)
	if err != nil {
		t.Fatalf("get resolved incident: %v", err)
	}
	if resolved.SLAResolvedAt == nil {
		t.Fatal("sla_resolved_at must be stamped after resolve")
	}

	// Close-time teardown with the new end reason must pass the widened CHECK.
	if err := stores.ICSRole.EndAllRolesForIncident(ctx, inc.IncidentNumber, ics.EndReasonIncidentClosed); err != nil {
		t.Fatalf("EndAllRolesForIncident with incident_closed must succeed on a migrated DB: %v", err)
	}
	active, err := stores.ICSRole.GetActiveRoles(ctx, inc.IncidentNumber)
	if err != nil {
		t.Fatalf("get active roles: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active roles after teardown = %d, want 0", len(active))
	}
	var ended models.ICSRoleAssignment
	if err := bunDB.NewSelect().Model(&ended).
		Where("incident_id = ?", inc.ID).
		Where("role_type = ?", "incident_commander").
		Scan(ctx); err != nil {
		t.Fatalf("load ended role: %v", err)
	}
	if ended.Status != "ended" || ended.EndedReason == nil || *ended.EndedReason != string(ics.EndReasonIncidentClosed) {
		t.Fatalf("ended role = status %q reason %v, want ended/incident_closed", ended.Status, ended.EndedReason)
	}

	// Reopen reset clears the SLA resolve stamp.
	if err := stores.Incident.ClearSLAResolvedAt(ctx, inc.IncidentNumber); err != nil {
		t.Fatalf("ClearSLAResolvedAt: %v", err)
	}
	reopened, err := stores.Incident.GetIncident(ctx, inc.IncidentNumber)
	if err != nil {
		t.Fatalf("get reopened incident: %v", err)
	}
	if reopened.SLAResolvedAt != nil {
		t.Fatalf("sla_resolved_at = %v, want nil after reopen reset", reopened.SLAResolvedAt)
	}
}
