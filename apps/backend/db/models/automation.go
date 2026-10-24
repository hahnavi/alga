package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AutomationEvent struct {
	IDModel      `bun:"table:automation_events"`
	EventType    string          `bun:"event_type,notnull"`
	EntityType   string          `bun:"entity_type,notnull"`
	EntityID     uuid.UUID       `bun:"entity_id,notnull"`
	EntityNumber int64           `bun:"entity_number,notnull"`
	Payload      json.RawMessage `bun:"payload,type:jsonb,notnull"`
	CreatedAt    time.Time       `bun:"created_at,notnull"`
	DispatchedAt *time.Time      `bun:"dispatched_at"`
}

type AutomationRule struct {
	BaseModel   `bun:"table:automation_rules"`
	Name        string          `bun:"name,notnull"`
	Description string          `bun:"description,notnull"`
	TriggerType string          `bun:"trigger_type,notnull"`
	Conditions  json.RawMessage `bun:"conditions,type:jsonb,notnull"`
	Actions     json.RawMessage `bun:"actions,type:jsonb,notnull"`
	Enabled     bool            `bun:"enabled,notnull"`
	Version     int             `bun:"version,notnull"`
	CreatedBy   *uuid.UUID      `bun:"created_by"`
	ArchivedAt  *time.Time      `bun:"archived_at"`
}

type AutomationRun struct {
	BaseModel     `bun:"table:automation_runs"`
	RuleID        uuid.UUID       `bun:"rule_id,notnull"`
	EventID       uuid.UUID       `bun:"event_id,notnull"`
	RuleName      string          `bun:"rule_name,notnull"`
	RuleVersion   int             `bun:"rule_version,notnull"`
	Actions       json.RawMessage `bun:"actions,type:jsonb,notnull"`
	EventPayload  json.RawMessage `bun:"event_payload,type:jsonb,notnull"`
	Status        string          `bun:"status,notnull"`
	NextAction    int             `bun:"next_action,notnull"`
	Attempt       int             `bun:"attempt,notnull"`
	NextAttemptAt time.Time       `bun:"next_attempt_at,notnull"`
	LeaseUntil    *time.Time      `bun:"lease_until"`
	LastError     string          `bun:"last_error,notnull"`
	CompletedAt   *time.Time      `bun:"completed_at"`
}

type AutomationRunStep struct {
	CreatedOnlyModel `bun:"table:automation_run_steps"`
	RunID            uuid.UUID `bun:"run_id,notnull"`
	ActionIndex      int       `bun:"action_index,notnull"`
	Attempt          int       `bun:"attempt,notnull"`
	Status           string    `bun:"status,notnull"`
	HTTPStatus       *int      `bun:"http_status"`
	ErrorCode        string    `bun:"error_code,notnull"`
}
