// alert_investigation.go defines the AlertInvestigationStore interface,
// record/event/summary types, the pgAlertInvestigationStore
// implementation, and the core create/get/list operations plus the
// standalone ent edge-creator helpers used across the package.
package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/alertinvestigation"
	"alga/ent/alertinvestigationalert"
	"alga/ent/alertinvestigationevent"
	"alga/ent/alertinvestigationupdateentry"
	"alga/ent/predicate"
	entschema "alga/ent/schema"
	"alga/rabbitmq"
)

type AlertInvestigationRecord struct {
	ID                              uuid.UUID                            `json:"id"`
	AlertInvestigationID            string                               `json:"alert_investigation_id"`
	Alerts                          []rabbitmq.CorrelatedAlert           `json:"alerts"`
	CorrelationKey                  string                               `json:"correlation_key"`
	Status                          string                               `json:"status"`
	AgentID                         string                               `json:"agent_id,omitempty"`
	AgentName                       string                               `json:"agent_name,omitempty"`
	AgentType                       string                               `json:"agent_type,omitempty"`
	PrimaryThreadID                 string                               `json:"primary_thread_id,omitempty"`
	SlackChannelID                  string                               `json:"slack_channel_id,omitempty"`
	SlackThreadTS                   string                               `json:"slack_thread_ts,omitempty"`
	MMPostID                        string                               `json:"mm_post_id,omitempty"`
	MMThreadID                      string                               `json:"mm_thread_id,omitempty"`
	PromotedIncidentID              *uuid.UUID                           `json:"promoted_incident_id,omitempty"`
	PromotedIncidentInvestigationID *uuid.UUID                           `json:"promoted_incident_investigation_id,omitempty"`
	Summary                         *entschema.AlertInvestigationSummary `json:"summary,omitempty"`
	Findings                        []entschema.InvestigationFinding     `json:"findings,omitempty"`
	Evidence                        []entschema.EvidenceItem             `json:"evidence,omitempty"`
	PrimaryAlertFingerprint         string                               `json:"primary_alert_fingerprint,omitempty"`
	PrimaryAlertNumber              int64                                `json:"primary_alert_number,omitempty"`
	EscalationLevel                 string                               `json:"escalation_level,omitempty"`
	TriageResultID                  *uuid.UUID                           `json:"triage_result_id,omitempty"`
	TriageDecision                  string                               `json:"triage_decision,omitempty"`
	TriageEnrichment                map[string]any                       `json:"triage_enrichment,omitempty"`
	AssigneeType                    string                               `json:"assignee_type,omitempty"`
	AssigneeID                      *uuid.UUID                           `json:"assignee_id,omitempty"`
	Updates                         []InvestigationUpdate                `json:"updates"`
	CreatedAt                       time.Time                            `json:"created_at"`
	UpdatedAt                       time.Time                            `json:"updated_at"`
	CompletedAt                     *time.Time                           `json:"completed_at,omitempty"`
	CompletedReason                 string                               `json:"completed_reason,omitempty"`
	CompletedByType                 string                               `json:"completed_by_type,omitempty"`
	CompletedByID                   string                               `json:"completed_by_id,omitempty"`
	CompletedByName                 string                               `json:"completed_by_name,omitempty"`
	StartedAt                       *time.Time                           `json:"started_at,omitempty"`
	InvestigatingDurationMs         int64                                `json:"investigating_duration_ms"`
	Events                          []AlertInvestigationEvent            `json:"events,omitempty"`
}

