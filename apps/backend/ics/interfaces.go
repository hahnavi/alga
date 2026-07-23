package ics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type RoleRecord struct {
	ID                 uuid.UUID  `json:"id"`
	IncidentNumber     int64      `json:"incident_number"`
	RoleType           string     `json:"role_type"`
	AssigneeType       string     `json:"assignee_type"`
	UserID             *uuid.UUID `json:"user_id,omitempty"`
	UserName           string     `json:"user_name,omitempty"`
	UserEmail          string     `json:"user_email,omitempty"`
	AgentTokenID       *uuid.UUID `json:"agent_token_id,omitempty"`
	AgentName          string     `json:"agent_name,omitempty"`
	AgentType          string     `json:"agent_type,omitempty"`
	ParentAssignmentID *uuid.UUID `json:"parent_assignment_id,omitempty"`
	ScopeDescription   *string    `json:"scope_description,omitempty"`
	Status             string     `json:"status"`
	EndedReason        *string    `json:"ended_reason,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
}

type RoleStore interface {
	AssignRole(ctx context.Context, incidentNumber int64, roleType RoleType, userID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*RoleRecord, error)
	AssignAgentRole(ctx context.Context, incidentNumber int64, roleType RoleType, agentTokenID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*RoleRecord, error)
	EndRole(ctx context.Context, assignmentID uuid.UUID, reason EndReason) error
	GetActiveRoles(ctx context.Context, incidentNumber int64) ([]RoleRecord, error)
	GetActiveIC(ctx context.Context, incidentNumber int64) (*RoleRecord, error)
	GetActiveRolesForAgent(ctx context.Context, agentTokenID uuid.UUID) ([]RoleRecord, error)
	EndRolesForAgent(ctx context.Context, agentTokenID uuid.UUID, reason EndReason) error
	EndAllRolesForIncident(ctx context.Context, incidentNumber int64, reason EndReason) error
}

type DocumentRecord struct {
	ID             uuid.UUID  `json:"id"`
	IncidentNumber int64      `json:"incident_number"`
	Section        string     `json:"section"`
	Content        string     `json:"content"`
	Version        int        `json:"version"`
	UpdatedBy      *uuid.UUID `json:"updated_by,omitempty"`
	UpdatedAt      string     `json:"updated_at"`
}

type DocumentStore interface {
	GetAllSections(ctx context.Context, incidentNumber int64) ([]DocumentRecord, error)
	UpsertSection(ctx context.Context, incidentNumber int64, section DocumentSection, content string, version int, userID uuid.UUID) (*DocumentRecord, error)
	InitializeDocument(ctx context.Context, incidentNumber int64, sections map[DocumentSection]string) error
}

type IncidentRecord struct {
	ID                     uuid.UUID      `json:"id"`
	IncidentNumber         int64          `json:"incident_number"`
	Title                  string         `json:"title"`
	Status                 string         `json:"status"`
	Severity               string         `json:"severity"`
	WarRoomChannelID       string         `json:"war_room_channel_id,omitempty"`
	WarRoomChannelProvider string         `json:"war_room_channel_provider,omitempty"`
	GoogleMeetSpaceName    string         `json:"google_meet_space_name,omitempty"`
	TriageReport           map[string]any `json:"triage_report,omitempty"`
}

type TimelineEntry struct {
	IncidentNumber int64          `json:"-"`
	EventType      string         `json:"event_type"`
	ActorID        *uuid.UUID     `json:"actor_id,omitempty"`
	ActorType      string         `json:"actor_type"`
	Message        string         `json:"message"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type IncidentStore interface {
	GetIncident(ctx context.Context, incidentNumber int64) (*IncidentRecord, error)
	AddTimelineEntry(ctx context.Context, entry *TimelineEntry) error
	SetWarRoomMeet(ctx context.Context, incidentNumber int64, spaceName, conferenceURL string) error
}
