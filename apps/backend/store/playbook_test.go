package store

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPlaybookCreateAndGet(t *testing.T) {
	s := newStubPlaybookStore()
	ctx := context.Background()
	userID := uuid.New()

	record, err := s.Create(ctx, &PlaybookRecord{
		Title:     "Test Playbook",
		Kind:      "procedure",
		Summary:   "A test",
		Tags:      []string{"test"},
		CreatedBy: userID,
	}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if record.ID == uuid.Nil {
		t.Fatal("expected non-nil ID")
	}

	got, steps, err := s.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Test Playbook" {
		t.Fatalf("expected 'Test Playbook', got '%s'", got.Title)
	}
	if len(steps) != 0 {
		t.Fatalf("expected 0 steps, got %d", len(steps))
	}
}

func TestPlaybookWithSteps(t *testing.T) {
	s := newStubPlaybookStore()
	ctx := context.Background()
	userID := uuid.New()

	record, err := s.Create(ctx, &PlaybookRecord{
		Title:     "With Steps",
		Kind:      "mitigation",
		CreatedBy: userID,
	}, []PlaybookStepRecord{
		{StepNumber: 1, Title: "Step 1", Description: "First", ExpectedDuration: "5m", Command: "kubectl get pods"},
		{StepNumber: 2, Title: "Step 2", Description: "Second", ExpectedDuration: "2m"},
	})
	if err != nil {
		t.Fatalf("Create with steps: %v", err)
	}

	_, steps, err := s.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Title != "Step 1" {
		t.Fatalf("expected 'Step 1', got '%s'", steps[0].Title)
	}
	if steps[0].Command != "kubectl get pods" {
		t.Fatalf("expected command, got '%s'", steps[0].Command)
	}
}

func TestPlaybookFindMatching(t *testing.T) {
	s := newStubPlaybookStore()
	ctx := context.Background()
	userID := uuid.New()

	_, _ = s.Create(ctx, &PlaybookRecord{
		Title:          "Error Playbook",
		Kind:           "procedure",
		CreatedBy:      userID,
		LabelSelectors: []map[string]any{{"alertname": "HighErrorRate"}},
	}, nil)
	_, _ = s.Create(ctx, &PlaybookRecord{
		Title:          "CPU Playbook",
		Kind:           "procedure",
		CreatedBy:      userID,
		LabelSelectors: []map[string]any{{"alertname": "HighCPU"}},
	}, nil)

	matching, err := s.FindMatching(ctx, map[string]string{"alertname": "HighErrorRate"})
	if err != nil {
		t.Fatalf("FindMatching: %v", err)
	}
	if len(matching) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matching))
	}
	if matching[0].Title != "Error Playbook" {
		t.Fatalf("expected 'Error Playbook', got '%s'", matching[0].Title)
	}
}

