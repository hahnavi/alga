package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
)

type service struct {
	memStore    store.AgentMemoryStore
	extractor   *Extractor
	embed       Embedder
	maxPerInv   int
	autoExtract bool
}

func NewService(memStore store.AgentMemoryStore, extractor *Extractor, embed Embedder, maxPerInv int, autoExtract bool) Service {
	if maxPerInv <= 0 {
		maxPerInv = 10
	}
	return &service{
		memStore:    memStore,
		extractor:   extractor,
		embed:       embed,
		maxPerInv:   maxPerInv,
		autoExtract: autoExtract,
	}
}

func (s *service) ExtractFromInvestigation(ctx context.Context, inv *store.AlertInvestigationRecord) error {
	if s == nil || !s.autoExtract || s.extractor == nil {
		return nil
	}

	updates := inv.Updates
	if updates == nil {
		updates = []store.InvestigationUpdate{}
	}

	if err := s.extractor.Extract(ctx, inv, updates); err != nil {
		logger.Debug("memory extraction failed", "alert_investigation_id", inv.AlertInvestigationID, "error", err)
		return fmt.Errorf("memory extraction: %w", err)
	}
	return nil
}

func (s *service) Search(ctx context.Context, query string, labels map[string]string, topK int) ([]store.ScoredMemory, error) {
	if s == nil || s.memStore == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}

	if s.embed != nil {
		embeddings, err := s.embed.Embed(ctx, []string{query})
		if err == nil && len(embeddings) > 0 && len(embeddings[0]) > 0 {
			results, err := s.memStore.Search(ctx, embeddings[0], topK, store.MemoryFilters{})
			if err == nil && len(results) > 0 {
				ids := make([]uuid.UUID, len(results))
				for i, r := range results {
					ids[i] = r.ID
				}
				_ = s.memStore.IncrementAccess(ctx, ids)
				return results, nil
			}
		}
	}

	results, err := s.memStore.SearchByText(ctx, query, topK, store.MemoryFilters{})
	if err != nil {
		return nil, fmt.Errorf("memory text search: %w", err)
	}
	if len(results) > 0 {
		ids := make([]uuid.UUID, len(results))
		for i, r := range results {
			ids[i] = r.ID
		}
		_ = s.memStore.IncrementAccess(ctx, ids)
	}
	return results, nil
}

func (s *service) CreateMemory(ctx context.Context, input CreateMemoryInput) (*store.AgentMemoryRecord, error) {
	if s == nil || s.memStore == nil {
		return nil, errors.New("memory service not available")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, errors.New("content is required")
	}

	content := strings.TrimSpace(input.Content)
	hash := memoryHash(content)

	exists, err := s.memStore.ExistsByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("check hash: %w", err)
	}
	if exists {
		return nil, errors.New("memory with this content already exists")
	}

	memType := strings.ToLower(strings.TrimSpace(input.MemoryType))
	if memType == "" {
		memType = store.MemoryTypeFact
	}
	if !store.IsValidMemoryType(memType) {
		return nil, fmt.Errorf("invalid memory_type %q", memType)
	}

	var embedding []float32
	if s.embed != nil {
		embeddings, err := s.embed.Embed(ctx, []string{content})
		if err == nil && len(embeddings) > 0 {
			embedding = embeddings[0]
		}
	}

	labels := input.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	record := &store.AgentMemoryRecord{
		Content:         content,
		MemoryType:      memType,
		Hash:            hash,
		Embedding:       embedding,
		AgentID:         input.AgentID,
		AgentName:       input.AgentName,
		AgentType:       input.AgentType,
		InvestigationID: input.InvestigationID,
		CorrelationKey:  input.CorrelationKey,
		Labels:          labels,
		Confidence:      input.Confidence,
		ExpiresAt:       input.ExpiresAt,
	}

	return s.memStore.Create(ctx, record)
}

func (s *service) Get(ctx context.Context, id uuid.UUID) (*store.AgentMemoryRecord, error) {
	if s == nil || s.memStore == nil {
		return nil, errors.New("memory service not available")
	}
	return s.memStore.Get(ctx, id)
}

func (s *service) Update(ctx context.Context, id uuid.UUID, content string) (*store.AgentMemoryRecord, error) {
	if s == nil || s.memStore == nil {
		return nil, errors.New("memory service not available")
	}

	var embedding []float32
	if s.embed != nil {
		embeddings, err := s.embed.Embed(ctx, []string{content})
		if err == nil && len(embeddings) > 0 {
			embedding = embeddings[0]
		}
	}

	return s.memStore.Update(ctx, id, content, embedding)
}

func (s *service) Delete(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.memStore == nil {
		return errors.New("memory service not available")
	}
	return s.memStore.Delete(ctx, id)
}

func (s *service) List(ctx context.Context, f store.MemoryFilters) ([]store.AgentMemoryRecord, int, error) {
	if s == nil || s.memStore == nil {
		return nil, 0, errors.New("memory service not available")
	}
	return s.memStore.List(ctx, f)
}

func (s *service) DeleteExpired(ctx context.Context) (int, error) {
	if s == nil || s.memStore == nil {
		return 0, nil
	}
	return s.memStore.DeleteExpired(ctx)
}
