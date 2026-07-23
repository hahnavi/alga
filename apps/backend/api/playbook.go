package api

import (
	"net/http"
	"strings"

	"alga/logger"
	"alga/matching"
	"alga/rbac"
	"alga/sse"
	"alga/store"

	"github.com/google/uuid"
)

func (s *Server) handlePlaybooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListPlaybooks(w, r)
	case http.MethodPost:
		s.handleCreatePlaybook(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListPlaybooks(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.PlaybookRead) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	var filter store.PlaybookFilter
	if v := r.URL.Query().Get("kind"); v != "" {
		filter.Kind = v
	}
	if v := r.URL.Query().Get("service_id"); v != "" {
		if uid, err := uuid.Parse(v); err == nil {
			filter.ServiceID = &uid
		}
	}
	if v := r.URL.Query().Get("tag"); v != "" {
		filter.Tag = v
	}
	if v := r.URL.Query().Get("search"); v != "" {
		filter.Search = v
	}

	limit, skip := parseLimitSkip(r, 50)
	records, total, err := s.playbookStore.List(r.Context(), filter, int(limit), int(skip))
	if err != nil {
		writeInternalError(w, err, "failed to list playbooks")
		return
	}
	writePaginatedJSON(w, ensureSlice(records), total)
}

func (s *Server) handleCreatePlaybook(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.PlaybookWrite) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Title          string           `json:"title"`
		Kind           string           `json:"kind"`
		Summary        string           `json:"summary,omitempty"`
		ServiceID      string           `json:"service_id,omitempty"`
		LabelSelectors []map[string]any `json:"label_selectors,omitempty"`
		Tags           []string         `json:"tags,omitempty"`
		Steps          []struct {
			Title            string `json:"title"`
			Description      string `json:"description,omitempty"`
			ExpectedDuration string `json:"expected_duration,omitempty"`
			Command          string `json:"command,omitempty"`
		} `json:"steps,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "title is required")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "incident"
	}

	if err := validatePlaybookLabelSelectors(req.LabelSelectors); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}

	record := &store.PlaybookRecord{
		Title:     strings.TrimSpace(req.Title),
		Kind:      kind,
		Summary:   req.Summary,
		Tags:      req.Tags,
		CreatedBy: user.ID,
	}
	if req.ServiceID != "" {
		if uid, err := uuid.Parse(req.ServiceID); err == nil {
			record.ServiceID = &uid
		}
	}
	if len(req.LabelSelectors) > 0 {
		record.LabelSelectors = req.LabelSelectors
	}

	var steps []store.PlaybookStepRecord
	for i, s := range req.Steps {
		steps = append(steps, store.PlaybookStepRecord{
			StepNumber:       i + 1,
			Title:            s.Title,
			Description:      s.Description,
			ExpectedDuration: s.ExpectedDuration,
			Command:          s.Command,
		})
	}

	created, err := s.playbookStore.Create(r.Context(), record, steps)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to create playbook", "component", "api", "error", err)
		writeInternalError(w, err, "failed to create playbook")
		return
	}

	logger.InfoCtx(r.Context(), "playbook created", "component", "api", "playbook_id", created.ID.String(), "title", req.Title)
	s.audit(r, store.AuditEvent("playbook_created"), map[string]any{
		"playbook_id": created.ID.String(),
		"title":       req.Title,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "playbook_created", Data: created})
	}
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handlePlaybookRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/playbooks/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing playbook id")
		return
	}

	if strings.HasSuffix(suffix, "/steps") || strings.Contains(suffix, "/steps/") {
		s.handlePlaybookStepRoutes(w, r, suffix)
		return
	}

	if strings.HasSuffix(suffix, "/reorder") {
		playbookID := strings.TrimSuffix(suffix, "/reorder")
		if playbookID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing playbook id")
			return
		}
		if r.Method == http.MethodPost {
			s.handleReorderPlaybookSteps(w, r, playbookID)
		} else {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetPlaybook(w, r, suffix)
	case http.MethodPut:
		s.handleUpdatePlaybook(w, r, suffix)
	case http.MethodDelete:
		s.handleDeletePlaybook(w, r, suffix)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleGetPlaybook(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.PlaybookRead) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid playbook id")
		return
	}

	record, steps, err := s.playbookStore.Get(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get playbook")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "playbook not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"playbook": record,
		"steps":    ensureSlice(steps),
	})
}

func (s *Server) handleUpdatePlaybook(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.PlaybookWrite) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid playbook id")
		return
	}

	var req struct {
		Title          *string          `json:"title"`
		Kind           *string          `json:"kind"`
		Summary        *string          `json:"summary"`
		ServiceID      *string          `json:"service_id"`
		LabelSelectors []map[string]any `json:"label_selectors"`
		Tags           []string         `json:"tags"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	existing, _, err := s.playbookStore.Get(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get playbook")
		return
	}
	if existing == nil {
		writeError(w, ErrorCodeNotFound, "playbook not found")
		return
	}

	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Kind != nil {
		existing.Kind = *req.Kind
	}
	if req.Summary != nil {
		existing.Summary = *req.Summary
	}
	if req.ServiceID != nil {
		if *req.ServiceID == "" {
			existing.ServiceID = nil
		} else if uid, err := uuid.Parse(*req.ServiceID); err == nil {
			existing.ServiceID = &uid
		}
	}
	if req.LabelSelectors != nil {
		if err := validatePlaybookLabelSelectors(req.LabelSelectors); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}
		existing.LabelSelectors = req.LabelSelectors
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}

	if err := s.playbookStore.Update(r.Context(), uid, existing); err != nil {
		logger.ErrorCtx(r.Context(), "failed to update playbook", "component", "api", "playbook_id", id, "error", err)
		writeInternalError(w, err, "failed to update playbook")
		return
	}

	logger.InfoCtx(r.Context(), "playbook updated", "component", "api", "playbook_id", id)
	s.audit(r, store.AuditEvent("playbook_updated"), map[string]any{
		"playbook_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "playbook_updated", Data: map[string]string{"playbook_id": id}})
	}
	writeStatus(w, "updated")
}