func TestPlaybookDelete(t *testing.T) {
	s := newStubPlaybookStore()
	ctx := context.Background()
	userID := uuid.New()

	record, _ := s.Create(ctx, &PlaybookRecord{
		Title:     "To Delete",
		Kind:      "procedure",
		CreatedBy: userID,
	}, nil)

	err := s.Delete(ctx, record.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _, err := s.Get(ctx, record.ID)
	if err == nil && got != nil {
		t.Fatal("expected playbook to be deleted")
	}
}

func TestPlaybookListWithFilters(t *testing.T) {
	s := newStubPlaybookStore()
	ctx := context.Background()
	userID := uuid.New()

	_, _ = s.Create(ctx, &PlaybookRecord{
		Title:     "Procedure A",
		Kind:      "procedure",
		CreatedBy: userID,
		Tags:      []string{"critical"},
	}, nil)
	_, _ = s.Create(ctx, &PlaybookRecord{
		Title:     "Mitigation B",
		Kind:      "mitigation",
		CreatedBy: userID,
	}, nil)

	filter := PlaybookFilter{Kind: "mitigation"}
	records, total, err := s.List(ctx, filter, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 total, got %d", total)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Title != "Mitigation B" {
		t.Fatalf("expected 'Mitigation B', got '%s'", records[0].Title)
	}
}

func TestPlaybookStepReorder(t *testing.T) {
	s := newStubPlaybookStore()
	ctx := context.Background()
	userID := uuid.New()

	record, _ := s.Create(ctx, &PlaybookRecord{
		Title:     "Reorder",
		Kind:      "procedure",
		CreatedBy: userID,
	}, []PlaybookStepRecord{
		{StepNumber: 1, Title: "First"},
		{StepNumber: 2, Title: "Second"},
	})

	_, steps, _ := s.Get(ctx, record.ID)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	err := s.ReorderSteps(ctx, record.ID, []StepOrder{
		{ID: steps[1].ID, StepNumber: 1},
		{ID: steps[0].ID, StepNumber: 2},
	})
	if err != nil {
		t.Fatalf("ReorderSteps: %v", err)
	}

	_, reordered, _ := s.Get(ctx, record.ID)
	if reordered[0].Title != "Second" {
		t.Fatalf("expected first step to be 'Second', got '%s'", reordered[0].Title)
	}
}

// stubPlaybookStore is an in-memory implementation of PlaybookStore for testing.
type stubPlaybookStore struct {
	playbooks map[string]*PlaybookRecord
	steps     map[string][]PlaybookStepRecord
}

func newStubPlaybookStore() *stubPlaybookStore {
	return &stubPlaybookStore{
		playbooks: make(map[string]*PlaybookRecord),
		steps:     make(map[string][]PlaybookStepRecord),
	}
}

func (s *stubPlaybookStore) Create(ctx context.Context, r *PlaybookRecord, steps []PlaybookStepRecord) (*PlaybookRecord, error) {
	r.ID = uuid.New()
	r.CreatedAt = now()
	r.UpdatedAt = now()
	s.playbooks[r.ID.String()] = r

	for i := range steps {
		steps[i].ID = uuid.New()
		steps[i].PlaybookID = r.ID
		steps[i].CreatedAt = now()
		steps[i].UpdatedAt = now()
	}
	s.steps[r.ID.String()] = steps
	return r, nil
}

func (s *stubPlaybookStore) Get(ctx context.Context, id uuid.UUID) (*PlaybookRecord, []PlaybookStepRecord, error) {
	r, ok := s.playbooks[id.String()]
	if !ok {
		return nil, nil, nil
	}
	steps := s.steps[id.String()]
	sorted := make([]PlaybookStepRecord, len(steps))
	copy(sorted, steps)
	slices.SortFunc(sorted, func(a, b PlaybookStepRecord) int {
		return cmp.Compare(a.StepNumber, b.StepNumber)
	})
	return r, sorted, nil
}

func (s *stubPlaybookStore) Update(ctx context.Context, id uuid.UUID, r *PlaybookRecord) error {
	existing, ok := s.playbooks[id.String()]
	if !ok {
		return nil
	}
	existing.Title = r.Title
	existing.Kind = r.Kind
	existing.Summary = r.Summary
	existing.ServiceID = r.ServiceID
	existing.LabelSelectors = r.LabelSelectors
	existing.Tags = r.Tags
	existing.UpdatedAt = now()
	return nil
}

func (s *stubPlaybookStore) Delete(ctx context.Context, id uuid.UUID) error {
	delete(s.playbooks, id.String())
	delete(s.steps, id.String())
	return nil
}

func (s *stubPlaybookStore) List(ctx context.Context, filter PlaybookFilter, limit, skip int) ([]*PlaybookRecord, int64, error) {
	var result []*PlaybookRecord
	for _, r := range s.playbooks {
		if filter.Kind != "" && r.Kind != filter.Kind {
			continue
		}
		if filter.ServiceID != nil && (r.ServiceID == nil || *r.ServiceID != *filter.ServiceID) {
			continue
		}
		if filter.Tag != "" {
			found := false
			for _, t := range r.Tags {
				if t == filter.Tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if filter.Search != "" {
			if !strings.Contains(strings.ToLower(r.Title), strings.ToLower(filter.Search)) {
				continue
			}
		}
		result = append(result, r)
	}
	if skip < len(result) {
		result = result[skip:]
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, int64(len(result)), nil
}

func (s *stubPlaybookStore) AddStep(ctx context.Context, step *PlaybookStepRecord) (*PlaybookStepRecord, error) {
	steps := s.steps[step.PlaybookID.String()]
	maxNum := 0
	for _, st := range steps {
		if st.StepNumber > maxNum {
			maxNum = st.StepNumber
		}
	}
	step.ID = uuid.New()
	step.StepNumber = maxNum + 1
	step.CreatedAt = now()
	step.UpdatedAt = now()
	s.steps[step.PlaybookID.String()] = append(steps, *step)
	return step, nil
}

func (s *stubPlaybookStore) UpdateStep(ctx context.Context, id uuid.UUID, step *PlaybookStepRecord) error {
	for pbID, steps := range s.steps {
		for i, st := range steps {
			if st.ID == id {
				steps[i].Title = step.Title
				steps[i].Description = step.Description
				steps[i].ExpectedDuration = step.ExpectedDuration
				steps[i].Command = step.Command
				steps[i].UpdatedAt = now()
				s.steps[pbID] = steps
				return nil
			}
		}
	}
	return nil
}

func (s *stubPlaybookStore) DeleteStep(ctx context.Context, id uuid.UUID) error {
	for pbID, steps := range s.steps {
		for i, st := range steps {
			if st.ID == id {
				s.steps[pbID] = append(steps[:i], steps[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

func (s *stubPlaybookStore) ReorderSteps(ctx context.Context, playbookID uuid.UUID, order []StepOrder) error {
	steps := s.steps[playbookID.String()]
	for _, o := range order {
		for i, st := range steps {
			if st.ID == o.ID {
				steps[i].StepNumber = o.StepNumber
			}
		}
	}
	s.steps[playbookID.String()] = steps
	return nil
}

func (s *stubPlaybookStore) FindMatching(ctx context.Context, labels map[string]string) ([]*PlaybookRecord, error) {
	var matching []*PlaybookRecord
	for _, p := range s.playbooks {
		if len(p.LabelSelectors) == 0 {
			continue
		}
		if matchLabelSelectors(p.LabelSelectors, labels) {
			matching = append(matching, p)
		}
	}
	return matching, nil
}

func now() time.Time {
	return time.Now()
}
