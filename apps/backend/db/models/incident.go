package models

import (
	"time"

	"github.com/google/uuid"
)

type Incident struct {
	BaseModel
	SoftDeleteModel

	IncidentNumber           int64          `bun:"incident_number,notnull,unique"`
	Title                    string         `bun:"title,notnull,default:''"`
	Description              string         `bun:"description,notnull,default:''"`
	Summary                  string         `bun:"summary,default:''"`
	Status                   string         `bun:"status,notnull,default:'detected'"`
	Severity                 string         `bun:"severity,notnull,default:'warning'"`
	ImpactLevel              string         `bun:"impact_level,notnull,default:'medium'"`
	Priority                 string         `bun:"priority,notnull,default:'P4'"`
	IncidentType             string         `bun:"incident_type,notnull,default:'real'"`
	CommanderID              *uuid.UUID     `bun:"commander_id"`
	CommunicatorID           *uuid.UUID     `bun:"communicator_id"`
	OnCallResponderID        *uuid.UUID     `bun:"on_call_responder_id"`
	CommanderAssigneeType    string         `bun:"commander_assignee_type,default:'user'"`
	CommunicatorAssigneeType string         `bun:"communicator_assignee_type,default:'user'"`
	ServiceID                *uuid.UUID     `bun:"service_id"`
	EscalationPolicyID       *uuid.UUID     `bun:"escalation_policy_id"`
	ConferenceURL            string         `bun:"conference_url,notnull,default:''"`
	SlackChannelID           *string        `bun:"slack_channel_id"`
	SlackChannelName         string         `bun:"slack_channel_name,default:''"`
	SlackChannelArchived     bool           `bun:"slack_channel_archived,default:false"`
	WarRoomChannelID         *string        `bun:"war_room_channel_id"`
	WarRoomChannelProvider   *string        `bun:"war_room_channel_provider"`
	GoogleMeetSpaceName      *string        `bun:"google_meet_space_name"`
	StatusPageIncidentID     string         `bun:"status_page_incident_id,notnull,default:''"`
	SLATargetRespondAt       *time.Time     `bun:"sla_target_respond_at"`
	SLATargetResolveAt       *time.Time     `bun:"sla_target_resolve_at"`
	SLAAcknowledgedAt        *time.Time     `bun:"sla_acknowledged_at"`
	SLAResolvedAt            *time.Time     `bun:"sla_resolved_at"`
	StartedAt                *time.Time     `bun:"started_at"`
	MitigatedAt              *time.Time     `bun:"mitigated_at"`
	ResolvedAt               *time.Time     `bun:"resolved_at"`
	ClosedAt                 *time.Time     `bun:"closed_at"`
	TriagedAt                *time.Time     `bun:"triaged_at"`
	TriageReport             map[string]any `bun:"triage_report,type:jsonb"`
	AutoConfirmed            bool           `bun:"auto_confirmed,notnull,default:false"`
	Tags                     []string       `bun:"tags,type:jsonb,notnull,default:'[]'"`
	CustomFields             map[string]any `bun:"custom_fields,type:jsonb,notnull,default:'{}'"`
}
