package memory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
	"alga/strutil"
)

type Extractor struct {
	llm       LLM
	embed     Embedder
	memStore  store.AgentMemoryStore
	maxPerInv int
}

func NewExtractor(llm LLM, embed Embedder, memStore store.AgentMemoryStore, maxPerInv int) *Extractor {
	if maxPerInv <= 0 {
		maxPerInv = 10
	}
	return &Extractor{llm: llm, embed: embed, memStore: memStore, maxPerInv: maxPerInv}
}

func (e *Extractor) Extract(ctx context.Context, inv *store.AlertInvestigationRecord, updates []store.InvestigationUpdate) error {
	if e == nil || e.llm == nil {
		return nil
	}
	if inv == nil {
		return nil
	}

	outcome := inv.Summary
	if outcome == nil || (strings.TrimSpace(outcome.RootCause) == "" && strings.TrimSpace(outcome.Resolution) == "" && len(updates) == 0) {
		return nil
	}

	userPrompt := buildExtractionUserPrompt(inv, updates)
	messages := []Message{
		{Role: "system", Content: extractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := e.llm.Generate(ctx, messages)
	if err != nil {
		return fmt.Errorf("memory extraction LLM call: %w", err)
	}

	resp = strings.TrimSpace(resp)
	if resp == "" || resp == "{}" {
		return nil
	}

	var result extractionResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return fmt.Errorf("parse extraction result: %w", err)
	}

	if len(result.Memories) == 0 {
		return nil
	}

	// Hard-cap the extraction list BEFORE embedding: nothing over the cap is
	// embedded or persisted (WP-A11; also bounds embedding cost).
	if len(result.Memories) > e.maxPerInv {
		logger.Debug("truncating extracted memories to per-investigation cap",
			"returned", len(result.Memories), "cap", e.maxPerInv)
		result.Memories = result.Memories[:e.maxPerInv]
	}

	texts := make([]string, 0, len(result.Memories))
	for _, m := range result.Memories {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}

	var embeddings [][]float32
	if e.embed != nil && len(texts) > 0 {
		embeddings, err = e.embed.Embed(ctx, texts)
		if err != nil {
			logger.Debug("memory embedding failed, continuing without vectors", "error", err)
			embeddings = nil
		}
	}

	var labels map[string]string
	if len(inv.Alerts) > 0 {
		labels = inv.Alerts[0].Labels
	}
	if labels == nil {
		labels = map[string]string{}
	}

	var agentID *uuid.UUID
	if inv.AgentID != "" {
		if id, err := uuid.Parse(inv.AgentID); err == nil {
			agentID = &id
		}
	}

	created := 0
	embedIdx := 0
	for _, m := range result.Memories {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}

		hash := memoryHash(text)
		exists, err := e.memStore.ExistsByHash(ctx, hash)
		if err != nil {
			logger.Debug("memory hash check failed", "error", err)
			continue
		}
		if exists {
			embedIdx++
			continue
		}

		memType := strings.ToLower(strings.TrimSpace(m.Type))
		if memType == "" || !store.IsValidMemoryType(memType) {
			memType = store.MemoryTypeFact
		}

		confidence := m.Confidence
		if confidence <= 0 {
			confidence = 0.7
		}

		entities := m.Entities
		if entities == nil {
			entities = []string{}
		}

		var embedding []float32
		if embeddings != nil && embedIdx < len(embeddings) {
			embedding = embeddings[embedIdx]
		}

		record := &store.AgentMemoryRecord{
			Content:         text,
			MemoryType:      memType,
			Hash:            hash,
			Embedding:       embedding,
			AgentID:         agentID,
			AgentName:       inv.AgentName,
			AgentType:       inv.AgentType,
			InvestigationID: inv.AlertInvestigationID,
			CorrelationKey:  inv.CorrelationKey,
			Labels:          labels,
			Entities:        entities,
			Confidence:      &confidence,
		}

		if _, err := e.memStore.Create(ctx, record); err != nil {
			logger.Debug("failed to create memory", "error", err, "text", strutil.TruncateOneLine(text, 80))
			continue
		}
		created++
		embedIdx++
	}

	if created > 0 {
		logger.Debug("extracted memories from investigation",
			"alert_investigation_id", inv.AlertInvestigationID,
			"count", created,
		)
	}

	return nil
}

func memoryHash(text string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(text))))
	return fmt.Sprintf("%x", h)
}
