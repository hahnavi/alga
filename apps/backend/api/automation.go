package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"alga/automation"
	"alga/store"
)

func (s *Server) automationReady(w http.ResponseWriter) bool {
	if s.automationStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "automation is unavailable")
		return false
	}
	return true
}

func automationPathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, ErrorCodeValidationFailed, "invalid automation id")
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) handleListAutomationRules(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	limit, skip := parseLimitSkip(r, 20)
	items, total, err := s.automationStore.ListRules(r.Context(), int(limit), int(skip))
	if err != nil {
		writeInternalError(w, err, "failed to list automation rules")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) handleCreateAutomationRule(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	var definition automation.Definition
	if !decodeJSON(w, r, &definition) {
		return
	}
	var creator *uuid.UUID
	if user := userFromContext(r.Context()); user != nil {
		id := user.ID
		creator = &id
	}
	record, err := s.automationStore.CreateRule(r.Context(), definition, creator)
	if err != nil {
		s.writeAutomationError(w, err, "failed to create automation rule")
		return
	}
	s.audit(r, store.AuditAutomationRuleCreated, map[string]any{"rule_id": record.ID.String(), "enabled": record.Enabled})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleGetAutomationRule(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	id, ok := automationPathID(w, r)
	if !ok {
		return
	}
	record, err := s.automationStore.GetRule(r.Context(), id)
	if err != nil {
		s.writeAutomationError(w, err, "failed to get automation rule")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleUpdateAutomationRule(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	id, ok := automationPathID(w, r)
	if !ok {
		return
	}
	var input struct {
		automation.Definition
		Version int `json:"version"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Version <= 0 {
		writeError(w, ErrorCodeValidationFailed, "version is required")
		return
	}
	record, err := s.automationStore.UpdateRule(r.Context(), id, input.Version, input.Definition)
	if err != nil {
		s.writeAutomationError(w, err, "failed to update automation rule")
		return
	}
	s.audit(r, store.AuditAutomationRuleUpdated, map[string]any{"rule_id": id.String(), "version": record.Version, "enabled": record.Enabled})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleArchiveAutomationRule(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	id, ok := automationPathID(w, r)
	if !ok {
		return
	}
	if err := s.automationStore.ArchiveRule(r.Context(), id); err != nil {
		s.writeAutomationError(w, err, "failed to archive automation rule")
		return
	}
	s.audit(r, store.AuditAutomationRuleArchived, map[string]any{"rule_id": id.String()})
	writeStatus(w, "archived")
}

func (s *Server) handleListAutomationRuns(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	var ruleID *uuid.UUID
	if raw := r.URL.Query().Get("rule_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, ErrorCodeValidationFailed, "invalid rule_id")
			return
		}
		ruleID = &id
	}
	limit, skip := parseLimitSkip(r, 20)
	items, total, err := s.automationStore.ListRuns(r.Context(), ruleID, int(limit), int(skip))
	if err != nil {
		writeInternalError(w, err, "failed to list automation runs")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) handleGetAutomationRun(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	id, ok := automationPathID(w, r)
	if !ok {
		return
	}
	record, err := s.automationStore.GetRun(r.Context(), id)
	if err != nil {
		s.writeAutomationError(w, err, "failed to get automation run")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleRetryAutomationRun(w http.ResponseWriter, r *http.Request) {
	if !s.automationReady(w) {
		return
	}
	id, ok := automationPathID(w, r)
	if !ok {
		return
	}
	record, err := s.automationStore.GetRun(r.Context(), id)
	if err != nil {
		s.writeAutomationError(w, err, "failed to get automation run")
		return
	}
	if record.Status != "failed" {
		writeError(w, ErrorCodeConflict, "only failed runs can be retried")
		return
	}
	if err := s.automationStore.RetryRun(r.Context(), id); err != nil {
		s.writeAutomationError(w, err, "failed to retry automation run")
		return
	}
	s.audit(r, store.AuditAutomationRunRetried, map[string]any{"run_id": id.String(), "rule_id": record.RuleID.String()})
	writeStatus(w, "pending")
}

func (s *Server) writeAutomationError(w http.ResponseWriter, err error, internalMessage string) {
	switch {
	case errors.Is(err, automation.ErrInvalidRule):
		writeError(w, ErrorCodeValidationFailed, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, ErrorCodeNotFound, "automation record not found")
	case errors.Is(err, store.ErrAutomationVersionConflict):
		writeError(w, ErrorCodeConflict, "automation rule was updated; reload before saving")
	default:
		writeInternalError(w, err, internalMessage)
	}
}
