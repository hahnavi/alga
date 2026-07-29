// alert_investigation.go defines the AlertInvestigationStore interface,
// record/event/summary types, the pgAlertInvestigationStore
// implementation, and the core create/get/list operations plus the
// standalone edge-creator helpers used across the package.
package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
	"alga/rabbitmq"
)

type AlertInvestigationRecord struct {
	ID                              uuid.UUID                         `json:"id"`
	AlertInvestigationID            string                            `json:"alert_investigation_id"`
	Alerts                          []rabbitmq.CorrelatedAlert        `json:"alerts"`
	CorrelationKey                  string                            `json:"correlation_key"`
	Status                          string                            `json:"status"`
	AgentID                         string                            `json:"agent_id,omitempty"`
	AgentName                       string                            `json:"agent_name,omitempty"`
	AgentType                       string                            `json:"agent_type,omitempty"`
	PrimaryThreadID                 string                            `json:"primary_thread_id,omitempty"`
	SlackChannelID                  string                            `json:"slack_channel_id,omitempty"`
	SlackThreadTS                   string                            `json:"slack_thread_ts,omitempty"`
	MMPostID                        string                            `json:"mm_post_id,omitempty"`
	MMThreadID                      string                            `json:"mm_thread_id,omitempty"`
	PromotedIncidentID              *uuid.UUID                        `json:"promoted_incident_id,omitempty"`
	PromotedIncidentInvestigationID *uuid.UUID                        `json:"promoted_incident_investigation_id,omitempty"`
	Summary                         *models.AlertInvestigationSummary `json:"summary,omitempty"`
	Findings                        []models.InvestigationFinding     `json:"findings,omitempty"`
	Evidence                        []models.EvidenceItem             `json:"evidence,omitempty"`
	PrimaryAlertFingerprint         string                            `json:"primary_alert_fingerprint,omitempty"`
	PrimaryAlertNumber              int64                             `json:"primary_alert_number,omitempty"`
	EscalationLevel                 string                            `json:"escalation_level,omitempty"`
	TriageResultID                  *uuid.UUID                        `json:"triage_result_id,omitempty"`
	TriageDecision                  string                            `json:"triage_decision,omitempty"`
	TriageEnrichment                map[string]any                    `json:"triage_enrichment,omitempty"`
	AssigneeType                    string                            `json:"assignee_type,omitempty"`
	AssigneeID                      *uuid.UUID                        `json:"assignee_id,omitempty"`
	Updates                         []InvestigationUpdate             `json:"updates"`
	CreatedAt                       time.Time                         `json:"created_at"`
	UpdatedAt                       time.Time                         `json:"updated_at"`
	CompletedAt                     *time.Time                        `json:"completed_at,omitempty"`
	CompletedReason                 string                            `json:"completed_reason,omitempty"`
	CompletedByType                 string                            `json:"completed_by_type,omitempty"`
	CompletedByID                   string                            `json:"completed_by_id,omitempty"`
	CompletedByName                 string                            `json:"completed_by_name,omitempty"`
	StartedAt                       *time.Time                        `json:"started_at,omitempty"`
	InvestigatingDurationMs         int64                             `json:"investigating_duration_ms"`
	Events                          []AlertInvestigationEvent         `json:"events,omitempty"`
}

