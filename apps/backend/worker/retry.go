package worker

import (
	"context"
	"errors"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/valkey"
)

func retryOrDeadLetter(
	ctx context.Context,
	vkClient *valkey.Client,
	dedupeKey string,
	retryFn func() error,
	d amqp.Delivery,
	entityType string,
	entityID string,
	stage string,
	cause error,
) {
	if retryFn == nil {
		logger.Error("Entity failed with no retry publisher; dead-lettering", "component", "retry-handler", "entity_type", entityType, "entity_id", entityID, "stage", stage, "error", cause)
		if entityType == "Investigation" {
			metrics.WorkerDLQTotal.Add(1)
		}
		_ = d.Nack(false, false)
		return
	}
	if err := retryFn(); err != nil {
		if errors.Is(err, rabbitmq.ErrMaxRetriesExceeded) {
			logger.Error("Entity exhausted retries", "component", "retry-handler", "entity_type", entityType, "entity_id", entityID, "stage", stage, "error", cause)
		} else {
			logger.Error("Entity retry publish failed; dead-lettering", "component", "retry-handler", "entity_type", entityType, "entity_id", entityID, "stage", stage, "error", err, "root_cause", cause)
		}
		if entityType == "Investigation" {
			metrics.WorkerDLQTotal.Add(1)
		}
		_ = d.Nack(false, false)
		return
	}
	_ = d.Ack(false)
	if vkClient != nil && dedupeKey != "" {
		_ = vkClient.Del(ctx, dedupeKey)
	}
}
