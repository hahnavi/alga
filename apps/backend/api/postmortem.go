package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

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

	// Workflow transitions. Checked after the action-items branch so a path
	// like `/post-mortem/action-items/{id}/publish` is not mistaken for a
	// lifecycle transition on the post-mortem itself.
	if strings.HasSuffix(suffix, "/submit-review") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		s.updatePostMortemStatus(w, r, incidentID, "in_review")
		return
	}

	if strings.HasSuffix(suffix, "/revert-to-draft") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		s.updatePostMortemStatus(w, r, incidentID, "draft")
		return
	}

	if strings.HasSuffix(suffix, "/revert-to-review") {
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

var errPostMortemIncidentStoreNotConfigured = errors.New("incident store not configured")

func (s *Server) requirePostMortemStore(w http.ResponseWriter) bool {
	return s.requireStore(w, s.postmortemStore, "post-mortem store")
}

func (s *Server) requireActionItemStore(w http.ResponseWriter) bool {
	return s.requireStore(w, s.actionItemStore, "action item store")
}

func (s *Server) requireIncidentStoreForPostMortem(w http.ResponseWriter) bool {
	return s.requireStore(w, s.incidentStore, "incident store")
}

// postMortemIncidentTarget is the canonical incident reference every
// post-mortem handler works against: the incident's UUID, its human-facing
// number (for timeline entries, SSE payloads, and audit records), and the
// incident record itself (for commander notification targeting). Resolving
// once up front keeps UUID-addressed requests from silently dropping side
// effects that only accept numbers.
type postMortemIncidentTarget struct {
	ID     uuid.UUID
	Number int64
	Record *store.IncidentRecord
}

// resolvePostMortemIncident resolves the incident referenced by a post-mortem
// route — by incident number or UUID — into the canonical target. ok=false
// means the incident does not exist (handlers translate that to 404).
func (s *Server) resolvePostMortemIncident(ctx context.Context, incidentID string) (postMortemIncidentTarget, bool, error) {
	if s.incidentStore == nil {
		return postMortemIncidentTarget{}, false, errPostMortemIncidentStoreNotConfigured
	}
	if uid, err := uuid.Parse(incidentID); err == nil {
		inc, err := s.incidentStore.GetIncidentByID(ctx, uid)
		if err != nil {
			return postMortemIncidentTarget{}, false, fmt.Errorf("get incident: %w", err)
		}
		if inc == nil || inc.DeletedAt != nil {
			return postMortemIncidentTarget{}, false, nil
		}
		return postMortemIncidentTarget{ID: inc.ID, Number: inc.IncidentNumber, Record: inc}, true, nil
	}
	incidentNumber, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		return postMortemIncidentTarget{}, false, nil
	}
	inc, err := s.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil {
		return postMortemIncidentTarget{}, false, fmt.Errorf("get incident: %w", err)
	}
	if inc == nil || inc.DeletedAt != nil {
		return postMortemIncidentTarget{}, false, nil
	}
	return postMortemIncidentTarget{ID: inc.ID, Number: inc.IncidentNumber, Record: inc}, true, nil
}

func writePostMortemIncidentResolveError(w http.ResponseWriter, err error) {
	if errors.Is(err, errPostMortemIncidentStoreNotConfigured) {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "incident store not configured")
		return
	}
	writeInternalError(w, err, "failed to resolve incident")
}

