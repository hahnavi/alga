package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type IncidentInvestigationRecord struct {
	ID                         uuid.UUID                     `json:"id"`
	IncidentInvestigationID    string                        `json:"incident_investigation_id"`
	IncidentNumber             int64                         `json:"incident_number,omitempty"`
	Status                     string                        `json:"status"`
	AgentID                    string                        `json:"agent_id,omitempty"`
	AgentName                  string                        `json:"agent_name,omitempty"`
	AgentType                  string                        `json:"agent_type,omitempty"`
	PrimaryThreadID            string                        `json:"primary_thread_id,omitempty"`
	SlackChannelID             string                        `json:"slack_channel_id,omitempty"`
	SlackThreadTS              string                        `json:"slack_thread_ts,omitempty"`
	MMPostID                   string                        `json:"mm_post_id,omitempty"`
	MMThreadID                 string                        `json:"mm_thread_id,omitempty"`
	SourceAlertInvestigationID *uuid.UUID                    `json:"source_alert_investigation_id,omitempty"`
	Summary                    *models.InvestigationSummary  `json:"summary,omitempty"`
	Findings                   []models.InvestigationFinding `json:"findings,omitempty"`
	Evidence                   []models.EvidenceItem         `json:"evidence,omitempty"`
	Updates                    []InvestigationUpdate         `json:"updates"`
	CreatedAt                  time.Time                     `json:"created_at"`
	UpdatedAt                  time.Time                     `json:"updated_at"`
	CompletedAt                *time.Time                    `json:"completed_at,omitempty"`
	StartedAt                  *time.Time                    `json:"started_at,omitempty"`
	InvestigatingDurationMs    int64                         `json:"investigating_duration_ms"`
	AssigneeType               string                        `json:"assignee_type,omitempty"`
	AssigneeID                 *uuid.UUID                    `json:"assignee_id,omitempty"`
}

type IncidentInvestigationStore interface {
	CreateIncidentInvestigation(ctx context.Context, record IncidentInvestigationRecord) (*IncidentInvestigationRecord, error)
	GetIncidentInvestigation(ctx context.Context, id string) (*IncidentInvestigationRecord, error)
	GetActiveIncidentInvestigationByIncident(ctx context.Context, incidentNumber int64) (*IncidentInvestigationRecord, error)
	ListIncidentInvestigationsByIncident(ctx context.Context, incidentNumber int64) ([]IncidentInvestigationRecord, error)
	AddIncidentInvestigationUpdate(ctx context.Context, id string, update InvestigationUpdate) error
	UpdateIncidentInvestigationStatus(ctx context.Context, id string, status string) error
	ClaimPendingIncidentInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*IncidentInvestigationRecord, error)
	ListPendingIncidentInvestigations(ctx context.Context, limit int64) ([]IncidentInvestigationRecord, error)
	SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *models.InvestigationSummary) error
	SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error
}

type pgIncidentInvestigationStore struct {
	pgStoreBase
}

func newPGIncidentInvestigationStore(db *bun.DB) IncidentInvestigationStore {
	return &pgIncidentInvestigationStore{pgStoreBase{db: db}}
}

func (s *pgIncidentInvestigationStore) CreateIncidentInvestigation(ctx context.Context, record IncidentInvestigationRecord) (*IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.Status == "" {
		record.Status = IncidentInvestigationStatusPending
	}
	if record.AssigneeType == "" {
		record.AssigneeType = "agent"
	}
	if record.IncidentInvestigationID == "" {
		record.IncidentInvestigationID = uuid.NewString()
	}

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if record.IncidentNumber != 0 {
			exists, err := tx.NewSelect().Model((*models.IncidentInvestigation)(nil)).
				Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", record.IncidentNumber).
				Where("status IN (?)", bun.List(activeIncidentInvestigationStatuses)).
				Exists(ctx)
			if err != nil {
				return fmt.Errorf("failed to check for active incident investigation: %w", err)
			}
			if exists {
				return ErrActiveIncidentInvestigationExists
			}
		}

		m := &models.IncidentInvestigation{
			IncidentInvestigationID:    record.IncidentInvestigationID,
			Status:                     record.Status,
			AgentID:                    record.AgentID,
			AgentName:                  record.AgentName,
			AgentType:                  record.AgentType,
			PrimaryThreadID:            record.PrimaryThreadID,
			SlackChannelID:             record.SlackChannelID,
			SlackThreadTs:              record.SlackThreadTS,
			MMPostID:                   record.MMPostID,
			MMThreadID:                 record.MMThreadID,
			SourceAlertInvestigationID: record.SourceAlertInvestigationID,
			Summary:                    record.Summary,
			Findings:                   record.Findings,
			Evidence:                   record.Evidence,
			StartedAt:                  record.StartedAt,
			CompletedAt:                record.CompletedAt,
			InvestigatingDurationMs:    record.InvestigatingDurationMs,
			AssigneeType:               record.AssigneeType,
			AssigneeID:                 record.AssigneeID,
		}
		m.ID = models.NewUUID()
		m.CreatedAt = now
		m.UpdatedAt = now

		if record.IncidentNumber != 0 {
			var inc models.Incident
			if err := tx.NewSelect().Model(&inc).
				Where("incident_number = ? AND deleted_at IS NULL", record.IncidentNumber).
				Scan(ctx); err != nil {
				if isNotFound(err) {
					return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
				}
				return fmt.Errorf("failed to find incident: %w", err)
			}
			m.IncidentID = &inc.ID
		}

		if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
			return fmt.Errorf("failed to insert incident investigation: %w", err)
		}

		for _, update := range record.Updates {
			if err := createIncidentInvestigationUpdate(ctx, tx, m.ID, update); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetIncidentInvestigation(ctx, record.IncidentInvestigationID)
}

