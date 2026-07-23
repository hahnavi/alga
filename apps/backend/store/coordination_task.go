package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/coordinationtask"
	"alga/ent/incident"
	"alga/ent/incidentinvestigation"
	"alga/ent/predicate"
	entschema "alga/ent/schema"
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

func newPGCoordinationTaskStore(client *ent.Client) CoordinationTaskStore {
	return &pgCoordinationTaskStore{pgStoreBase{client: client}}
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

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin coordination task transaction: %w", err)
	}
	defer rollbackTx(tx)

	// Resolve the incident when an incident number is provided.
	var inc *ent.Incident
	if record.IncidentNumber != 0 {
		inc, err = tx.Client().Incident.Query().
			Where(incident.IncidentNumber(record.IncidentNumber), incident.DeletedAtIsNil()).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
			}
			return nil, fmt.Errorf("failed to find incident for coordination task: %w", err)
		}
	}

	b := tx.Client().CoordinationTask.Create().
		SetKind(record.Kind).
		SetAssigneeRole(record.AssigneeRole).
		SetAssigneeAgentID(record.AssigneeAgentID).
		SetAssigneeAgentName(record.AssigneeAgentName).
		SetGoal(record.Goal).
		SetInputContext(inputContext).
		SetStatus(status).
		SetPriority(record.Priority).
		SetCreatedByAgentID(record.CreatedByAgentID).
		SetCreatedByName(record.CreatedByName).
		SetFailureReason(record.FailureReason).
		SetDispatchAttempts(record.DispatchAttempts).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if inc != nil {
		b.SetIncidentID(inc.ID)
	}
	if record.ParentTaskID != nil {
		b.SetParentTaskID(*record.ParentTaskID)
	}
	if record.Result != nil {
		b.SetResult(record.Result)
	}
	if record.ResultSchema != nil {
		b.SetResultSchema(record.ResultSchema)
	}
	if record.DueAt != nil {
		b.SetDueAt(*record.DueAt)
	}
	if record.ClaimedAt != nil {
		b.SetClaimedAt(*record.ClaimedAt)
	}
	if record.CompletedAt != nil {
		b.SetCompletedAt(*record.CompletedAt)
	}

	// Auto-link a child investigation for top-level investigate tasks.
	if record.Kind == CoordinationTaskKindInvestigate &&
		record.ParentTaskID == nil &&
		inc != nil {
		parent, qerr := tx.Client().IncidentInvestigation.Query().
			Where(
				incidentinvestigation.HasIncidentWith(incident.ID(inc.ID)),
				incidentinvestigation.StatusEQ(IncidentInvestigationStatusCoordinating),
			).
			Only(ctx)
		if qerr == nil && parent != nil {
			childID := fmt.Sprintf("incident_inv_%d_%s", record.IncidentNumber, uuid.NewString()[:8])
			child, cerr := tx.Client().IncidentInvestigation.Create().
				SetIncidentInvestigationID(childID).
				SetIncidentID(inc.ID).
				SetStatus(IncidentInvestigationStatusPending).
				SetParentInvestigationID(parent.ID).
				SetCreatedAt(now).
				SetUpdatedAt(now).
				Save(ctx)
			if cerr != nil {
				return nil, fmt.Errorf("failed to create child incident investigation: %w", cerr)
			}
			b.SetLinkedInvestigationID(child.ID)
		} else if qerr != nil && !ent.IsNotFound(qerr) {
			return nil, fmt.Errorf("failed to query coordinating investigation: %w", qerr)
		}
		// If no coordinating investigation found, proceed without a link.
	}

	created, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create coordination task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit coordination task transaction: %w", err)
	}

	return s.GetTask(ctx, created.ID)
}

func (s *pgCoordinationTaskStore) GetTask(ctx context.Context, taskID uuid.UUID) (*CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	t, err := s.client.CoordinationTask.Query().
		Where(coordinationtask.ID(taskID)).
		WithIncident().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("coordination task not found: %w", ErrCoordinationTaskNotFound)
		}
		return nil, fmt.Errorf("failed to query coordination task: %w", err)
	}
	return coordinationTaskFromEnt(t), nil
}

