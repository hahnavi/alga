package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"alga/db/models"
	"alga/store"
)

// fakeMemoryStore scripts search results and records access increments so the
// threshold post-filter and its access-bookkeeping can be asserted.
type fakeMemoryStore struct {
	vectorResults []store.ScoredMemory
	textResults   []store.ScoredMemory
	accessed      []uuid.UUID
}

func (f *fakeMemoryStore) Create(ctx context.Context, mem *store.AgentMemoryRecord) (*store.AgentMemoryRecord, error) {
	return mem, nil
}
func (f *fakeMemoryStore) Get(ctx context.Context, id uuid.UUID) (*store.AgentMemoryRecord, error) {
	return nil, store.ErrNotFound
}
func (f *fakeMemoryStore) Update(ctx context.Context, id uuid.UUID, content string, embedding []float32) (*store.AgentMemoryRecord, error) {
	return nil, store.ErrNotFound
}
func (f *fakeMemoryStore) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (f *fakeMemoryStore) List(ctx context.Context, fl store.MemoryFilters) ([]store.AgentMemoryRecord, int, error) {
	return nil, 0, nil
}
func (f *fakeMemoryStore) Search(ctx context.Context, embedding []float32, topK int, fl store.MemoryFilters) ([]store.ScoredMemory, error) {
	return f.vectorResults, nil
}
func (f *fakeMemoryStore) SearchByText(ctx context.Context, query string, topK int, fl store.MemoryFilters) ([]store.ScoredMemory, error) {
	return f.textResults, nil
}
func (f *fakeMemoryStore) IncrementAccess(ctx context.Context, ids []uuid.UUID) error {
	f.accessed = append(f.accessed, ids...)
	return nil
}
func (f *fakeMemoryStore) DeleteExpired(ctx context.Context) (int, error) { return 0, nil }
func (f *fakeMemoryStore) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	return false, nil
}
func (f *fakeMemoryStore) FindByInvestigation(ctx context.Context, id string) ([]store.AgentMemoryRecord, error) {
	return nil, nil
}

// staticEmbedder returns a fixed vector so the search path takes the vector
// branch without a real embedding backend.
type staticEmbedder struct{}

func (staticEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{0.1, 0.2, 0.3}
	}
	return vectors, nil
}
func (staticEmbedder) Dimension() int { return 3 }

func scored(contents string, scores ...float64) []store.ScoredMemory {
	out := make([]store.ScoredMemory, len(scores))
	for i, s := range scores {
		out[i] = store.ScoredMemory{
			AgentMemoryRecord: store.AgentMemoryRecord{ID: uuid.New(), Content: fmt.Sprintf("%s-%d", contents, i)},
			Score:             s,
		}
	}
	return out
}

func TestSearchAppliesSimilarityThreshold(t *testing.T) {
	st := &fakeMemoryStore{vectorResults: scored("mem", 0.95, 0.82, 0.4, 0.79)}
	svc := NewService(st, nil, staticEmbedder{}, ServiceOptions{SimilarityThreshold: 0.8})

	results, err := svc.Search(context.Background(), "query", nil, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 survivors above the 0.8 threshold", len(results))
	}
	for _, r := range results {
		if r.Score < 0.8 {
			t.Fatalf("result score %v below threshold survived", r.Score)
		}
	}
	if len(st.accessed) != 2 {
		t.Fatalf("access incremented for %d ids, want only the 2 survivors", len(st.accessed))
	}
}

func TestSearchZeroThresholdReturnsRawTopK(t *testing.T) {
	st := &fakeMemoryStore{vectorResults: scored("mem", 0.95, 0.4, 0.01)}
	svc := NewService(st, nil, staticEmbedder{}, ServiceOptions{})

	results, err := svc.Search(context.Background(), "query", nil, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want the raw 3 with filtering disabled", len(results))
	}
	if len(st.accessed) != 3 {
		t.Fatalf("access incremented for %d ids, want 3", len(st.accessed))
	}
}

func TestSearchTextPathAlsoFiltered(t *testing.T) {
	// No embedder configured: the ts_rank text path is the only source.
	st := &fakeMemoryStore{textResults: scored("mem", 1.2, 0.3)}
	svc := NewService(st, nil, nil, ServiceOptions{SimilarityThreshold: 0.8})

	results, err := svc.Search(context.Background(), "query", nil, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Score < 0.8 {
		t.Fatalf("text results = %+v, want only the 1.2-scored survivor", results)
	}
}

// capLLM returns an extraction payload with `count` memories.
type capLLM struct{ count int }

func (c capLLM) Generate(ctx context.Context, messages []Message) (string, error) {
	memories := make([]extractedMemory, c.count)
	for i := range memories {
		memories[i] = extractedMemory{Text: fmt.Sprintf("memory number %d", i), Type: "fact"}
	}
	data, err := json.Marshal(extractionResult{Memories: memories})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type capMemStore struct {
	fakeMemoryStore
	created []store.AgentMemoryRecord
}

func (f *capMemStore) Create(ctx context.Context, mem *store.AgentMemoryRecord) (*store.AgentMemoryRecord, error) {
	f.created = append(f.created, *mem)
	return mem, nil
}

func TestExtractorHardCapsMemoriesBeforeEmbedding(t *testing.T) {
	st := &capMemStore{}
	e := NewExtractor(capLLM{count: 25}, nil, st, 10)

	if err := e.Extract(context.Background(), &store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-1",
		Summary:              &models.AlertInvestigationSummary{RootCause: "root cause"},
	}, nil); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(st.created) > 10 {
		t.Fatalf("persisted %d memories, want <= 10 (the cap)", len(st.created))
	}
}
