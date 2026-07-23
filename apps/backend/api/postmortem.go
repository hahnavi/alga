package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"alga/logger"
	"alga/rabbitmq"
	"alga/rbac"
	"alga/store"

	"github.com/google/uuid"
)

func (s *Server) handlePostMortemRoutes(w http.ResponseWriter, r *http.Request, incidentID string) {
	suffix := extractPostMortemSuffix(r)
	if suffix == "" || suffix == "post-mortem" {
		switch r.Method {
		case http.MethodGet:
			s.getPostMortem(w, r, incidentID)
		case http.MethodPost:
			s.createPostMortem(w, r, incidentID)
		case http.MethodPatch:
			s.updatePostMortem(w, r, incidentID)
		case http.MethodDelete:
			s.deletePostMortem(w, r, incidentID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if strings.HasSuffix(suffix, "/submit-review") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		s.updatePostMortemStatus(w, r, incidentID, "in_review")
		return
	}

	if strings.HasSuffix(suffix, "/approve") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		s.updatePostMortemStatusWithApprover(w, r, incidentID, "approved")
		return
	}

	if strings.HasSuffix(suffix, "/publish") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		s.updatePostMortemStatus(w, r, incidentID, "published")
		return
	}

	if suffix == "post-mortem/action-items" || strings.HasSuffix(suffix, "/post-mortem/action-items") {
		parts := strings.Split(suffix, "/")
		s.handleActionItemRoutes(w, r, incidentID, parts)
		return
	}

	if idx := strings.Index(suffix, "/action-items/"); idx != -1 {
		rest := suffix[idx+len("/action-items/"):]
		trimmedSuffix := suffix[:idx]
		parts := strings.Split(trimmedSuffix, "/")
		parts = append(parts, "action-items", rest)
		s.handleActionItemRoutes(w, r, incidentID, parts)
		return
	}
	writeError(w, ErrorCodeNotFound, "not found")
}

func extractPostMortemSuffix(r *http.Request) string {
	path := r.URL.Path
	idx := strings.Index(path, "/post-mortem")
	if idx == -1 {
		return ""
	}
	return path[idx+1:]
}

func (s *Server) requirePostMortemStore(w http.ResponseWriter) bool {
	return s.requireStore(w, s.postmortemStore, "post-mortem store")
}

func (s *Server) requireActionItemStore(w http.ResponseWriter) bool {
	return s.requireStore(w, s.actionItemStore, "action item store")
}

var errPostMortemIncidentStoreNotConfigured = errors.New("incident store not configured")

func (s *Server) resolveIncidentUUIDForPostMortem(ctx context.Context, incidentID string) (uuid.UUID, bool, error) {
	if uid, err := uuid.Parse(incidentID); err == nil {
		return uid, true, nil
	}
	incidentNumber, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		return uuid.Nil, false, nil
	}
	if s.incidentStore == nil {
		return uuid.Nil, false, errPostMortemIncidentStoreNotConfigured
	}
	inc, err := s.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil {
		if errors.Is(err, store.ErrIncidentNotFound) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, fmt.Errorf("get incident: %w", err)
	}
	if inc == nil {
		return uuid.Nil, false, nil
	}
	return inc.ID, true, nil
}

func writePostMortemIncidentResolveError(w http.ResponseWriter, err error) {
	if errors.Is(err, errPostMortemIncidentStoreNotConfigured) {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "incident store not configured")
		return
	}
	writeInternalError(w, err, "failed to resolve incident")
}

