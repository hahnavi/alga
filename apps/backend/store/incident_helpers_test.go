package store

import (
	"testing"
	"time"

	"alga/ent"

	"github.com/google/uuid"
)

func TestIncidentListFilter_DefaultZeroValues(t *testing.T) {
	t.Parallel()
	var f IncidentListFilter
	if f.Status != "" {
		t.Errorf("default Status = %q, want empty", f.Status)
	}
	if f.Severity != "" {
		t.Errorf("default Severity = %q, want empty", f.Severity)
	}
	if f.ServiceID != "" {
		t.Errorf("default ServiceID = %q, want empty", f.ServiceID)
	}
	if f.CommanderID != "" {
		t.Errorf("default CommanderID = %q, want empty", f.CommanderID)
	}
	if f.Search != "" {
		t.Errorf("default Search = %q, want empty", f.Search)
	}
	if f.StartDate != nil {
		t.Error("default StartDate should be nil")
	}
	if f.EndDate != nil {
		t.Error("default EndDate should be nil")
	}
	if f.Limit != 0 {
		t.Errorf("default Limit = %d, want 0", f.Limit)
	}
	if f.Skip != 0 {
		t.Errorf("default Skip = %d, want 0", f.Skip)
	}
	if f.Sort != "" {
		t.Errorf("default Sort = %q, want empty", f.Sort)
	}
}

func TestIncidentListFilter_ConstructedWithAllFields(t *testing.T) {
	t.Parallel()
	now := time.Now()
	f := IncidentListFilter{
		Status:      "active",
		Severity:    "critical",
		ServiceID:   uuid.New().String(),
		CommanderID: uuid.New().String(),
		Search:      "database",
		StartDate:   &now,
		EndDate:     &now,
		Limit:       50,
		Skip:        10,
		Sort:        "severity",
	}
	if f.Status != "active" {
		t.Errorf("Status = %q", f.Status)
	}
	if f.Severity != "critical" {
		t.Errorf("Severity = %q", f.Severity)
	}
	if f.Limit != 50 {
		t.Errorf("Limit = %d", f.Limit)
	}
	if f.Skip != 10 {
		t.Errorf("Skip = %d", f.Skip)
	}
	if f.Sort != "severity" {
		t.Errorf("Sort = %q", f.Sort)
	}
}

func TestIncidentRecord_DefaultsForCreation(t *testing.T) {
	t.Parallel()
	var r IncidentRecord
	if r.Status != "" {
		t.Errorf("default Status = %q, want empty (store sets detected)", r.Status)
	}
	if r.IncidentNumber != 0 {
		t.Errorf("default IncidentNumber = %d, want 0", r.IncidentNumber)
	}
	if r.AutoConfirmed != false {
		t.Error("default AutoConfirmed should be false")
	}
	if r.Tags != nil {
		t.Errorf("default Tags = %v, want nil", r.Tags)
	}
	if r.CustomFields != nil {
		t.Errorf("default CustomFields = %v, want nil", r.CustomFields)
	}
	if r.CreatedAt.IsZero() == false {
		t.Error("default CreatedAt should be zero (store sets it)")
	}
}

func TestBuildIncidentPredicates_EmptyFilter(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	preds := s.buildIncidentPredicates(IncidentListFilter{})
	if len(preds) != 1 {
		t.Fatalf("empty filter produced %d predicates, want 1 (DeletedAtIsNil baseline)", len(preds))
	}
}

func TestBuildIncidentPredicates_StatusFilter(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	preds := s.buildIncidentPredicates(IncidentListFilter{Status: "active"})
	if len(preds) != 2 {
		t.Fatalf("expected 2 predicates (DeletedAtIsNil + status), got %d", len(preds))
	}
}

func TestBuildIncidentPredicates_SeverityFilter(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	preds := s.buildIncidentPredicates(IncidentListFilter{Severity: "critical"})
	if len(preds) != 2 {
		t.Fatalf("expected 2 predicates (DeletedAtIsNil + severity), got %d", len(preds))
	}
}

func TestBuildIncidentPredicates_SearchFilter(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	preds := s.buildIncidentPredicates(IncidentListFilter{Search: "database"})
	if len(preds) != 2 {
		t.Fatalf("expected 2 predicates (DeletedAtIsNil + search), got %d", len(preds))
	}
}

func TestBuildIncidentPredicates_DateRangeFilter(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	preds := s.buildIncidentPredicates(IncidentListFilter{
		StartDate: &start,
		EndDate:   &end,
	})
	if len(preds) != 3 {
		t.Fatalf("expected 3 predicates (DeletedAtIsNil + start + end), got %d", len(preds))
	}
}

