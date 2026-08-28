package api

import (
	"net/http"
	"strings"
	"time"

	"alga/logger"
	"alga/rbac"
	"alga/store"
)

// validTriageDecision reports whether decision is one of the decisions the
// triage engine can emit (mirrors the store constants).
func validTriageDecision(decision string) bool {
	switch decision {
	case store.TriageDecisionInvestigate,
		store.TriageDecisionAutoResolve,
		store.TriageDecisionSuppress,
		store.TriageDecisionEscalate,
		store.TriageDecisionEnrichOnly:
		return true
	}
	return false
}

type triageRuleRequest struct {
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Conditions  []map[string]any `json:"conditions,omitempty"`
	MatchMode   string           `json:"match_mode,omitempty"`
	Decision    string           `json:"decision,omitempty"`
	Severity    string           `json:"severity,omitempty"`
	Category    string           `json:"category,omitempty"`
	Enrichment  map[string]any   `json:"enrichment,omitempty"`
	Priority    *int             `json:"priority,omitempty"`
	Enabled     *bool            `json:"enabled,omitempty"`
}

func (s *Server) handleTriageRules(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.triageRuleStore, "triage rule store") {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listTriageRules(w, r)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.TriageWrite) {
			return
		}
		s.createTriageRule(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleTriageRuleByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.triageRuleStore, "triage rule store") {
		return
	}
	id := pathID(r, "/api/v1/triage/rules/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, err := s.triageRuleStore.Get(r.Context(), id)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}
		if rec == nil {
			writeError(w, ErrorCodeNotFound, "triage rule not found")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.TriageWrite) {
			return
		}
		var req triageRuleRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		patch := &store.TriageRuleRecord{
			Name:        req.Name,
			Description: req.Description,
			Conditions:  req.Conditions,
			MatchMode:   req.MatchMode,
			Decision:    req.Decision,
			Severity:    req.Severity,
			Category:    req.Category,
			Enrichment:  req.Enrichment,
		}
		if req.Priority != nil {
			patch.Priority = *req.Priority
		}
		if req.Enabled != nil {
			patch.Enabled = *req.Enabled
		}
		out, err := s.triageRuleStore.Update(r.Context(), id, patch)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}
		logger.InfoCtx(r.Context(), "triage rule updated", "component", "api", "rule_id", id)
		writeData(w, http.StatusOK, out)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.TriageWrite) {
			return
		}
		if err := s.triageRuleStore.Delete(r.Context(), id); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}
		logger.InfoCtx(r.Context(), "triage rule deleted", "component", "api", "rule_id", id)
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleTriageRulesReorder(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.triageRuleStore, "triage rule store") {
		return
	}
	if r.Method != http.MethodPut {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.TriageWrite) {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "ids is required")
		return
	}
	if err := s.triageRuleStore.Reorder(r.Context(), req.IDs); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}
	logger.InfoCtx(r.Context(), "triage rules reordered", "component", "api", "count", len(req.IDs))
	writeStatus(w, "reordered")
}

func (s *Server) handleTriageResults(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.triageResultStore, "triage result store") {
		return
	}
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	s.listTriageResults(w, r)
}

func (s *Server) handleTriageResultByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.triageResultStore, "triage result store") {
		return
	}
	id := pathID(r, "/api/v1/triage/results/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, err := s.triageResultStore.Get(r.Context(), id)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}
		if rec == nil {
			writeError(w, ErrorCodeNotFound, "triage result not found")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.TriageOverride) {
			return
		}
		var req struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if !validTriageDecision(req.Decision) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "decision must be one of: investigate, auto_resolve, suppress, escalate, enrich_only")
			return
		}
		now := time.Now().UTC()
		patch := &store.TriageResultRecord{
			Outcome:        store.TriageResultOutcomeOverridden,
			OverriddenTo:   req.Decision,
			OverriddenAt:   &now,
			OverrideReason: req.Reason,
		}
		if user := userFromContext(r.Context()); user != nil {
			id := user.ID
			patch.OverriddenBy = &id
		}
		out, err := s.triageResultStore.Update(r.Context(), id, patch)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}
		s.audit(r, store.AuditTriageOverridden, map[string]any{
			"triage_result_id": id,
			"decision":         req.Decision,
			"reason":           req.Reason,
		})
		logger.InfoCtx(r.Context(), "triage result overridden", "component", "api", "result_id", id, "decision", req.Decision)
		writeData(w, http.StatusOK, out)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleTriageStats(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.triageResultStore, "triage result store") {
		return
	}
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	confirmed, overridden, pending, err := s.triageResultStore.CountByOutcome(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to count by outcome")
		return
	}

	byDecision, err := s.triageResultStore.CountByDecision(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to count by decision")
		return
	}

	byCategory, err := s.triageResultStore.CountByCategory(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to count by category")
		return
	}

	avgConfidence, err := s.triageResultStore.AvgConfidence(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to get avg confidence")
		return
	}

	avgDurationMs, err := s.triageResultStore.AvgDurationMs(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to get avg duration")
		return
	}

	trend, err := s.triageResultStore.VolumeTrend(r.Context(), 30)
	if err != nil {
		writeInternalError(w, err, "failed to get volume trend")
		return
	}

	total := confirmed + overridden + pending
	var accuracy float64
	if completed := confirmed + overridden; completed > 0 {
		accuracy = float64(confirmed) / float64(completed) * 100
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":            total,
		"by_decision":      byDecision,
		"accuracy":         accuracy,
		"by_category":      byCategory,
		"avg_confidence":   avgConfidence,
		"avg_duration_ms":  avgDurationMs,
		"volume_trend_30d": trend,
	})
}

func (s *Server) listTriageRules(w http.ResponseWriter, r *http.Request) {
	q := store.TriageRuleQuery{
		Search: r.URL.Query().Get("search"),
	}
	if v := r.URL.Query().Get("enabled"); v != "" {
		enabled := v == "true" || v == "1"
		q.Enabled = &enabled
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.triageRuleStore.List(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list triage rules")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) createTriageRule(w http.ResponseWriter, r *http.Request) {
	var req triageRuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	record := &store.TriageRuleRecord{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Conditions:  req.Conditions,
		MatchMode:   req.MatchMode,
		Decision:    req.Decision,
		Severity:    req.Severity,
		Category:    req.Category,
		Enrichment:  req.Enrichment,
		Enabled:     true,
	}
	if req.Priority != nil {
		record.Priority = *req.Priority
	}
	if req.Enabled != nil {
		record.Enabled = *req.Enabled
	}
	out, err := s.triageRuleStore.Create(r.Context(), record)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}
	logger.InfoCtx(r.Context(), "triage rule created", "component", "api", "rule_id", out.ID.String(), "name", req.Name)
	writeData(w, http.StatusCreated, out)
}

func (s *Server) listTriageResults(w http.ResponseWriter, r *http.Request) {
	q := store.TriageResultQuery{
		Decision: r.URL.Query().Get("decision"),
		Outcome:  r.URL.Query().Get("outcome"),
		Category: r.URL.Query().Get("category"),
		Severity: r.URL.Query().Get("severity"),
		Search:   r.URL.Query().Get("search"),
	}
	if v := r.URL.Query().Get("start_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			q.StartDate = t
		}
	}
	if v := r.URL.Query().Get("end_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			q.EndDate = t
		}
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.triageResultStore.List(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list triage results")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}
