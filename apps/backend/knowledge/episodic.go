package knowledge

import (
	"context"
	"strconv"
	"strings"
	"time"

	"alga/rabbitmq"
	"alga/store"
)

// episodicDiscriminatorLabels is the ordered list of labels considered when
// matching past investigations to the current one. The first label that is
// present on the current alert is used as an additional OR clause on top of
// a matching correlation_key / alertname. The set is intentionally small to
// keep the query cheap and the signal high.
var episodicDiscriminatorLabels = []string{
	"service",
	"deployment",
	"namespace",
	"cluster",
}

// EpisodicFinder produces episodic memory entries for a given investigation.
type EpisodicFinder interface {
	Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []EpisodicEntry
}

type alertInvestigationEpisodicFinder struct {
	store store.AlertInvestigationStore
}

func NewEpisodicFinder(s store.AlertInvestigationStore) EpisodicFinder {
	return &alertInvestigationEpisodicFinder{store: s}
}

func (f *alertInvestigationEpisodicFinder) Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []EpisodicEntry {
	if f == nil || f.store == nil || inv == nil || limit <= 0 {
		return nil
	}
	labels := primaryLabels(inv)
	alertname := strings.TrimSpace(labels["alertname"])
	correlationKey := strings.TrimSpace(inv.CorrelationKey)
	if alertname == "" && correlationKey == "" {
		return nil
	}

	discriminators := map[string]string{}
	for _, k := range episodicDiscriminatorLabels {
		if v := strings.TrimSpace(labels[k]); v != "" {
			discriminators[k] = v
		}
	}

	records, err := f.store.FindSimilarAlertInvestigations(ctx, store.SimilarAlertInvestigationsQuery{
		CorrelationKey:         correlationKey,
		AlertName:              alertname,
		DiscriminatorLabels:    discriminators,
		ExcludeInvestigationID: inv.AlertInvestigationID,
		Limit:                  limit,
	})
	if err != nil || len(records) == 0 {
		return nil
	}

	entries := make([]EpisodicEntry, 0, len(records))
	for _, r := range records {
		e := EpisodicEntry{
			InvestigationID: r.AlertInvestigationID,
			AgentType:       r.AgentType,
			Severity:        rabbitmq.DetermineAlertSeverity(r.Alerts),
			Status:          r.Status,
			AlertName:       primaryAlertName(&r),
			Namespace:       primaryLabels(&r)["namespace"],
		}
		if r.Summary != nil {
			e.RootCause = r.Summary.RootCause
			e.Resolution = r.Summary.Resolution
			e.Summary = r.Summary.Summary
		}
		if r.CompletedAt != nil {
			e.CompletedAt = *r.CompletedAt
		} else {
			e.CompletedAt = r.UpdatedAt
		}
		entries = append(entries, e)
	}
	return entries
}

func primaryLabels(inv *store.AlertInvestigationRecord) map[string]string {
	if inv == nil || len(inv.Alerts) == 0 || inv.Alerts[0].Labels == nil {
		return map[string]string{}
	}
	return inv.Alerts[0].Labels
}

func primaryAlertName(inv *store.AlertInvestigationRecord) string {
	if v := strings.TrimSpace(primaryLabels(inv)["alertname"]); v != "" {
		return v
	}
	if inv != nil && len(inv.Alerts) > 0 {
		return inv.Alerts[0].Fingerprint
	}
	return ""
}

// humanizeAge returns a short human-readable age string (e.g. "2d", "3h",
// "15m", "just now").
func humanizeAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return intTrunc(d.Minutes()) + "m ago"
	case d < 24*time.Hour:
		return intTrunc(d.Hours()) + "h ago"
	default:
		days := int(d.Hours() / 24)
		return strconv.Itoa(days) + "d ago"
	}
}

func intTrunc(f float64) string {
	f = max(f, 1)
	return strconv.Itoa(int(f))
}