func (s *pgCoordinationTaskStore) ListTasksByIncident(ctx context.Context, incidentNumber int64, filter map[string]any) ([]CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if filter == nil {
		filter = map[string]any{}
	}
	limit, skip := extractLimitSkip(filter, 100)

	inc, err := s.client.Incident.Query().
		Where(incident.IncidentNumber(incidentNumber), incident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident for coordination task listing: %w", err)
	}

	preds := []predicate.CoordinationTask{
		coordinationtask.HasIncidentWith(incident.ID(inc.ID)),
	}
	if v, ok := filter["parent_task_id"]; ok {
		switch p := v.(type) {
		case uuid.UUID:
			preds = append(preds, coordinationtask.ParentTaskID(p))
		case *uuid.UUID:
			if p != nil {
				preds = append(preds, coordinationtask.ParentTaskID(*p))
			}
		case string:
			if pid, perr := uuid.Parse(p); perr == nil {
				preds = append(preds, coordinationtask.ParentTaskID(pid))
			}
		}
	}
	if v, ok := filter["status"].(string); ok && v != "" {
		preds = append(preds, coordinationtask.StatusEQ(v))
	}
	if v, ok := filter["assignee_role"].(string); ok && v != "" {
		preds = append(preds, coordinationtask.AssigneeRoleEQ(v))
	}

	sortField, _ := filter["$sort"].(string)
	q := s.client.CoordinationTask.Query().Where(preds...).WithIncident()
	switch sortField {
	case "priority":
		q.Order(coordinationtask.ByPriority(), coordinationtask.ByCreatedAt())
	case "-priority":
		q.Order(coordinationtask.ByPriority(sql.OrderDesc()), coordinationtask.ByCreatedAt())
	default:
		q.Order(ent.Desc(coordinationtask.FieldCreatedAt))
	}
	q.Limit(limit).Offset(skip)

	items, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list coordination tasks: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for _, item := range items {
		rec := coordinationTaskFromEnt(item)
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

	items, err := s.client.CoordinationTask.Query().
		Where(
			coordinationtask.StatusEQ(CoordinationTaskStatusPending),
			coordinationtask.AssigneeRoleEQ(role),
		).
		WithIncident().
		Order(
			coordinationtask.ByPriority(sql.OrderDesc()),
			coordinationtask.ByCreatedAt(),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending coordination tasks: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *coordinationTaskFromEnt(item))
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

	items, err := s.client.CoordinationTask.Query().
		Where(
			coordinationtask.StatusEQ(CoordinationTaskStatusInProgress),
			coordinationtask.DueAtNotNil(),
			coordinationtask.DueAtLT(now),
		).
		WithIncident().
		Order(coordinationtask.ByDueAt()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list overdue coordination tasks: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *coordinationTaskFromEnt(item))
	}
	return records, nil
}

func (s *pgCoordinationTaskStore) ListInProgressByAgent(ctx context.Context, agentIDHex string) ([]CoordinationTaskRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	items, err := s.client.CoordinationTask.Query().
		Where(
			coordinationtask.AssigneeAgentID(agentIDHex),
			coordinationtask.StatusIn(CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress),
		).
		WithIncident().
		Order(coordinationtask.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list in-progress coordination tasks by agent: %w", err)
	}

	records := make([]CoordinationTaskRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *coordinationTaskFromEnt(item))
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
	n, err := s.client.CoordinationTask.Update().
		Where(
			coordinationtask.ID(taskID),
			coordinationtask.StatusEQ(CoordinationTaskStatusPending),
			coordinationtask.AssigneeRoleEQ(role),
		).
		SetStatus(CoordinationTaskStatusAssigned).
		SetAssigneeAgentID(agentIDHex).
		SetAssigneeAgentName(agentName).
		SetClaimedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
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
	n, err := s.client.CoordinationTask.Update().
		Where(
			coordinationtask.ID(taskID),
			coordinationtask.StatusEQ(CoordinationTaskStatusAssigned),
		).
		SetStatus(CoordinationTaskStatusInProgress).
		SetUpdatedAt(now).
		Save(ctx)
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
	n, err := s.client.CoordinationTask.Update().
		Where(
			coordinationtask.AssigneeAgentID(agentIDHex),
			coordinationtask.StatusIn(CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress),
		).
		SetStatus(CoordinationTaskStatusPending).
		SetAssigneeAgentID("").
		SetAssigneeAgentName("").
		ClearClaimedAt().
		AddDispatchAttempts(1).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to revert coordination tasks by agent: %w", err)
	}
	return n, nil
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

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin coordination task completion transaction: %w", err)
	}
	defer rollbackTx(tx)

	n, err := tx.Client().CoordinationTask.Update().
		Where(
			coordinationtask.ID(taskID),
			coordinationtask.StatusEQ(CoordinationTaskStatusInProgress),
		).
		SetStatus(CoordinationTaskStatusComplete).
		SetResult(result).
		SetCompletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to complete coordination task: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("coordination task %s not in progress: %w", taskID, ErrCoordinationTaskStatusConflict)
	}

	// Roll up linked child investigation findings/evidence into the parent.
	task, err := tx.Client().CoordinationTask.Query().
		Where(coordinationtask.ID(taskID)).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to reload completed coordination task: %w", err)
	}
	if task.LinkedInvestigationID != nil {
		if err := rollupChildInvestigation(ctx, tx.Client(), *task.LinkedInvestigationID); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit coordination task completion: %w", err)
	}
	return nil
}

func (s *pgCoordinationTaskStore) FailTask(ctx context.Context, taskID uuid.UUID, reason string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	n, err := s.client.CoordinationTask.Update().
		Where(coordinationtask.ID(taskID)).
		SetStatus(CoordinationTaskStatusFailed).
		SetFailureReason(reason).
		SetCompletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
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
	n, err := s.client.CoordinationTask.Update().
		Where(coordinationtask.ID(taskID)).
		SetStatus(CoordinationTaskStatusCancelled).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to cancel coordination task: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("coordination task %s not found: %w", taskID, ErrCoordinationTaskNotFound)
	}
	return s.cancelChildTasks(ctx, taskID, now)
}

