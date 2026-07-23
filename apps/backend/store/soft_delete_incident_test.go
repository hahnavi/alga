//go:build integration

package store

import (
	"context"
	"testing"
)

// TestDeleteIncident_SoftDeletesPreservesChildren verifies that deleting an
// incident sets deleted_at but keeps the row and all of its children (timeline,
// ICS roles, coordination, documents, incident_investigations, incident_alerts
// junction) queryable.
func TestDeleteIncident_SoftDeletesPreservesChildren(t *testing.T) {
	ctx := context.Background()

	inc, err := incidentStore.CreateIncident(ctx, &IncidentRecord{
		Title:       "SoftDel Incident",
		Status:      "detected",
		Severity:    "warning",
		ImpactLevel: "low",
		Priority:    "P4",
	})
	if err != nil {
		t.Fatalf("Create incident: %v", err)
	}
	num := inc.IncidentNumber

	if err := incidentStore.AddTimelineEntry(ctx, &IncidentTimelineEntryRecord{
		IncidentNumber: num,
		EventType:      "manual",
		ActorType:      "user",
		Message:        "note",
	}); err != nil {
		t.Fatalf("AddTimelineEntry: %v", err)
	}

	if err := incidentStore.DeleteIncident(ctx, num); err != nil {
		t.Fatalf("DeleteIncident: %v", err)
	}

	// List excludes the soft-deleted incident.
	list, _, err := incidentStore.ListIncidents(ctx, IncidentListFilter{Search: "SoftDel Incident"})
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListIncidents returned %d incidents (expected 0) for soft-deleted title %q; the DeletedAtIsNil filter in buildIncidentPredicates regressed", len(list), "ExcludeMe SoftDel")
	}
	for _, i := range list {
		if i.IncidentNumber == num {
			t.Fatalf("ListIncidents returned soft-deleted incident %d", num)
		}
	}

	// Detail-by-number still resolves it, with deleted_at set (tombstone).
	got, err := incidentStore.GetIncident(ctx, num)
	if err != nil {
		t.Fatalf("GetIncident after delete: %v", err)
	}
	if got == nil || got.DeletedAt == nil {
		t.Fatal("expected soft-deleted incident with deleted_at set")
	}
	// Children are preserved (timeline still attached).
	if len(got.Timeline) == 0 {
		t.Error("expected timeline children to be preserved after soft-delete")
	}
}

// TestListIncidents_ExcludesSoftDeleted asserts the default list filter drops
// soft-deleted incidents. List/count paths use buildIncidentPredicates which
// always adds DeletedAtIsNil().
func TestListIncidents_ExcludesSoftDeleted(t *testing.T) {
	ctx := context.Background()
	inc, err := incidentStore.CreateIncident(ctx, &IncidentRecord{
		Title:       "ExcludeMe SoftDel",
		Status:      "detected",
		Severity:    "warning",
		ImpactLevel: "low",
		Priority:    "P5",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := incidentStore.DeleteIncident(ctx, inc.IncidentNumber); err != nil {
		t.Fatalf("DeleteIncident: %v", err)
	}

	list, _, err := incidentStore.ListIncidents(ctx, IncidentListFilter{Search: "ExcludeMe SoftDel"})
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	for _, i := range list {
		if i.IncidentNumber == inc.IncidentNumber {
			t.Fatal("ListIncidents returned soft-deleted incident")
		}
	}
	if len(list) != 0 {
		t.Fatalf("expected ListIncidents to exclude soft-deleted incident %d, got %d results", inc.IncidentNumber, len(list))
	}
}
