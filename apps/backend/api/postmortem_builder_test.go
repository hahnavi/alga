package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/ics"
	"alga/store"
)

func TestBuildPostMortemDraft_PullsDocumentSections(t *testing.T) {
	incidentNumber := int64(42)
	incidentID := uuid.New()
	started := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	resolved := time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC)

	incident := &store.IncidentRecord{
		ID:             incidentID,
		IncidentNumber: incidentNumber,
		Title:          "DB Pool Exhaustion",
		Summary:        "Connection pool exhausted causing API timeouts",
		Status:         "resolved",
		Severity:       "SEV2",
		ImpactLevel:    "high",
		StartedAt:      &started,
		ResolvedAt:     &resolved,
		Tags:           []string{"database", "payment"},
	}

	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{IncidentNumber: incidentNumber, Section: string(ics.SectionRootCause), Content: "Pool max connections too low for traffic spike"},
		{IncidentNumber: incidentNumber, Section: string(ics.SectionImpactAssessment), Content: "503 errors for 150 minutes affecting checkout"},
		{IncidentNumber: incidentNumber, Section: string(ics.SectionActionsTaken), Content: "1. Scaled connection pool\n2. Restarted API pods"},
		{IncidentNumber: incidentNumber, Section: string(ics.SectionResolution), Content: "Increased pool size; added autoscaling policy"},
	}}

	draft := buildPostMortemDraft(context.Background(), postMortemDraftDeps{
		documentStore: docStore,
	}, incident, "Incident resolved")

	if draft.IncidentID != incidentID {
		t.Fatalf("IncidentID = %s, want %s", draft.IncidentID, incidentID)
	}
	if draft.Status != "draft" {
		t.Fatalf("Status = %q, want %q", draft.Status, "draft")
	}
	if draft.Title != "Post-Mortem: DB Pool Exhaustion" {
		t.Fatalf("Title = %q, want %q", draft.Title, "Post-Mortem: DB Pool Exhaustion")
	}
	if draft.RootCause != "Pool max connections too low for traffic spike" {
		t.Fatalf("RootCause = %q", draft.RootCause)
	}
	if draft.Impact != "503 errors for 150 minutes affecting checkout" {
		t.Fatalf("Impact = %q", draft.Impact)
	}
	if draft.LessonsLearned != "Increased pool size; added autoscaling policy" {
		t.Fatalf("LessonsLearned = %q", draft.LessonsLearned)
	}
	if draft.WhatWentWrong == "" {
		t.Fatal("WhatWentWrong should be populated from actions_taken")
	}
	if draft.Summary == "" {
		t.Fatal("Summary should not be empty")
	}
	if !strings.Contains(draft.Summary, "SEV2") {
		t.Fatalf("Summary should contain severity, got: %q", draft.Summary)
	}
	if !strings.Contains(draft.Summary, "2h 30m") {
		t.Fatalf("Summary should contain duration, got: %q", draft.Summary)
	}
}

func TestBuildPostMortemDraft_ContributingFactorsFromTags(t *testing.T) {
	incident := &store.IncidentRecord{
		ID:             uuid.New(),
		IncidentNumber: 1,
		Tags:           []string{"database", "payment"},
		IncidentType:   "degradation",
	}

	draft := buildPostMortemDraft(context.Background(), postMortemDraftDeps{}, incident, "")

	if len(draft.ContributingFactors) != 3 {
		t.Fatalf("ContributingFactors = %v (len %d), want 3", draft.ContributingFactors, len(draft.ContributingFactors))
	}
	if !containsString(draft.ContributingFactors, "database") {
		t.Fatalf("expected 'database' in factors: %v", draft.ContributingFactors)
	}
	if !containsString(draft.ContributingFactors, "payment") {
		t.Fatalf("expected 'payment' in factors: %v", draft.ContributingFactors)
	}
	if !containsString(draft.ContributingFactors, "Incident type: degradation") {
		t.Fatalf("expected incident type in factors: %v", draft.ContributingFactors)
	}
}

