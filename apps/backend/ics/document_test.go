package ics

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestDocumentManager_InitializeForIncident(t *testing.T) {
	stub := newStubDocumentStore()
	dm := NewDocumentManager(stub)

	err := dm.InitializeForIncident(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("InitializeForIncident: %v", err)
	}

	recs, err := dm.GetAllSections(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetAllSections: %v", err)
	}
	if len(recs) != 6 {
		t.Errorf("got %d sections, want 6", len(recs))
	}
}

func TestDocumentManager_InitializeForIncident_WithTriageReport(t *testing.T) {
	stub := newStubDocumentStore()
	dm := NewDocumentManager(stub)

	triage := map[string]any{"severity": "critical"}
	err := dm.InitializeForIncident(context.Background(), 2, triage)
	if err != nil {
		t.Fatalf("InitializeForIncident: %v", err)
	}

	recs, err := dm.GetAllSections(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetAllSections: %v", err)
	}
	if len(recs) != 6 {
		t.Errorf("got %d sections, want 6", len(recs))
	}
}

func TestDocumentManager_UpdateSection(t *testing.T) {
	stub := newStubDocumentStore()
	dm := NewDocumentManager(stub)
	userID := uuid.New()

	err := dm.InitializeForIncident(context.Background(), 3, nil)
	if err != nil {
		t.Fatalf("InitializeForIncident: %v", err)
	}

	rec, err := dm.UpdateSection(context.Background(), 3, SectionCurrentStatus, "System is stable", 1, userID)
	if err != nil {
		t.Fatalf("UpdateSection: %v", err)
	}
	if rec.Version != 2 {
		t.Errorf("version = %d, want 2", rec.Version)
	}
	if rec.Content != "System is stable" {
		t.Errorf("content = %q, want %q", rec.Content, "System is stable")
	}
}

func TestDocumentManager_UpdateSection_VersionConflict(t *testing.T) {
	stub := newStubDocumentStore()
	dm := NewDocumentManager(stub)
	userID := uuid.New()

	err := dm.InitializeForIncident(context.Background(), 4, nil)
	if err != nil {
		t.Fatalf("InitializeForIncident: %v", err)
	}

	_, err = dm.UpdateSection(context.Background(), 4, SectionImpactAssessment, "something", 99, userID)
	if err == nil {
		t.Fatal("expected version conflict error")
	}
}

func TestDocumentManager_UpdateSection_InvalidSection(t *testing.T) {
	stub := newStubDocumentStore()
	dm := NewDocumentManager(stub)

	_, err := dm.UpdateSection(context.Background(), 5, DocumentSection("nonexistent"), "content", 1, uuid.New())
	if err == nil {
		t.Fatal("expected error for invalid section")
	}
}

func TestDocumentManager_GetAllSections_Empty(t *testing.T) {
	stub := newStubDocumentStore()
	dm := NewDocumentManager(stub)

	recs, err := dm.GetAllSections(context.Background(), 6)
	if err != nil {
		t.Fatalf("GetAllSections: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("got %d sections, want 0", len(recs))
	}
}
