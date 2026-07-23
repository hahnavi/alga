package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/incident"
	"alga/ent/incidentcoordinationmessage"
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

func newPGIncidentCoordinationStore(client *ent.Client) IncidentCoordinationStore {
	return &pgIncidentCoordinationStore{pgStoreBase{client: client}}
}

func (s *pgIncidentCoordinationStore) CreateMessage(ctx context.Context, record *IncidentCoordinationMessageRecord) (*IncidentCoordinationMessageRecord, error) {
	inc, err := s.client.Incident.Query().
		Where(incident.IncidentNumber(record.IncidentNumber), incident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident for coordination message: %w", err)
	}

	metadata := record.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	b := s.client.IncidentCoordinationMessage.Create().
		SetIncidentID(inc.ID).
		SetKind(record.Kind).
		SetActorType(record.ActorType).
		SetActorDisplayName(record.ActorDisplayName).
		SetBody(record.Body).
		SetInternal(record.Internal).
		SetSource(record.Source).
		SetSlackChannelID(record.SlackChannelID).
		SetSlackMessageTs(record.SlackMessageTS).
		SetSlackThreadTs(record.SlackThreadTS).
		SetProviderMessageID(record.ProviderMessageID).
		SetLinkedInvestigationID(record.LinkedInvestigationID).
		SetMetadata(metadata)
	if record.ActorID != nil {
		b.SetActorID(*record.ActorID)
	}
	if !record.CreatedAt.IsZero() {
		b.SetCreatedAt(record.CreatedAt)
	}
	if !record.UpdatedAt.IsZero() {
		b.SetUpdatedAt(record.UpdatedAt)
	}

	created, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident coordination message: %w", err)
	}
	out := incidentCoordinationFromEnt(created)
	out.IncidentNumber = record.IncidentNumber
	return out, nil
}

func (s *pgIncidentCoordinationStore) ListMessages(ctx context.Context, incidentNumber int64, limit, skip int) ([]IncidentCoordinationMessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	items, err := s.client.IncidentCoordinationMessage.Query().
		Where(
			incidentcoordinationmessage.HasIncidentWith(incident.IncidentNumber(incidentNumber)),
			incidentcoordinationmessage.Not(incidentcoordinationmessage.Kind(IncidentCoordinationKindStatusUpdate)),
		).
		WithIncident().
		Order(ent.Asc(incidentcoordinationmessage.FieldCreatedAt)).
		Limit(limit).
		Offset(skip).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list incident coordination messages: %w", err)
	}

	records := make([]IncidentCoordinationMessageRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *incidentCoordinationFromEnt(item))
	}
	return records, nil
}

func (s *pgIncidentCoordinationStore) ListMessagesByKind(ctx context.Context, incidentNumber int64, kind string, limit, skip int) ([]IncidentCoordinationMessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	items, err := s.client.IncidentCoordinationMessage.Query().
		Where(
			incidentcoordinationmessage.HasIncidentWith(incident.IncidentNumber(incidentNumber)),
			incidentcoordinationmessage.Kind(kind),
		).
		Order(ent.Desc(incidentcoordinationmessage.FieldCreatedAt)).
		Limit(limit).
		Offset(skip).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list incident coordination messages by kind: %w", err)
	}

	records := make([]IncidentCoordinationMessageRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *incidentCoordinationFromEnt(item))
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

	msg, err := s.client.IncidentCoordinationMessage.Query().
		Where(incidentcoordinationmessage.ProviderMessageID(providerMessageID)).
		WithIncident().
		Only(ctx)
	if err != nil {
		return handleQueryErr[*IncidentCoordinationMessageRecord](err, "incident coordination message")
	}
	return incidentCoordinationFromEnt(msg), nil
}

func (s *pgIncidentCoordinationStore) SetSlackMessageTS(ctx context.Context, id uuid.UUID, channelID, messageTS, threadTS string) error {
	_, err := s.client.IncidentCoordinationMessage.UpdateOneID(id).
		SetSlackChannelID(channelID).
		SetSlackMessageTs(messageTS).
		SetSlackThreadTs(threadTS).
		SetProviderMessageID(SlackProviderMessageID(channelID, messageTS)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update incident coordination message slack ts: %w", err)
	}
	return nil
}

func (s *pgIncidentCoordinationStore) UpdateMessageBody(ctx context.Context, incidentNumber int64, messageID uuid.UUID, body string) (*IncidentCoordinationMessageRecord, error) {
	msg, err := s.client.IncidentCoordinationMessage.Query().
		Where(
			incidentcoordinationmessage.ID(messageID),
			incidentcoordinationmessage.HasIncidentWith(incident.IncidentNumber(incidentNumber), incident.DeletedAtIsNil()),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return handleQueryErr[*IncidentCoordinationMessageRecord](err, "incident coordination message")
	}
	updated, err := s.client.IncidentCoordinationMessage.UpdateOneID(msg.ID).
		SetBody(body).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update incident coordination message body: %w", err)
	}
	out := incidentCoordinationFromEnt(updated)
	out.IncidentNumber = incidentNumber
	return out, nil
}

func (s *pgIncidentCoordinationStore) NewestStatusUpdate(ctx context.Context, incidentNumber int64) (*IncidentCoordinationMessageRecord, error) {
	items, err := s.client.IncidentCoordinationMessage.Query().
		Where(
			incidentcoordinationmessage.HasIncidentWith(incident.IncidentNumber(incidentNumber)),
			incidentcoordinationmessage.Kind(IncidentCoordinationKindStatusUpdate),
		).
		Order(ent.Desc(incidentcoordinationmessage.FieldCreatedAt)).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find newest status update: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	rec := incidentCoordinationFromEnt(items[0])
	rec.IncidentNumber = incidentNumber
	return rec, nil
}

func (s *pgIncidentCoordinationStore) NewestAgentCoordinationReply(ctx context.Context, incidentNumber int64) (*IncidentCoordinationMessageRecord, error) {
	items, err := s.client.IncidentCoordinationMessage.Query().
		Where(
			incidentcoordinationmessage.HasIncidentWith(incident.IncidentNumber(incidentNumber)),
			incidentcoordinationmessage.Kind(IncidentCoordinationKindAgentReply),
			incidentcoordinationmessage.ActorTypeEQ(IncidentCoordinationActorAgent),
		).
		Order(ent.Desc(incidentcoordinationmessage.FieldCreatedAt)).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find newest agent coordination reply: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	rec := incidentCoordinationFromEnt(items[0])
	rec.IncidentNumber = incidentNumber
	return rec, nil
}

func incidentCoordinationFromEnt(e *ent.IncidentCoordinationMessage) *IncidentCoordinationMessageRecord {
	if e == nil {
		return nil
	}
	record := &IncidentCoordinationMessageRecord{
		ID:                    e.ID,
		Kind:                  e.Kind,
		ActorType:             e.ActorType,
		ActorID:               e.ActorID,
		ActorDisplayName:      e.ActorDisplayName,
		Body:                  e.Body,
		Internal:              e.Internal,
		Source:                e.Source,
		SlackChannelID:        e.SlackChannelID,
		SlackMessageTS:        e.SlackMessageTs,
		SlackThreadTS:         e.SlackThreadTs,
		ProviderMessageID:     e.ProviderMessageID,
		LinkedInvestigationID: e.LinkedInvestigationID,
		Metadata:              e.Metadata,
		CreatedAt:             e.CreatedAt,
		UpdatedAt:             e.UpdatedAt,
	}
	if e.Edges.Incident != nil {
		record.IncidentNumber = e.Edges.Incident.IncidentNumber
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
