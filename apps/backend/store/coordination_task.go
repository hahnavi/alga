package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

const (
	CoordinationTaskStatusPending    = "pending"
	CoordinationTaskStatusAssigned   = "assigned"
	CoordinationTaskStatusInProgress = "in_progress"
	CoordinationTaskStatusComplete   = "complete"
	CoordinationTaskStatusFailed     = "failed"
	CoordinationTaskStatusCancelled  = "cancelled"

	CoordinationTaskKindInvestigate = "investigate"
	CoordinationTaskKindCommunicate = "communicate"
	CoordinationTaskKindVerify      = "verify"
	CoordinationTaskKindMitigate    = "mitigate"
	CoordinationTaskKindSynthesize  = "synthesize"

	CoordinationTaskRoleCommander    = "commander"
	CoordinationTaskRoleCommunicator = "communicator"
	CoordinationTaskRoleResponder    = "responder"
)

var (
	ErrCoordinationTaskNotFound       = errors.New("coordination task not found")
	ErrCoordinationTaskStatusConflict = errors.New("coordination task status conflict")
)

// CoordinationTaskRecord is the transfer object for a coordination task row.
type CoordinationTaskRecord struct {
	ID                    uuid.UUID      `json:"id"`
	IncidentID            *uuid.UUID     `json:"incident_id,omitempty"`
	IncidentNumber        int64          `json:"incident_number,omitempty"`
	ParentTaskID          *uuid.UUID     `json:"parent_task_id,omitempty"`
	Kind                  string         `json:"kind"`
	AssigneeRole          string         `json:"assignee_role"`
	AssigneeAgentID       string         `json:"assignee_agent_id,omitempty"`
	AssigneeAgentName     string         `json:"assignee_agent_name,omitempty"`
	Goal                  string         `json:"goal"`
	InputContext          map[string]any `json:"input_context,omitempty"`
	Result                map[string]any `json:"result,omitempty"`
	ResultSchema          map[string]any `json:"result_schema,omitempty"`
	LinkedInvestigationID *uuid.UUID     `json:"linked_investigation_id,omitempty"`
	Status                string         `json:"status"`
	Priority              int            `json:"priority"`
	DueAt                 *time.Time     `json:"due_at,omitempty"`
	ClaimedAt             *time.Time     `json:"claimed_at,omitempty"`
	CompletedAt           *time.Time     `json:"completed_at,omitempty"`
	CreatedByAgentID      string         `json:"created_by_agent_id,omitempty"`
	CreatedByName         string         `json:"created_by_name,omitempty"`
	FailureReason         string         `json:"failure_reason,omitempty"`
	DispatchAttempts      int            `json:"dispatch_attempts"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

type CoordinationTaskStore interface {
	CreateTask(ctx context.Context, record *CoordinationTaskRecord) (*CoordinationTaskRecord, error)
	GetTask(ctx context.Context, taskID uuid.UUID) (*CoordinationTaskRecord, error)
	ListTasksByIncident(ctx context.Context, incidentNumber int64, filter map[string]any) ([]CoordinationTaskRecord, error)
	ListPendingTasks(ctx context.Context, role string, limit int) ([]CoordinationTaskRecord, error)
	ListOverdueTasks(ctx context.Context, now time.Time, limit int) ([]CoordinationTaskRecord, error)
	ListInProgressByAgent(ctx context.Context, agentIDHex string) ([]CoordinationTaskRecord, error)
	ClaimTask(ctx context.Context, taskID uuid.UUID, role, agentIDHex, agentName string) (*CoordinationTaskRecord, error)
	MarkInProgress(ctx context.Context, taskID uuid.UUID) error
	RevertByAgent(ctx context.Context, agentIDHex string) (int, error)
	CompleteTask(ctx context.Context, taskID uuid.UUID, result map[string]any) error
	FailTask(ctx context.Context, taskID uuid.UUID, reason string) error
	CancelTask(ctx context.Context, taskID uuid.UUID) error
	UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, fromStatuses []string, toStatus string) error
	BumpDispatchAttempts(ctx context.Context, taskID uuid.UUID) error
}

type pgCoordinationTaskStore struct {
	pgStoreBase
}

func newPGCoordinationTaskStore(db *bun.DB) CoordinationTaskStore {
	return &pgCoordinationTaskStore{pgStoreBase{db: db}}
}

// CreateTask creates a coordination task. When the kind is "investigate", the
// task has no parent task, and the incident already has a "coordinating" parent
// investigation, a child IncidentInvestigation is auto-created (status pending)
// and linked via LinkedInvestigationID. This is performed in a transaction; if
// no coordinating investigation exists, the task is still created without a
// linked investigation.
func (s *pgCoordinationTaskStore) CreateTask(ctx context.Context, record *CoordinationTaskRecord) (*CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()

	status := record.Status
	if status == "" {
		status = CoordinationTaskStatusPending
	}
	inputContext := record.InputContext
	if inputContext == nil {
		inputContext = map[string]any{}
	}

	var createdID uuid.UUID
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Resolve the incident when an incident number is provided.
		var inc *models.Incident
		if record.IncidentNumber != 0 {
			var found models.Incident
			if err := tx.NewSelect().Model(&found).
				Where("incident_number = ? AND deleted_at IS NULL", record.IncidentNumber).
				Scan(ctx); err != nil {
				if isNotFound(err) {
					return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
				}
				return fmt.Errorf("failed to find incident for coordination task: %w", err)
			}
			inc = &found
		}

		m := &models.CoordinationTask{
			IncidentID:        nil,
			ParentTaskID:      record.ParentTaskID,
			Kind:              record.Kind,
			AssigneeRole:      record.AssigneeRole,
			AssigneeAgentID:   record.AssigneeAgentID,
			AssigneeAgentName: record.AssigneeAgentName,
			Goal:              record.Goal,
			InputContext:      inputContext,
			Result:            record.Result,
			ResultSchema:      record.ResultSchema,
			Status:            status,
			Priority:          record.Priority,
			DueAt:             record.DueAt,
			ClaimedAt:         record.ClaimedAt,
			CompletedAt:       record.CompletedAt,
			CreatedByAgentID:  record.CreatedByAgentID,
			CreatedByName:     record.CreatedByName,
			FailureReason:     record.FailureReason,
			DispatchAttempts:  record.DispatchAttempts,
		}
		m.ID = models.NewUUID()
		m.CreatedAt = now
		m.UpdatedAt = now

		if inc != nil {
			m.IncidentID = &inc.ID
		}

		// Auto-link a child investigation for top-level investigate tasks.
		if record.Kind == CoordinationTaskKindInvestigate &&
			record.ParentTaskID == nil &&
			inc != nil {
			var parent models.IncidentInvestigation
			qerr := tx.NewSelect().Model(&parent).
				Where("incident_id = ?", inc.ID).
				Where("status = ?", IncidentInvestigationStatusCoordinating).
				Scan(ctx)
			if qerr == nil {
				childID := fmt.Sprintf("incident_inv_%d_%s", record.IncidentNumber, uuid.NewString()[:8])
				child := &models.IncidentInvestigation{
					IncidentInvestigationID: childID,
					IncidentID:              &inc.ID,
					Status:                  IncidentInvestigationStatusPending,
					ParentInvestigationID:   &parent.ID,
				}
				child.ID = models.NewUUID()
				child.CreatedAt = now
				child.UpdatedAt = now
				if _, cerr := tx.NewInsert().Model(child).Exec(ctx); cerr != nil {
					return fmt.Errorf("failed to create child incident investigation: %w", cerr)
				}
				m.LinkedInvestigationID = &child.ID
			} else if !isNotFound(qerr) {
				return fmt.Errorf("failed to query coordinating investigation: %w", qerr)
			}
			// If no coordinating investigation found, proceed without a link.
		}

		if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
			return fmt.Errorf("failed to create coordination task: %w", err)
		}
		createdID = m.ID
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetTask(ctx, createdID)
}

func (s *pgCoordinationTaskStore) GetTask(ctx context.Context, taskID uuid.UUID) (*CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var t models.CoordinationTask
	err := s.db.NewSelect().Model(&t).Where("id = ?", taskID).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("coordination task not found: %w", ErrCoordinationTaskNotFound)
		}
		return nil, fmt.Errorf("failed to query coordination task: %w", err)
	}
	return s.coordinationTaskFromModel(ctx, &t)
}

func (s *pgCoordinationTaskStore) ListTasksByIncident(ctx context.Context, incidentNumber int64, filter map[string]any) ([]CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if filter == nil {
		filter = map[string]any{}
	}
	limit, skip := extractLimitSkip(filter, 100)

	var inc models.Incident
	if err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ? AND deleted_at IS NULL", incidentNumber).
		Scan(ctx); err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident for coordination task listing: %w", err)
	}

	q := s.db.NewSelect().Model((*models.CoordinationTask)(nil)).
		Where("incident_id = ?", inc.ID)

	if v, ok := filter["parent_task_id"]; ok {
		switch p := v.(type) {
		case uuid.UUID:
			q = q.Where("parent_task_id = ?", p)
		case *uuid.UUID:
			if p != nil {
				q = q.Where("parent_task_id = ?", *p)
			}
		case string:
			if pid, perr := uuid.Parse(p); perr == nil {
				q = q.Where("parent_task_id = ?", pid)
			}
		}
	}
	if v, ok := filter["status"].(string); ok && v != "" {
		q = q.Where("status = ?", v)
	}
	if v, ok := filter["assignee_role"].(string); ok && v != "" {
		q = q.Where("assignee_role = ?", v)
	}

	sortField, _ := filter["$sort"].(string)
	switch sortField {
	case "priority":
		q = q.Order("priority ASC, created_at ASC")
	case "-priority":
		q = q.Order("priority DESC, created_at ASC")
	default:
		q = q.Order("created_at DESC")
	}
	q = q.Limit(limit).Offset(skip)

	var items []models.CoordinationTask
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list coordination tasks: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for i := range items {
		rec, err := s.coordinationTaskFromModel(ctx, &items[i])
		if err != nil {
			return nil, err
		}
		rec.IncidentNumber = incidentNumber
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgCoordinationTaskStore) ListPendingTasks(ctx context.Context, role string, limit int) ([]CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var items []models.CoordinationTask
	if err := s.db.NewSelect().Model(&items).
		Where("status = ?", CoordinationTaskStatusPending).
		Where("assignee_role = ?", role).
		Order("priority DESC, created_at ASC").
		Limit(limit).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list pending coordination tasks: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for i := range items {
		rec, err := s.coordinationTaskFromModel(ctx, &items[i])
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

// ListOverdueTasks returns in_progress coordination tasks whose due_at has
// passed, ordered by due_at ascending. Tasks without a due_at are ignored.
// Used by the scheduler's stall sweep to revert or dead-letter stale work.
func (s *pgCoordinationTaskStore) ListOverdueTasks(ctx context.Context, now time.Time, limit int) ([]CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var items []models.CoordinationTask
	if err := s.db.NewSelect().Model(&items).
		Where("status = ?", CoordinationTaskStatusInProgress).
		Where("due_at IS NOT NULL").
		Where("due_at < ?", now).
		Order("due_at ASC").
		Limit(limit).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list overdue coordination tasks: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for i := range items {
		rec, err := s.coordinationTaskFromModel(ctx, &items[i])
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgCoordinationTaskStore) ListInProgressByAgent(ctx context.Context, agentIDHex string) ([]CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var items []models.CoordinationTask
	if err := s.db.NewSelect().Model(&items).
		Where("assignee_agent_id = ?", agentIDHex).
		Where("status IN (?)", bun.In([]string{CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress})).
		Order("created_at ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list in-progress coordination tasks by agent: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for i := range items {
		rec, err := s.coordinationTaskFromModel(ctx, &items[i])
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

// ClaimTask performs an atomic compare-and-set from pending to assigned. The
// claimant's role must match the task's targeted role. Returns
// ErrCoordinationTaskStatusConflict if the row is no longer pending.
func (s *pgCoordinationTaskStore) ClaimTask(ctx context.Context, taskID uuid.UUID, role, agentIDHex, agentName string) (*CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
		Set("status = ?", CoordinationTaskStatusAssigned).
		Set("assignee_agent_id = ?", agentIDHex).
		Set("assignee_agent_name = ?", agentName).
		Set("claimed_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", taskID).
		Where("status = ?", CoordinationTaskStatusPending).
		Where("assignee_role = ?", role).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to claim coordination task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to claim coordination task: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("coordination task %s no longer pending for role %q: %w", taskID, role, ErrCoordinationTaskStatusConflict)
	}
	return s.GetTask(ctx, taskID)
}

// MarkInProgress performs an atomic CAS from assigned to in_progress.
func (s *pgCoordinationTaskStore) MarkInProgress(ctx context.Context, taskID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
		Set("status = ?", CoordinationTaskStatusInProgress).
		Set("updated_at = ?", now).
		Where("id = ?", taskID).
		Where("status = ?", CoordinationTaskStatusAssigned).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark coordination task in progress: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to mark coordination task in progress: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("coordination task %s not assigned: %w", taskID, ErrCoordinationTaskStatusConflict)
	}
	return nil
}

// RevertByAgent atomically resets all tasks owned by an agent that are still in
// flight (assigned or in_progress) back to pending, increments the dispatch
// attempt counter, and clears assignment metadata. Returns the number of rows
// reverted.
func (s *pgCoordinationTaskStore) RevertByAgent(ctx context.Context, agentIDHex string) (int, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
		Set("status = ?", CoordinationTaskStatusPending).
		Set("assignee_agent_id = ''").
		Set("assignee_agent_name = ''").
		Set("claimed_at = NULL").
		Set("dispatch_attempts = dispatch_attempts + 1").
		Set("updated_at = ?", now).
		Where("assignee_agent_id = ?", agentIDHex).
		Where("status IN (?)", bun.In([]string{CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress})).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to revert coordination tasks by agent: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to revert coordination tasks by agent: %w", err)
	}
	return int(n), nil
}

// CompleteTask performs an atomic CAS from in_progress to complete, persists the
// result, and — when a linked child investigation exists — rolls the child
// investigation's findings and evidence up into the incident's coordinating
// (parent) investigation. The parent summary is intentionally left untouched;
// the commander owns the conclusion.
func (s *pgCoordinationTaskStore) CompleteTask(ctx context.Context, taskID uuid.UUID, result map[string]any) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewUpdate().Model((*models.CoordinationTask)(nil)).
			Set("status = ?", CoordinationTaskStatusComplete).
			Set("result = ?", result).
			Set("completed_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", taskID).
			Where("status = ?", CoordinationTaskStatusInProgress).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to complete coordination task: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to complete coordination task: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("coordination task %s not in progress: %w", taskID, ErrCoordinationTaskStatusConflict)
		}

		// Roll up linked child investigation findings/evidence into the parent.
		var task models.CoordinationTask
		if err := tx.NewSelect().Model(&task).Where("id = ?", taskID).Scan(ctx); err != nil {
			return fmt.Errorf("failed to reload completed coordination task: %w", err)
		}
		if task.LinkedInvestigationID != nil {
			if err := rollupChildInvestigation(ctx, tx, *task.LinkedInvestigationID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *pgCoordinationTaskStore) FailTask(ctx context.Context, taskID uuid.UUID, reason string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
		Set("status = ?", CoordinationTaskStatusFailed).
		Set("failure_reason = ?", reason).
		Set("completed_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", taskID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to fail coordination task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to fail coordination task: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("coordination task %s not found: %w", taskID, ErrCoordinationTaskNotFound)
	}
	return nil
}

// CancelTask cancels a task and recursively cancels its child tasks. It does
// not transition any linked investigation — investigation cancellation is the
// caller's responsibility (kept here to avoid an import cycle).
func (s *pgCoordinationTaskStore) CancelTask(ctx context.Context, taskID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
		Set("status = ?", CoordinationTaskStatusCancelled).
		Set("updated_at = ?", now).
		Where("id = ?", taskID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to cancel coordination task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to cancel coordination task: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("coordination task %s not found: %w", taskID, ErrCoordinationTaskNotFound)
	}
	return s.cancelChildTasks(ctx, taskID, now)
}

func (s *pgCoordinationTaskStore) cancelChildTasks(ctx context.Context, parentTaskID uuid.UUID, now time.Time) error {
	var children []models.CoordinationTask
	if err := s.db.NewSelect().Model(&children).
		Where("parent_task_id = ?", parentTaskID).
		Where("status NOT IN (?)", bun.In([]string{CoordinationTaskStatusComplete, CoordinationTaskStatusFailed, CoordinationTaskStatusCancelled})).
		Scan(ctx); err != nil {
		return fmt.Errorf("failed to query child coordination tasks: %w", err)
	}
	for _, child := range children {
		if _, err := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
			Set("status = ?", CoordinationTaskStatusCancelled).
			Set("updated_at = ?", now).
			Where("id = ?", child.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to cancel child coordination task %s: %w", child.ID, err)
		}
		if err := s.cancelChildTasks(ctx, child.ID, now); err != nil {
			return err
		}
	}
	return nil
}

// UpdateTaskStatus performs a generic atomic CAS from any of fromStatuses to
// toStatus. A zero-rows-affected result yields ErrCoordinationTaskStatusConflict.
func (s *pgCoordinationTaskStore) UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, fromStatuses []string, toStatus string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
		Set("status = ?", toStatus).
		Set("updated_at = ?", now).
		Where("id = ?", taskID)
	if len(fromStatuses) > 0 {
		q = q.Where("status IN (?)", bun.In(fromStatuses))
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update coordination task status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update coordination task status: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("coordination task %s status conflict: %w", taskID, ErrCoordinationTaskStatusConflict)
	}
	return nil
}

// BumpDispatchAttempts increments the dispatch attempt counter, ensures the task
// is pending, and clears stale assignee fields. Used by the scheduler when a
// dispatch attempt fails to acquire an agent.
func (s *pgCoordinationTaskStore) BumpDispatchAttempts(ctx context.Context, taskID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.CoordinationTask)(nil)).
		Set("status = ?", CoordinationTaskStatusPending).
		Set("assignee_agent_id = ''").
		Set("assignee_agent_name = ''").
		Set("claimed_at = NULL").
		Set("dispatch_attempts = dispatch_attempts + 1").
		Set("updated_at = ?", now).
		Where("id = ?", taskID).
		Where("status IN (?)", bun.In([]string{CoordinationTaskStatusPending, CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress})).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to bump coordination task dispatch attempts: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to bump coordination task dispatch attempts: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("coordination task %s not found or terminal: %w", taskID, ErrCoordinationTaskStatusConflict)
	}
	return nil
}

// rollupChildInvestigation merges the child investigation's findings and
// evidence into its parent investigation (the coordinating investigation of the
// incident), deduping findings by Title and evidence by Source+Type+Content.
// The parent summary is left untouched.
func rollupChildInvestigation(ctx context.Context, db bun.IDB, childID uuid.UUID) error {
	var child models.IncidentInvestigation
	if err := db.NewSelect().Model(&child).Where("id = ?", childID).Scan(ctx); err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to load linked child investigation: %w", err)
	}

	// Resolve the parent: prefer the child's parent_investigation_id, otherwise
	// the incident's coordinating investigation.
	var parent *models.IncidentInvestigation
	if child.ParentInvestigationID != nil {
		var p models.IncidentInvestigation
		if err := db.NewSelect().Model(&p).Where("id = ?", *child.ParentInvestigationID).Scan(ctx); err == nil {
			parent = &p
		} else if !isNotFound(err) {
			return fmt.Errorf("failed to load parent investigation: %w", err)
		}
	}
	if parent == nil && child.IncidentID != nil {
		var p models.IncidentInvestigation
		err := db.NewSelect().Model(&p).
			Where("incident_id = ?", *child.IncidentID).
			Where("status = ?", IncidentInvestigationStatusCoordinating).
			Scan(ctx)
		if err == nil {
			parent = &p
		} else if !isNotFound(err) {
			return fmt.Errorf("failed to load coordinating investigation: %w", err)
		}
	}
	if parent == nil {
		return nil
	}

	mergedFindings := mergeFindings(parent.Findings, child.Findings)
	mergedEvidence := mergeEvidence(parent.Evidence, child.Evidence)

	if _, err := db.NewUpdate().Model((*models.IncidentInvestigation)(nil)).
		Set("findings = ?", mergedFindings).
		Set("evidence = ?", mergedEvidence).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", parent.ID).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to roll up child investigation into parent: %w", err)
	}
	return nil
}

func mergeFindings(existing, incoming []models.InvestigationFinding) []models.InvestigationFinding {
	seen := make(map[string]bool, len(existing))
	out := make([]models.InvestigationFinding, 0, len(existing)+len(incoming))
	for _, f := range existing {
		if !seen[f.Title] {
			seen[f.Title] = true
			out = append(out, f)
		}
	}
	for _, f := range incoming {
		if !seen[f.Title] {
			seen[f.Title] = true
			out = append(out, f)
		}
	}
	return out
}

func mergeEvidence(existing, incoming []models.EvidenceItem) []models.EvidenceItem {
	type key struct{ source, typ, content string }
	seen := make(map[key]bool, len(existing))
	out := make([]models.EvidenceItem, 0, len(existing)+len(incoming))
	for _, e := range existing {
		k := key{e.Source, e.Type, e.Content}
		if !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	for _, e := range incoming {
		k := key{e.Source, e.Type, e.Content}
		if !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out
}

func (s *pgCoordinationTaskStore) coordinationTaskFromModel(ctx context.Context, m *models.CoordinationTask) (*CoordinationTaskRecord, error) {
	if m == nil {
		return nil, nil
	}
	rec := &CoordinationTaskRecord{
		ID:                    m.ID,
		IncidentID:            m.IncidentID,
		ParentTaskID:          m.ParentTaskID,
		Kind:                  m.Kind,
		AssigneeRole:          m.AssigneeRole,
		AssigneeAgentID:       m.AssigneeAgentID,
		AssigneeAgentName:     m.AssigneeAgentName,
		Goal:                  m.Goal,
		InputContext:          m.InputContext,
		Result:                m.Result,
		ResultSchema:          m.ResultSchema,
		LinkedInvestigationID: m.LinkedInvestigationID,
		Status:                m.Status,
		Priority:              m.Priority,
		DueAt:                 m.DueAt,
		ClaimedAt:             m.ClaimedAt,
		CompletedAt:           m.CompletedAt,
		CreatedByAgentID:      m.CreatedByAgentID,
		CreatedByName:         m.CreatedByName,
		FailureReason:         m.FailureReason,
		DispatchAttempts:      m.DispatchAttempts,
		CreatedAt:             m.CreatedAt,
		UpdatedAt:             m.UpdatedAt,
	}
	if m.IncidentID != nil {
		var inc models.Incident
		if err := s.db.NewSelect().Model(&inc).Column("incident_number").Where("id = ?", *m.IncidentID).Scan(ctx); err == nil {
			rec.IncidentNumber = inc.IncidentNumber
		}
	}
	if rec.InputContext == nil {
		rec.InputContext = map[string]any{}
	}
	return rec, nil
}
