package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/automation"
	"alga/crypto"
	"alga/db/models"
)

var ErrAutomationVersionConflict = errors.New("automation rule version conflict")
var ErrAutomationClaimLost = errors.New("automation run claim lost")

type AutomationRuleRecord struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	TriggerType string                 `json:"trigger_type"`
	Conditions  []automation.Condition `json:"conditions"`
	Actions     []automation.Action    `json:"actions"`
	Enabled     bool                   `json:"enabled"`
	Version     int                    `json:"version"`
	CreatedBy   *uuid.UUID             `json:"created_by,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type AutomationRunRecord struct {
	ID           uuid.UUID              `json:"id"`
	RuleID       uuid.UUID              `json:"rule_id"`
	EventID      uuid.UUID              `json:"event_id"`
	RuleName     string                 `json:"rule_name"`
	RuleVersion  int                    `json:"rule_version"`
	Actions      []automation.Action    `json:"actions"`
	EventPayload json.RawMessage        `json:"event_payload"`
	Status       string                 `json:"status"`
	NextAction   int                    `json:"next_action"`
	Attempt      int                    `json:"attempt"`
	LastError    string                 `json:"last_error,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Steps        []AutomationStepRecord `json:"steps,omitempty"`
}

type AutomationStepRecord struct {
	ID          uuid.UUID `json:"id"`
	ActionIndex int       `json:"action_index"`
	Attempt     int       `json:"attempt"`
	Status      string    `json:"status"`
	HTTPStatus  *int      `json:"http_status,omitempty"`
	ErrorCode   string    `json:"error_code,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type ClaimedAutomationRun struct {
	ID           uuid.UUID
	RuleID       uuid.UUID
	EventID      uuid.UUID
	Actions      []automation.StoredAction
	EventPayload json.RawMessage
	ActionIndex  int
	Attempt      int
}

type AutomationStore interface {
	CreateRule(context.Context, automation.Definition, *uuid.UUID) (*AutomationRuleRecord, error)
	UpdateRule(context.Context, uuid.UUID, int, automation.Definition) (*AutomationRuleRecord, error)
	ArchiveRule(context.Context, uuid.UUID) error
	GetRule(context.Context, uuid.UUID) (*AutomationRuleRecord, error)
	ListRules(context.Context, int, int) ([]AutomationRuleRecord, int64, error)
	ListRuns(context.Context, *uuid.UUID, int, int) ([]AutomationRunRecord, int64, error)
	GetRun(context.Context, uuid.UUID) (*AutomationRunRecord, error)
	RetryRun(context.Context, uuid.UUID) error
	DispatchOne(context.Context) (bool, error)
	ClaimRun(context.Context) (*ClaimedAutomationRun, error)
	CompleteAction(context.Context, ClaimedAutomationRun, *int, bool) error
	FailAction(context.Context, ClaimedAutomationRun, string, *int, bool, time.Time) error
	PruneHistory(context.Context, time.Time) error
}

type pgAutomationStore struct {
	pgStoreBase
	policy automation.HostPolicy
}

func newPGAutomationStore(db *bun.DB, policy automation.HostPolicy) AutomationStore {
	return &pgAutomationStore{pgStoreBase: pgStoreBase{db: db}, policy: policy}
}

func encodeAutomationActions(actions []automation.Action, old []automation.StoredAction) (json.RawMessage, error) {
	stored := make([]automation.StoredAction, 0, len(actions))
	for i, action := range actions {
		item := automation.StoredAction{Type: action.Type, URL: action.URL, Message: action.Message}
		if action.BearerToken != "" {
			ciphertext, err := crypto.Default().EncryptString(action.BearerToken)
			if err != nil {
				return nil, fmt.Errorf("encrypt automation credential: %w", err)
			}
			item.BearerTokenEnc = ciphertext
		} else if !action.ClearBearerToken && i < len(old) && old[i].Type == action.Type && old[i].URL == action.URL {
			item.BearerTokenEnc = old[i].BearerTokenEnc
		}
		stored = append(stored, item)
	}
	return json.Marshal(stored)
}

func decodeStoredActions(raw json.RawMessage) ([]automation.StoredAction, error) {
	var actions []automation.StoredAction
	if err := json.Unmarshal(raw, &actions); err != nil {
		return nil, fmt.Errorf("decode automation actions: %w", err)
	}
	return actions, nil
}

func publicActions(raw json.RawMessage) ([]automation.Action, error) {
	stored, err := decodeStoredActions(raw)
	if err != nil {
		return nil, err
	}
	out := make([]automation.Action, 0, len(stored))
	for _, action := range stored {
		out = append(out, action.Public())
	}
	return out, nil
}

func ruleFromModel(m *models.AutomationRule) (*AutomationRuleRecord, error) {
	var conditions []automation.Condition
	if err := json.Unmarshal(m.Conditions, &conditions); err != nil {
		return nil, fmt.Errorf("decode automation conditions: %w", err)
	}
	actions, err := publicActions(m.Actions)
	if err != nil {
		return nil, err
	}
	return &AutomationRuleRecord{ID: m.ID, Name: m.Name, Description: m.Description, TriggerType: m.TriggerType,
		Conditions: conditions, Actions: actions, Enabled: m.Enabled, Version: m.Version, CreatedBy: m.CreatedBy,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}, nil
}

func runFromModel(m *models.AutomationRun) (*AutomationRunRecord, error) {
	actions, err := publicActions(m.Actions)
	if err != nil {
		return nil, err
	}
	return &AutomationRunRecord{ID: m.ID, RuleID: m.RuleID, EventID: m.EventID, RuleName: m.RuleName,
		RuleVersion: m.RuleVersion, Actions: actions, EventPayload: m.EventPayload, Status: m.Status,
		NextAction: m.NextAction, Attempt: m.Attempt, LastError: m.LastError, CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt, CompletedAt: m.CompletedAt}, nil
}

func (s *pgAutomationStore) CreateRule(ctx context.Context, d automation.Definition, creator *uuid.UUID) (*AutomationRuleRecord, error) {
	// Creation is always a draft; activation requires a separate, reviewable edit.
	d.Enabled = false
	if err := automation.Validate(d, s.policy); err != nil {
		return nil, err
	}
	conditions, err := json.Marshal(d.Conditions)
	if err != nil {
		return nil, fmt.Errorf("encode conditions: %w", err)
	}
	actions, err := encodeAutomationActions(d.Actions, nil)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	m := &models.AutomationRule{BaseModel: models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		Name: d.Name, Description: d.Description, TriggerType: d.TriggerType, Conditions: conditions, Actions: actions,
		Enabled: d.Enabled, Version: 1, CreatedBy: creator}
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("create automation rule: %w", err)
	}
	return ruleFromModel(m)
}

func (s *pgAutomationStore) UpdateRule(ctx context.Context, id uuid.UUID, version int, d automation.Definition) (*AutomationRuleRecord, error) {
	if err := automation.Validate(d, s.policy); err != nil {
		return nil, err
	}
	var updated *models.AutomationRule
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var current models.AutomationRule
		if err := tx.NewSelect().Model(&current).Where("id = ? AND archived_at IS NULL", id).For("UPDATE").Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("get automation rule for update: %w", err)
		}
		if current.Version != version {
			return ErrAutomationVersionConflict
		}
		old, err := decodeStoredActions(current.Actions)
		if err != nil {
			return err
		}
		conditions, err := json.Marshal(d.Conditions)
		if err != nil {
			return fmt.Errorf("encode conditions: %w", err)
		}
		actions, err := encodeAutomationActions(d.Actions, old)
		if err != nil {
			return err
		}
		current.Name, current.Description, current.TriggerType = d.Name, d.Description, d.TriggerType
		current.Conditions, current.Actions, current.Enabled = conditions, actions, d.Enabled
		current.Version++
		current.UpdatedAt = time.Now().UTC()
		if _, err := tx.NewUpdate().Model(&current).WherePK().Exec(ctx); err != nil {
			return fmt.Errorf("update automation rule: %w", err)
		}
		updated = &current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ruleFromModel(updated)
}

func (s *pgAutomationStore) ArchiveRule(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.AutomationRule)(nil)).Set("enabled = false").Set("archived_at = ?", now).
		Where("id = ? AND archived_at IS NULL", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("archive automation rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("archive automation rule rows: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *pgAutomationStore) GetRule(ctx context.Context, id uuid.UUID) (*AutomationRuleRecord, error) {
	var m models.AutomationRule
	if err := s.db.NewSelect().Model(&m).Where("id = ? AND archived_at IS NULL", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get automation rule: %w", err)
	}
	return ruleFromModel(&m)
}

func (s *pgAutomationStore) ListRules(ctx context.Context, limit, skip int) ([]AutomationRuleRecord, int64, error) {
	limit = min(max(limit, 1), 100)
	total, err := s.db.NewSelect().Model((*models.AutomationRule)(nil)).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count automation rules: %w", err)
	}
	var rows []models.AutomationRule
	if err := s.db.NewSelect().Model(&rows).Where("archived_at IS NULL").Order("created_at DESC").Limit(limit).Offset(max(skip, 0)).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("list automation rules: %w", err)
	}
	out := make([]AutomationRuleRecord, 0, len(rows))
	for i := range rows {
		record, err := ruleFromModel(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *record)
	}
	return out, int64(total), nil
}

func (s *pgAutomationStore) ListRuns(ctx context.Context, ruleID *uuid.UUID, limit, skip int) ([]AutomationRunRecord, int64, error) {
	limit = min(max(limit, 1), 100)
	countQ := s.db.NewSelect().Model((*models.AutomationRun)(nil))
	listQ := s.db.NewSelect()
	if ruleID != nil {
		countQ = countQ.Where("rule_id = ?", *ruleID)
		listQ = listQ.Where("rule_id = ?", *ruleID)
	}
	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count automation runs: %w", err)
	}
	var rows []models.AutomationRun
	if err := listQ.Model(&rows).Order("created_at DESC").Limit(limit).Offset(max(skip, 0)).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("list automation runs: %w", err)
	}
	out := make([]AutomationRunRecord, 0, len(rows))
	for i := range rows {
		record, err := runFromModel(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *record)
	}
	return out, int64(total), nil
}

func (s *pgAutomationStore) GetRun(ctx context.Context, id uuid.UUID) (*AutomationRunRecord, error) {
	var m models.AutomationRun
	if err := s.db.NewSelect().Model(&m).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get automation run: %w", err)
	}
	record, err := runFromModel(&m)
	if err != nil {
		return nil, err
	}
	var steps []models.AutomationRunStep
	if err := s.db.NewSelect().Model(&steps).Where("run_id = ?", id).Order("action_index ASC", "attempt ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("list automation run steps: %w", err)
	}
	record.Steps = make([]AutomationStepRecord, 0, len(steps))
	for _, step := range steps {
		record.Steps = append(record.Steps, AutomationStepRecord{ID: step.ID, ActionIndex: step.ActionIndex, Attempt: step.Attempt, Status: step.Status, HTTPStatus: step.HTTPStatus, ErrorCode: step.ErrorCode, CreatedAt: step.CreatedAt})
	}
	return record, nil
}

func (s *pgAutomationStore) RetryRun(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.NewUpdate().Model((*models.AutomationRun)(nil)).Set("status = 'pending'").Set("attempt = 0").
		Set("next_attempt_at = ?", time.Now().UTC()).Set("lease_until = NULL").Set("completed_at = NULL").Set("last_error = ''").
		Where("id = ? AND status = 'failed'", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("retry automation run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("retry automation run rows: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *pgAutomationStore) DispatchOne(ctx context.Context) (bool, error) {
	processed := false
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var event models.AutomationEvent
		if err := tx.NewSelect().Model(&event).Where("dispatched_at IS NULL").Order("created_at ASC", "id ASC").Limit(1).For("UPDATE SKIP LOCKED").Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("claim automation event: %w", err)
		}
		processed = true
		var rules []models.AutomationRule
		if err := tx.NewSelect().Model(&rules).Where("enabled = true AND archived_at IS NULL AND trigger_type = ?", event.EventType).For("SHARE").Scan(ctx); err != nil {
			return fmt.Errorf("select automation rules: %w", err)
		}
		now := time.Now().UTC()
		for _, rule := range rules {
			var conditions []automation.Condition
			if err := json.Unmarshal(rule.Conditions, &conditions); err != nil {
				return fmt.Errorf("decode automation conditions: %w", err)
			}
			if !automation.Match(event.Payload, conditions) {
				continue
			}
			run := &models.AutomationRun{BaseModel: models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
				RuleID: rule.ID, EventID: event.ID, RuleName: rule.Name, RuleVersion: rule.Version, Actions: rule.Actions,
				EventPayload: event.Payload, Status: "pending", NextAttemptAt: now}
			if _, err := tx.NewInsert().Model(run).On("CONFLICT (rule_id, event_id) DO NOTHING").Exec(ctx); err != nil {
				return fmt.Errorf("create automation run: %w", err)
			}
		}
		if _, err := tx.NewUpdate().Model(&event).Set("dispatched_at = ?", now).WherePK().Exec(ctx); err != nil {
			return fmt.Errorf("mark automation event dispatched: %w", err)
		}
		return nil
	})
	return processed, err
}

func (s *pgAutomationStore) ClaimRun(ctx context.Context) (*ClaimedAutomationRun, error) {
	var claimed *ClaimedAutomationRun
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now().UTC()
		var run models.AutomationRun
		if err := tx.NewSelect().Model(&run).Where("(status = 'pending' OR (status = 'running' AND lease_until < ?)) AND next_attempt_at <= ?", now, now).
			Order("next_attempt_at ASC", "created_at ASC").Limit(1).For("UPDATE SKIP LOCKED").Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("claim automation run: %w", err)
		}
		if run.Status == "running" && run.Attempt >= 5 {
			step := &models.AutomationRunStep{CreatedOnlyModel: models.CreatedOnlyModel{ID: models.NewUUID(), CreatedAt: now},
				RunID: run.ID, ActionIndex: run.NextAction, Attempt: run.Attempt, Status: "failed", ErrorCode: "lease_expired"}
			if _, err := tx.NewInsert().Model(step).Exec(ctx); err != nil {
				return fmt.Errorf("record expired automation lease: %w", err)
			}
			run.Status, run.LastError, run.LeaseUntil, run.CompletedAt = "failed", "lease_expired", nil, &now
			if _, err := tx.NewUpdate().Model(&run).WherePK().Exec(ctx); err != nil {
				return fmt.Errorf("fail expired automation run: %w", err)
			}
			return nil
		}
		lease := now.Add(30 * time.Second)
		run.Status, run.Attempt, run.LeaseUntil = "running", run.Attempt+1, &lease
		run.UpdatedAt = now
		if _, err := tx.NewUpdate().Model(&run).WherePK().Exec(ctx); err != nil {
			return fmt.Errorf("lease automation run: %w", err)
		}
		actions, err := decodeStoredActions(run.Actions)
		if err != nil {
			return err
		}
		claimed = &ClaimedAutomationRun{ID: run.ID, RuleID: run.RuleID, EventID: run.EventID,
			Actions: actions, EventPayload: run.EventPayload, ActionIndex: run.NextAction, Attempt: run.Attempt}
		return nil
	})
	return claimed, err
}

func (s *pgAutomationStore) withClaim(ctx context.Context, claim ClaimedAutomationRun, fn func(context.Context, bun.Tx, *models.AutomationRun) error) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var run models.AutomationRun
		if err := tx.NewSelect().Model(&run).Where("id = ?", claim.ID).For("UPDATE").Scan(ctx); err != nil {
			return fmt.Errorf("lock automation run: %w", err)
		}
		if run.Status != "running" || run.Attempt != claim.Attempt || run.NextAction != claim.ActionIndex {
			return ErrAutomationClaimLost
		}
		return fn(ctx, tx, &run)
	})
}

func (s *pgAutomationStore) CompleteAction(ctx context.Context, claim ClaimedAutomationRun, httpStatus *int, incidentNote bool) error {
	return s.withClaim(ctx, claim, func(ctx context.Context, tx bun.Tx, run *models.AutomationRun) error {
		if incidentNote {
			var event automation.Event
			if err := json.Unmarshal(claim.EventPayload, &event); err != nil {
				return fmt.Errorf("decode automation incident event: %w", err)
			}
			if event.EntityType != "incident" {
				return automation.ErrInvalidRule
			}
			entityID, err := uuid.Parse(event.EntityID)
			if err != nil {
				return fmt.Errorf("invalid automation incident id: %w", err)
			}
			var inc models.Incident
			if err := tx.NewSelect().Model(&inc).Where("id = ? AND deleted_at IS NULL", entityID).Scan(ctx); err != nil {
				return fmt.Errorf("find automation incident: %w", err)
			}
			entry := &models.IncidentTimelineEntry{ID: models.NewUUID(), IncidentID: inc.ID, EventType: "note_added", ActorType: "system",
				Message: claim.Actions[claim.ActionIndex].Message, Metadata: map[string]any{"automation_run_id": claim.ID.String(), "automation_rule_id": claim.RuleID.String()}, CreatedAt: time.Now().UTC()}
			if _, err := tx.NewInsert().Model(entry).Exec(ctx); err != nil {
				return fmt.Errorf("insert automation incident note: %w", err)
			}
		}
		step := &models.AutomationRunStep{CreatedOnlyModel: models.CreatedOnlyModel{ID: models.NewUUID(), CreatedAt: time.Now().UTC()}, RunID: claim.ID,
			ActionIndex: claim.ActionIndex, Attempt: claim.Attempt, Status: "succeeded", HTTPStatus: httpStatus}
		if _, err := tx.NewInsert().Model(step).Exec(ctx); err != nil {
			return fmt.Errorf("record automation step: %w", err)
		}
		run.NextAction++
		run.Attempt = 0
		run.LeaseUntil = nil
		run.LastError = ""
		run.NextAttemptAt = time.Now().UTC()
		if run.NextAction >= len(claim.Actions) {
			run.Status = "succeeded"
			now := time.Now().UTC()
			run.CompletedAt = &now
		} else {
			run.Status = "pending"
		}
		if _, err := tx.NewUpdate().Model(run).WherePK().Exec(ctx); err != nil {
			return fmt.Errorf("advance automation run: %w", err)
		}
		return nil
	})
}

func (s *pgAutomationStore) FailAction(ctx context.Context, claim ClaimedAutomationRun, code string, httpStatus *int, retry bool, next time.Time) error {
	return s.withClaim(ctx, claim, func(ctx context.Context, tx bun.Tx, run *models.AutomationRun) error {
		status := "failed"
		if retry && claim.Attempt < 5 {
			status = "retrying"
			run.Status = "pending"
			run.NextAttemptAt = next
		} else {
			run.Status = "failed"
			now := time.Now().UTC()
			run.CompletedAt = &now
		}
		step := &models.AutomationRunStep{CreatedOnlyModel: models.CreatedOnlyModel{ID: models.NewUUID(), CreatedAt: time.Now().UTC()}, RunID: claim.ID,
			ActionIndex: claim.ActionIndex, Attempt: claim.Attempt, Status: status, HTTPStatus: httpStatus, ErrorCode: code}
		if _, err := tx.NewInsert().Model(step).Exec(ctx); err != nil {
			return fmt.Errorf("record automation failed step: %w", err)
		}
		run.LastError = code
		run.LeaseUntil = nil
		if _, err := tx.NewUpdate().Model(run).WherePK().Exec(ctx); err != nil {
			return fmt.Errorf("update failed automation run: %w", err)
		}
		return nil
	})
}

func (s *pgAutomationStore) PruneHistory(ctx context.Context, olderThan time.Time) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*models.AutomationRun)(nil)).Where("status IN ('succeeded', 'failed') AND completed_at < ?", olderThan).Exec(ctx); err != nil {
			return fmt.Errorf("prune automation runs: %w", err)
		}
		if _, err := tx.NewDelete().Model((*models.AutomationEvent)(nil)).Where("dispatched_at IS NOT NULL AND created_at < ?", olderThan).
			Where("NOT EXISTS (SELECT 1 FROM automation_runs WHERE automation_runs.event_id = automation_events.id)").Exec(ctx); err != nil {
			return fmt.Errorf("prune automation events: %w", err)
		}
		return nil
	})
}