func (s *pgIncidentInvestigationStore) GetIncidentInvestigation(ctx context.Context, id string) (*IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.IncidentInvestigation
	err := s.db.NewSelect().Model(&inv).Where("public_id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}
	return s.toIncidentInvestigationRecord(ctx, &inv)
}

func (s *pgIncidentInvestigationStore) GetActiveIncidentInvestigationByIncident(ctx context.Context, incidentNumber int64) (*IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.IncidentInvestigation
	err := s.db.NewSelect().Model(&inv).
		Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", incidentNumber).
		Where("status IN (?)", bun.List(activeIncidentInvestigationStatuses)).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}
	return s.toIncidentInvestigationRecord(ctx, &inv)
}

func (s *pgIncidentInvestigationStore) ListIncidentInvestigationsByIncident(ctx context.Context, incidentNumber int64) ([]IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var invs []models.IncidentInvestigation
	if err := s.db.NewSelect().Model(&invs).
		Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", incidentNumber).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query incident investigations: %w", err)
	}

	records := make([]IncidentInvestigationRecord, 0, len(invs))
	for i := range invs {
		rec, err := s.toIncidentInvestigationRecord(ctx, &invs[i])
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}

	slices.SortFunc(records, func(a, b IncidentInvestigationRecord) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return records, nil
}

func (s *pgIncidentInvestigationStore) AddIncidentInvestigationUpdate(ctx context.Context, id string, update InvestigationUpdate) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.IncidentInvestigation
	if err := s.db.NewSelect().Model(&inv).Where("public_id = ?", id).Scan(ctx); err != nil {
		return fmt.Errorf("incident investigation not found: %w", ErrInvestigationNotFound)
	}

	if err := createIncidentInvestigationUpdate(ctx, s.db, inv.ID, update); err != nil {
		return err
	}
	if _, err := s.db.NewUpdate().Model((*models.IncidentInvestigation)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", inv.ID).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to update incident investigation timestamp: %w", err)
	}
	return nil
}

func (s *pgIncidentInvestigationStore) UpdateIncidentInvestigationStatus(ctx context.Context, id string, status string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.IncidentInvestigation)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now().UTC()).
		Where("public_id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update incident investigation status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update incident investigation status: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("incident investigation not found: %w", ErrInvestigationNotFound)
	}
	return nil
}

// SetIncidentInvestigationSummary overwrites the summary of the investigation
// identified by its string IncidentInvestigationID. Used by the commander's
// synthesize_findings tool to record the synthesized conclusion.
func (s *pgIncidentInvestigationStore) SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *models.InvestigationSummary) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if summary == nil {
		summary = &models.InvestigationSummary{}
	}
	res, err := s.db.NewUpdate().Model((*models.IncidentInvestigation)(nil)).
		Set("summary = ?", summary).
		Set("updated_at = ?", time.Now().UTC()).
		Where("public_id = ?", incidentInvestigationID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update incident investigation summary: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update incident investigation summary: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("incident investigation not found: %w", ErrInvestigationNotFound)
	}
	return nil
}

func (s *pgIncidentInvestigationStore) ClaimPendingIncidentInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.IncidentInvestigation
	if err := s.db.NewSelect().Model(&inv).Where("public_id = ?", id).Scan(ctx); err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.IncidentInvestigation)(nil)).
		Set("status = ?", IncidentInvestigationStatusAssigned).
		Set("agent_id = ?", agentID).
		Set("agent_name = ?", agentName).
		Set("agent_type = ?", agentType).
		Set("started_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", inv.ID).
		Where("status = ?", IncidentInvestigationStatusPending).
		Exec(ctx)
	if err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}
	if n == 0 {
		return nil, fmt.Errorf("incident investigation not found: %w", ErrInvestigationNotFound)
	}
	return s.GetIncidentInvestigation(ctx, id)
}

