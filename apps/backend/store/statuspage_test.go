package store

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestStatusPageCreateAndGetBySlug(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGStatusPageStore(client)

	page, err := s.CreatePage(context.Background(), &StatusPageRecord{
		Name:    "Platform Status",
		Slug:    "platform",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if page.ID == uuid.Nil {
		t.Fatal("expected non-nil ID")
	}
	if page.Visibility != StatusPageVisibilityInternal {
		t.Fatalf("visibility = %q, want internal", page.Visibility)
	}

	got, err := s.GetPageBySlug(context.Background(), "platform")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if got.ID != page.ID {
		t.Fatalf("slug lookup returned wrong page")
	}
}

func TestStatusPageComponentsOrdering(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGStatusPageStore(client)

	page, err := s.CreatePage(context.Background(), &StatusPageRecord{
		Name: "Ops", Slug: "ops", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	c1, err := s.CreateComponent(context.Background(), &StatusPageComponentRecord{
		StatusPageID: page.ID, Name: "API", DisplayOrder: 2,
	})
	if err != nil {
		t.Fatalf("CreateComponent c1: %v", err)
	}
	c2, err := s.CreateComponent(context.Background(), &StatusPageComponentRecord{
		StatusPageID: page.ID, Name: "DB", DisplayOrder: 1,
	})
	if err != nil {
		t.Fatalf("CreateComponent c2: %v", err)
	}

	items, err := s.ListComponents(context.Background(), page.ID)
	if err != nil {
		t.Fatalf("ListComponents: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 components, got %d", len(items))
	}
	// Ordered by display_order ascending: DB (1) before API (2).
	if items[0].ID != c2.ID || items[1].ID != c1.ID {
		t.Fatalf("expected DB then API, got %s then %s", items[0].Name, items[1].Name)
	}

	// Update component status.
	updated, err := s.UpdateComponent(context.Background(), c1.ID, &StatusPageComponentRecord{Status: StatusComponentMajorOutage})
	if err != nil {
		t.Fatalf("UpdateComponent: %v", err)
	}
	if updated.Status != StatusComponentMajorOutage {
		t.Fatalf("status = %q", updated.Status)
	}
}

func TestStatusPageDeleteCascadesComponents(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGStatusPageStore(client)

	page, err := s.CreatePage(context.Background(), &StatusPageRecord{
		Name: "Gone", Slug: "gone", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if _, err := s.CreateComponent(context.Background(), &StatusPageComponentRecord{
		StatusPageID: page.ID, Name: "Comp",
	}); err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}

	if err := s.DeletePage(context.Background(), page.ID); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	items, err := s.ListComponents(context.Background(), page.ID)
	if err != nil {
		t.Fatalf("ListComponents after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected components cascade-deleted, got %d", len(items))
	}

	got, err := s.GetPage(context.Background(), page.ID)
	if err != nil {
		t.Fatalf("GetPage after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected page to be deleted")
	}
}

func TestStatusPageDuplicateSlugRejected(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGStatusPageStore(client)

	if _, err := s.CreatePage(context.Background(), &StatusPageRecord{Name: "A", Slug: "dup", Enabled: true}); err != nil {
		t.Fatalf("CreatePage first: %v", err)
	}
	_, err := s.CreatePage(context.Background(), &StatusPageRecord{Name: "B", Slug: "dup", Enabled: true})
	if err == nil {
		t.Fatal("expected duplicate slug error")
	}
	if !IsDuplicateKey(err) && !strings.Contains(err.Error(), "unique") && !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-key error, got %v", err)
	}
}
