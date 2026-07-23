package rabbitmq

import (
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/google/uuid"
)

const MaxIncidentRetries = 4

var incidentRetryQueues = map[int]string{
	1: QueueIncidentRetry1,
	2: QueueIncidentRetry2,
	3: QueueIncidentRetry3,
	4: QueueIncidentRetry4,
}

type IncidentMessage struct {
	EventEnvelope
	DedupeKey       string            `json:"dedupe_key"`
	TraceID         string            `json:"trace_id"`
	InvestigationID string            `json:"investigation_id,omitempty"`
	Alerts          []CorrelatedAlert `json:"alerts"`
	CorrelationKey  string            `json:"correlation_key"`
	Severity        string            `json:"severity"`
	ImpactLevel     string            `json:"impact_level"`
	TriageResultID  *uuid.UUID        `json:"triage_result_id,omitempty"`
	TriageDecision  string            `json:"triage_decision"`
	ServiceID       *uuid.UUID        `json:"service_id,omitempty"`
	RetryCount      int               `json:"retry_count"`
	MaxRetries      int               `json:"max_retries"`
}

func UnmarshalIncidentMessage(d amqp.Delivery) (IncidentMessage, error) {
	var msg IncidentMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		return msg, err
	}
	return msg, nil
}
