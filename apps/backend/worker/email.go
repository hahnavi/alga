package worker

import (
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/email"
	"alga/logger"
	"alga/rabbitmq"
)

const maxEmailRetries = 3

type EmailWorker struct {
	sender    *email.Sender
	publisher *rabbitmq.Publisher
}

func NewEmailWorker(sender *email.Sender, publisher *rabbitmq.Publisher) *EmailWorker {
	return &EmailWorker{sender: sender, publisher: publisher}
}

func (w *EmailWorker) Queue() string      { return rabbitmq.QueueEmailSend }
func (w *EmailWorker) PrefetchCount() int { return 10 }

func (w *EmailWorker) Handle(ctx context.Context, d amqp.Delivery) {
	if w.sender == nil || !w.sender.Enabled() {
		_ = d.Ack(false)
		return
	}

	var msg rabbitmq.EmailMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Email worker unmarshal failed", "component", "email-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := w.sender.Send(sendCtx, msg.To, msg.Subject, msg.TextBody, msg.HtmlBody); err != nil {
		logger.Error("Email send failed", "component", "email-worker", "to", msg.To, "error", err)
		w.retryOrDeadLetterEmail(ctx, d, msg)
		return
	}

	logger.Info("Email sent", "component", "email-worker", "to", msg.To, "subject", msg.Subject)
	_ = d.Ack(false)
}

func (w *EmailWorker) retryOrDeadLetterEmail(ctx context.Context, d amqp.Delivery, msg rabbitmq.EmailMessage) {
	if w.publisher == nil {
		_ = d.Nack(false, false)
		return
	}
	msg.RetryCount++
	if msg.RetryCount <= maxEmailRetries {
		timer := time.NewTimer(time.Duration(msg.RetryCount) * 5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = d.Nack(false, false)
			return
		case <-timer.C:
		}
		retryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := w.publisher.PublishEmail(retryCtx, msg); err != nil {
			logger.Error("Failed to publish email retry; nacking", "component", "email-worker", "error", err)
			_ = d.Nack(false, false)
			return
		}
		_ = d.Ack(false)
		return
	}
	logger.Error("Email exhausted retries; dead-lettering", "component", "email-worker", "to", msg.To, "subject", msg.Subject)
	_ = d.Nack(false, false)
}