type AlertInvestigationEvent struct {
	ID                   uuid.UUID      `json:"id"`
	AlertInvestigationID uuid.UUID      `json:"alert_investigation_id"`
	EventType            string         `json:"event_type"`
	Reason               string         `json:"reason,omitempty"`
	ActorType            string         `json:"actor_type,omitempty"`
	ActorID              string         `json:"actor_id,omitempty"`
	ActorName            string         `json:"actor_name,omitempty"`
	AgentID              string         `json:"agent_id,omitempty"`
	AgentName            string         `json:"agent_name,omitempty"`
	AgentType            string         `json:"agent_type,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
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

func newPGAlertInvestigationStore(db *bun.DB) AlertInvestigationStore {
	return &pgAlertInvestigationStore{pgStoreBase{db: db}}
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
	if record.AssigneeType == "" {
		record.AssigneeType = "agent"
	}
	if record.AlertInvestigationID == "" {
		record.AlertInvestigationID = uuid.NewString()
	}

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		m := &models.AlertInvestigation{
			AlertInvestigationID:            record.AlertInvestigationID,
			CorrelationKey:                  record.CorrelationKey,
			Status:                          record.Status,
			AgentID:                         record.AgentID,
			AgentName:                       record.AgentName,
			AgentType:                       record.AgentType,
			PrimaryThreadID:                 record.PrimaryThreadID,
			SlackChannelID:                  record.SlackChannelID,
			SlackThreadTS:                   record.SlackThreadTS,
			MMPostID:                        record.MMPostID,
			MMThreadID:                      record.MMThreadID,
			PrimaryAlertFingerprint:         record.PrimaryAlertFingerprint,
			PrimaryAlertNumber:              record.PrimaryAlertNumber,
			EscalationLevel:                 record.EscalationLevel,
			TriageDecision:                  record.TriageDecision,
			AssigneeType:                    record.AssigneeType,
			InvestigatingDurationMs:         record.InvestigatingDurationMs,
			PromotedIncidentID:              record.PromotedIncidentID,
			PromotedIncidentInvestigationID: record.PromotedIncidentInvestigationID,
			Summary:                         record.Summary,
			Findings:                        record.Findings,
			Evidence:                        record.Evidence,
			StartedAt:                       record.StartedAt,
			CompletedAt:                     record.CompletedAt,
			TriageResultID:                  record.TriageResultID,
			TriageEnrichment:                record.TriageEnrichment,
			AssigneeID:                      record.AssigneeID,
		}
		m.ID = models.NewUUID()
		m.CreatedAt = now
		m.UpdatedAt = now

		if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
			return fmt.Errorf("failed to insert alert investigation: %w", err)
		}

		if err := retireCurrentAlertInvestigationLinks(ctx, tx, record.Alerts); err != nil {
			return err
		}
		for _, alert := range record.Alerts {
			if err := createAlertInvestigationAlert(ctx, tx, m.ID, alert); err != nil {
				return err
			}
		}
		for _, update := range record.Updates {
			if err := createAlertInvestigationUpdate(ctx, tx, m.ID, update); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetAlertInvestigation(ctx, record.AlertInvestigationID)
}

func (s *pgAlertInvestigationStore) GetAlertInvestigation(ctx context.Context, id string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.AlertInvestigation
	err := s.db.NewSelect().Model(&inv).Where("alert_investigation_id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*AlertInvestigationRecord](err, "alert investigation")
	}
	return s.toAlertInvestigationRecord(ctx, &inv)
}

func (s *pgAlertInvestigationStore) toAlertInvestigationRecord(ctx context.Context, inv *models.AlertInvestigation) (*AlertInvestigationRecord, error) {
	var alerts []models.AlertInvestigationAlert
	if err := s.db.NewSelect().Model(&alerts).
		Where("alert_investigation_id = ?", inv.ID).
		Order("id ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query alert investigation alerts: %w", err)
	}

	var updates []models.AlertInvestigationUpdate
	if err := s.db.NewSelect().Model(&updates).
		Where("alert_investigation_id = ?", inv.ID).
		Order("created_at ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query alert investigation updates: %w", err)
	}

	var events []models.AlertInvestigationEvent
	if err := s.db.NewSelect().Model(&events).
		Where("alert_investigation_id = ?", inv.ID).
		Order("created_at ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query alert investigation events: %w", err)
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
			MMPostID:       update.MMPostID,
			SlackMessageTS: update.SlackMessageTS,
			QuotedUpdateID: update.QuotedUpdateID,
			Mentions:       update.Mentions,
			CreatedAt:      update.CreatedAt,
		})
	}

	investigationEvents := make([]AlertInvestigationEvent, 0, len(events))
	for _, event := range events {
		investigationEvents = append(investigationEvents, AlertInvestigationEvent{
			ID:                   event.ID,
			AlertInvestigationID: event.AlertInvestigationID,
			EventType:            event.EventType,
			Reason:               event.Reason,
			ActorType:            event.ActorType,
			ActorID:              event.ActorID,
			ActorName:            event.ActorName,
			AgentID:              event.AgentID,
			AgentName:            event.AgentName,
			AgentType:            event.AgentType,
			Metadata:             event.Metadata,
			CreatedAt:            event.CreatedAt,
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
		SlackThreadTS:                   inv.SlackThreadTS,
		MMPostID:                        inv.MMPostID,
		MMThreadID:                      inv.MMThreadID,
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

func createAlertInvestigationAlert(ctx context.Context, db bun.IDB, alertInvestigationID uuid.UUID, alert rabbitmq.CorrelatedAlert) error {
	alertname := alert.Labels["alertname"]
	namespace := alert.Labels["namespace"]
	summary := alert.Annotations["summary"]

	m := &models.AlertInvestigationAlert{
		AlertInvestigationID: alertInvestigationID,
		Fingerprint:          alert.Fingerprint,
		AlertNumber:          alert.AlertNumber,
		Status:               alert.Status,
		Alertname:            alertname,
		Namespace:            namespace,
		Labels:               alert.Labels,
		Annotations:          alert.Annotations,
		GeneratorURL:         alert.GeneratorURL,
		Summary:              summary,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = time.Now().UTC()
	m.UpdatedAt = time.Now().UTC()

	if alert.StartsAt != "" {
		if startsAt, err := time.Parse(time.RFC3339, alert.StartsAt); err == nil {
			m.StartsAt = &startsAt
		}
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return fmt.Errorf("failed to create alert investigation alert for fingerprint %s: %w", alert.Fingerprint, err)
	}
	return nil
}

func createAlertInvestigationEvent(ctx context.Context, db bun.IDB, alertInvestigationID uuid.UUID, event AlertInvestigationEvent) error {
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	m := &models.AlertInvestigationEvent{
		AlertInvestigationID: alertInvestigationID,
		EventType:            event.EventType,
		Reason:               event.Reason,
		ActorType:            event.ActorType,
		ActorID:              event.ActorID,
		ActorName:            event.ActorName,
		AgentID:              event.AgentID,
		AgentName:            event.AgentName,
		AgentType:            event.AgentType,
		Metadata:             event.Metadata,
	}
	if event.ID != uuid.Nil {
		m.ID = event.ID
	} else {
		m.ID = models.NewUUID()
	}
	m.CreatedAt = createdAt
	m.UpdatedAt = createdAt

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return fmt.Errorf("failed to create alert investigation event: %w", err)
	}
	return nil
}

func createAlertInvestigationUpdate(ctx context.Context, db bun.IDB, alertInvestigationID uuid.UUID, update InvestigationUpdate) error {
	createdAt := update.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	m := &models.AlertInvestigationUpdate{
		AlertInvestigationID: alertInvestigationID,
		Type:                 string(update.Type),
		Message:              update.Message,
		Source:               string(update.Source),
		Internal:             update.Internal,
		Edited:               update.Edited,
		UserID:               update.UserID,
		Username:             update.Username,
		MMPostID:             update.MMPostID,
		SlackMessageTS:       update.SlackMessageTS,
		QuotedUpdateID:       update.QuotedUpdateID,
		Mentions:             update.Mentions,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = createdAt
	m.UpdatedAt = createdAt

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return fmt.Errorf("failed to create alert investigation update: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) ListAlertInvestigations(ctx context.Context, filter map[string]any) ([]AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	q := s.db.NewSelect().Model((*models.AlertInvestigation)(nil))

	if v, ok := filter["status"].(string); ok && v != "" {
		q = q.Where("status = ?", v)
	}
	if v, ok := filter["status_in"].([]string); ok && len(v) > 0 {
		q = q.Where("status IN (?)", bun.List(v))
	}
	if v, ok := filter["correlation_key"].(string); ok && v != "" {
		q = q.Where("correlation_key = ?", v)
	}
	if v, ok := filter["promoted_incident_id"].(string); ok && v != "" {
		if u, err := uuid.Parse(v); err == nil {
			q = q.Where("promoted_incident_id = ?", u)
		}
	} else if v, ok := filter["promoted_incident_id"].(uuid.UUID); ok {
		q = q.Where("promoted_incident_id = ?", v)
	}

	q = q.Order("created_at DESC")

	if v, ok := filter["limit"].(int); ok && v > 0 {
		q = q.Limit(v)
	} else {
		q = q.Limit(100)
	}
	if v, ok := filter["skip"].(int); ok && v > 0 {
		q = q.Offset(v)
	}

	var invs []models.AlertInvestigation
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list alert investigations: %w", err)
	}

	records := make([]AlertInvestigationRecord, 0, len(invs))
	for i := range invs {
		rec, err := s.toAlertInvestigationRecord(ctx, &invs[i])
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

	var links []models.AlertInvestigationAlert
	if err := s.db.NewSelect().Model(&links).
		Where("alert_number = ?", alertNumber).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query alert investigation alerts by alert_number: %w", err)
	}
	if len(links) == 0 {
		return []AlertInvestigationRecord{}, nil
	}

	seen := make(map[uuid.UUID]struct{}, len(links))
	records := make([]AlertInvestigationRecord, 0, len(links))
	for _, link := range links {
		if _, ok := seen[link.AlertInvestigationID]; ok {
			continue
		}
		seen[link.AlertInvestigationID] = struct{}{}

		var inv models.AlertInvestigation
		if err := s.db.NewSelect().Model(&inv).Where("id = ?", link.AlertInvestigationID).Scan(ctx); err != nil {
			continue
		}
		rec, err := s.toAlertInvestigationRecord(ctx, &inv)
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

	var link models.AlertInvestigationAlert
	err := s.db.NewSelect().Model(&link).
		Where("alert_number = ? AND current = true", alertNumber).
		Order("id DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("current alert investigation not found for alert_number %d: %w", alertNumber, ErrInvestigationNotFound)
	}

	var inv models.AlertInvestigation
	if err := s.db.NewSelect().Model(&inv).Where("id = ?", link.AlertInvestigationID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("current alert investigation not found for alert_number %d: %w", alertNumber, ErrInvestigationNotFound)
	}
	return s.toAlertInvestigationRecord(ctx, &inv)
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

	var links []models.AlertInvestigationAlert
	if err := s.db.NewSelect().Model(&links).
		Where("alert_number IN (?) AND current = true", bun.List(alertNumbers)).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query current alert investigations: %w", err)
	}

	for _, link := range links {
		if link.AlertNumber <= 0 {
			continue
		}
		var inv models.AlertInvestigation
		if err := s.db.NewSelect().Model(&inv).Where("id = ?", link.AlertInvestigationID).Scan(ctx); err != nil {
			continue
		}
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
			var inc models.Incident
			if err := s.db.NewSelect().Model(&inc).Where("id = ?", *inv.PromotedIncidentID).Scan(ctx); err == nil && inc.IncidentNumber > 0 {
				summary.PromotedIncidentNumber = inc.IncidentNumber
			}
		}
		// The (alert_number, current=true) partial unique index guarantees at
		// most one current row per alert_number, so a later row will never
		// overwrite the first. Still, last-write-wins keeps the map deterministic
		// if the index is ever relaxed.
		out[link.AlertNumber] = summary
	}
	return out, nil
}

func (s *pgAlertInvestigationStore) GetActiveAlertInvestigationByCorrelationKey(ctx context.Context, correlationKey string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.AlertInvestigation
	err := s.db.NewSelect().Model(&inv).
		Where("correlation_key = ?", correlationKey).
		Where("status IN (?)", bun.List([]string{
			AlertInvestigationStatusPending,
			AlertInvestigationStatusAssigned,
			AlertInvestigationStatusInvestigating,
		})).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*AlertInvestigationRecord](err, "alert investigation")
	}
	return s.toAlertInvestigationRecord(ctx, &inv)
}

func (s *pgAlertInvestigationStore) GetAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var links []models.AlertInvestigationAlert
	if err := s.db.NewSelect().Model(&links).
		Where("alert_number = ?", alertNumber).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query alert investigation alerts by alert_number: %w", err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("alert investigation not found for alert_number %d: %w", alertNumber, ErrInvestigationNotFound)
	}

	var inv models.AlertInvestigation
	if err := s.db.NewSelect().Model(&inv).Where("id = ?", links[0].AlertInvestigationID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("alert investigation not found for alert_number %d: %w", alertNumber, ErrInvestigationNotFound)
	}
	return s.toAlertInvestigationRecord(ctx, &inv)
}