func (s *Server) getPostMortem(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsRead) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	record, err := s.postmortemStore.GetByIncidentID(r.Context(), target.ID)
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

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	record := &store.PostMortemRecord{
		IncidentID:          target.ID,
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
		logger.ErrorCtx(r.Context(), "failed to create post-mortem", "component", "api", "incident_number", target.Number, "error", err)
		writeInternalError(w, err, "failed to create post-mortem")
		return
	}

	logger.InfoCtx(r.Context(), "post-mortem created", "component", "api", "incident_number", target.Number, "pm_id", created.ID.String())
	s.addIncidentTimeline(r, strconv.FormatInt(target.Number, 10), "postmortem_created", "Post-mortem created")
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": strconv.FormatInt(target.Number, 10)})
	s.audit(r, store.AuditPostMortemCreated, map[string]any{
		"incident_number": target.Number,
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
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), target.ID)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	// Published post-mortems are immutable: the lifecycle's only terminal
	// state must not be silently rewritable via PATCH.
	if existing.Status == "published" {
		writeErrorStatus(w, http.StatusConflict, ErrorCodeValidationFailed, "published post-mortems cannot be edited")
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
		logger.ErrorCtx(r.Context(), "failed to update post-mortem", "component", "api", "incident_number", target.Number, "error", err)
		writeInternalError(w, err, "failed to update post-mortem")
		return
	}

	logger.InfoCtx(r.Context(), "post-mortem updated", "component", "api", "incident_number", target.Number, "pm_id", existing.ID.String())
	s.audit(r, store.AuditPostMortemUpdated, map[string]any{
		"incident_number": target.Number,
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
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), target.ID)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	if err := s.postmortemStore.Delete(r.Context(), existing.ID); err != nil {
		writeInternalError(w, err, "failed to delete post-mortem")
		return
	}

	logger.InfoCtx(r.Context(), "post-mortem deleted", "component", "api", "incident_number", target.Number, "pm_id", existing.ID.String())
	s.addIncidentTimeline(r, strconv.FormatInt(target.Number, 10), "postmortem_deleted", "Post-mortem deleted")
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": strconv.FormatInt(target.Number, 10)})
	s.audit(r, store.AuditPostMortemDeleted, map[string]any{
		"incident_number": target.Number,
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
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), target.ID)
	if err != nil {
		writeInternalError(w, err, "failed to get post-mortem")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return
	}

	// The blameless confirmation is the gate into review: a document whose
	// author has not confirmed the review focuses on systems, not people,
	// cannot enter the review/publish pipeline.
	if status == "in_review" && !existing.BlamelessConfirmed {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "blameless_confirmed must be set before submitting for review")
		return
	}

	if !isValidPostMortemTransition(existing.Status, status) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, fmt.Sprintf("cannot transition post-mortem from %s to %s", existing.Status, status))
		return
	}

	updated, err := s.postmortemStore.UpdateStatus(r.Context(), existing.ID, status, nil)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to update post-mortem status", "component", "api", "incident_number", target.Number, "status", status, "error", err)
		writeInternalError(w, err, "failed to update post-mortem status")
		return
	}

	incidentNumberStr := strconv.FormatInt(target.Number, 10)
	logger.InfoCtx(r.Context(), "post-mortem status changed", "component", "api", "incident_number", target.Number, "pm_id", existing.ID.String(), "status", status)
	s.addIncidentTimeline(r, incidentNumberStr, "postmortem_"+status, "Post-mortem status changed to "+status)
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentNumberStr})
	s.audit(r, store.AuditPostMortemStatusChanged, map[string]any{
		"incident_number": target.Number,
		"pm_id":           existing.ID.String(),
		"status":          status,
	})

	// Review requests notify the incident commander. Targeting comes from the
	// resolved incident record, never the request body; publish failures are
	// logged by the worker and must not fail the committed transition.
	if status == "in_review" && s.rabbitmqPublisher != nil && target.Record.CommanderID != nil {
		_ = s.rabbitmqPublisher.PublishNotificationDispatch(r.Context(), rabbitmq.NotificationDispatchMessage{
			UserID:           target.Record.CommanderID.String(),
			IncidentNumber:   target.Number,
			NotificationType: "post_mortem_review_requested",
			Title:            "Post-mortem review requested",
			Message:          fmt.Sprintf("Post-mortem for incident %s has been submitted for review", incidentNumberStr),
			ResourceType:     "incident",
			ResourceID:       target.ID.String(),
		})
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
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, err := s.postmortemStore.GetByIncidentID(r.Context(), target.ID)
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

	incidentNumberStr := strconv.FormatInt(target.Number, 10)
	s.addIncidentTimeline(r, incidentNumberStr, "postmortem_approved", "Post-mortem approved")
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentNumberStr})
	s.audit(r, store.AuditPostMortemStatusChanged, map[string]any{
		"incident_number": target.Number,
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

// resolveActionItemPostMortemID resolves the post-mortem owned by the path
// incident. Returns uuid.Nil when the incident exists but has no post-mortem
// (callers translate to 404 for writes, empty list for reads).
func (s *Server) resolveActionItemPostMortemID(ctx context.Context, target postMortemIncidentTarget) (uuid.UUID, error) {
	pm, err := s.postmortemStore.GetByIncidentID(ctx, target.ID)
	if err != nil {
		return uuid.Nil, err
	}
	if pm == nil {
		return uuid.Nil, nil
	}
	return pm.ID, nil
}

// validActionItemStatuses / validActionItemTypes / validActionItemPriorities
// mirror the action_items CHECK constraints. Validating in the handler turns
// constraint violations into 400s instead of 500s.
var (
	validActionItemStatuses   = []string{"open", "in_progress", "completed", "cancelled"}
	validActionItemTypes      = []string{"prevent", "mitigate", "detect", "investigate"}
	validActionItemPriorities = []string{"low", "medium", "high"}
)

func (s *Server) validateActionItemAssignee(w http.ResponseWriter, r *http.Request, raw string) (uuid.UUID, bool) {
	uid, err := uuid.Parse(raw)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "assignee_id must be a valid UUID")
		return uuid.Nil, false
	}
	if s.userStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "user store not configured")
		return uuid.Nil, false
	}
	user, err := s.userStore.GetByID(uid)
	if err != nil {
		writeInternalError(w, err, "failed to validate assignee")
		return uuid.Nil, false
	}
	if user == nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "assignee_id does not reference an existing user")
		return uuid.Nil, false
	}
	return uid, true
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
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	pmID, err := s.resolveActionItemPostMortemID(r.Context(), target)
	if err != nil {
		writeInternalError(w, err, "failed to resolve post-mortem")
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
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	pmID, err := s.resolveActionItemPostMortemID(r.Context(), target)
	if err != nil {
		writeInternalError(w, err, "failed to resolve post-mortem")
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
	if req.Priority != "" && !slices.Contains(validActionItemPriorities, req.Priority) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "priority must be one of low, medium, high")
		return
	}
	if req.Type != "" && !slices.Contains(validActionItemTypes, req.Type) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "type must be one of prevent, mitigate, detect, investigate")
		return
	}

	var assigneeID *uuid.UUID
	if req.AssigneeID != nil && strings.TrimSpace(*req.AssigneeID) != "" {
		uid, valid := s.validateActionItemAssignee(w, r, strings.TrimSpace(*req.AssigneeID))
		if !valid {
			return
		}
		assigneeID = &uid
	}

	var dueDate *time.Time
	if req.DueDate != nil && strings.TrimSpace(*req.DueDate) != "" {
		t, parseErr := parseActionItemDueDate(strings.TrimSpace(*req.DueDate))
		if parseErr != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, parseErr.Error())
			return
		}
		dueDate = t
	}

	record := &store.ActionItemRecord{
		PostMortemID: pmID,
		Description:  req.Description,
		Priority:     req.Priority,
		Type:         req.Type,
		AssigneeName: req.AssigneeName,
		AssigneeID:   assigneeID,
		DueDate:      dueDate,
	}

	created, err := s.actionItemStore.Create(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create action item")
		return
	}

	s.audit(r, store.AuditActionItemCreated, map[string]any{
		"incident_number": target.Number,
		"pm_id":           pmID.String(),
		"ai_id":           created.ID.String(),
	})

	if assigneeID != nil && s.rabbitmqPublisher != nil {
		_ = s.rabbitmqPublisher.PublishNotificationDispatch(r.Context(), rabbitmq.NotificationDispatchMessage{
			UserID:           assigneeID.String(),
			IncidentNumber:   target.Number,
			NotificationType: "action_item_assigned",
			Title:            "Action item assigned",
			Message:          fmt.Sprintf("Action item: %s", created.Description),
			ResourceType:     "action_item",
			ResourceID:       created.ID.String(),
		})
	}
	writeData(w, http.StatusCreated, created)
}

