package worker

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/ics"
	"alga/logger"
	"alga/rabbitmq"
)

const maxICSRetries = 3

type ICSWorker struct {
	provisioner *ics.WarRoomProvisioner
	publisher   *rabbitmq.Publisher
}

func NewICSWorker(provisioner *ics.WarRoomProvisioner, publisher *rabbitmq.Publisher) *ICSWorker {
	return &ICSWorker{provisioner: provisioner, publisher: publisher}
}

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

	_ = d.Ack(false)
}
