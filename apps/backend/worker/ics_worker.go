package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/ics"
	"alga/logger"
	"alga/rabbitmq"
	"alga/sse"
	"alga/valkey"
)

const maxICSRetries = 3

type ICSWorker struct {
	provisioner  *ics.WarRoomProvisioner
	publisher    *rabbitmq.Publisher
	ssePublisher ssePublisher
	vkClient     *valkey.Client
}

func NewICSWorker(provisioner *ics.WarRoomProvisioner, publisher *rabbitmq.Publisher) *ICSWorker {
	return &ICSWorker{provisioner: provisioner, publisher: publisher}
}

// SetSSEPublisher wires the realtime publisher used to announce freshly
// provisioned war rooms to frontend dashboards.
func (w *ICSWorker) SetSSEPublisher(p ssePublisher) { w.ssePublisher = p }

// SetValkeyClient wires the Valkey client used to dedupe war_room_created
// events across retries and duplicate provision deliveries.
func (w *ICSWorker) SetValkeyClient(c *valkey.Client) { w.vkClient = c }

func (w *ICSWorker) Queue() string {
	return rabbitmq.QueueICSProvision
}

func (w *ICSWorker) PrefetchCount() int {
	return 1
}

func (w *ICSWorker) Handle(ctx context.Context, d amqp.Delivery) {
	var msg rabbitmq.ICSProvisionMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Failed to unmarshal ICS provision message", "component", "ics-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	logger.Info("Provisioning war room", "component", "ics-worker", "incident_number", msg.IncidentNumber)

	if err := w.provisioner.ProvisionWarRoom(ctx, msg.IncidentNumber); err != nil {
		logger.Error("Failed to provision war room", "component", "ics-worker", "incident_number", msg.IncidentNumber, "error", err)
		msg.RetryCount++
		if msg.RetryCount <= maxICSRetries && w.publisher != nil {
			if pubErr := w.publisher.PublishICSProvision(ctx, msg); pubErr != nil {
				logger.Error("Failed to publish ICS retry; dead-lettering", "component", "ics-worker", "error", pubErr)
				_ = d.Nack(false, false)
				return
			}
			_ = d.Ack(false)
			return
		}
		_ = d.Nack(false, false)
		return
	}

	w.notifyWarRoomCreated(ctx, msg.IncidentNumber)
	_ = d.Ack(false)
}

// notifyWarRoomCreated publishes a global war_room_created SSE event so open
// incident dashboards reload the just-provisioned war room. Emission is
// deduped in Valkey so retried or duplicate provision deliveries announce
// once; Valkey failure degrades fail-open like sibling sweeps.
func (w *ICSWorker) notifyWarRoomCreated(ctx context.Context, incidentNumber int64) {
	if w.ssePublisher == nil {
		return
	}
	if !w.markWarRoomEventDeduped(ctx, incidentNumber) {
		return
	}
	w.ssePublisher.Publish(sse.Event{
		Type: "war_room_created",
		Data: map[string]any{"incident_number": incidentNumber},
	})
}

func (w *ICSWorker) markWarRoomEventDeduped(ctx context.Context, incidentNumber int64) bool {
	if w.vkClient == nil {
		return true
	}
	key := fmt.Sprintf("alga:warroom:created:%d", incidentNumber)
	ok, err := w.vkClient.SetNX(ctx, key, "1", 24*time.Hour)
	if err != nil {
		logger.Warn("war_room_created dedup SETNX failed", "component", "ics-worker", "key", key, "error", err)
		return true
	}
	return ok
}
