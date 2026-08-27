package rabbitmq

import (
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/types"
)

// InvestigationKind distinguishes alert investigations from incident investigations.
type InvestigationKind string

const (
	InvestigationKindAlert    InvestigationKind = "alert"
	InvestigationKindIncident InvestigationKind = "incident"
)

func (k InvestigationKind) Valid() bool {
	return k == InvestigationKindAlert || k == InvestigationKindIncident
}

// AlertMessage wraps the full Grafana payload for async processing.
type AlertMessage struct {
	EventEnvelope
	Payload    types.GrafanaAlertingPayload `json:"payload"`
	ReceivedAt time.Time                    `json:"received_at"`
	RetryCount int                          `json:"retry_count"`
}

// InvestigateMessage is published when correlated alerts need AI investigation.
type InvestigateMessage struct {
	EventEnvelope
	InvestigationID         string            `json:"investigation_id"`
	InvestigationKind       InvestigationKind `json:"investigation_kind"`
	Alerts                  []CorrelatedAlert `json:"alerts"`
	Severity                string            `json:"severity"`
	CorrelationKey          string            `json:"correlation_key"`
	RetryCount              int               `json:"retry_count"`
	TraceID                 string            `json:"trace_id,omitempty"`
	DedupeKey               string            `json:"dedupe_key,omitempty"`
	IncidentNumber          int64             `json:"incident_number,omitempty"`
	TriageResultID          string            `json:"triage_result_id,omitempty"`
	TriageDecision          string            `json:"triage_decision,omitempty"`
	TriageSeverity          string            `json:"triage_severity,omitempty"`
	TriageCategory          string            `json:"triage_category,omitempty"`
	TriageEnrichment        TriageEnrichment  `json:"triage_enrichment,omitempty"`
	TriageReasoning         string            `json:"triage_reasoning,omitempty"`
	TriageConfidence        float64           `json:"triage_confidence,omitempty"`
	PrimaryAlertFingerprint string            `json:"primary_alert_fingerprint,omitempty"`
	PrimaryAlertNumber      int64             `json:"primary_alert_number,omitempty"`
}

// EmailMessage is published when an email needs to be sent.
type EmailMessage struct {
	EventEnvelope
	To         string `json:"to"`
	Subject    string `json:"subject"`
	TextBody   string `json:"text_body"`
	HtmlBody   string `json:"html_body,omitempty"`
	RetryCount int    `json:"retry_count"`
}

// TriageMessage is published when correlated alerts need enrichment before investigation.
type TriageMessage struct {
	EventEnvelope
	CorrelationKey  string            `json:"correlation_key"`
	Alerts          []CorrelatedAlert `json:"alerts"`
	Severity        string            `json:"severity"`
	RetryCount      int               `json:"retry_count"`
	TraceID         string            `json:"trace_id,omitempty"`
	DedupeKey       string            `json:"dedupe_key"`
	InvestigationID string            `json:"investigation_id,omitempty"`
}

// TriageEnrichment contains contextual data gathered during the triage phase.
type TriageEnrichment struct {
	ServiceOwner            string            `json:"service_owner,omitempty"`
	RunbookURL              string            `json:"runbook_url,omitempty"`
	PastRootCause           string            `json:"past_root_cause,omitempty"`
	PastResolution          string            `json:"past_resolution,omitempty"`
	SuggestedActions        []string          `json:"suggested_actions,omitempty"`
	SimilarInvestigationIDs []string          `json:"similar_investigation_ids,omitempty"`
	Custom                  map[string]string `json:"custom,omitempty"`
}

const MaxTriageRetries = 4

var triageRetryQueues = map[int]string{
	1: QueueTriageRetry1,
	2: QueueTriageRetry2,
	3: QueueTriageRetry3,
	4: QueueTriageRetry4,
}

// CorrelatedAlert represents a single alert within a correlated group.
type CorrelatedAlert struct {
	Fingerprint  string             `json:"fingerprint"`
	AlertNumber  int64              `json:"alert_number,omitempty"`
	Labels       map[string]string  `json:"labels"`
	Annotations  map[string]string  `json:"annotations"`
	Status       string             `json:"status"`
	StartsAt     string             `json:"starts_at"`
	Values       map[string]float64 `json:"values,omitempty"`
	GeneratorURL string             `json:"generator_url,omitempty"`
}

// Encode serializes a message to JSON bytes.
func Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Decode deserializes JSON bytes into v.
func Decode(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// PersistentPublishing creates a persistent AMQP publishing.
func PersistentPublishing(contentType string, body []byte) amqp.Publishing {
	return amqp.Publishing{
		ContentType:  contentType,
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}
}
