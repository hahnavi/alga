package rabbitmq

import "github.com/google/uuid"

const MaxEscalationRetries = 4

var escalationRetryQueues = map[int]string{
	1: QueueEscalationRetry1,
	2: QueueEscalationRetry2,
	3: QueueEscalationRetry3,
	4: QueueEscalationRetry4,
}

type EscalationMessage struct {
	EventEnvelope
	IncidentNumber int64     `json:"incident_number"`
	PolicyID       uuid.UUID `json:"policy_id"`
	Level          int       `json:"level"`
	RetryCount     int       `json:"retry_count"`
	MaxRetries     int       `json:"max_retries"`
}

type SLASweepMessage struct {
	EventEnvelope
}
