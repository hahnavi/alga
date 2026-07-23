package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/incident"
	"alga/ent/incidentinvestigation"
	"alga/ent/incidentinvestigationupdateentry"
	"alga/ent/predicate"
	entschema "alga/ent/schema"
)

type IncidentInvestigationRecord struct {
	ID                         uuid.UUID                        `json:"id"`
	IncidentInvestigationID    string                           `json:"incident_investigation_id"`
	IncidentNumber             int64                            `json:"incident_number,omitempty"`
	Status                     string                           `json:"status"`
	AgentID                    string                           `json:"agent_id,omitempty"`
	AgentName                  string                           `json:"agent_name,omitempty"`
	AgentType                  string                           `json:"agent_type,omitempty"`
	PrimaryThreadID            string                           `json:"primary_thread_id,omitempty"`
	SlackChannelID             string                           `json:"slack_channel_id,omitempty"`
	SlackThreadTS              string                           `json:"slack_thread_ts,omitempty"`
	MMPostID                   string                           `json:"mm_post_id,omitempty"`
	MMThreadID                 string                           `json:"mm_thread_id,omitempty"`
	SourceAlertInvestigationID *uuid.UUID                       `json:"source_alert_investigation_id,omitempty"`
	Summary                    *entschema.InvestigationSummary  `json:"summary,omitempty"`
	Findings                   []entschema.InvestigationFinding `json:"findings,omitempty"`
	Evidence                   []entschema.EvidenceItem         `json:"evidence,omitempty"`
	Updates                    []InvestigationUpdate            `json:"updates"`
	CreatedAt                  time.Time                        `json:"created_at"`
	UpdatedAt                  time.Time                        `json:"updated_at"`
	CompletedAt                *time.Time                       `json:"completed_at,omitempty"`
	StartedAt                  *time.Time                       `json:"started_at,omitempty"`
	InvestigatingDurationMs    int64                            `json:"investigating_duration_ms"`
	AssigneeType               string                           `json:"assignee_type,omitempty"`
	AssigneeID                 *uuid.UUID                       `json:"assignee_id,omitempty"`
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
	SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *entschema.InvestigationSummary) error
	SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error
}

type pgIncidentInvestigationStore struct {
	pgStoreBase
}

func newPGIncidentInvestigationStore(client *ent.Client) IncidentInvestigationStore {
	return &pgIncidentInvestigationStore{pgStoreBase{client: client}}
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
	if record.IncidentInvestigationID == "" {
		record.IncidentInvestigationID = uuid.NewString()
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin incident investigation transaction: %w", err)
	}
	defer rollbackTx(tx)

	if record.IncidentNumber != 0 {
		active, err := tx.Client().IncidentInvestigation.Query().
			Where(
				incidentinvestigation.HasIncidentWith(incident.IncidentNumber(record.IncidentNumber)),
				incidentinvestigation.StatusIn(activeIncidentInvestigationStatuses...),
			).
			Exist(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check for active incident investigation: %w", err)
		}
		if active {
			return nil, ErrActiveIncidentInvestigationExists
		}
	}

	b := tx.Client().IncidentInvestigation.Create().
		SetIncidentInvestigationID(record.IncidentInvestigationID).
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
		SetInvestigatingDurationMs(record.InvestigatingDurationMs)

	if record.IncidentNumber != 0 {
		inc, err := tx.Client().Incident.Query().
			Where(incident.IncidentNumber(record.IncidentNumber), incident.DeletedAtIsNil()).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
			}
			return nil, fmt.Errorf("failed to find incident: %w", err)
		}
		b.SetIncidentID(inc.ID)
	}
	if record.SourceAlertInvestigationID != nil {
		b.SetSourceAlertInvestigationID(*record.SourceAlertInvestigationID)
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
	if record.AssigneeType != "" {
		b.SetAssigneeType(record.AssigneeType)
	}
	if record.AssigneeID != nil {
		b.SetAssigneeID(*record.AssigneeID)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert incident investigation: %w", err)
	}

	for _, update := range record.Updates {
		if err := createIncidentInvestigationUpdate(ctx, tx.Client(), saved.ID, update); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit incident investigation transaction: %w", err)
	}

	return s.GetIncidentInvestigation(ctx, record.IncidentInvestigationID)
}

func (s *pgIncidentInvestigationStore) GetIncidentInvestigation(ctx context.Context, id string) (*IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.getIncidentInvestigationBy(ctx, incidentinvestigation.IncidentInvestigationID(id))
}

func (s *pgIncidentInvestigationStore) GetActiveIncidentInvestigationByIncident(ctx context.Context, incidentNumber int64) (*IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.getIncidentInvestigationBy(ctx,
		incidentinvestigation.HasIncidentWith(incident.IncidentNumber(incidentNumber)),
		incidentinvestigation.StatusIn(activeIncidentInvestigationStatuses...),
	)
}