func (s *pgIncidentInvestigationStore) ListPendingIncidentInvestigations(ctx context.Context, limit int64) ([]IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var invs []models.IncidentInvestigation
	if err := s.db.NewSelect().Model(&invs).
		Where("status = ?", IncidentInvestigationStatusPending).
		Order("created_at ASC").
		Limit(int(limit)).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list pending incident investigations: %w", err)
	}

	records := make([]IncidentInvestigationRecord, 0, len(invs))
	for i := range invs {
		rec, recErr := s.toIncidentInvestigationRecord(ctx, &invs[i])
		if recErr != nil {
			return nil, recErr
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgIncidentInvestigationStore) toIncidentInvestigationRecord(ctx context.Context, inv *models.IncidentInvestigation) (*IncidentInvestigationRecord, error) {
	var updates []models.IncidentInvestigationUpdate
	if err := s.db.NewSelect().Model(&updates).
		Where("incident_investigation_id = ?", inv.ID).
		Order("created_at ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to query incident investigation updates: %w", err)
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
			SlackMessageTS: update.SlackMessageTs,
			QuotedUpdateID: update.QuotedUpdateID,
			Mentions:       update.Mentions,
			CreatedAt:      update.CreatedAt,
		})
	}

	incidentNumber := int64(0)
	if inv.IncidentID != nil {
		var inc models.Incident
		if err := s.db.NewSelect().Model(&inc).Column("incident_number").Where("id = ?", *inv.IncidentID).Scan(ctx); err == nil {
			incidentNumber = inc.IncidentNumber
		}
	}

	return &IncidentInvestigationRecord{
		ID:                         inv.ID,
		IncidentInvestigationID:    inv.IncidentInvestigationID,
		IncidentNumber:             incidentNumber,
		Status:                     inv.Status,
		AgentID:                    inv.AgentID,
		AgentName:                  inv.AgentName,
		AgentType:                  inv.AgentType,
		PrimaryThreadID:            inv.PrimaryThreadID,
		SlackChannelID:             inv.SlackChannelID,
		SlackThreadTS:              inv.SlackThreadTs,
		MMPostID:                   inv.MMPostID,
		MMThreadID:                 inv.MMThreadID,
		SourceAlertInvestigationID: inv.SourceAlertInvestigationID,
		Summary:                    inv.Summary,
		Findings:                   inv.Findings,
		Evidence:                   inv.Evidence,
		Updates:                    investigationUpdates,
		CreatedAt:                  inv.CreatedAt,
		UpdatedAt:                  inv.UpdatedAt,
		CompletedAt:                inv.CompletedAt,
		StartedAt:                  inv.StartedAt,
		InvestigatingDurationMs:    inv.InvestigatingDurationMs,
		AssigneeType:               inv.AssigneeType,
		AssigneeID:                 inv.AssigneeID,
	}, nil
}

func createIncidentInvestigationUpdate(ctx context.Context, db bun.IDB, incidentInvestigationID uuid.UUID, update InvestigationUpdate) error {
	createdAt := update.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	m := &models.IncidentInvestigationUpdate{
		ID:                      models.NewUUID(),
		IncidentInvestigationID: incidentInvestigationID,
		Type:                    string(update.Type),
		Message:                 update.Message,
		Source:                  string(update.Source),
		Internal:                update.Internal,
		Edited:                  update.Edited,
		UserID:                  update.UserID,
		Username:                update.Username,
		MMPostID:                update.MMPostID,
		SlackMessageTs:          update.SlackMessageTS,
		QuotedUpdateID:          update.QuotedUpdateID,
		Mentions:                update.Mentions,
		CreatedAt:               createdAt,
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return fmt.Errorf("failed to create incident investigation update: %w", err)
	}
	return nil
}

func (s *pgIncidentInvestigationStore) SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	q := s.db.NewUpdate().Model((*models.IncidentInvestigation)(nil)).
		Set("assignee_type = ?", assigneeType).
		Set("updated_at = ?", time.Now().UTC()).
		Where("public_id = ?", id)

	if assigneeID != nil {
		q = q.Set("assignee_id = ?", *assigneeID)
	} else {
		q = q.Set("assignee_id = NULL")
	}

	if assigneeType != InvestigationActorAgent {
		q = q.Set("agent_id = ''").Set("agent_name = ''").Set("agent_type = ''")
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set incident investigation assignee: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to set incident investigation assignee: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("incident investigation not found: %w", ErrInvestigationNotFound)
	}
	return nil
}
