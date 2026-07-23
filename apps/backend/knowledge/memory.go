package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"alga/memory"
	"alga/store"
	"alga/strutil"
)

type MemoryEntry struct {
	ID         string
	Content    string
	MemoryType string
	Confidence float64
	AgentName  string
	AgentType  string
	CreatedAt  time.Time
}

type MemoryFinder interface {
	Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []MemoryEntry
}

type memoryFinder struct {
	svc memory.Service
}

func NewMemoryFinder(svc memory.Service) MemoryFinder {
	if svc == nil {
		return nil
	}
	return &memoryFinder{svc: svc}
}

func (f *memoryFinder) Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []MemoryEntry {
	if f == nil || f.svc == nil || inv == nil || limit <= 0 {
		return nil
	}

	var queryParts []string
	labels := primaryLabels(inv)

	if alertname := strings.TrimSpace(labels["alertname"]); alertname != "" {
		queryParts = append(queryParts, alertname)
	}
	if ns := strings.TrimSpace(labels["namespace"]); ns != "" {
		queryParts = append(queryParts, ns)
	}
	if svc := strings.TrimSpace(labels["service"]); svc != "" {
		queryParts = append(queryParts, svc)
	}
	if rc := strings.TrimSpace(inv.CorrelationKey); rc != "" {
		queryParts = append(queryParts, rc)
	}

	if len(queryParts) == 0 {
		return nil
	}

	query := strings.Join(queryParts, " ")
	results, err := f.svc.Search(ctx, query, labels, limit)
	if err != nil || len(results) == 0 {
		return nil
	}

	out := make([]MemoryEntry, 0, len(results))
	for _, r := range results {
		confidence := 0.0
		if r.Confidence != nil {
			confidence = *r.Confidence
		}
		out = append(out, MemoryEntry{
			ID:         r.ID.String(),
			Content:    r.Content,
			MemoryType: r.MemoryType,
			Confidence: confidence,
			AgentName:  r.AgentName,
			AgentType:  r.AgentType,
			CreatedAt:  r.CreatedAt,
		})
	}
	return out
}

func writeMemory(b *strings.Builder, m MemoryEntry) {
	mt := strings.TrimSpace(m.MemoryType)
	if mt == "" {
		mt = "fact"
	}
	agent := strings.TrimSpace(m.AgentType)
	if agent == "" {
		agent = "agent"
	}
	fmt.Fprintf(b, "- [%s] (confidence: %.2f, by %s, %s)\n",
		mt, m.Confidence, agent, humanizeAge(m.CreatedAt))
	fmt.Fprintf(b, "  %s\n", strutil.TruncateOneLine(m.Content, 240))
}