func writePostMortemResolveError(w http.ResponseWriter, err error) {
	if errors.Is(err, errPostMortemIncidentStoreNotConfigured) {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	writeInternalError(w, err, "failed to resolve post-mortem")
}

func (s *Server) getPostMortem(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsRead) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	uid, ok, err := s.resolveIncidentUUIDForPostMortem(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeData(w, http.StatusOK, nil)
		return
	}

	record, err := s.postmortemStore.GetByIncidentID(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if record == nil {
		writeData(w, http.StatusOK, nil)
		return
	}
	writeData(w, http.StatusOK, record)
}

func (s *Server) createPostMortem(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsWrite) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	var req struct {
		Title               string           `json:"title"`
		Summary             string           `json:"summary"`
		Timeline            []map[string]any `json:"timeline"`
		RootCause           string           `json:"root_cause"`
		ContributingFactors []string         `json:"contributing_factors"`
		Impact              string           `json:"impact"`
		LessonsLearned      string           `json:"lessons_learned"`
		WhatWentWell        string           `json:"what_went_well"`
		WhatWentWrong       string           `json:"what_went_wrong"`
		BlamelessConfirmed  bool             `json:"blameless_confirmed"`
		BlamelessNotes      string           `json:"blameless_notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	uid, ok, err := s.resolveIncidentUUIDForPostMortem(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	record := &store.PostMortemRecord{
		IncidentID:          uid,
		Title:               strings.TrimSpace(req.Title),
		Status:              "draft",
		Summary:             req.Summary,
		Timeline:            req.Timeline,
		RootCause:           req.RootCause,
		ContributingFactors: req.ContributingFactors,
		Impact:              req.Impact,
		LessonsLearned:      req.LessonsLearned,
		WhatWentWell:        req.WhatWentWell,
		WhatWentWrong:       req.WhatWentWrong,
		BlamelessConfirmed:  req.BlamelessConfirmed,
		BlamelessNotes:      req.BlamelessNotes,
	}

	created, err := s.postmortemStore.Create(r.Context(), record)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to create post-mortem", "component", "api", "incident_number", incidentID, "error", err)
		writeInternalError(w, err, "failed to create post-mortem")
		return
	}

	logger.InfoCtx(r.Context(), "post-mortem created", "component", "api", "incident_number", incidentID, "pm_id", created.ID.String())
	s.addIncidentTimeline(r, incidentID, "postmortem_created", "Post-mortem created")
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	s.audit(r, store.AuditPostMortemCreated, map[string]any{
		"incident_number": incidentID,
		"pm_id":           created.ID.String(),
	})
	writeData(w, http.StatusCreated, created)
}

func (s *Server) updatePostMortem(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsWrite) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	uid, ok, err := s.resolveIncidentUUIDForPostMortem(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	var req struct {
		Title               *string          `json:"title"`
		Summary             *string          `json:"summary"`
		Timeline            []map[string]any `json:"timeline"`
		RootCause           *string          `json:"root_cause"`
		ContributingFactors []string         `json:"contributing_factors"`
		Impact              *string          `json:"impact"`
		LessonsLearned      *string          `json:"lessons_learned"`
		WhatWentWell        *string          `json:"what_went_well"`
		WhatWentWrong       *string          `json:"what_went_wrong"`
		BlamelessConfirmed  *bool            `json:"blameless_confirmed"`
		BlamelessNotes      *string          `json:"blameless_notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Title != nil {
		existing.Title = strings.TrimSpace(*req.Title)
	}
	if req.Summary != nil {
		existing.Summary = *req.Summary
	}
	if req.Timeline != nil {
		existing.Timeline = req.Timeline
	}
	if req.RootCause != nil {
		existing.RootCause = *req.RootCause
	}
	if req.ContributingFactors != nil {
		existing.ContributingFactors = req.ContributingFactors
	}
	if req.Impact != nil {
		existing.Impact = *req.Impact
	}
	if req.LessonsLearned != nil {
		existing.LessonsLearned = *req.LessonsLearned
	}
	if req.WhatWentWell != nil {
		existing.WhatWentWell = *req.WhatWentWell
	}
	if req.WhatWentWrong != nil {
		existing.WhatWentWrong = *req.WhatWentWrong
	}
	if req.BlamelessConfirmed != nil {
		existing.BlamelessConfirmed = *req.BlamelessConfirmed
	}
	if req.BlamelessNotes != nil {
		existing.BlamelessNotes = *req.BlamelessNotes
	}

	updated, err := s.postmortemStore.Update(r.Context(), existing.ID, existing)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to update post-mortem", "component", "api", "incident_number", incidentID, "error", err)
		writeInternalError(w, err, "failed to update post-mortem")
		return
	}

	logger.InfoCtx(r.Context(), "post-mortem updated", "component", "api", "incident_number", incidentID, "pm_id", existing.ID.String())
	s.audit(r, store.AuditPostMortemUpdated, map[string]any{
		"incident_number": incidentID,
		"pm_id":           existing.ID.String(),
	})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) deletePostMortem(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsDelete) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}
	if !s.requireActionItemStore(w) {
		return
	}

	uid, ok, err := s.resolveIncidentUUIDForPostMortem(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	if err := s.actionItemStore.DeleteByPostMortemID(r.Context(), existing.ID); err != nil {
		writeInternalError(w, err, "failed to delete action items")
		return
	}

	if err := s.postmortemStore.Delete(r.Context(), existing.ID); err != nil {
		writeInternalError(w, err, "failed to delete post-mortem")
		return
	}

	logger.InfoCtx(r.Context(), "post-mortem deleted", "component", "api", "incident_number", incidentID, "pm_id", existing.ID.String())
	s.addIncidentTimeline(r, incidentID, "postmortem_deleted", "Post-mortem deleted")
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	s.audit(r, store.AuditPostMortemDeleted, map[string]any{
		"incident_number": incidentID,
		"pm_id":           existing.ID.String(),
	})

	writeStatus(w, "deleted")
}