func TestBuildIncidentPredicates_ValidUUIDs(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	svcID := uuid.New()
	cmdID := uuid.New()
	preds := s.buildIncidentPredicates(IncidentListFilter{
		ServiceID:   svcID.String(),
		CommanderID: cmdID.String(),
	})
	if len(preds) != 3 {
		t.Fatalf("expected 3 predicates (DeletedAtIsNil + service + commander), got %d", len(preds))
	}
}

func TestBuildIncidentPredicates_InvalidUUIDsIgnored(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	preds := s.buildIncidentPredicates(IncidentListFilter{
		ServiceID:   "not-a-uuid",
		CommanderID: "also-not-a-uuid",
	})
	if len(preds) != 1 {
		t.Errorf("invalid UUIDs should be silently ignored, expected 1 predicate (DeletedAtIsNil), got %d", len(preds))
	}
}

func TestBuildIncidentPredicates_CombinedFilters(t *testing.T) {
	t.Parallel()
	s := &pgIncidentStore{}
	start := time.Now().Add(-24 * time.Hour)
	preds := s.buildIncidentPredicates(IncidentListFilter{
		Status:    "active",
		Severity:  "critical",
		Search:    "outage",
		StartDate: &start,
	})
	if len(preds) != 5 {
		t.Fatalf("expected 5 predicates (DeletedAtIsNil + status + severity + search + start), got %d", len(preds))
	}
}

func newTestIncidentUpdate(t *testing.T) *ent.IncidentUpdate {
	t.Helper()
	client := ent.NewClient()
	t.Cleanup(func() {})
	return client.Incident.Update()
}

func TestApplyStatusTimestamps_Triaging(t *testing.T) {
	b := newTestIncidentUpdate(t)
	now := time.Now().UTC()

	applyStatusTimestamps(b, "triaging", now)

	v, exists := b.Mutation().TriagedAt()
	if !exists {
		t.Fatal("triaging status should set triaged_at")
	}
	if !v.Equal(now) {
		t.Errorf("triaged_at = %v, want %v", v, now)
	}
}

func TestApplyStatusTimestamps_Active(t *testing.T) {
	b := newTestIncidentUpdate(t)
	now := time.Now().UTC()

	applyStatusTimestamps(b, "active", now)

	v, exists := b.Mutation().SLAAcknowledgedAt()
	if !exists {
		t.Fatal("active status should set sla_acknowledged_at")
	}
	if !v.Equal(now) {
		t.Errorf("sla_acknowledged_at = %v, want %v", v, now)
	}
}

func TestApplyStatusTimestamps_Mitigated(t *testing.T) {
	b := newTestIncidentUpdate(t)
	now := time.Now().UTC()

	applyStatusTimestamps(b, "mitigated", now)

	v, exists := b.Mutation().MitigatedAt()
	if !exists {
		t.Fatal("mitigated status should set mitigated_at")
	}
	if !v.Equal(now) {
		t.Errorf("mitigated_at = %v, want %v", v, now)
	}
}

func TestApplyStatusTimestamps_Resolved(t *testing.T) {
	b := newTestIncidentUpdate(t)
	now := time.Now().UTC()

	applyStatusTimestamps(b, "resolved", now)

	v, exists := b.Mutation().ResolvedAt()
	if !exists {
		t.Fatal("resolved status should set resolved_at")
	}
	if !v.Equal(now) {
		t.Errorf("resolved_at = %v, want %v", v, now)
	}
}

func TestApplyStatusTimestamps_Closed(t *testing.T) {
	b := newTestIncidentUpdate(t)
	now := time.Now().UTC()

	applyStatusTimestamps(b, "closed", now)

	v, exists := b.Mutation().ClosedAt()
	if !exists {
		t.Fatal("closed status should set closed_at")
	}
	if !v.Equal(now) {
		t.Errorf("closed_at = %v, want %v", v, now)
	}
}

func TestApplyStatusTimestamps_Detected_NoTimestamp(t *testing.T) {
	b := newTestIncidentUpdate(t)
	now := time.Now().UTC()

	applyStatusTimestamps(b, "detected", now)

	if _, exists := b.Mutation().TriagedAt(); exists {
		t.Error("detected status should NOT set triaged_at")
	}
	if _, exists := b.Mutation().SLAAcknowledgedAt(); exists {
		t.Error("detected status should NOT set sla_acknowledged_at")
	}
	if _, exists := b.Mutation().MitigatedAt(); exists {
		t.Error("detected status should NOT set mitigated_at")
	}
	if _, exists := b.Mutation().ResolvedAt(); exists {
		t.Error("detected status should NOT set resolved_at")
	}
	if _, exists := b.Mutation().ClosedAt(); exists {
		t.Error("detected status should NOT set closed_at")
	}
}