// resolveActionItemForIncident loads the action item and enforces that it
// belongs to the post-mortem of the path incident, so a cross-incident item
// UUID cannot be mutated (or audited) under a different incident's number.
func (s *Server) resolveActionItemForIncident(w http.ResponseWriter, r *http.Request, target postMortemIncidentTarget, aiID string) (*store.ActionItemRecord, bool) {
	id, err := uuid.Parse(aiID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid action item id")
		return nil, false
	}
	pmID, err := s.resolveActionItemPostMortemID(r.Context(), target)
	if err != nil {
		writeInternalError(w, err, "failed to resolve post-mortem")
		return nil, false
	}
	if pmID == uuid.Nil {
		writeError(w, ErrorCodeNotFound, "post-mortem not found")
		return nil, false
	}
	item, err := s.actionItemStore.GetByID(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "failed to get action item")
		return nil, false
	}
	if item == nil || item.PostMortemID != pmID {
		writeError(w, ErrorCodeNotFound, "action item not found")
		return nil, false
	}
	return item, true
}

func (s *Server) updateActionItem(w http.ResponseWriter, r *http.Request, incidentID, aiID string) {
	if !s.checkPermission(w, r, rbac.PostMortemsWrite) {
		return
	}
	if !s.requireActionItemStore(w) {
		return
	}
	if !s.requirePostMortemStore(w) {
		return
	}
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	existing, ok := s.resolveActionItemForIncident(w, r, target, aiID)
	if !ok {
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

	// Snapshot the previous assignee before mutating `existing` so a
	// reassignment can be detected for the new-assignee notification.
	oldAssigneeID := existing.AssigneeID

	if req.Description != nil {
		if strings.TrimSpace(*req.Description) == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "description cannot be empty")
			return
		}
		existing.Description = *req.Description
	}
	if req.Status != nil {
		if !slices.Contains(validActionItemStatuses, *req.Status) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "status must be one of open, in_progress, completed, cancelled")
			return
		}
		existing.Status = *req.Status
	}
	if req.Priority != nil {
		if !slices.Contains(validActionItemPriorities, *req.Priority) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "priority must be one of low, medium, high")
			return
		}
		existing.Priority = *req.Priority
	}
	if req.Type != nil {
		if !slices.Contains(validActionItemTypes, *req.Type) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "type must be one of prevent, mitigate, detect, investigate")
			return
		}
		existing.Type = *req.Type
	}
	if req.AssigneeID != nil {
		if strings.TrimSpace(*req.AssigneeID) == "" {
			existing.AssigneeID = nil
		} else {
			uid, valid := s.validateActionItemAssignee(w, r, strings.TrimSpace(*req.AssigneeID))
			if !valid {
				return
			}
			existing.AssigneeID = &uid
		}
	}
	if req.DueDate != nil {
		if strings.TrimSpace(*req.DueDate) == "" {
			existing.DueDate = nil
		} else {
			t, parseErr := parseActionItemDueDate(strings.TrimSpace(*req.DueDate))
			if parseErr != nil {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, parseErr.Error())
				return
			}
			existing.DueDate = t
		}
	}
	if req.AssigneeName != nil {
		existing.AssigneeName = *req.AssigneeName
	}

	updated, err := s.actionItemStore.Update(r.Context(), existing.ID, existing)
	if err != nil {
		writeInternalError(w, err, "failed to update action item")
		return
	}

	s.audit(r, store.AuditActionItemUpdated, map[string]any{
		"incident_number": target.Number,
		"ai_id":           aiID,
	})

	// Notify only when the assignment actually changed and there is a new
	// assignee (oldAssigneeID was captured before mutation).
	if existing.AssigneeID != nil &&
		(oldAssigneeID == nil || *oldAssigneeID != *existing.AssigneeID) &&
		s.rabbitmqPublisher != nil {
		_ = s.rabbitmqPublisher.PublishNotificationDispatch(r.Context(), rabbitmq.NotificationDispatchMessage{
			UserID:           existing.AssigneeID.String(),
			IncidentNumber:   target.Number,
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
	if !s.requirePostMortemStore(w) {
		return
	}
	if !s.requireIncidentStoreForPostMortem(w) {
		return
	}

	target, ok, err := s.resolvePostMortemIncident(r.Context(), incidentID)
	if err != nil {
		writePostMortemIncidentResolveError(w, err)
		return
	}
	if !ok {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	item, ok := s.resolveActionItemForIncident(w, r, target, aiID)
	if !ok {
		return
	}

	if err := s.actionItemStore.Delete(r.Context(), item.ID); err != nil {
		writeInternalError(w, err, "failed to delete action item")
		return
	}

	s.audit(r, store.AuditActionItemDeleted, map[string]any{
		"incident_number": target.Number,
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