var validPostMortemTransitions = map[string]map[string]bool{
	"draft":     {"in_review": true},
	"in_review": {"draft": true, "approved": true},
	"approved":  {"in_review": true, "published": true},
	"published": {},
}

func isValidPostMortemTransition(from, to string) bool {
	allowed, ok := validPostMortemTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

func (s *Server) updatePostMortemStatus(w http.ResponseWriter, r *http.Request, incidentID, status string) {
	if !s.checkPermission(w, r, rbac.PostMortemsWrite) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	uid, ok, err := s.resolveIncidentUUIDForPostMortem(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	if !isValidPostMortemTransition(existing.Status, status) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, fmt.Sprintf("cannot transition post-mortem from %s to %s", existing.Status, status))
		return
	}

	updated, err := s.postmortemStore.UpdateStatus(r.Context(), existing.ID, status, nil)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to update post-mortem status", "component", "api", "incident_number", incidentID, "status", status, "error", err)
		writeInternalError(w, err, "failed to update post-mortem status")
		return
	}

	logger.InfoCtx(r.Context(), "post-mortem status changed", "component", "api", "incident_number", incidentID, "pm_id", existing.ID.String(), "status", status)
	s.addIncidentTimeline(r, incidentID, "postmortem_"+status, "Post-mortem status changed to "+status)
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	s.audit(r, store.AuditPostMortemStatusChanged, map[string]any{
		"incident_number": incidentID,
		"pm_id":           existing.ID.String(),
		"status":          status,
	})

	if status == "in_review" && s.rabbitmqPublisher != nil {
		incidentNumber, parseErr := strconv.ParseInt(incidentID, 10, 64)
		if parseErr != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid incident number")
			return
		}
		inc, err := s.incidentStore.GetIncident(r.Context(), incidentNumber)
		if err == nil && inc != nil && inc.CommanderID != nil {
			_ = s.rabbitmqPublisher.PublishNotificationDispatch(r.Context(), rabbitmq.NotificationDispatchMessage{
				UserID:           inc.CommanderID.String(),
				IncidentNumber:   incidentNumber,
				NotificationType: "post_mortem_review_requested",
				Title:            "Post-mortem review requested",
				Message:          fmt.Sprintf("Post-mortem for incident %s has been submitted for review", incidentID),
				ResourceType:     "incident",
				ResourceID:       incidentID,
			})
		}
	}
	writeData(w, http.StatusOK, updated)
}