func (s *pgIncidentInvestigationStore) ListIncidentInvestigationsByIncident(ctx context.Context, incidentNumber int64) ([]IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invs, err := s.client.IncidentInvestigation.Query().
		Where(incidentinvestigation.HasIncidentWith(incident.IncidentNumber(incidentNumber))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query incident investigations: %w", err)
	}

	records := make([]IncidentInvestigationRecord, 0, len(invs))
	for _, inv := range invs {
		rec, err := s.toIncidentInvestigationRecord(ctx, inv)
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

	inv, err := s.client.IncidentInvestigation.Query().
		Where(incidentinvestigation.IncidentInvestigationID(id)).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("incident investigation not found: %w", ErrInvestigationNotFound)
	}

	if err := createIncidentInvestigationUpdate(ctx, s.client, inv.ID, update); err != nil {
		return err
	}
	if _, err := s.client.IncidentInvestigation.UpdateOneID(inv.ID).SetUpdatedAt(time.Now().UTC()).Save(ctx); err != nil {
		return fmt.Errorf("failed to update incident investigation timestamp: %w", err)
	}
	return nil
}

func (s *pgIncidentInvestigationStore) UpdateIncidentInvestigationStatus(ctx context.Context, id string, status string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	n, err := s.client.IncidentInvestigation.Update().
		Where(incidentinvestigation.IncidentInvestigationID(id)).
		SetStatus(status).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
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
func (s *pgIncidentInvestigationStore) SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *entschema.InvestigationSummary) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if summary == nil {
		summary = &entschema.InvestigationSummary{}
	}
	n, err := s.client.IncidentInvestigation.Update().
		Where(incidentinvestigation.IncidentInvestigationID(incidentInvestigationID)).
		SetSummary(summary).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
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

	inv, err := s.client.IncidentInvestigation.Query().
		Where(incidentinvestigation.IncidentInvestigationID(id)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}
	now := time.Now().UTC()
	_, err = s.client.IncidentInvestigation.UpdateOneID(inv.ID).
		Where(incidentinvestigation.StatusEQ(IncidentInvestigationStatusPending)).
		SetStatus(IncidentInvestigationStatusAssigned).
		SetAgentID(agentID).
		SetAgentName(agentName).
		SetAgentType(agentType).
		SetStartedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}
	return s.GetIncidentInvestigation(ctx, id)
}

func (s *pgIncidentInvestigationStore) ListPendingIncidentInvestigations(ctx context.Context, limit int64) ([]IncidentInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invs, err := s.client.IncidentInvestigation.Query().
		Where(incidentinvestigation.StatusEQ(IncidentInvestigationStatusPending)).
		Order(ent.Asc(incidentinvestigation.FieldCreatedAt)).
		Limit(int(limit)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending incident investigations: %w", err)
	}

	records := make([]IncidentInvestigationRecord, 0, len(invs))
	for _, inv := range invs {
		rec, recErr := s.toIncidentInvestigationRecord(ctx, inv)
		if recErr != nil {
			return nil, recErr
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgIncidentInvestigationStore) getIncidentInvestigationBy(ctx context.Context, preds ...predicate.IncidentInvestigation) (*IncidentInvestigationRecord, error) {
	inv, err := s.client.IncidentInvestigation.Query().Where(preds...).Only(ctx)
	if err != nil {
		return handleQueryErr[*IncidentInvestigationRecord](err, "incident investigation")
	}
	return s.toIncidentInvestigationRecord(ctx, inv)
}

func (s *pgIncidentInvestigationStore) toIncidentInvestigationRecord(ctx context.Context, inv *ent.IncidentInvestigation) (*IncidentInvestigationRecord, error) {
	updates := inv.Edges.Updates
	if updates == nil {
		var err error
		updates, err = inv.QueryUpdates().Order(ent.Asc(incidentinvestigationupdateentry.FieldCreatedAt)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query incident investigation updates: %w", err)
		}
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

	incidentNumber := int64(0)
	if inv.Edges.Incident != nil {
		incidentNumber = inv.Edges.Incident.IncidentNumber
	} else if inv.IncidentID != nil {
		inc, err := inv.QueryIncident().Select(incident.FieldIncidentNumber).Only(ctx)
		if err == nil {
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
		MMPostID:                   inv.MmPostID,
		MMThreadID:                 inv.MmThreadID,
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

func createIncidentInvestigationUpdate(ctx context.Context, client *ent.Client, incidentInvestigationID uuid.UUID, update InvestigationUpdate) error {
	createdAt := update.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	b := client.IncidentInvestigationUpdateEntry.Create().
		SetIncidentInvestigationUUID(incidentInvestigationID).
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
		return fmt.Errorf("failed to create incident investigation update: %w", err)
	}
	return nil
}

func (s *pgIncidentInvestigationStore) SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	u := s.client.IncidentInvestigation.Update().
		Where(incidentinvestigation.IncidentInvestigationID(id)).
		SetAssigneeType(assigneeType).
		SetUpdatedAt(time.Now().UTC())

	if assigneeID != nil {
		u.SetAssigneeID(*assigneeID)
	} else {
		u.ClearAssigneeID()
	}

	if assigneeType != InvestigationActorAgent {
		u.SetAgentID("").SetAgentName("").SetAgentType("")
	}

	n, err := u.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set incident investigation assignee: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("incident investigation not found: %w", ErrInvestigationNotFound)
	}
	return nil
}
