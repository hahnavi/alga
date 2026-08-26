package worker

import (
	"context"
	"fmt"
	"time"

	"alga/logger"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

const (
	actionItemSweepTick       = 5 * time.Minute
	actionItemOverdueDedupTTL = 24 * time.Hour
)

type ActionItemSweepWorker struct {
	actionItemStore   store.ActionItemStore
	incidentStore     store.IncidentStore
	notificationStore store.NotificationStore
	ssePublisher      *sse.DualPublisher
	vkClient          *valkey.Client
}

// NewActionItemSweepWorker creates the overdue action-item sweep. The
// notification store, SSE publisher, and Valkey client are optional signals;
// when nil, the sweep still runs but stays log-only (previous behavior).
func NewActionItemSweepWorker(
	actionItemStore store.ActionItemStore,
	incidentStore store.IncidentStore,
) *ActionItemSweepWorker {
	return &ActionItemSweepWorker{
		actionItemStore: actionItemStore,
		incidentStore:   incidentStore,
	}
}

// SetSignals wires the optional user-visible signal paths (SSE + in-app
// notifications, deduplicated via Valkey).
func (w *ActionItemSweepWorker) SetSignals(
	notificationStore store.NotificationStore,
	ssePublisher *sse.DualPublisher,
	vkClient *valkey.Client,
) {
	w.notificationStore = notificationStore
	w.ssePublisher = ssePublisher
	w.vkClient = vkClient
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

		if signaled := w.signalOverdue(ctx, item); signaled {
			logger.Info("Action item overdue signal delivered",
				"component", "actionitem-sweep",
				"ai_id", item.ID.String())
		}
	}
}

// signalOverdue publishes an SSE event and (when an assignee exists) creates a
// per-assignee in-app notification for an overdue item. Both paths are best-
// effort: failures are logged and never abort the sweep. Returns true when at
// least one fresh signal was delivered.
func (w *ActionItemSweepWorker) signalOverdue(ctx context.Context, item store.ActionItemRecord) bool {
	signaled := false

	data := map[string]any{
		"id":             item.ID.String(),
		"post_mortem_id": item.PostMortemID.String(),
		"description":    item.Description,
		"assignee_name":  item.AssigneeName,
		"assignee_id":    "",
		"due_date":       item.DueDate.Format(time.RFC3339),
	}
	if item.AssigneeID != nil {
		data["assignee_id"] = item.AssigneeID.String()
	}

	if w.ssePublisher != nil {
		w.ssePublisher.Publish(sse.Event{
			Type: "action_item_overdue",
			Data: data,
		})
		if item.AssigneeID != nil {
			w.ssePublisher.PublishToUser(item.AssigneeID.String(), sse.Event{
				Type: "action_item_overdue",
				Data: data,
			})
		}
		signaled = true
	}

	if w.notificationStore != nil && item.AssigneeID != nil && w.markOverdueNotified(ctx, item.ID) {
		title := "Action item overdue"
		msg := fmt.Sprintf("%q is past its due date (%s).",
			truncateForNotification(item.Description), item.DueDate.Format(time.RFC3339))
		record := &store.NotificationRecord{
			UserID:       item.AssigneeID.String(),
			Type:         "info",
			Title:        title,
			Message:      msg,
			ResourceType: "post_mortem",
			ResourceID:   item.PostMortemID.String(),
		}
		if _, err := w.notificationStore.Create(ctx, record); err != nil {
			logger.Error("Failed to create overdue action-item notification",
				"component", "actionitem-sweep", "ai_id", item.ID.String(), "error", err)
		} else {
			signaled = true
		}
	}

	return signaled
}

// markOverdueNotified deduplicates assignee notifications with a 24h Valkey
// SETNX so the 5-minute sweep does not spam users. On Valkey failure it
// returns true (notify anyway): a duplicate reminder beats silence.
func (w *ActionItemSweepWorker) markOverdueNotified(ctx context.Context, itemID fmt.Stringer) bool {
	if w.vkClient == nil {
		return true
	}
	key := "alga:actionitem:overdue_notified:" + itemID.String()
	ok, err := w.vkClient.SetNX(ctx, key, "1", actionItemOverdueDedupTTL)
	if err != nil {
		logger.Warn("Overdue action-item SETNX failed; notifying anyway",
			"component", "actionitem-sweep", "key", key, "error", err)
		return true
	}
	return ok
}

// truncateForNotification keeps notification message bodies bounded when
// operators write long action-item descriptions.
func truncateForNotification(s string) string {
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