func (s *pgCoordinationTaskStore) cancelChildTasks(ctx context.Context, parentTaskID uuid.UUID, now time.Time) error {
	children, err := s.client.CoordinationTask.Query().
		Where(
			coordinationtask.ParentTaskID(parentTaskID),
			coordinationtask.StatusNotIn(
				CoordinationTaskStatusComplete,
				CoordinationTaskStatusFailed,
				CoordinationTaskStatusCancelled,
			),
		).
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query child coordination tasks: %w", err)
	}
	for _, child := range children {
		if _, err := s.client.CoordinationTask.Update().
			Where(coordinationtask.ID(child.ID)).
			SetStatus(CoordinationTaskStatusCancelled).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
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
	q := s.client.CoordinationTask.Update().
		Where(coordinationtask.ID(taskID)).
		SetStatus(toStatus).
		SetUpdatedAt(now)
	if len(fromStatuses) > 0 {
		q.Where(coordinationtask.StatusIn(fromStatuses...))
	}
	n, err := q.Save(ctx)
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
	n, err := s.client.CoordinationTask.Update().
		Where(
			coordinationtask.ID(taskID),
			coordinationtask.StatusIn(
				CoordinationTaskStatusPending,
				CoordinationTaskStatusAssigned,
				CoordinationTaskStatusInProgress,
			),
		).
		SetStatus(CoordinationTaskStatusPending).
		SetAssigneeAgentID("").
		SetAssigneeAgentName("").
		ClearClaimedAt().
		AddDispatchAttempts(1).
		SetUpdatedAt(now).
		Save(ctx)
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
func rollupChildInvestigation(ctx context.Context, client *ent.Client, childID uuid.UUID) error {
	child, err := client.IncidentInvestigation.Get(ctx, childID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to load linked child investigation: %w", err)
	}

	// Resolve the parent: prefer the child's parent_investigation_id, otherwise
	// the incident's coordinating investigation.
	var parent *ent.IncidentInvestigation
	if child.ParentInvestigationID != nil {
		parent, err = client.IncidentInvestigation.Get(ctx, *child.ParentInvestigationID)
		if err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("failed to load parent investigation: %w", err)
		}
	}
	if parent == nil && child.IncidentID != nil {
		parent, err = client.IncidentInvestigation.Query().
			Where(
				incidentinvestigation.HasIncidentWith(incident.ID(*child.IncidentID)),
				incidentinvestigation.StatusEQ(IncidentInvestigationStatusCoordinating),
			).
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("failed to load coordinating investigation: %w", err)
		}
	}
	if parent == nil {
		return nil
	}

	mergedFindings := mergeFindings(parent.Findings, child.Findings)
	mergedEvidence := mergeEvidence(parent.Evidence, child.Evidence)

	upd := client.IncidentInvestigation.UpdateOneID(parent.ID).
		SetFindings(mergedFindings).
		SetEvidence(mergedEvidence).
		SetUpdatedAt(time.Now().UTC())
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("failed to roll up child investigation into parent: %w", err)
	}
	return nil
}

func mergeFindings(existing, incoming []entschema.InvestigationFinding) []entschema.InvestigationFinding {
	seen := make(map[string]bool, len(existing))
	out := make([]entschema.InvestigationFinding, 0, len(existing)+len(incoming))
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

func mergeEvidence(existing, incoming []entschema.EvidenceItem) []entschema.EvidenceItem {
	type key struct{ source, typ, content string }
	seen := make(map[key]bool, len(existing))
	out := make([]entschema.EvidenceItem, 0, len(existing)+len(incoming))
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

func coordinationTaskFromEnt(e *ent.CoordinationTask) *CoordinationTaskRecord {
	if e == nil {
		return nil
	}
	rec := &CoordinationTaskRecord{
		ID:                    e.ID,
		IncidentID:            e.IncidentID,
		ParentTaskID:          e.ParentTaskID,
		Kind:                  e.Kind,
		AssigneeRole:          e.AssigneeRole,
		AssigneeAgentID:       e.AssigneeAgentID,
		AssigneeAgentName:     e.AssigneeAgentName,
		Goal:                  e.Goal,
		InputContext:          e.InputContext,
		Result:                e.Result,
		ResultSchema:          e.ResultSchema,
		LinkedInvestigationID: e.LinkedInvestigationID,
		Status:                e.Status,
		Priority:              e.Priority,
		DueAt:                 e.DueAt,
		ClaimedAt:             e.ClaimedAt,
		CompletedAt:           e.CompletedAt,
		CreatedByAgentID:      e.CreatedByAgentID,
		CreatedByName:         e.CreatedByName,
		FailureReason:         e.FailureReason,
		DispatchAttempts:      e.DispatchAttempts,
		CreatedAt:             e.CreatedAt,
		UpdatedAt:             e.UpdatedAt,
	}
	if e.Edges.Incident != nil {
		rec.IncidentNumber = e.Edges.Incident.IncidentNumber
	}
	if rec.InputContext == nil {
		rec.InputContext = map[string]any{}
	}
	return rec
}