type AlertInvestigationEvent struct {
	ID                     uuid.UUID      `json:"id"`
	AlertInvestigationUUID uuid.UUID      `json:"alert_investigation_uuid"`
	EventType              string         `json:"event_type"`
	Reason                 string         `json:"reason,omitempty"`
	ActorType              string         `json:"actor_type,omitempty"`
	ActorID                string         `json:"actor_id,omitempty"`
	ActorName              string         `json:"actor_name,omitempty"`
	AgentID                string         `json:"agent_id,omitempty"`
	AgentName              string         `json:"agent_name,omitempty"`
	AgentType              string         `json:"agent_type,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
}

// AlertInvestigationSummary is a slim, list-friendly view of the current
// alert investigation for an alert. Used to surface the assigned actor
// (agent) on alert list rows without loading the full investigation
// record (updates, events, evidence, etc).
type AlertInvestigationSummary struct {
	AlertInvestigationID   string `json:"alert_investigation_id"`
	Status                 string `json:"status"`
	AgentID                string `json:"agent_id,omitempty"`
	AgentName              string `json:"agent_name,omitempty"`
	AgentType              string `json:"agent_type,omitempty"`
	AssigneeType           string `json:"assignee_type,omitempty"`
	PromotedIncidentID     string `json:"promoted_incident_id,omitempty"`
	PromotedIncidentNumber int64  `json:"promoted_incident_number,omitempty"`
}

type AlertInvestigationCompletion struct {
	Reason      string
	ActorType   string
	ActorID     string
	ActorName   string
	EventReason string
}

type AlertInvestigationLifecycleCompletionRequest struct {
	AlertNumber int64
	Reason      string
	ActorType   string
	ActorID     string
	ActorName   string
}

type AlertInvestigationRequeue struct {
	EventType string
	Reason    string
	ActorType string
	ActorName string
	Metadata  map[string]any
}

type AlertInvestigationStore interface {
	CreateAlertInvestigation(ctx context.Context, record AlertInvestigationRecord) (*AlertInvestigationRecord, error)
	GetAlertInvestigation(ctx context.Context, id string) (*AlertInvestigationRecord, error)
	ListAlertInvestigationsByAlertNumber(ctx context.Context, alertNumber int64) ([]AlertInvestigationRecord, error)
	GetCurrentAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*AlertInvestigationRecord, error)
	GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx context.Context, alertNumbers []int64) (map[int64]AlertInvestigationSummary, error)
	GetActiveAlertInvestigationByCorrelationKey(ctx context.Context, correlationKey string) (*AlertInvestigationRecord, error)
	AppendAlertsToAlertInvestigation(ctx context.Context, id string, alerts []rabbitmq.CorrelatedAlert) error
	MarkAlertInvestigationAlertsCurrent(ctx context.Context, investigationID string, current bool) error
	AddAlertInvestigationUpdate(ctx context.Context, id string, update InvestigationUpdate) error
	AppendAlertInvestigationEvent(ctx context.Context, investigationUUID uuid.UUID, event AlertInvestigationEvent) error
	CompleteAlertInvestigation(ctx context.Context, id string, completion AlertInvestigationCompletion) error
	RequeueAlertInvestigation(ctx context.Context, id string, requeue AlertInvestigationRequeue) error
	MarkAlertInvestigationPromoted(ctx context.Context, id string, incidentID string, incidentNumber int64, incidentInvestigationID string) (*AlertInvestigationRecord, error)
	UpdateAlertInvestigationStatus(ctx context.Context, id string, status string) error
	GetAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*AlertInvestigationRecord, error)
	ListPendingAlertInvestigations(ctx context.Context, limit int64) ([]AlertInvestigationRecord, error)
	ClaimPendingAlertInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*AlertInvestigationRecord, error)
	TransitionAlertInvestigationStatus(ctx context.Context, id string, fromStatuses []string, toStatus string) error
	PatchAlertInvestigationOutcome(ctx context.Context, id string, rootCause *string, resolution *string) error
	UpdateAlertInvestigationAgent(ctx context.Context, id string, agentID string, agentName string, agentType string) error
	SetAlertInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error
	ResetInvestigatingByAgent(ctx context.Context, agentID string) error
	ResetAssignedByAgent(ctx context.Context, agentID string) error
	CountActiveByAgent(ctx context.Context, agentID string) (int, error)
	CountActiveByAgents(ctx context.Context, agentIDs []string) (map[string]int, error)
	DeleteAlertInvestigation(ctx context.Context, id string) error
	ListAlertInvestigations(ctx context.Context, filter map[string]any) ([]AlertInvestigationRecord, error)
	UpdateAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string, message string) error
	DeleteAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string) error
	SetAlertInvestigationUpdateMMPostID(ctx context.Context, investigationID string, updateID string, mmPostID string) error
	SetAlertInvestigationUpdateSlackMessageTS(ctx context.Context, investigationID string, updateID string, slackMessageTS string) error
	GetAlertInvestigationByMMThread(ctx context.Context, mmThreadID string) (*AlertInvestigationRecord, error)
	GetAlertInvestigationBySlackThread(ctx context.Context, channelID string, threadTS string) (*AlertInvestigationRecord, error)
	FindSimilarAlertInvestigations(ctx context.Context, q SimilarAlertInvestigationsQuery) ([]AlertInvestigationRecord, error)
	ListStalledAssignedAlertInvestigations(ctx context.Context, threshold time.Duration) ([]AlertInvestigationRecord, error)
	ListStalledInvestigatingAlertInvestigations(ctx context.Context, threshold time.Duration) ([]AlertInvestigationRecord, error)
	ResetStalledAssignedAlertInvestigations(timeout time.Duration) ([]string, error)
	ResetStalledInvestigatingAlertInvestigations(timeout time.Duration) ([]string, error)
}

type SimilarAlertInvestigationsQuery struct {
	CorrelationKey         string
	AlertName              string
	DiscriminatorLabels    map[string]string
	ExcludeInvestigationID string
	Limit                  int
}

func AllAlertInvestigationAlertsResolved(checker FingerprintChecker, inv *AlertInvestigationRecord) bool {
	if len(inv.Alerts) == 0 {
		return false
	}
	for _, a := range inv.Alerts {
		rec, err := checker.GetByFingerprint(a.Fingerprint)
		if err != nil {
			return false
		}
		if rec != nil && rec.Status != "resolved" {
			return false
		}
	}
	return true
}

type pgAlertInvestigationStore struct {
	pgStoreBase
}

func newPGAlertInvestigationStore(client *ent.Client) AlertInvestigationStore {
	return &pgAlertInvestigationStore{pgStoreBase{client: client}}
}

func (s *pgAlertInvestigationStore) CreateAlertInvestigation(ctx context.Context, record AlertInvestigationRecord) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.Status == "" {
		record.Status = AlertInvestigationStatusPending
	}
	if record.AlertInvestigationID == "" {
		record.AlertInvestigationID = uuid.NewString()
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin alert investigation transaction: %w", err)
	}
	defer rollbackTx(tx)

	b := tx.Client().AlertInvestigation.Create().
		SetAlertInvestigationID(record.AlertInvestigationID).
		SetCorrelationKey(record.CorrelationKey).
		SetStatus(record.Status).
		SetAgentID(record.AgentID).
		SetAgentName(record.AgentName).
		SetAgentType(record.AgentType).
		SetPrimaryThreadID(record.PrimaryThreadID).
		SetSlackChannelID(record.SlackChannelID).
		SetSlackThreadTs(record.SlackThreadTS).
		SetMmPostID(record.MMPostID).
		SetMmThreadID(record.MMThreadID).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetInvestigatingDurationMs(record.InvestigatingDurationMs).
		SetPrimaryAlertFingerprint(record.PrimaryAlertFingerprint).
		SetEscalationLevel(record.EscalationLevel).
		SetTriageDecision(record.TriageDecision).
		SetAssigneeType(record.AssigneeType)

	if record.PromotedIncidentID != nil {
		b.SetPromotedIncidentID(*record.PromotedIncidentID)
	}
	if record.PromotedIncidentInvestigationID != nil {
		b.SetPromotedIncidentInvestigationID(*record.PromotedIncidentInvestigationID)
	}
	if record.Summary != nil {
		b.SetSummary(record.Summary)
	}
	if record.Findings != nil {
		b.SetFindings(record.Findings)
	}
	if record.Evidence != nil {
		b.SetEvidence(record.Evidence)
	}
	if record.StartedAt != nil {
		b.SetStartedAt(*record.StartedAt)
	}
	if record.CompletedAt != nil {
		b.SetCompletedAt(*record.CompletedAt)
	}
	if record.PrimaryAlertNumber != 0 {
		b.SetPrimaryAlertNumber(record.PrimaryAlertNumber)
	}
	if record.TriageResultID != nil {
		b.SetTriageResultID(*record.TriageResultID)
	}
	if record.TriageEnrichment != nil {
		b.SetTriageEnrichment(record.TriageEnrichment)
	}
	if record.AssigneeID != nil {
		b.SetAssigneeID(*record.AssigneeID)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert alert investigation: %w", err)
	}

	if err := retireCurrentAlertInvestigationLinks(ctx, tx.Client(), record.Alerts); err != nil {
		return nil, err
	}
	for _, alert := range record.Alerts {
		if err := createAlertInvestigationAlert(ctx, tx.Client(), saved.ID, alert); err != nil {
			return nil, err
		}
	}
	for _, update := range record.Updates {
		if err := createAlertInvestigationUpdate(ctx, tx.Client(), saved.ID, update); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit alert investigation transaction: %w", err)
	}

	return s.GetAlertInvestigation(ctx, record.AlertInvestigationID)
}

func (s *pgAlertInvestigationStore) GetAlertInvestigation(ctx context.Context, id string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.getAlertInvestigationBy(ctx, alertinvestigation.AlertInvestigationID(id))
}

func (s *pgAlertInvestigationStore) getAlertInvestigationBy(ctx context.Context, preds ...predicate.AlertInvestigation) (*AlertInvestigationRecord, error) {
	inv, err := s.client.AlertInvestigation.Query().Where(preds...).Only(ctx)
	if err != nil {
		return handleQueryErr[*AlertInvestigationRecord](err, "alert investigation")
	}
	return s.toAlertInvestigationRecord(ctx, inv)
}

func (s *pgAlertInvestigationStore) toAlertInvestigationRecord(ctx context.Context, inv *ent.AlertInvestigation) (*AlertInvestigationRecord, error) {
	alerts := inv.Edges.Alerts
	if alerts == nil {
		var err error
		alerts, err = inv.QueryAlerts().All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query alert investigation alerts: %w", err)
		}
	}

	updates := inv.Edges.Updates
	if updates == nil {
		var err error
		updates, err = inv.QueryUpdates().Order(ent.Asc(alertinvestigationupdateentry.FieldCreatedAt)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query alert investigation updates: %w", err)
		}
	}

	events := inv.Edges.Events
	if events == nil {
		var err error
		events, err = inv.QueryEvents().Order(ent.Asc(alertinvestigationevent.FieldCreatedAt)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query alert investigation events: %w", err)
		}
	}

	correlatedAlerts := make([]rabbitmq.CorrelatedAlert, 0, len(alerts))
	for _, alert := range alerts {
		correlatedAlert := rabbitmq.CorrelatedAlert{
			Fingerprint:  alert.Fingerprint,
			AlertNumber:  alert.AlertNumber,
			Labels:       alert.Labels,
			Annotations:  alert.Annotations,
			Status:       alert.Status,
			GeneratorURL: alert.GeneratorURL,
		}
		if alert.StartsAt != nil {
			correlatedAlert.StartsAt = alert.StartsAt.Format(time.RFC3339)
		}
		correlatedAlerts = append(correlatedAlerts, correlatedAlert)
	}

	investigationUpdates := make([]InvestigationUpdate, 0, len(updates))
	for _, update := range updates {
		investigationUpdates = append(investigationUpdates, InvestigationUpdate{
			ID:             update.ID,
			Type:           InvestigationUpdateType(update.Type),
			Message:        update.Message,
			Source:         InvestigationUpdateSource(update.Source),
			Internal:       update.Internal,
			Edited:         update.Edited,
			UserID:         update.UserID,
			Username:       update.Username,
			MMPostID:       update.MmPostID,
			SlackMessageTS: update.SlackMessageTs,
			QuotedUpdateID: update.QuotedUpdateID,
			Mentions:       update.Mentions,
			CreatedAt:      update.CreatedAt,
		})
	}

	investigationEvents := make([]AlertInvestigationEvent, 0, len(events))
	for _, event := range events {
		investigationEvents = append(investigationEvents, AlertInvestigationEvent{
			ID:                     event.ID,
			AlertInvestigationUUID: event.AlertInvestigationUUID,
			EventType:              event.EventType,
			Reason:                 event.Reason,
			ActorType:              event.ActorType,
			ActorID:                event.ActorID,
			ActorName:              event.ActorName,
			AgentID:                event.AgentID,
			AgentName:              event.AgentName,
			AgentType:              event.AgentType,
			Metadata:               event.Metadata,
			CreatedAt:              event.CreatedAt,
		})
	}

	return &AlertInvestigationRecord{
		ID:                              inv.ID,
		AlertInvestigationID:            inv.AlertInvestigationID,
		Alerts:                          correlatedAlerts,
		CorrelationKey:                  inv.CorrelationKey,
		Status:                          inv.Status,
		AgentID:                         inv.AgentID,
		AgentName:                       inv.AgentName,
		AgentType:                       inv.AgentType,
		PrimaryThreadID:                 inv.PrimaryThreadID,
		SlackChannelID:                  inv.SlackChannelID,
		SlackThreadTS:                   inv.SlackThreadTs,
		MMPostID:                        inv.MmPostID,
		MMThreadID:                      inv.MmThreadID,
		PromotedIncidentID:              inv.PromotedIncidentID,
		PromotedIncidentInvestigationID: inv.PromotedIncidentInvestigationID,
		Summary:                         inv.Summary,
		Findings:                        inv.Findings,
		Evidence:                        inv.Evidence,
		PrimaryAlertFingerprint:         inv.PrimaryAlertFingerprint,
		PrimaryAlertNumber:              inv.PrimaryAlertNumber,
		EscalationLevel:                 inv.EscalationLevel,
		TriageResultID:                  inv.TriageResultID,
		TriageDecision:                  inv.TriageDecision,
		TriageEnrichment:                inv.TriageEnrichment,
		AssigneeType:                    inv.AssigneeType,
		AssigneeID:                      inv.AssigneeID,
		Updates:                         investigationUpdates,
		CreatedAt:                       inv.CreatedAt,
		UpdatedAt:                       inv.UpdatedAt,
		CompletedAt:                     inv.CompletedAt,
		CompletedReason:                 inv.CompletedReason,
		CompletedByType:                 inv.CompletedByType,
		CompletedByID:                   inv.CompletedByID,
		CompletedByName:                 inv.CompletedByName,
		StartedAt:                       inv.StartedAt,
		InvestigatingDurationMs:         inv.InvestigatingDurationMs,
		Events:                          investigationEvents,
	}, nil
}

func createAlertInvestigationAlert(ctx context.Context, client *ent.Client, alertInvestigationID uuid.UUID, alert rabbitmq.CorrelatedAlert) error {
	alertname := alert.Labels["alertname"]
	namespace := alert.Labels["namespace"]
	summary := alert.Annotations["summary"]

	b := client.AlertInvestigationAlert.Create().
		SetAlertInvestigationID(alertInvestigationID).
		SetFingerprint(alert.Fingerprint).
		SetAlertNumber(alert.AlertNumber).
		SetStatus(alert.Status).
		SetAlertname(alertname).
		SetNamespace(namespace).
		SetLabels(alert.Labels).
		SetAnnotations(alert.Annotations).
		SetGeneratorURL(alert.GeneratorURL).
		SetSummary(summary)
	if alert.StartsAt != "" {
		if startsAt, err := time.Parse(time.RFC3339, alert.StartsAt); err == nil {
			b.SetStartsAt(startsAt)
		}
	}

	if _, err := b.Save(ctx); err != nil {
		return fmt.Errorf("failed to create alert investigation alert for fingerprint %s: %w", alert.Fingerprint, err)
	}
	return nil
}

func createAlertInvestigationEvent(ctx context.Context, client *ent.Client, alertInvestigationID uuid.UUID, event AlertInvestigationEvent) error {
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	b := client.AlertInvestigationEvent.Create().
		SetAlertInvestigationID(alertInvestigationID).
		SetEventType(event.EventType).
		SetReason(event.Reason).
		SetActorType(event.ActorType).
		SetActorID(event.ActorID).
		SetActorName(event.ActorName).
		SetAgentID(event.AgentID).
		SetAgentName(event.AgentName).
		SetAgentType(event.AgentType).
		SetCreatedAt(createdAt)
	if event.ID != uuid.Nil {
		b.SetID(event.ID)
	}
	if event.Metadata != nil {
		b.SetMetadata(event.Metadata)
	}

	if _, err := b.Save(ctx); err != nil {
		return fmt.Errorf("failed to create alert investigation event: %w", err)
	}
	return nil
}

func createAlertInvestigationUpdate(ctx context.Context, client *ent.Client, alertInvestigationID uuid.UUID, update InvestigationUpdate) error {
	createdAt := update.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	b := client.AlertInvestigationUpdateEntry.Create().
		SetAlertInvestigationID(alertInvestigationID).
		SetType(string(update.Type)).
		SetMessage(update.Message).
		SetSource(string(update.Source)).
		SetInternal(update.Internal).
		SetEdited(update.Edited).
		SetCreatedAt(createdAt)

	if update.UserID != nil {
		b.SetUserID(*update.UserID)
	}
	if update.Username != nil {
		b.SetUsername(*update.Username)
	}
	if update.MMPostID != "" {
		b.SetMmPostID(update.MMPostID)
	}
	if update.SlackMessageTS != "" {
		b.SetSlackMessageTs(update.SlackMessageTS)
	}
	if update.QuotedUpdateID != nil {
		b.SetQuotedUpdateID(*update.QuotedUpdateID)
	}
	if update.Mentions != nil {
		b.SetMentions(update.Mentions)
	}

	if _, err := b.Save(ctx); err != nil {
		return fmt.Errorf("failed to create alert investigation update: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) ListAlertInvestigations(ctx context.Context, filter map[string]any) ([]AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var preds []predicate.AlertInvestigation
	if v, ok := filter["status"].(string); ok && v != "" {
		preds = append(preds, alertinvestigation.StatusEQ(v))
	}
	if v, ok := filter["status_in"].([]string); ok && len(v) > 0 {
		preds = append(preds, alertinvestigation.StatusIn(v...))
	}
	if v, ok := filter["correlation_key"].(string); ok && v != "" {
		preds = append(preds, alertinvestigation.CorrelationKeyEQ(v))
	}
	if v, ok := filter["promoted_incident_id"].(string); ok && v != "" {
		if u, err := uuid.Parse(v); err == nil {
			preds = append(preds, alertinvestigation.PromotedIncidentIDEQ(u))
		}
	} else if v, ok := filter["promoted_incident_id"].(uuid.UUID); ok {
		preds = append(preds, alertinvestigation.PromotedIncidentIDEQ(v))
	}

	q := s.client.AlertInvestigation.Query().Where(preds...).
		Order(ent.Desc(alertinvestigation.FieldCreatedAt))

	if v, ok := filter["limit"].(int); ok && v > 0 {
		q = q.Limit(v)
	} else {
		q = q.Limit(100)
	}
	if v, ok := filter["skip"].(int); ok && v > 0 {
		q = q.Offset(v)
	}

	invs, err := q.
		WithAlerts().
		WithUpdates(func(q *ent.AlertInvestigationUpdateEntryQuery) {
			q.Order(ent.Asc(alertinvestigationupdateentry.FieldCreatedAt))
		}).
		WithEvents(func(q *ent.AlertInvestigationEventQuery) { q.Order(ent.Asc(alertinvestigationevent.FieldCreatedAt)) }).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list alert investigations: %w", err)
	}

	records := make([]AlertInvestigationRecord, 0, len(invs))
	for _, inv := range invs {
		rec, err := s.toAlertInvestigationRecord(ctx, inv)
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgAlertInvestigationStore) ListAlertInvestigationsByAlertNumber(ctx context.Context, alertNumber int64) ([]AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	alerts, err := s.client.AlertInvestigationAlert.Query().
		Where(alertinvestigationalert.AlertNumber(alertNumber)).
		WithAlertInvestigation().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alert investigation alerts by alert_number: %w", err)
	}
	if len(alerts) == 0 {
		return []AlertInvestigationRecord{}, nil
	}

	seen := make(map[uuid.UUID]struct{}, len(alerts))
	records := make([]AlertInvestigationRecord, 0, len(alerts))
	for _, alert := range alerts {
		inv := alert.Edges.AlertInvestigation
		if inv == nil {
			continue
		}
		if _, ok := seen[inv.ID]; ok {
			continue
		}
		seen[inv.ID] = struct{}{}
		rec, err := s.toAlertInvestigationRecord(ctx, inv)
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}

	slices.SortFunc(records, func(a, b AlertInvestigationRecord) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return records, nil
}

func (s *pgAlertInvestigationStore) GetCurrentAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	alerts, err := s.client.AlertInvestigationAlert.Query().
		Where(
			alertinvestigationalert.AlertNumber(alertNumber),
			alertinvestigationalert.Current(true),
		).
		WithAlertInvestigation().
		Order(ent.Desc(alertinvestigationalert.FieldID)).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query current alert investigation alert by alert_number: %w", err)
	}
	if len(alerts) == 0 || alerts[0].Edges.AlertInvestigation == nil {
		return nil, fmt.Errorf("current alert investigation not found for alert_number %d: %w", alertNumber, ErrInvestigationNotFound)
	}
	return s.toAlertInvestigationRecord(ctx, alerts[0].Edges.AlertInvestigation)
}

// GetCurrentAlertInvestigationSummariesByAlertNumbers returns a slim
// investigation summary (assigned agent + status) for each alert number
// that has a current (= true) alert investigation. Alert numbers without
// a current investigation are simply absent from the result map; callers
// should treat a missing key as "no investigation".
//
// This is the batched companion to GetCurrentAlertInvestigationByAlertNumber,
// used to surface the assigned actor in alert list responses without an
// N+1 lookup per alert.
func (s *pgAlertInvestigationStore) GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx context.Context, alertNumbers []int64) (map[int64]AlertInvestigationSummary, error) {
	out := make(map[int64]AlertInvestigationSummary, len(alertNumbers))
	if len(alertNumbers) == 0 {
		return out, nil
	}

	ctx, cancel := pgctx(ctx)
	defer cancel()

	rows, err := s.client.AlertInvestigationAlert.Query().
		Where(
			alertinvestigationalert.AlertNumberIn(alertNumbers...),
			alertinvestigationalert.Current(true),
		).
		WithAlertInvestigation(func(q *ent.AlertInvestigationQuery) {
			q.WithPromotedIncident()
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query current alert investigations: %w", err)
	}

	for _, row := range rows {
		if row.AlertNumber <= 0 || row.Edges.AlertInvestigation == nil {
			continue
		}
		inv := row.Edges.AlertInvestigation
		summary := AlertInvestigationSummary{
			AlertInvestigationID: inv.AlertInvestigationID,
			Status:               inv.Status,
			AgentID:              inv.AgentID,
			AgentName:            inv.AgentName,
			AgentType:            inv.AgentType,
			AssigneeType:         inv.AssigneeType,
		}
		if inv.PromotedIncidentID != nil {
			summary.PromotedIncidentID = inv.PromotedIncidentID.String()
			if inc := inv.Edges.PromotedIncident; inc != nil && inc.IncidentNumber > 0 {
				summary.PromotedIncidentNumber = inc.IncidentNumber
			}
		}
		// The (alert_number, current=true) partial unique index guarantees at
		// most one current row per alert_number, so a later row will never
		// overwrite the first. Still, last-write-wins keeps the map deterministic
		// if the index is ever relaxed.
		out[row.AlertNumber] = summary
	}
	return out, nil
}

func (s *pgAlertInvestigationStore) GetActiveAlertInvestigationByCorrelationKey(ctx context.Context, correlationKey string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.getAlertInvestigationBy(ctx,
		alertinvestigation.CorrelationKey(correlationKey),
		alertinvestigation.StatusIn(
			AlertInvestigationStatusPending,
			AlertInvestigationStatusAssigned,
			AlertInvestigationStatusInvestigating,
		),
	)
}

func (s *pgAlertInvestigationStore) GetAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	alerts, err := s.client.AlertInvestigationAlert.Query().
		Where(alertinvestigationalert.AlertNumber(alertNumber)).
		WithAlertInvestigation().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alert investigation alerts by alert_number: %w", err)
	}
	if len(alerts) == 0 {
		return nil, fmt.Errorf("alert investigation not found for alert_number %d: %w", alertNumber, ErrInvestigationNotFound)
	}
	inv := alerts[0].Edges.AlertInvestigation
	if inv == nil {
		return nil, fmt.Errorf("alert investigation not found for alert_number %d: %w", alertNumber, ErrInvestigationNotFound)
	}
	return s.toAlertInvestigationRecord(ctx, inv)
}
