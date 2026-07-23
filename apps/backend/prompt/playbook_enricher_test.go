package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/store"
)

func TestPlaybookEnricherIncludesMatchingSteps(t *testing.T) {
	playbookID := uuid.New()
	enricher := NewPlaybookEnricher(&stubPlaybookStore{
		matches: []*store.PlaybookRecord{{
			ID:      playbookID,
			Title:   "High CPU Runbook",
			Kind:    "procedure",
			Summary: "CPU pressure response",
		}},
		steps: map[uuid.UUID][]store.PlaybookStepRecord{
			playbookID: {
				{StepNumber: 1, Title: "Check pods", Description: "Find the hottest workload", ExpectedDuration: "2m", Command: "kubectl top pods"},
			},
		},
	})

	got := enricher.Enrich(context.Background(), map[string]string{"alertname": "HighCPU"})

	for _, want := range []string{"## Relevant Playbooks", "High CPU Runbook", "CPU pressure response", "1. Check pods", "Find the hottest workload", "kubectl top pods"} {
		if !strings.Contains(got, want) {
			t.Fatalf("enriched prompt missing %q:\n%s", want, got)
		}
	}
}

type stubPlaybookStore struct {
	matches []*store.PlaybookRecord
	steps   map[uuid.UUID][]store.PlaybookStepRecord
}

func (s *stubPlaybookStore) Create(context.Context, *store.PlaybookRecord, []store.PlaybookStepRecord) (*store.PlaybookRecord, error) {
	return nil, nil
}

func (s *stubPlaybookStore) Get(_ context.Context, id uuid.UUID) (*store.PlaybookRecord, []store.PlaybookStepRecord, error) {
	return nil, s.steps[id], nil
}

func (s *stubPlaybookStore) Update(context.Context, uuid.UUID, *store.PlaybookRecord) error {
	return nil
}
func (s *stubPlaybookStore) Delete(context.Context, uuid.UUID) error { return nil }
func (s *stubPlaybookStore) List(context.Context, store.PlaybookFilter, int, int) ([]*store.PlaybookRecord, int64, error) {
	return nil, 0, nil
}
func (s *stubPlaybookStore) AddStep(context.Context, *store.PlaybookStepRecord) (*store.PlaybookStepRecord, error) {
	return nil, nil
}
func (s *stubPlaybookStore) UpdateStep(context.Context, uuid.UUID, *store.PlaybookStepRecord) error {
	return nil
}
func (s *stubPlaybookStore) DeleteStep(context.Context, uuid.UUID) error { return nil }
func (s *stubPlaybookStore) ReorderSteps(context.Context, uuid.UUID, []store.StepOrder) error {
	return nil
}
func (s *stubPlaybookStore) FindMatching(context.Context, map[string]string) ([]*store.PlaybookRecord, error) {
	return s.matches, nil
}