func (s *Server) updatePostMortemStatusWithApprover(w http.ResponseWriter, r *http.Request, incidentID, status string) {
	if !s.checkPermission(w, r, rbac.PostMortemsWrite) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	uid, ok, err := s.resolveIncidentUUIDForPostMortem(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	if !isValidPostMortemTransition(existing.Status, status) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, fmt.Sprintf("cannot transition post-mortem from %s to %s", existing.Status, status))
		return
	}

	user := userFromContext(r.Context())
	var approverID *uuid.UUID
	if user != nil {
		id := user.ID
		approverID = &id
	}

	updated, err := s.postmortemStore.UpdateStatus(r.Context(), existing.ID, status, approverID)
	if err != nil {
		writeInternalError(w, err, "failed to approve post-mortem")
		return
	}

	s.addIncidentTimeline(r, incidentID, "postmortem_approved", "Post-mortem approved")
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	s.audit(r, store.AuditPostMortemStatusChanged, map[string]any{
		"incident_number": incidentID,
		"pm_id":           existing.ID.String(),
		"status":          status,
		"approved_by":     approverID,
	})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleActionItemRoutes(w http.ResponseWriter, r *http.Request, incidentID string, parts []string) {
	actionPartIdx := -1
	for i, p := range parts {
		if p == "action-items" {
			actionPartIdx = i
			break
		}
	}
	if actionPartIdx == -1 {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	afterActionItems := parts[actionPartIdx+1:]

	if len(afterActionItems) == 0 || (len(afterActionItems) == 1 && afterActionItems[0] == "") {
		switch r.Method {
		case http.MethodGet:
			s.listActionItems(w, r, incidentID)
		case http.MethodPost:
			s.createActionItem(w, r, incidentID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	aiID := afterActionItems[0]
	if aiID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing action item id")
		return
	}

	switch r.Method {
	case http.MethodPatch:
		s.updateActionItem(w, r, incidentID, aiID)
	case http.MethodDelete:
		s.deleteActionItem(w, r, incidentID, aiID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) resolvePostMortemID(ctx context.Context, incidentID string) (uuid.UUID, error) {
	uid, ok, err := s.resolveIncidentUUIDForPostMortem(ctx, incidentID)
	if err != nil || !ok {
		return uuid.Nil, err
	}
	pm, err := s.postmortemStore.GetByIncidentID(ctx, uid)
	if err != nil {
		return uuid.Nil, err
	}
	if pm == nil {
		return uuid.Nil, nil
	}
	return pm.ID, nil
}

func (s *Server) listActionItems(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsRead) {
		return
	}
	if !s.requireActionItemStore(w) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	pmID, err := s.resolvePostMortemID(r.Context(), incidentID)
	if err != nil {
		writePostMortemResolveError(w, err)
		return
	}
	if pmID == uuid.Nil {
		writeData(w, http.StatusOK, []store.ActionItemRecord{})
		return
	}

	items, err := s.actionItemStore.ListByPostMortem(r.Context(), pmID)
	if err != nil {
		writeInternalError(w, err, "failed to list action items")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(items))
}

func (s *Server) createActionItem(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsWrite) {
		return
	}
	if !s.requireActionItemStore(w) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	pmID, err := s.resolvePostMortemID(r.Context(), incidentID)
	if err != nil {
		writePostMortemResolveError(w, err)
		return
	}
	if pmID == uuid.Nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	var req struct {
		Description  string  `json:"description"`
		AssigneeID   *string `json:"assignee_id,omitempty"`
		Priority     string  `json:"priority,omitempty"`
		DueDate      *string `json:"due_date,omitempty"`
		Type         string  `json:"type"`
		AssigneeName string  `json:"assignee_name,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "description is required")
		return
	}

	record := &store.ActionItemRecord{
		PostMortemID: pmID,
		Description:  req.Description,
		Status:       "detected",
		Priority:     req.Priority,
	}
	record.Type = req.Type
	record.AssigneeName = req.AssigneeName

	if req.AssigneeID != nil {
		if uid, err := uuid.Parse(*req.AssigneeID); err == nil {
			record.AssigneeID = &uid
		}
	}
	if req.DueDate != nil {
		if t, err := parseOptionalExpiry(*req.DueDate); err == nil && t != nil {
			record.DueDate = t
		}
	}

	created, err := s.actionItemStore.Create(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create action item")
		return
	}

	s.audit(r, store.AuditActionItemCreated, map[string]any{
		"incident_number": incidentID,
		"pm_id":           pmID.String(),
		"ai_id":           created.ID.String(),
	})

	if req.AssigneeID != nil && *req.AssigneeID != "" && s.rabbitmqPublisher != nil {
		_ = s.rabbitmqPublisher.PublishNotificationDispatch(r.Context(), rabbitmq.NotificationDispatchMessage{
			UserID:           *req.AssigneeID,
			NotificationType: "action_item_assigned",
			Title:            "Action item assigned",
			Message:          fmt.Sprintf("Action item: %s", created.Description),
			ResourceType:     "action_item",
			ResourceID:       created.ID.String(),
		})
	}
	writeData(w, http.StatusCreated, created)
}

func (s *Server) updateActionItem(w http.ResponseWriter, r *http.Request, incidentID, aiID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsWrite) {
		return
	}
	if !s.requireActionItemStore(w) {
		return
	}

	id, err := uuid.Parse(aiID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid action item id")
		return
	}

	existing, err := s.actionItemStore.GetByID(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "failed to get action item")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "action item not found")
		return
	}

	var req struct {
		Description  *string `json:"description"`
		AssigneeID   *string `json:"assignee_id"`
		Status       *string `json:"status"`
		Priority     *string `json:"priority"`
		DueDate      *string `json:"due_date"`
		Type         *string `json:"type"`
		AssigneeName *string `json:"assignee_name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.AssigneeID != nil {
		if uid, err := uuid.Parse(*req.AssigneeID); err == nil {
			existing.AssigneeID = &uid
		} else {
			existing.AssigneeID = nil
		}
	}
	if req.DueDate != nil {
		if t, err := parseOptionalExpiry(*req.DueDate); err == nil {
			existing.DueDate = t
		}
	}
	if req.Type != nil {
		existing.Type = *req.Type
	}
	if req.AssigneeName != nil {
		existing.AssigneeName = *req.AssigneeName
	}

	oldAssigneeID := existing.AssigneeID

	updated, err := s.actionItemStore.Update(r.Context(), id, existing)
	if err != nil {
		writeInternalError(w, err, "failed to update action item")
		return
	}

	s.audit(r, store.AuditActionItemUpdated, map[string]any{
		"incident_number": incidentID,
		"ai_id":           aiID,
	})

	if req.AssigneeID != nil && *req.AssigneeID != "" && (oldAssigneeID == nil || oldAssigneeID.String() != *req.AssigneeID) && s.rabbitmqPublisher != nil {
		_ = s.rabbitmqPublisher.PublishNotificationDispatch(r.Context(), rabbitmq.NotificationDispatchMessage{
			UserID:           *req.AssigneeID,
			NotificationType: "action_item_assigned",
			Title:            "Action item assigned",
			Message:          fmt.Sprintf("Action item: %s", updated.Description),
			ResourceType:     "action_item",
			ResourceID:       updated.ID.String(),
		})
	}
	writeData(w, http.StatusOK, updated)
}

func (s *Server) deleteActionItem(w http.ResponseWriter, r *http.Request, incidentID, aiID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsDelete) {
		return
	}
	if !s.requireActionItemStore(w) {
		return
	}

	id, err := uuid.Parse(aiID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid action item id")
		return
	}

	if err := s.actionItemStore.Delete(r.Context(), id); err != nil {
		writeInternalError(w, err, "failed to delete action item")
		return
	}

	s.audit(r, store.AuditActionItemDeleted, map[string]any{
		"incident_number": incidentID,
		"ai_id":           aiID,
	})

	writeStatus(w, "deleted")
}

func (s *Server) handleGlobalActionItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if !s.checkPermission(w, r, rbac.PostMortemsRead) {
		return
	}
	if !s.requireActionItemStore(w) {
		return
	}

	items, err := s.actionItemStore.ListOpen(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to list open action items")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(items))
}

func (s *Server) handlePostMortemsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if !s.checkPermission(w, r, rbac.PostMortemsRead) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}

	q := r.URL.Query()
	filter := store.PostMortemListFilter{}
	if v := q.Get("status"); v != "" {
		filter.Status = v
	}

	limit, skip := parseLimitSkip(r, 20)
	filter.Limit = int(limit)
	filter.Skip = int(skip)

	records, total, err := s.postmortemStore.List(r.Context(), filter)
	if err != nil {
		writeInternalError(w, err, "failed to list post-mortems")
		return
	}

	s.enrichPostMortemRecords(r.Context(), records)
	writePaginatedJSON(w, ensureSlice(records), int64(total))
}

func (s *Server) enrichPostMortemRecords(ctx context.Context, records []store.PostMortemRecord) {
	s.mu.RLock()
	incStore := s.incidentStore
	s.mu.RUnlock()
	if incStore == nil {
		return
	}

	for i := range records {
		inc, err := incStore.GetIncidentByID(ctx, records[i].IncidentID)
		if err != nil || inc == nil {
			continue
		}
		records[i].IncidentTitle = inc.Title
		records[i].IncidentNumber = inc.IncidentNumber
		records[i].IncidentSeverity = inc.Severity
	}
}
