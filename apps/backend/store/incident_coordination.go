package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

const (
	IncidentCoordinationKindChat                 = "chat"
	IncidentCoordinationKindSystem               = "system"
	IncidentCoordinationKindDecision             = "decision"
	IncidentCoordinationKindAction               = "action"
	IncidentCoordinationKindAgentReply           = "agent_reply"
	IncidentCoordinationKindInvestigationSummary = "investigation_summary"
	IncidentCoordinationKindStatusUpdate         = "status_update"

	IncidentCoordinationActorUser     = "user"
	IncidentCoordinationActorAgent    = "agent"
	IncidentCoordinationActorSystem   = "system"
	IncidentCoordinationActorExternal = "external"

	IncidentCoordinationSourceAlga       = "alga"
	IncidentCoordinationSourceSlack      = "slack"
	IncidentCoordinationSourceMattermost = "mattermost"
	IncidentCoordinationSourceAgent      = "agent"
	IncidentCoordinationSourceSystem     = "system"

	MetadataKeyCommsTask = "comms_task"
)

type IncidentCoordinationMessageRecord struct {
	ID                    uuid.UUID      `json:"id"`
	IncidentNumber        int64          `json:"incident_number"`
	Kind                  string         `json:"kind"`
	ActorType             string         `json:"actor_type"`
	ActorID               *uuid.UUID     `json:"actor_id,omitempty"`
	ActorDisplayName      string         `json:"actor_display_name"`
	Body                  string         `json:"body"`
	Internal              bool           `json:"internal"`
	Source                string         `json:"source"`
	SlackChannelID        string         `json:"slack_channel_id,omitempty"`
	SlackMessageTS        string         `json:"slack_message_ts,omitempty"`
	SlackThreadTS         string         `json:"slack_thread_ts,omitempty"`
	ProviderMessageID     string         `json:"provider_message_id,omitempty"`
	LinkedInvestigationID string         `json:"linked_investigation_id,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

type IncidentCoordinationStore interface {
	CreateMessage(ctx context.Context, record *IncidentCoordinationMessageRecord) (*IncidentCoordinationMessageRecord, error)
	ListMessages(ctx context.Context, incidentNumber int64, limit, skip int) ([]IncidentCoordinationMessageRecord, error)
	FindByProviderMessageID(ctx context.Context, providerMessageID string) (*IncidentCoordinationMessageRecord, error)
	SetSlackMessageTS(ctx context.Context, id uuid.UUID, channelID, messageTS, threadTS string) error
	ListMessagesByKind(ctx context.Context, incidentNumber int64, kind string, limit, skip int) ([]IncidentCoordinationMessageRecord, error)
	CreateStatusUpdate(ctx context.Context, incidentNumber int64, statusLevel string, body string, internal bool, actorID uuid.UUID, actorDisplayName string) (*IncidentCoordinationMessageRecord, error)
	NewestStatusUpdate(ctx context.Context, incidentNumber int64) (*IncidentCoordinationMessageRecord, error)
	// NewestAgentCoordinationReply returns the most recent coordination message
	// authored by an agent in the coordination thread (kind=agent_reply,
	// actor_type=agent). The SLA worker uses this — paired with NewestStatusUpdate
	// — to detect a stale public-update cadence even when no
	// `report_to_communicator` call was made (e.g. the responder's
	// `post_handoff` audience=commander handoff path).
	NewestAgentCoordinationReply(ctx context.Context, incidentNumber int64) (*IncidentCoordinationMessageRecord, error)
}

type pgIncidentCoordinationStore struct {
	pgStoreBase
}

func newPGIncidentCoordinationStore(db *bun.DB) IncidentCoordinationStore {
	return &pgIncidentCoordinationStore{pgStoreBase{db: db}}
}

func (s *pgIncidentCoordinationStore) CreateMessage(ctx context.Context, record *IncidentCoordinationMessageRecord) (*IncidentCoordinationMessageRecord, error) {
	var inc models.Incident
	if err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ? AND deleted_at IS NULL", record.IncidentNumber).
		Scan(ctx); err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident for coordination message: %w", err)
	}

	metadata := record.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	now := time.Now().UTC()
	m := &models.IncidentCoordinationMessage{
		IncidentID:            inc.ID,
		Kind:                  record.Kind,
		ActorType:             record.ActorType,
		ActorID:               record.ActorID,
		ActorDisplayName:      record.ActorDisplayName,
		Body:                  record.Body,
		Internal:              record.Internal,
		Source:                record.Source,
		SlackChannelID:        record.SlackChannelID,
		SlackMessageTs:        record.SlackMessageTS,
		SlackThreadTs:         record.SlackThreadTS,
		ProviderMessageID:     record.ProviderMessageID,
		LinkedInvestigationID: record.LinkedInvestigationID,
		Metadata:              metadata,
	}
	m.ID = models.NewUUID()
	if !record.CreatedAt.IsZero() {
		m.CreatedAt = record.CreatedAt
	} else {
		m.CreatedAt = now
	}
	if !record.UpdatedAt.IsZero() {
		m.UpdatedAt = record.UpdatedAt
	} else {
		m.UpdatedAt = now
	}

	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create incident coordination message: %w", err)
	}
	out := incidentCoordinationFromModel(m)
	out.IncidentNumber = record.IncidentNumber
	return out, nil
}

func (s *pgIncidentCoordinationStore) ListMessages(ctx context.Context, incidentNumber int64, limit, skip int) ([]IncidentCoordinationMessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	var items []models.IncidentCoordinationMessage
	if err := s.db.NewSelect().Model(&items).
		Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", incidentNumber).
		Where("kind != ?", IncidentCoordinationKindStatusUpdate).
		Order("created_at ASC").
		Limit(limit).
		Offset(skip).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list incident coordination messages: %w", err)
	}

	records := make([]IncidentCoordinationMessageRecord, 0, len(items))
	for i := range items {
		rec := incidentCoordinationFromModel(&items[i])
		rec.IncidentNumber = incidentNumber
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgIncidentCoordinationStore) ListMessagesByKind(ctx context.Context, incidentNumber int64, kind string, limit, skip int) ([]IncidentCoordinationMessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	var items []models.IncidentCoordinationMessage
	if err := s.db.NewSelect().Model(&items).
		Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", incidentNumber).
		Where("kind = ?", kind).
		Order("created_at DESC").
		Limit(limit).
		Offset(skip).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list incident coordination messages by kind: %w", err)
	}

	records := make([]IncidentCoordinationMessageRecord, 0, len(items))
	for i := range items {
		rec := incidentCoordinationFromModel(&items[i])
		rec.IncidentNumber = incidentNumber
		records = append(records, *rec)
	}
	return records, nil
}

var validStatusLevels = map[string]bool{
	"investigating": true,
	"identified":    true,
	"mitigated":     true,
	"monitoring":    true,
	"resolved":      true,
}

func (s *pgIncidentCoordinationStore) CreateStatusUpdate(ctx context.Context, incidentNumber int64, statusLevel string, body string, internal bool, actorID uuid.UUID, actorDisplayName string) (*IncidentCoordinationMessageRecord, error) {
	if !validStatusLevels[statusLevel] {
		return nil, fmt.Errorf("invalid status_level: %q", statusLevel)
	}
	return s.CreateMessage(ctx, &IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             IncidentCoordinationKindStatusUpdate,
		ActorType:        IncidentCoordinationActorUser,
		ActorID:          &actorID,
		ActorDisplayName: actorDisplayName,
		Body:             body,
		Internal:         internal,
		Source:           IncidentCoordinationSourceAlga,
		Metadata:         map[string]any{"status_level": statusLevel},
	})
}

func (s *pgIncidentCoordinationStore) FindByProviderMessageID(ctx context.Context, providerMessageID string) (*IncidentCoordinationMessageRecord, error) {
	if providerMessageID == "" {
		return nil, nil
	}

	var msg models.IncidentCoordinationMessage
	err := s.db.NewSelect().Model(&msg).
		Where("provider_message_id = ?", providerMessageID).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*IncidentCoordinationMessageRecord](err, "incident coordination message")
	}
	rec := incidentCoordinationFromModel(&msg)
	// Resolve incident number.
	var inc models.Incident
	if err := s.db.NewSelect().Model(&inc).Column("incident_number").Where("id = ?", msg.IncidentID).Scan(ctx); err == nil {
		rec.IncidentNumber = inc.IncidentNumber
	}
	return rec, nil
}

func (s *pgIncidentCoordinationStore) SetSlackMessageTS(ctx context.Context, id uuid.UUID, channelID, messageTS, threadTS string) error {
	_, err := s.db.NewUpdate().Model((*models.IncidentCoordinationMessage)(nil)).
		Set("slack_channel_id = ?", channelID).
		Set("slack_message_ts = ?", messageTS).
		Set("slack_thread_ts = ?", threadTS).
		Set("provider_message_id = ?", SlackProviderMessageID(channelID, messageTS)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update incident coordination message slack ts: %w", err)
	}
	return nil
}

func (s *pgIncidentCoordinationStore) UpdateMessageBody(ctx context.Context, incidentNumber int64, messageID uuid.UUID, body string) (*IncidentCoordinationMessageRecord, error) {
	var msg models.IncidentCoordinationMessage
	err := s.db.NewSelect().Model(&msg).
		Where("id = ?", messageID).
		Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", incidentNumber).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return handleQueryErr[*IncidentCoordinationMessageRecord](err, "incident coordination message")
	}

	if _, err := s.db.NewUpdate().Model((*models.IncidentCoordinationMessage)(nil)).
		Set("body = ?", body).
		Where("id = ?", msg.ID).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update incident coordination message body: %w", err)
	}
	msg.Body = body
	out := incidentCoordinationFromModel(&msg)
	out.IncidentNumber = incidentNumber
	return out, nil
}

func (s *pgIncidentCoordinationStore) NewestStatusUpdate(ctx context.Context, incidentNumber int64) (*IncidentCoordinationMessageRecord, error) {
	var items []models.IncidentCoordinationMessage
	if err := s.db.NewSelect().Model(&items).
		Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", incidentNumber).
		Where("kind = ?", IncidentCoordinationKindStatusUpdate).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to find newest status update: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	rec := incidentCoordinationFromModel(&items[0])
	rec.IncidentNumber = incidentNumber
	return rec, nil
}

func (s *pgIncidentCoordinationStore) NewestAgentCoordinationReply(ctx context.Context, incidentNumber int64) (*IncidentCoordinationMessageRecord, error) {
	var items []models.IncidentCoordinationMessage
	if err := s.db.NewSelect().Model(&items).
		Where("incident_id IN (SELECT id FROM incidents WHERE incident_number = ? AND deleted_at IS NULL)", incidentNumber).
		Where("kind = ?", IncidentCoordinationKindAgentReply).
		Where("actor_type = ?", IncidentCoordinationActorAgent).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to find newest agent coordination reply: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	rec := incidentCoordinationFromModel(&items[0])
	rec.IncidentNumber = incidentNumber
	return rec, nil
}

func incidentCoordinationFromModel(m *models.IncidentCoordinationMessage) *IncidentCoordinationMessageRecord {
	if m == nil {
		return nil
	}
	record := &IncidentCoordinationMessageRecord{
		ID:                    m.ID,
		Kind:                  m.Kind,
		ActorType:             m.ActorType,
		ActorID:               m.ActorID,
		ActorDisplayName:      m.ActorDisplayName,
		Body:                  m.Body,
		Internal:              m.Internal,
		Source:                m.Source,
		SlackChannelID:        m.SlackChannelID,
		SlackMessageTS:        m.SlackMessageTs,
		SlackThreadTS:         m.SlackThreadTs,
		ProviderMessageID:     m.ProviderMessageID,
		LinkedInvestigationID: m.LinkedInvestigationID,
		Metadata:              m.Metadata,
		CreatedAt:             m.CreatedAt,
		UpdatedAt:             m.UpdatedAt,
	}
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	return record
}

func SlackProviderMessageID(channelID, messageTS string) string {
	if channelID == "" || messageTS == "" {
		return ""
	}
	return "slack:" + channelID + ":" + messageTS
}