func TestApplyStatusTimestamps_UnknownStatus_NoTimestamp(t *testing.T) {
	b := newTestIncidentUpdate(t)
	now := time.Now().UTC()

	applyStatusTimestamps(b, "nonexistent", now)

	if _, exists := b.Mutation().TriagedAt(); exists {
		t.Error("unknown status should NOT set triaged_at")
	}
	if _, exists := b.Mutation().SLAAcknowledgedAt(); exists {
		t.Error("unknown status should NOT set sla_acknowledged_at")
	}
	if _, exists := b.Mutation().MitigatedAt(); exists {
		t.Error("unknown status should NOT set mitigated_at")
	}
	if _, exists := b.Mutation().ResolvedAt(); exists {
		t.Error("unknown status should NOT set resolved_at")
	}
	if _, exists := b.Mutation().ClosedAt(); exists {
		t.Error("unknown status should NOT set closed_at")
	}
}

func TestApplyStatusTimestamps_EachStatusSetsOnlyItsField(t *testing.T) {
	client := ent.NewClient()

	cases := []struct {
		status      string
		expectField string
	}{
		{"triaging", "triaged_at"},
		{"active", "sla_acknowledged_at"},
		{"mitigated", "mitigated_at"},
		{"resolved", "resolved_at"},
		{"closed", "closed_at"},
	}

	allTimestampChecks := map[string]func(b *ent.IncidentUpdate) bool{
		"triaged_at":          func(b *ent.IncidentUpdate) bool { _, ok := b.Mutation().TriagedAt(); return ok },
		"sla_acknowledged_at": func(b *ent.IncidentUpdate) bool { _, ok := b.Mutation().SLAAcknowledgedAt(); return ok },
		"mitigated_at":        func(b *ent.IncidentUpdate) bool { _, ok := b.Mutation().MitigatedAt(); return ok },
		"resolved_at":         func(b *ent.IncidentUpdate) bool { _, ok := b.Mutation().ResolvedAt(); return ok },
		"closed_at":           func(b *ent.IncidentUpdate) bool { _, ok := b.Mutation().ClosedAt(); return ok },
	}

	now := time.Now().UTC()
	for _, tc := range cases {
		b := client.Incident.Update()
		applyStatusTimestamps(b, tc.status, now)

		for field, check := range allTimestampChecks {
			result := check(b)
			if field == tc.expectField {
				if !result {
					t.Errorf("status %q should set %s", tc.status, field)
				}
			} else {
				if result {
					t.Errorf("status %q should NOT set %s", tc.status, field)
				}
			}
		}
	}
}

func TestErrIncidentStatusConflict(t *testing.T) {
	if ErrIncidentStatusConflict == nil {
		t.Fatal("ErrIncidentStatusConflict should not be nil")
	}
	if ErrIncidentStatusConflict.Error() != "incident status changed concurrently" {
		t.Errorf("ErrIncidentStatusConflict.Error() = %q", ErrIncidentStatusConflict.Error())
	}
}

func TestPostMortemRecord_NewFields(t *testing.T) {
	t.Parallel()
	r := PostMortemRecord{
		WhatWentWell:       "Good communication",
		WhatWentWrong:      "Slow rollback",
		BlamelessConfirmed: true,
		BlamelessNotes:     "System issue, not human error",
	}
	if r.WhatWentWell != "Good communication" {
		t.Errorf("WhatWentWell = %q", r.WhatWentWell)
	}
	if r.WhatWentWrong != "Slow rollback" {
		t.Errorf("WhatWentWrong = %q", r.WhatWentWrong)
	}
	if !r.BlamelessConfirmed {
		t.Error("BlamelessConfirmed should be true")
	}
	if r.BlamelessNotes != "System issue, not human error" {
		t.Errorf("BlamelessNotes = %q", r.BlamelessNotes)
	}
}

func TestActionItemRecord_TypeAndAssigneeName(t *testing.T) {
	t.Parallel()
	r := ActionItemRecord{
		Type:         "prevent",
		AssigneeName: "Alice",
	}
	if r.Type != "prevent" {
		t.Errorf("Type = %q", r.Type)
	}
	if r.AssigneeName != "Alice" {
		t.Errorf("AssigneeName = %q", r.AssigneeName)
	}
}
