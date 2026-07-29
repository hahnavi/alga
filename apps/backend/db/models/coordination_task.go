package models

import (
	"time"

	"github.com/google/uuid"
)

type CoordinationTask struct {
	BaseModel

	IncidentID            *uuid.UUID     `bun:"incident_id"`
	ParentTaskID          *uuid.UUID     `bun:"parent_task_id"`
	Kind                  string         `bun:"kind,notnull,default:'investigate'"`
	AssigneeRole          string         `bun:"assignee_role,notnull,default:'responder'"`
	AssigneeAgentID       string         `bun:"assignee_agent_id"`
	AssigneeAgentName     string         `bun:"assignee_agent_name"`
	Goal                  string         `bun:"goal,notnull"`
	InputContext          map[string]any `bun:"input_context,type:jsonb,notnull,default:'{}'"`
	Result                map[string]any `bun:"result,type:jsonb"`
	ResultSchema          map[string]any `bun:"result_schema,type:jsonb"`
	LinkedInvestigationID *uuid.UUID     `bun:"linked_investigation_id"`
	Status                string         `bun:"status,notnull,default:'pending'"`
	Priority              int            `bun:"priority,notnull,default:0"`
	DueAt                 *time.Time     `bun:"due_at"`
	ClaimedAt             *time.Time     `bun:"claimed_at"`
	CompletedAt           *time.Time     `bun:"completed_at"`
	CreatedByAgentID      string         `bun:"created_by_agent_id"`
	CreatedByName         string         `bun:"created_by_name"`
	FailureReason         string         `bun:"failure_reason"`
	DispatchAttempts      int            `bun:"dispatch_attempts,notnull,default:0"`
}

func (*CoordinationTask) TableName() string { return "coordination_tasks" }
