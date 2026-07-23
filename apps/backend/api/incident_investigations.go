// Package api: incident_investigations.go covers incident investigation
// handlers — listing investigations for an incident and creating a new one.
package api

import (
	"net/http"

	"alga/rbac"
	"alga/store"

	"github.com/google/uuid"
)

func (s *Server) handleListIncidentInvestigations(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireIncidentInvestigationStore(w) {
		return
	}

	records, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(r.Context(), mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to list incident investigations")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(records))
}

func (s *Server) handleCreateIncidentInvestigation(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireIncidentInvestigationStore(w) {
		return
	}

	var req struct {
		AssigneeType string `json:"assignee_type,omitempty"`
		AssigneeID   string `json:"assignee_id,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	_, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}

	investigationID := uuid.New().String()

	record := store.IncidentInvestigationRecord{
		IncidentInvestigationID: investigationID,
		IncidentNumber:          mustParseIncidentNumber(incidentID),
		Status:                  "pending",
	}
	if req.AssigneeType != "" {
		record.AssigneeType = req.AssigneeType
	}
	if req.AssigneeID != "" {
		if uid, err := uuid.Parse(req.AssigneeID); err == nil {
			record.AssigneeID = &uid
		}
	}
	created, err := s.incidentInvestigationStore.CreateIncidentInvestigation(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create investigation")
		return
	}

	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	s.publishInvestigationEvent(investigationID, "investigation_created", created)
	s.audit(r, store.AuditInvestigationCreated, map[string]any{
		"incident_number":  incidentID,
		"investigation_id": investigationID,
	})
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handleIncidentInvestigationAssign(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentInvestigationStore(w) {
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, ErrorCodeValidationFailed, "missing incident investigation id")
		return
	}

	var req struct {
		AssigneeType string `json:"assignee_type"`
		AssigneeID   string `json:"assignee_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.AssigneeType != store.InvestigationActorUser && req.AssigneeType != store.InvestigationActorAgent {
		writeErrorStatus(w, http.StatusUnprocessableEntity, ErrorCodeValidationFailed, "assignee_type must be \"user\" or \"agent\"")
		return
	}

	var assigneeID *uuid.UUID
	if req.AssigneeType == store.InvestigationActorUser {
		if req.AssigneeID == "" {
			writeErrorStatus(w, http.StatusUnprocessableEntity, ErrorCodeValidationFailed, "assignee_id is required when assignee_type is \"user\"")
			return
		}
		uid, err := uuid.Parse(req.AssigneeID)
		if err != nil {
			writeErrorStatus(w, http.StatusUnprocessableEntity, ErrorCodeValidationFailed, "invalid assignee_id")
			return
		}
		if s.userStore == nil {
			writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "user store not available")
			return
		}
		if _, err := s.userStore.GetByID(uid); err != nil {
			writeError(w, ErrorCodeNotFound, "user not found")
			return
		}
		assigneeID = &uid
	}

	inv, err := s.incidentInvestigationStore.GetIncidentInvestigation(r.Context(), id)
	if err != nil {
		writeError(w, ErrorCodeNotFound, "incident investigation not found")
		return
	}

	if inv.Status == store.IncidentInvestigationStatusComplete || inv.Status == store.IncidentInvestigationStatusCancelled {
		writeError(w, ErrorCodeConflict, "cannot reassign a terminal investigation")
		return
	}

	if err := s.incidentInvestigationStore.SetIncidentInvestigationAssignee(r.Context(), id, req.AssigneeType, assigneeID); err != nil {
		writeInternalError(w, err, "failed to set assignee")
		return
	}

	if req.AssigneeType == store.InvestigationActorUser && inv.Status == store.IncidentInvestigationStatusPending {
		_ = s.incidentInvestigationStore.UpdateIncidentInvestigationStatus(r.Context(), id, store.IncidentInvestigationStatusAssigned)
	} else if req.AssigneeType == store.InvestigationActorAgent && inv.Status == store.IncidentInvestigationStatusAssigned {
		_ = s.incidentInvestigationStore.UpdateIncidentInvestigationStatus(r.Context(), id, store.IncidentInvestigationStatusPending)
	}

	updated, err := s.incidentInvestigationStore.GetIncidentInvestigation(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "failed to fetch updated investigation")
		return
	}

	s.publishInvestigationEvent(id, "investigation_updated", updated)
	s.audit(r, store.AuditInvestigationUpdated, map[string]any{
		"incident_investigation_id": id,
		"assignee_type":             req.AssigneeType,
		"assignee_id":               req.AssigneeID,
	})
	writeData(w, http.StatusOK, updated)
}
