package models

import (
	"time"

	"github.com/google/uuid"
)

type IncidentInvestigation struct {
	BaseModel

	IncidentInvestigationID    string                 `bun:"public_id,notnull,unique"`
	IncidentID                 *uuid.UUID             `bun:"incident_id"`
	Status                     string                 `bun:"status,notnull,default:'pending'"`
	AgentID                    string                 `bun:"agent_id,default:''"`
	AgentName                  string                 `bun:"agent_name,default:''"`
	AgentType                  string                 `bun:"agent_type,default:''"`
	PrimaryThreadID            string                 `bun:"primary_thread_id,default:''"`
	SlackChannelID             string                 `bun:"slack_channel_id,default:''"`
	SlackThreadTs              string                 `bun:"slack_thread_ts,default:''"`
	MMPostID                   string                 `bun:"mm_post_id,default:''"`
	MMThreadID                 string                 `bun:"mm_thread_id,default:''"`
	SourceAlertInvestigationID *uuid.UUID             `bun:"source_alert_investigation_id"`
	Summary                    *InvestigationSummary  `bun:"summary,type:jsonb"`
	Findings                   []InvestigationFinding `bun:"findings,type:jsonb"`
	Evidence                   []EvidenceItem         `bun:"evidence,type:jsonb"`
	CompletedAt                *time.Time             `bun:"completed_at"`
	StartedAt                  *time.Time             `bun:"started_at"`
	InvestigatingDurationMs    int64                  `bun:"investigating_duration_ms,default:0"`
	ParentInvestigationID      *uuid.UUID             `bun:"parent_investigation_id"`
	AssigneeType               string                 `bun:"assignee_type,notnull,default:'agent'"`
	AssigneeID                 *uuid.UUID             `bun:"assignee_id"`
}
