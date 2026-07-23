package knowledge

import (
	"context"
	"strings"
	"time"

	"alga/store"
	"alga/valkey"
)

// ConcurrentFinder surfaces other currently in-flight investigations that
// share at least one discriminator label with the caller's investigation.
type ConcurrentFinder interface {
	Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []ConcurrentEntry
}

type valkeyConcurrentFinder struct {
	client *valkey.Client
}

// NewConcurrentFinder returns a ConcurrentFinder backed by Valkey's active
// investigations registry. Returns nil when no Valkey client is configured.
func NewConcurrentFinder(c *valkey.Client) ConcurrentFinder {
	if c == nil {
		return nil
	}
	return &valkeyConcurrentFinder{client: c}
}

func (f *valkeyConcurrentFinder) Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []ConcurrentEntry {
	if f == nil || f.client == nil || inv == nil {
		return nil
	}
	discriminators := concurrentDiscriminators(inv)
	if len(discriminators) == 0 {
		return nil
	}
	actives, err := f.client.ListActiveByDiscriminators(ctx, discriminators, inv.AlertInvestigationID, limit)
	if err != nil || len(actives) == 0 {
		return nil
	}
	now := time.Now().UTC()
	out := make([]ConcurrentEntry, 0, len(actives))
	for _, a := range actives {
		ago := time.Duration(0)
		if !a.StartedAt.IsZero() {
			ago = now.Sub(a.StartedAt)
			ago = max(ago, 0)
		}
		out = append(out, ConcurrentEntry{
			InvestigationID: a.InvestigationID,
			AgentType:       a.AgentType,
			AlertName:       a.AlertName,
			Namespace:       a.Namespace,
			Severity:        a.Severity,
			StartedAgo:      ago,
		})
	}
	return out
}

// WithConcurrent returns a shallow copy of the aggregator with the given
// concurrent finder wired.
func (a *Aggregator) WithConcurrent(c ConcurrentFinder) *Aggregator {
	if a == nil {
		return nil
	}
	cp := *a
	cp.concurrent = c
	return &cp
}

// concurrentDiscriminators returns the labels we use to detect overlap
// between in-flight investigations. Kept in sync with
// episodicDiscriminatorLabels so episodic and concurrent layers surface
// related incidents consistently.
func concurrentDiscriminators(inv *store.AlertInvestigationRecord) map[string]string {
	out := map[string]string{}
	if inv == nil || len(inv.Alerts) == 0 {
		return out
	}
	keys := []string{"namespace", "service", "deployment", "cluster", "region", "alertname"}
	for _, a := range inv.Alerts {
		for _, k := range keys {
			if v, ok := a.Labels[k]; ok {
				v = strings.TrimSpace(v)
				if v != "" {
					if _, exists := out[k]; !exists {
						out[k] = v
					}
				}
			}
		}
	}
	return out
}
