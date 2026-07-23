package servicetracker

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"alga/logger"
	"alga/sse"
	"alga/store"
)

type StatusTracker struct {
	serviceStore  store.ServiceStore
	incidentStore store.IncidentStore
	ssePublisher  *sse.DualPublisher
}

func NewStatusTracker(serviceStore store.ServiceStore, incidentStore store.IncidentStore, ssePublisher *sse.DualPublisher) *StatusTracker {
	return &StatusTracker{
		serviceStore:  serviceStore,
		incidentStore: incidentStore,
		ssePublisher:  ssePublisher,
	}
}

func priorityWeight(priority string) float64 {
	switch priority {
	case "P1":
		return 5
	case "P2":
		return 4
	case "P3":
		return 3
	case "P4":
		return 2
	case "P5":
		return 1
	default:
		return 1
	}
}

func scoreToStatus(score float64) string {
	switch {
	case score == 0:
		return "operational"
	case score <= 4:
		return "degraded"
	case score <= 9:
		return "partial_outage"
	default:
		return "major_outage"
	}
}

func (t *StatusTracker) calculateWeightedStatus(ctx context.Context, serviceID string) (string, error) {
	priorities, err := t.incidentStore.CountActiveByPriority(ctx, serviceID)
	if err != nil {
		return "", fmt.Errorf("count active incidents by priority: %w", err)
	}

	var score float64
	for p, count := range priorities {
		score += priorityWeight(p) * float64(count)
	}

	return scoreToStatus(score), nil
}

func (t *StatusTracker) PropagateAndCascade(ctx context.Context, serviceID string) error {
	visited := map[string]bool{}
	queue := []string{serviceID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current, err := t.serviceStore.GetService(ctx, currentID)
		if err != nil {
			logger.WarnCtx(ctx, "cascade: failed to get service", "component", "servicetracker", "service_id", currentID, "error", err)
			continue
		}
		if current == nil {
			continue
		}

		newStatus, err := t.calculateWeightedStatus(ctx, currentID)
		if err != nil {
			logger.WarnCtx(ctx, "cascade: failed to calculate status", "component", "servicetracker", "service_id", currentID, "error", err)
			continue
		}

		if current.Status == newStatus {
			continue
		}

		if err := t.serviceStore.UpdateServiceStatus(ctx, currentID, newStatus); err != nil {
			logger.WarnCtx(ctx, "cascade: failed to update status", "component", "servicetracker", "service_id", currentID, "error", err)
			continue
		}

		logger.InfoCtx(ctx, "service status changed", "component", "servicetracker",
			"service_id", currentID, "from", current.Status, "to", newStatus)

		if t.ssePublisher != nil {
			t.ssePublisher.Publish(sse.Event{
				Type: "service_status_changed",
				Data: map[string]any{
					"service_id": currentID,
					"old_status": current.Status,
					"new_status": newStatus,
				},
			})
		}

		depID, parseErr := uuid.Parse(currentID)
		if parseErr != nil {
			continue
		}

		dependents, err := t.serviceStore.GetDependents(ctx, depID)
		if err != nil {
			logger.WarnCtx(ctx, "cascade: failed to get dependents", "component", "servicetracker", "service_id", currentID, "error", err)
			continue
		}

		for _, dep := range dependents {
			queue = append(queue, dep.ServiceID.String())
		}
	}

	return nil
}
