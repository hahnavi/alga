package api

import (
	"net/http"

	"github.com/google/uuid"

	"alga/rbac"
	"alga/store"
)

func (s *Server) handleAlertInvestigationAssign(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.AlertsWrite) {
		return
	}
	if s.alertInvestigationStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "alert investigation store not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, ErrorCodeValidationFailed, "missing alert investigation id")
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

	inv, err := s.alertInvestigationStore.GetAlertInvestigation(r.Context(), id)
	if err != nil {
		writeError(w, ErrorCodeNotFound, "alert investigation not found")
		return
	}

	if store.IsTerminalInvestigationStatus(inv.Status) {
		writeError(w, ErrorCodeConflict, "cannot reassign a terminal investigation")
		return
	}

	if err := s.alertInvestigationStore.SetAlertInvestigationAssignee(r.Context(), inv.ID.String(), req.AssigneeType, assigneeID); err != nil {
		writeInternalError(w, err, "failed to set assignee")
		return
	}

	if req.AssigneeType == store.InvestigationActorUser && inv.Status == store.AlertInvestigationStatusPending {
		_ = s.alertInvestigationStore.UpdateAlertInvestigationStatus(r.Context(), inv.ID.String(), store.AlertInvestigationStatusAssigned)
	} else if req.AssigneeType == store.InvestigationActorAgent && inv.Status == store.AlertInvestigationStatusAssigned {
		_ = s.alertInvestigationStore.UpdateAlertInvestigationStatus(r.Context(), inv.ID.String(), store.AlertInvestigationStatusPending)
	}

	updated, err := s.alertInvestigationStore.GetAlertInvestigation(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "failed to fetch updated investigation")
		return
	}

	s.publishInvestigationEvent(id, "investigation_updated", updated)
	s.audit(r, store.AuditInvestigationUpdated, map[string]any{
		"alert_investigation_id": id,
		"assignee_type":          req.AssigneeType,
		"assignee_id":            req.AssigneeID,
	})
	writeData(w, http.StatusOK, updated)
}