func TestBuildPMTimeline_MergesAndSorts(t *testing.T) {
	started := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	mitigated := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)
	resolved := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	incident := &store.IncidentRecord{
		IncidentNumber: 1,
		StartedAt:      &started,
		MitigatedAt:    &mitigated,
		ResolvedAt:     &resolved,
	}

	timelineEntries := []store.IncidentTimelineEntryRecord{
		{EventType: "status_changed", Message: "Status changed to active", CreatedAt: started.Add(5 * time.Minute), ActorType: "system"},
		{EventType: "postmortem_created", Message: "should be filtered", CreatedAt: resolved, ActorType: "system"},
	}

	statusUpdates := []store.IncidentCoordinationMessageRecord{
		{Body: "Investigating connection pool", Metadata: map[string]any{"status_level": "investigating"}, CreatedAt: started.Add(10 * time.Minute), ActorType: "agent"},
		{Body: "Issue mitigated", Metadata: map[string]any{"status_level": "mitigated"}, CreatedAt: mitigated, ActorType: "agent"},
	}

	tl := buildPMTimeline(incident, timelineEntries, statusUpdates)

	if len(tl) < 4 {
		t.Fatalf("timeline too short: %d entries", len(tl))
	}

	var prev time.Time
	for _, entry := range tl {
		tsStr, ok := entry["timestamp"].(string)
		if !ok {
			t.Fatalf("missing timestamp in entry: %v", entry)
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			t.Fatalf("invalid timestamp %q: %v", tsStr, err)
		}
		if !prev.IsZero() && ts.Before(prev) {
			t.Fatalf("timeline not sorted: %s before %s", ts, prev)
		}
		prev = ts

		desc, _ := entry["description"].(string)
		if desc == "should be filtered" {
			t.Fatal("postmortem_created entry was not filtered out")
		}
	}

	last := tl[len(tl)-1]
	desc, _ := last["description"].(string)
	if desc != "Incident resolved" {
		t.Fatalf("last entry should be 'Incident resolved', got %q", desc)
	}
}

func TestBuildPMTitle(t *testing.T) {
	tests := []struct {
		name     string
		incident *store.IncidentRecord
		want     string
	}{
		{
			name:     "with incident title",
			incident: &store.IncidentRecord{IncidentNumber: 7, Title: "API Outage"},
			want:     "Post-Mortem: API Outage",
		},
		{
			name:     "without title falls back to number",
			incident: &store.IncidentRecord{IncidentNumber: 7},
			want:     "Post-Mortem: Incident #7",
		},
		{
			name:     "blank title trimmed",
			incident: &store.IncidentRecord{IncidentNumber: 7, Title: "   "},
			want:     "Post-Mortem: Incident #7",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPMTitle(tc.incident)
			if got != tc.want {
				t.Fatalf("buildPMTitle = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildPMImpact_FallbackFromIncidentMetadata(t *testing.T) {
	incident := &store.IncidentRecord{
		Severity:    "SEV1",
		ImpactLevel: "critical",
	}
	impact := buildPMImpact(incident, map[string]string{})
	if impact == "" {
		t.Fatal("impact should fall back to metadata")
	}
	if !strings.Contains(impact, "SEV1") {
		t.Fatalf("fallback impact should contain severity: %q", impact)
	}
}

func TestBuildPMImpact_FromDocSection(t *testing.T) {
	incident := &store.IncidentRecord{}
	sections := map[string]string{
		string(ics.SectionImpactAssessment): "Checkout down for 2 hours",
	}
	impact := buildPMImpact(incident, sections)
	if impact != "Checkout down for 2 hours" {
		t.Fatalf("Impact = %q, want doc section content", impact)
	}
}

func TestFormatHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "45s"},
		{3 * time.Minute, "3m"},
		{3*time.Minute + 30*time.Second, "3m 30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{2 * time.Hour, "2h"},
		{25 * time.Hour, "1d 1h"},
		{50 * time.Hour, "2d 2h"},
	}
	for _, tc := range tests {
		got := formatHumanDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatHumanDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestBuildPostMortemDraft_NilStoresProduceValidDraft(t *testing.T) {
	incident := &store.IncidentRecord{
		ID:             uuid.New(),
		IncidentNumber: 5,
		Title:          "Test Incident",
		Summary:        "Something broke",
	}
	draft := buildPostMortemDraft(context.Background(), postMortemDraftDeps{}, incident, "Resolved by operator")

	if draft == nil {
		t.Fatal("draft should not be nil")
	}
	if draft.Status != "draft" {
		t.Fatalf("Status = %q", draft.Status)
	}
	if draft.Title != "Post-Mortem: Test Incident" {
		t.Fatalf("Title = %q, want %q", draft.Title, "Post-Mortem: Test Incident")
	}
	if draft.Summary == "" {
		t.Fatal("Summary should not be empty even with nil stores")
	}
	if len(draft.ContributingFactors) != 0 {
		t.Fatalf("ContributingFactors should be empty with nil alert store, got %v", draft.ContributingFactors)
	}
}
