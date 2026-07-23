package worker

import (
	"context"
	"time"

	"alga/logger"
	"alga/store"
)

const actionItemSweepTick = 5 * time.Minute

type ActionItemSweepWorker struct {
	actionItemStore store.ActionItemStore
	incidentStore   store.IncidentStore
}

func NewActionItemSweepWorker(
	actionItemStore store.ActionItemStore,
	incidentStore store.IncidentStore,
) *ActionItemSweepWorker {
	return &ActionItemSweepWorker{
		actionItemStore: actionItemStore,
		incidentStore:   incidentStore,
	}
}

func (w *ActionItemSweepWorker) Run(ctx context.Context) {
	runTickerLoop(ctx, actionItemSweepTick, "actionitem-sweep", w.tick)
}

func (w *ActionItemSweepWorker) tick(ctx context.Context) {
	items, err := w.actionItemStore.ListOverdue(ctx)
	if err != nil {
		logger.Error("Action item sweep: failed to list overdue items", "component", "actionitem-sweep", "error", err)
		return
	}

	for _, item := range items {
		logger.Info("Action item overdue",
			"component", "actionitem-sweep",
			"ai_id", item.ID.String(),
			"assignee", item.AssigneeName,
			"due_date", item.DueDate.Format(time.RFC3339))
	}
}
