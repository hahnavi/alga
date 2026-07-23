package rabbitmq

type ICSProvisionMessage struct {
	EventEnvelope
	IncidentNumber int64  `json:"incident_number"`
	TraceID        string `json:"trace_id,omitempty"`
	RetryCount     int    `json:"retry_count,omitempty"`
}
