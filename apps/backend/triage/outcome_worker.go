package triage

import (
	"context"
	"runtime/debug"
	"time"

	"alga/logger"
	"alga/store"
)

type OutcomeWorker struct {
	triageStore      store.TriageResultStore
	alertStore       store.Store
	sweepInterval    time.Duration
	evaluationDelay  time.Duration
	autoPromoteCount int
}

func NewOutcomeWorker(
	triageStore store.TriageResultStore,
	alertStore store.Store,
	sweepInterval time.Duration,
	evaluationDelay time.Duration,
	autoPromoteCount int,
) *OutcomeWorker {
	return &OutcomeWorker{
		triageStore:      triageStore,
		alertStore:       alertStore,
		sweepInterval:    sweepInterval,
		evaluationDelay:  evaluationDelay,
		autoPromoteCount: autoPromoteCount,
	}
}

func (w *OutcomeWorker) Start(ctx context.Context) {
	logger.InfoCtx(ctx, "Triage outcome worker started", "component", "triage", "sweep_interval", w.sweepInterval, "evaluation_delay", w.evaluationDelay, "auto_promote_count", w.autoPromoteCount)
	ticker := time.NewTicker(w.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.InfoCtx(ctx, "Triage outcome worker stopped")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("triage outcome worker tick panicked", "component", "triage", "tick", "sweep", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				w.sweep(ctx)
			}()
		}
	}
}

func (w *OutcomeWorker) sweep(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-w.evaluationDelay)

	results, _, err := w.triageStore.List(ctx, store.TriageResultQuery{
		Outcome: store.TriageResultOutcomePending,
		EndDate: cutoff,
		Limit:   50,
	})
	if err != nil {
		logger.ErrorCtx(ctx, "Failed to list pending triage results", "component", "triage", "error", err)
		return
	}

	for i := range results {
		w.evaluateOutcome(ctx, &results[i])
	}
}

func (w *OutcomeWorker) evaluateOutcome(ctx context.Context, result *store.TriageResultRecord) {
	patch := &store.TriageResultRecord{}

	switch result.Decision {
	case "auto_resolve":
		for _, fp := range result.AlertFingerprints {
			a, err := w.alertStore.GetByFingerprint(fp)
			if err != nil {
				continue
			}
			if a != nil && a.Status == "firing" {
				patch.Outcome = store.TriageResultOutcomeOverridden
				patch.OverriddenTo = "investigate"
				break
			}
		}
		if patch.Outcome == "" {
			patch.Outcome = store.TriageResultOutcomeConfirmed
		}

	case "suppress":
		patch.Outcome = store.TriageResultOutcomeConfirmed

	case "investigate", "enrich_only":
		patch.Outcome = store.TriageResultOutcomeConfirmed

	default:
		patch.Outcome = store.TriageResultOutcomeConfirmed
	}

	if patch.Outcome != "" {
		now := time.Now().UTC()
		patch.OverriddenAt = &now
		if _, err := w.triageStore.Update(ctx, result.ID.String(), patch); err != nil {
			logger.ErrorCtx(ctx, "Failed to update triage outcome", "component", "triage", "id", result.ID, "error", err)
		}
	}

	if patch.Outcome == store.TriageResultOutcomeConfirmed && w.autoPromoteCount > 0 && result.CorrelationKey != "" && len(result.AlertFingerprints) > 0 {
		w.checkAutoPromote(ctx, result)
	}
}

func (w *OutcomeWorker) checkAutoPromote(ctx context.Context, result *store.TriageResultRecord) {
	confirmedResults, _, err := w.triageStore.List(ctx, store.TriageResultQuery{
		Outcome: store.TriageResultOutcomeConfirmed,
		Limit:   w.autoPromoteCount,
	})
	if err != nil {
		return
	}

	count := 0
	for _, r := range confirmedResults {
		if r.CorrelationKey == result.CorrelationKey && r.Decision == result.Decision {
			count++
		}
	}

	if count >= w.autoPromoteCount {
		alertName := "unknown"
		if len(result.AlertLabels) > 0 {
			if n, ok := result.AlertLabels["alertname"]; ok {
				alertName = n
			}
		}
		logger.InfoCtx(ctx, "Auto-promote candidate", "component", "triage", "correlation_key", result.CorrelationKey, "alert_name", alertName, "confirmed_count", count)
	}
}