func (s *Server) handleDeletePlaybook(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.PlaybookDelete) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid playbook id")
		return
	}

	if err := s.playbookStore.Delete(r.Context(), uid); err != nil {
		writeInternalError(w, err, "failed to delete playbook")
		return
	}

	logger.InfoCtx(r.Context(), "playbook deleted", "component", "api", "playbook_id", id)
	s.audit(r, store.AuditEvent("playbook_deleted"), map[string]any{
		"playbook_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "playbook_deleted", Data: map[string]string{"playbook_id": id}})
	}
	writeStatus(w, "deleted")
}

func (s *Server) handlePlaybookStepRoutes(w http.ResponseWriter, r *http.Request, suffix string) {
	if strings.HasSuffix(suffix, "/steps") {
		playbookID := strings.TrimSuffix(suffix, "/steps")
		if playbookID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing playbook id")
			return
		}
		if r.Method == http.MethodPost {
			s.handleAddPlaybookStep(w, r, playbookID)
		} else {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	idx := strings.Index(suffix, "/steps/")
	if idx < 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid step path")
		return
	}
	playbookID := suffix[:idx]
	stepID := suffix[idx+len("/steps/"):]
	if playbookID == "" || stepID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing playbook id or step id")
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.handleUpdatePlaybookStep(w, r, stepID)
	case http.MethodDelete:
		s.handleDeletePlaybookStep(w, r, stepID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleAddPlaybookStep(w http.ResponseWriter, r *http.Request, playbookID string) {
	if !s.checkPermission(w, r, rbac.PlaybookWrite) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	pid, err := uuid.Parse(playbookID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid playbook id")
		return
	}

	_, steps, err := s.playbookStore.Get(r.Context(), pid)
	if err != nil {
		writeInternalError(w, err, "failed to get playbook")
		return
	}

	var req struct {
		Title            string `json:"title"`
		Description      string `json:"description,omitempty"`
		ExpectedDuration string `json:"expected_duration,omitempty"`
		Command          string `json:"command,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "title is required")
		return
	}

	stepNumber := 1
	if len(steps) > 0 {
		stepNumber = steps[len(steps)-1].StepNumber + 1
	}

	step := &store.PlaybookStepRecord{
		PlaybookID:       pid,
		StepNumber:       stepNumber,
		Title:            strings.TrimSpace(req.Title),
		Description:      req.Description,
		ExpectedDuration: req.ExpectedDuration,
		Command:          req.Command,
	}

	created, err := s.playbookStore.AddStep(r.Context(), step)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to add playbook step", "component", "api", "playbook_id", playbookID, "error", err)
		writeInternalError(w, err, "failed to add playbook step")
		return
	}

	logger.InfoCtx(r.Context(), "playbook step added", "component", "api", "playbook_id", playbookID, "step_id", created.ID.String())
	s.audit(r, store.AuditEvent("playbook_step_added"), map[string]any{
		"playbook_id": playbookID,
		"step_id":     created.ID.String(),
	})
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handleUpdatePlaybookStep(w http.ResponseWriter, r *http.Request, stepID string) {
	if !s.checkPermission(w, r, rbac.PlaybookWrite) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	sid, err := uuid.Parse(stepID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid step id")
		return
	}

	var req struct {
		StepNumber       *int    `json:"step_number"`
		Title            *string `json:"title"`
		Description      *string `json:"description"`
		ExpectedDuration *string `json:"expected_duration"`
		Command          *string `json:"command"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	step := &store.PlaybookStepRecord{
		Title:       "",
		Description: "",
		StepNumber:  0,
	}
	if req.StepNumber != nil {
		step.StepNumber = *req.StepNumber
	}
	if req.Title != nil {
		step.Title = *req.Title
	}
	if req.Description != nil {
		step.Description = *req.Description
	}
	if req.ExpectedDuration != nil {
		step.ExpectedDuration = *req.ExpectedDuration
	}
	if req.Command != nil {
		step.Command = *req.Command
	}

	if err := s.playbookStore.UpdateStep(r.Context(), sid, step); err != nil {
		writeInternalError(w, err, "failed to update playbook step")
		return
	}

	writeStatus(w, "updated")
}

func (s *Server) handleDeletePlaybookStep(w http.ResponseWriter, r *http.Request, stepID string) {
	if !s.checkPermission(w, r, rbac.PlaybookDelete) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	sid, err := uuid.Parse(stepID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid step id")
		return
	}

	if err := s.playbookStore.DeleteStep(r.Context(), sid); err != nil {
		writeInternalError(w, err, "failed to delete playbook step")
		return
	}

	writeStatus(w, "deleted")
}

func (s *Server) handleReorderPlaybookSteps(w http.ResponseWriter, r *http.Request, playbookID string) {
	if !s.checkPermission(w, r, rbac.PlaybookWrite) {
		return
	}
	if s.playbookStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "playbook store not configured")
		return
	}

	pid, err := uuid.Parse(playbookID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid playbook id")
		return
	}

	var req []struct {
		ID         string `json:"id"`
		StepNumber int    `json:"step_number"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	var order []store.StepOrder
	for _, item := range req {
		id, err := uuid.Parse(item.ID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid step id: "+item.ID)
			return
		}
		order = append(order, store.StepOrder{ID: id, StepNumber: item.StepNumber})
	}

	if err := s.playbookStore.ReorderSteps(r.Context(), pid, order); err != nil {
		writeInternalError(w, err, "failed to reorder playbook steps")
		return
	}

	writeStatus(w, "ok")
}

func validatePlaybookLabelSelectors(selectors []map[string]any) error {
	for _, sel := range selectors {
		for _, val := range sel {
			s, ok := val.(string)
			if !ok || s == "" {
				return &validationError{msg: "selector values must be non-empty strings"}
			}
			if strings.HasPrefix(s, "~") {
				if _, err := matching.GetCompiledRegex(strings.TrimPrefix(s, "~")); err != nil {
					return &validationError{msg: "invalid regex pattern in selector: " + err.Error()}
				}
			}
		}
	}
	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}
