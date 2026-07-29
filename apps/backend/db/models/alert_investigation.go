package models

import (
	"time"

	"github.com/google/uuid"
)

type AlertInvestigation struct {
	BaseModel

	AlertInvestigationID            string                     `bun:"alert_investigation_id,notnull,unique"`
	CorrelationKey                  string                     `bun:"correlation_key"`
	Status                          string                     `bun:"status,notnull,default:'pending'"`
	AgentID                         string                     `bun:"agent_id"`
	AgentName                       string                     `bun:"agent_name"`
	AgentType                       string                     `bun:"agent_type"`
	PrimaryThreadID                 string                     `bun:"primary_thread_id"`
	SlackChannelID                  string                     `bun:"slack_channel_id"`
	SlackThreadTS                   string                     `bun:"slack_thread_ts"`
	MMPostID                        string                     `bun:"mm_post_id"`
	MMThreadID                      string                     `bun:"mm_thread_id"`
	PromotedIncidentID              *uuid.UUID                 `bun:"promoted_incident_id"`
	PromotedIncidentInvestigationID *uuid.UUID                 `bun:"promoted_incident_investigation_id"`
	Summary                         *AlertInvestigationSummary `bun:"summary,type:jsonb"`
	Findings                        []InvestigationFinding     `bun:"findings,type:jsonb"`
	Evidence                        []EvidenceItem             `bun:"evidence,type:jsonb"`
	CompletedAt                     *time.Time                 `bun:"completed_at"`
	CompletedReason                 string                     `bun:"completed_reason"`
	CompletedByType                 string                     `bun:"completed_by_type"`
	CompletedByID                   string                     `bun:"completed_by_id"`
	CompletedByName                 string                     `bun:"completed_by_name"`
	StartedAt                       *time.Time                 `bun:"started_at"`
	InvestigatingDurationMs         int64                      `bun:"investigating_duration_ms,default:0"`
	PrimaryAlertFingerprint         string                     `bun:"primary_alert_fingerprint"`
	PrimaryAlertNumber              int64                      `bun:"primary_alert_number"`
	EscalationLevel                 string                     `bun:"escalation_level"`
	TriageResultID                  *uuid.UUID                 `bun:"triage_result_id"`
	TriageDecision                  string                     `bun:"triage_decision"`
	TriageEnrichment                map[string]any             `bun:"triage_enrichment,type:jsonb"`
	AssigneeType                    string                     `bun:"assignee_type,notnull,default:'agent'"`
	AssigneeID                      *uuid.UUID                 `bun:"assignee_id"`
}

func (*AlertInvestigation) TableName() string { return "alert_investigations" }
