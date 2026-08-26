// Package api: incident_coordination_tasks.go holds the operator/frontend
// (non-agent) REST handlers for incident coordination tasks. These mirror the
// agent-side dispatch_task/complete_task tool flow but are gated by RBAC and
// address tasks by incident + task id. They back the coordination/tasks route
// registered in handleIncidentRoutes.
package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"alga/rbac"
	"alga/store"
)

type incidentCoordinationTaskRequest struct {
	Kind            string         `json:"kind"`
	AssigneeRole    string         `json:"assignee_role"`
	AssigneeAgentID string         `json:"assignee_agent_id,omitempty"`
	Goal            string         `json:"goal"`
	InputContext    map[string]any `json:"input_context,omitempty"`
	ParentTaskID    string         `json:"parent_task_id,omitempty"`
	DueAt           string         `json:"due_at,omitempty"`
}

type incidentCoordinationTaskPatchRequest struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

func (s *Server) handleIncidentCoordinationTasks(w http.ResponseWriter, r *http.Request, incidentID string) {
	if s.coordinationTaskStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "coordination task store not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListIncidentCoordinationTasks(w, r, incidentID)
	case http.MethodPost:
		s.handleCreateIncidentCoordinationTask(w, r, incidentID)
	case http.MethodPatch:
		s.handlePatchIncidentCoordinationTask(w, r, incidentID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListIncidentCoordinationTasks(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	q := r.URL.Query()
	filter := map[string]any{}
	if status := strings.TrimSpace(q.Get("status")); status != "" {
		filter["status"] = status
	}
	if role := strings.TrimSpace(q.Get("assignee_role")); role != "" {
		filter["assignee_role"] = role
	}
	if parent := strings.TrimSpace(q.Get("parent_task_id")); parent != "" {
		filter["parent_task_id"] = parent
	}
	limit, skip := parseLimitSkip(r, 100)
	filter["$limit"] = limit
	filter["$skip"] = skip
	if v := strings.TrimSpace(q.Get("sort")); v != "" {
		filter["$sort"] = v
	}
	tasks, err := s.coordinationTaskStore.ListTasksByIncident(r.Context(), mustParseIncidentNumber(incidentID), filter)
	if err != nil {
		if errors.Is(err, store.ErrIncidentNotFound) {
			writeError(w, ErrorCodeNotFound, "incident not found")
			return
		}
		writeInternalError(w, err, "failed to list incident coordination tasks")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(tasks))
}

func (s *Server) handleCreateIncidentCoordinationTask(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	var req incidentCoordinationTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "goal is required")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = store.CoordinationTaskKindInvestigate
	}
	if !validOperatorCoordinationTaskKind(kind) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid kind: must be investigate, communicate, verify, or mitigate")
		return
	}
	role := strings.TrimSpace(req.AssigneeRole)
	if role == "" {
		role = store.CoordinationTaskRoleResponder
	}
	if !validOperatorCoordinationTaskRole(role) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid assignee_role: must be commander, communicator, or responder")
		return
	}

	record := &store.CoordinationTaskRecord{
		IncidentNumber:   mustParseIncidentNumber(incidentID),
		Kind:             kind,
		AssigneeRole:     role,
		AssigneeAgentID:  strings.TrimSpace(req.AssigneeAgentID),
		Goal:             goal,
		InputContext:     req.InputContext,
		CreatedByAgentID: "",
		Status:           store.CoordinationTaskStatusPending,
	}
	user := userFromContext(r.Context())
	if user != nil {
		record.CreatedByName = user.DisplayName()
		if record.CreatedByName == "" {
			record.CreatedByName = user.Email
		}
	}
	if parent := strings.TrimSpace(req.ParentTaskID); parent != "" {
		parentID, perr := uuid.Parse(parent)
		if perr != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid parent_task_id")
			return
		}
		record.ParentTaskID = &parentID
	}

	created, err := s.coordinationTaskStore.CreateTask(r.Context(), record)
	if err != nil {
		if errors.Is(err, store.ErrIncidentNotFound) {
			writeError(w, ErrorCodeNotFound, "incident not found")
			return
		}
		writeInternalError(w, err, "failed to create coordination task")
		return
	}

	s.publishIncidentEvent("coordination_task_created", map[string]any{
		"incident_number": incidentID,
		"task_id":         created.ID.String(),
		"kind":            created.Kind,
		"assignee_role":   created.AssigneeRole,
	})
	s.audit(r, store.AuditIncidentUpdated, map[string]any{
		"incident_number": incidentID,
		"action":          "create_coordination_task",
		"task_id":         created.ID.String(),
		"kind":            created.Kind,
		"assignee_role":   created.AssigneeRole,
	})
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handlePatchIncidentCoordinationTask(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	var req incidentCoordinationTaskPatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	taskIDStr := strings.TrimSpace(req.TaskID)
	if taskIDStr == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "task_id is required")
		return
	}
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid task_id")
		return
	}
	// v1: only cancellation is supported via the operator route. Status
	// transitions that require role/assignee authority (claim/complete) go
	// through the agent tool flow.
	status := strings.TrimSpace(strings.ToLower(req.Status))
	if status == "" {
		status = store.CoordinationTaskStatusCancelled
	}
	if status != store.CoordinationTaskStatusCancelled {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "only status=cancelled is supported")
		return
	}
	if err := s.coordinationTaskStore.CancelTask(r.Context(), taskID); err != nil {
		if errors.Is(err, store.ErrCoordinationTaskNotFound) {
			writeError(w, ErrorCodeNotFound, "coordination task not found")
			return
		}
		writeInternalError(w, err, "failed to cancel coordination task")
		return
	}
	// coordination_task_updated is kept for pre-existing listeners; the
	// dedicated cancelled event matches the dedicated task-list filter so the
	// UI can drop only the cancelled row without treating it as an update.
	for _, eventType := range []string{"coordination_task_updated", "coordination_task_cancelled"} {
		s.publishIncidentEvent(eventType, map[string]any{
			"incident_number": incidentID,
			"task_id":         taskIDStr,
			"status":          store.CoordinationTaskStatusCancelled,
		})
	}
	s.audit(r, store.AuditIncidentUpdated, map[string]any{
		"incident_number": incidentID,
		"action":          "cancel_coordination_task",
		"task_id":         taskIDStr,
	})
	writeStatus(w, "cancelled")
}

// validOperatorCoordinationTaskKind reports whether a kind is dispatchable from
// the operator route. "synthesize" is excluded — it is embodied by the
// synthesize_findings agent tool, not a dispatchable task.
func validOperatorCoordinationTaskKind(kind string) bool {
	switch kind {
	case store.CoordinationTaskKindInvestigate,
		store.CoordinationTaskKindCommunicate,
		store.CoordinationTaskKindVerify,
		store.CoordinationTaskKindMitigate:
		return true
	}
	return false
}

func validOperatorCoordinationTaskRole(role string) bool {
	switch role {
	case store.CoordinationTaskRoleCommander,
		store.CoordinationTaskRoleCommunicator,
		store.CoordinationTaskRoleResponder:
		return true
	}
	return false
}
