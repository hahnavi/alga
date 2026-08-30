// Package api: incident.go holds the incident collection/instance dispatch
// and core CRUD handlers (list, create, get, patch, delete) plus the shared
// parse/lookup helpers used across the incident_* split files.
package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"alga/escalation"
	"alga/incident"
	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/rbac"
	"alga/store"
	"alga/valkey"
	"alga/worker"

	"github.com/google/uuid"
)

func (s *Server) handleIncidents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListIncidents(w, r)
	case http.MethodPost:
		s.handleCreateIncident(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	q := r.URL.Query()
	filter := store.IncidentListFilter{}
	if v := q.Get("status"); v != "" {
		filter.Status = v
	}
	if v := q.Get("severity"); v != "" {
		filter.Severity = v
	}
	if v := q.Get("priority"); v != "" {
		filter.Priority = v
	}
	if v := q.Get("service_id"); v != "" {
		filter.ServiceID = v
	}
	if v := q.Get("commander_id"); v != "" {
		filter.CommanderID = v
	}
	if v := q.Get("search"); v != "" {
		filter.Search = v
	}
	if v := q.Get("start_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			filter.StartDate = &t
		}
	}
	if v := q.Get("end_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			filter.EndDate = &t
		}
	}

	limit, skip := parseLimitSkip(r, 20)
	filter.Limit = int(limit)
	filter.Skip = int(skip)
	if v := q.Get("sort"); v != "" {
		filter.Sort = v
	}

	records, total, err := s.incidentStore.ListIncidents(r.Context(), filter)
	if err != nil {
		writeInternalError(w, err, "failed to list incidents")
		return
	}
	writePaginatedJSON(w, records, total)
}

func (s *Server) handleCreateIncident(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	var req struct {
		Title              string         `json:"title"`
		Description        string         `json:"description,omitempty"`
		Severity           string         `json:"severity,omitempty"`
		ImpactLevel        string         `json:"impact_level,omitempty"`
		Priority           string         `json:"priority,omitempty"`
		IncidentType       string         `json:"incident_type,omitempty"`
		CommanderID        *string        `json:"commander_id,omitempty"`
		CommunicatorID     *string        `json:"communicator_id,omitempty"`
		OnCallResponderID  *string        `json:"on_call_responder_id,omitempty"`
		ServiceID          *string        `json:"service_id,omitempty"`
		ConferenceURL      string         `json:"conference_url,omitempty"`
		Tags               []string       `json:"tags,omitempty"`
		CustomFields       map[string]any `json:"custom_fields,omitempty"`
		SLATargetRespondAt *string        `json:"sla_target_respond_at,omitempty"`
		SLATargetResolveAt *string        `json:"sla_target_resolve_at,omitempty"`
		AlertNumbers       []int64        `json:"alert_numbers,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "title is required")
		return
	}

	if req.Severity != "" && !incident.ValidSeverity(req.Severity) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid severity: must be critical, high, warning, or info")
		return
	}
	if req.ImpactLevel != "" && !incident.ValidImpact(req.ImpactLevel) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid impact_level: must be high, medium, or low")
		return
	}
	if req.Severity == "" {
		req.Severity = "warning"
	}
	if req.ImpactLevel == "" {
		req.ImpactLevel = "medium"
	}
	if req.IncidentType != "" && !incident.ValidIncidentType(req.IncidentType) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid incident_type: must be real, alert, or degradation")
		return
	}
	if req.Priority == "" {
		req.Priority = incident.ComputePriority(req.Severity, req.ImpactLevel)
	} else if !incident.ValidPriority(req.Priority) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid priority: must be P1, P2, P3, P4, or P5")
		return
	}

	incidentNumber, err := s.incidentStore.ReserveIncidentNumber(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to reserve incident number")
		return
	}

	now := time.Now().UTC()
	record := &store.IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          strings.TrimSpace(req.Title),
		Description:    req.Description,
		Status:         "detected",
		Severity:       req.Severity,
		ImpactLevel:    req.ImpactLevel,
		Priority:       req.Priority,
		IncidentType:   req.IncidentType,
		ConferenceURL:  req.ConferenceURL,
		Tags:           req.Tags,
		CustomFields:   req.CustomFields,
		StartedAt:      &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if req.CommanderID != nil && *req.CommanderID != "" {
		uid, err := uuid.Parse(*req.CommanderID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid commander_id")
			return
		}
		record.CommanderID = &uid
	}
	if req.CommunicatorID != nil && *req.CommunicatorID != "" {
		uid, err := uuid.Parse(*req.CommunicatorID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid communicator_id")
			return
		}
		record.CommunicatorID = &uid
	}
	if req.OnCallResponderID != nil && *req.OnCallResponderID != "" {
		uid, err := uuid.Parse(*req.OnCallResponderID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid on_call_responder_id")
			return
		}
		record.OnCallResponderID = &uid
	}
	if req.ServiceID != nil && *req.ServiceID != "" {
		uid, err := uuid.Parse(*req.ServiceID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service_id")
			return
		}
		record.ServiceID = &uid
	}
	if req.SLATargetRespondAt != nil {
		if t, err := time.Parse(time.RFC3339, *req.SLATargetRespondAt); err == nil {
			record.SLATargetRespondAt = &t
		}
	}
	if req.SLATargetResolveAt != nil {
		if t, err := time.Parse(time.RFC3339, *req.SLATargetResolveAt); err == nil {
			record.SLATargetResolveAt = &t
		}
	}

	if record.SLATargetRespondAt == nil || record.SLATargetResolveAt == nil {
		respondDuration, resolveDuration := worker.PriorityToSLATargets(record.Priority)
		if record.SLATargetRespondAt == nil {
			respondAt := now.Add(respondDuration)
			record.SLATargetRespondAt = &respondAt
		}
		if record.SLATargetResolveAt == nil {
			resolveAt := now.Add(resolveDuration)
			record.SLATargetResolveAt = &resolveAt
		}
	}
	linkedAlerts := make([]store.AlertRecord, 0, len(req.AlertNumbers))
	if len(req.AlertNumbers) > 0 && s.alertStore != nil {
		for _, num := range req.AlertNumbers {
			rec, err := s.alertStore.GetByAlertNumber(num)
			if err != nil || rec == nil {
				continue
			}
			linkedAlerts = append(linkedAlerts, *rec)
		}
	}
	if len(linkedAlerts) > 0 {
		record.Status = "active"
		if strings.TrimSpace(record.Description) == "" {
			record.Description = incidentDescriptionFromAlert(linkedAlerts[0])
		}
	}

	created, err := s.incidentStore.CreateIncident(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create incident")
		return
	}

	for _, rec := range linkedAlerts {
		if err := s.alertStore.LinkAlertToIncident(r.Context(), rec.Fingerprint, created.IncidentNumber); err != nil {
			logger.Warn("Failed to link alert to incident during create", "component", "api", "incident_number", created.IncidentNumber, "fingerprint", rec.Fingerprint, "error", err)
		}
		s.postAlertIncidentHandoffMessage(r.Context(), rec.AlertNumber, strconv.FormatInt(created.IncidentNumber, 10))
	}
	if len(linkedAlerts) > 0 && s.alertInvestigationStore != nil {
		primary := linkedAlerts[0]
		existing, listErr := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(r.Context(), primary.AlertNumber)
		switch {
		case listErr != nil:
			// Cannot tell whether an investigation exists; creating one here
			// could duplicate it. Skip and let the alert pipeline proceed.
			logger.WarnCtx(r.Context(), "failed to list alert investigations during incident create; skipping auto-investigation", "incident_number", created.IncidentNumber, "alert_number", primary.AlertNumber, "error", listErr)
		case len(existing) == 0:
			correlated := make([]rabbitmq.CorrelatedAlert, 0, len(linkedAlerts))
			for _, rec := range linkedAlerts {
				correlated = append(correlated, correlatedAlertFromRecord(rec))
			}
			_, createErr := s.alertInvestigationStore.CreateAlertInvestigation(r.Context(), store.AlertInvestigationRecord{
				Alerts:                  correlated,
				CorrelationKey:          strconv.FormatInt(created.IncidentNumber, 10),
				Status:                  store.AlertInvestigationStatusPending,
				PromotedIncidentID:      &created.ID,
				PrimaryAlertFingerprint: primary.Fingerprint,
				PrimaryAlertNumber:      primary.AlertNumber,
			})
			if createErr != nil {
				logger.WarnCtx(r.Context(), "failed to queue incident-scoped alert investigation", "incident_number", created.IncidentNumber, "error", createErr)
			} else if s.pendingNotifier != nil {
				s.pendingNotifier.NotifyPending()
			}
		}
	}

	user := userFromContext(r.Context())
	actorID := ""
	if user != nil {
		actorID = user.ID.String()
	}
	if err := s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
		IncidentNumber: created.IncidentNumber,
		EventType:      "created",
		ActorID:        parseUUIDPtr(actorID),
		ActorType:      "user",
		Message:        "Incident created",
	}); err != nil {
		logger.Warn("Failed to add incident-created timeline entry", "component", "api", "incident_number", created.IncidentNumber, "error", err)
	}
	s.ensureIncidentInvestigation(r.Context(), created)
	if s.rabbitmqPublisher != nil {
		s.autoAssignICOnPromote(r, created)
		_ = s.rabbitmqPublisher.PublishICSProvision(r.Context(), rabbitmq.ICSProvisionMessage{IncidentNumber: created.IncidentNumber})
	}

	metrics.IncidentsCreatedTotal.Add(1)
	metrics.IncidentsActive.Add(1)
	s.publishIncidentEvent("incident_created", created)
	s.tryAutoCreateSlackChannel(r, created)
	s.audit(r, store.AuditIncidentCreated, map[string]any{
		"incident_number": created.IncidentNumber,
		"title":           req.Title,
	})

	s.invalidateDashboardCache(r)
	writeData(w, http.StatusCreated, created)
}

func incidentDescriptionFromAlert(alert store.AlertRecord) string {
	if desc := strings.TrimSpace(alert.Annotations["description"]); desc != "" {
		return desc
	}
	return strings.TrimSpace(alert.Annotations["summary"])
}

func correlatedAlertFromRecord(record store.AlertRecord) rabbitmq.CorrelatedAlert {
	values := make(map[string]float64)
	for k, v := range record.Values {
		if f, ok := v.(float64); ok {
			values[k] = f
		}
	}
	return rabbitmq.CorrelatedAlert{
		Fingerprint:  record.Fingerprint,
		AlertNumber:  record.AlertNumber,
		Labels:       record.Labels,
		Annotations:  record.Annotations,
		Status:       record.Status,
		StartsAt:     record.StartsAt.Format(time.RFC3339),
		Values:       values,
		GeneratorURL: record.GeneratorURL,
	}
}

func parseUUIDPtr(s string) *uuid.UUID {
	uid, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &uid
}

func (s *Server) handleIncidentRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/incidents/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
		return
	}

	postActions := map[string]func(w http.ResponseWriter, r *http.Request, incidentID string){
		"acknowledge":     s.handleAcknowledgeIncident,
		"mitigate":        s.handleMitigateIncident,
		"resolve":         s.handleResolveIncident,
		"close":           s.handleCloseIncident,
		"reopen":          s.handleReopenIncident,
		"cancel":          s.handleCancelIncident,
		"escalate":        s.handleEscalateIncident,
		"request-summary": s.handleRequestSummary,
	}

	for action, handler := range postActions {
		suffixStr := "/" + action
		if strings.HasSuffix(suffix, suffixStr) {
			if r.Method != http.MethodPost {
				writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
				return
			}
			incidentID := strings.TrimSuffix(suffix, suffixStr)
			handler(w, r, incidentID)
			return
		}
	}

	if suffix == "coordination/messages" || strings.HasSuffix(suffix, "/coordination/messages") {
		incidentID := strings.TrimSuffix(suffix, "/coordination/messages")
		if incidentID == "coordination/messages" {
			incidentID = ""
		}
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		s.handleIncidentCoordinationMessages(w, r, incidentID)
		return
	}

	if suffix == "status-updates" || strings.HasSuffix(suffix, "/status-updates") {
		incidentID := strings.TrimSuffix(suffix, "/status-updates")
		if incidentID == "status-updates" {
			incidentID = ""
		}
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		s.handleIncidentStatusUpdates(w, r, incidentID)
		return
	}

	if suffix == "slack-channel" || strings.HasSuffix(suffix, "/slack-channel") {
		incidentID := strings.TrimSuffix(suffix, "/slack-channel")
		if incidentID == "slack-channel" {
			incidentID = ""
		}
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		switch r.Method {
		case http.MethodPost:
			s.handleCreateSlackChannel(w, r, incidentID)
		case http.MethodDelete:
			s.handleDeleteSlackChannel(w, r, incidentID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if suffix == "google-meet" || strings.HasSuffix(suffix, "/google-meet") {
		incidentID := strings.TrimSuffix(suffix, "/google-meet")
		if incidentID == "google-meet" {
			incidentID = ""
		}
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		switch r.Method {
		case http.MethodPost:
			s.handleCreateGoogleMeet(w, r, incidentID)
		case http.MethodDelete:
			s.handleUnlinkGoogleMeet(w, r, incidentID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if idx := strings.Index(suffix, "/alerts/"); idx != -1 {
		if r.Method != http.MethodDelete {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		incidentID := suffix[:idx]
		numStr := suffix[idx+len("/alerts/"):]
		alertNumber, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
			return
		}
		s.handleUnlinkAlertFromIncident(w, r, incidentID, alertNumber)
		return
	}

	if suffix == "alerts" || strings.HasSuffix(suffix, "/alerts") {
		incidentID := strings.TrimSuffix(suffix, "/alerts")
		if incidentID == "alerts" {
			incidentID = ""
		}
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleListIncidentAlerts(w, r, incidentID)
		case http.MethodPost:
			s.handleLinkAlertToIncident(w, r, incidentID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if suffix == "timeline" || strings.HasSuffix(suffix, "/timeline") {
		incidentID := strings.TrimSuffix(suffix, "/timeline")
		if incidentID == "timeline" {
			incidentID = ""
		}
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleListIncidentTimeline(w, r, incidentID)
		case http.MethodPost:
			s.handleAddIncidentTimelineEntry(w, r, incidentID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if suffix == "investigations" || strings.HasSuffix(suffix, "/investigations") {
		incidentID := strings.TrimSuffix(suffix, "/investigations")
		if incidentID == "investigations" {
			incidentID = ""
		}
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleListIncidentInvestigations(w, r, incidentID)
		case http.MethodPost:
			s.handleCreateIncidentInvestigation(w, r, incidentID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if strings.Contains(suffix, "post-mortem") {
		incidentID := extractIncidentIDBeforePostMortem(suffix)
		if incidentID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
			return
		}
		s.handlePostMortemRoutes(w, r, incidentID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetIncident(w, r, suffix)
	case http.MethodPatch:
		s.handlePatchIncident(w, r, suffix)
	case http.MethodDelete:
		s.handleDeleteIncident(w, r, suffix)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) getIncidentOrError(w http.ResponseWriter, r *http.Request, incidentID string) (*store.IncidentRecord, bool) {
	incidentNumber, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid incident number")
		return nil, false
	}
	record, err := s.incidentStore.GetIncident(r.Context(), incidentNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get incident")
		return nil, false
	}
	// GetIncident is a tombstone read; operator handlers must not serve or
	// mutate a soft-deleted incident.
	if record == nil || record.DeletedAt != nil {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return nil, false
	}
	return record, true
}

func (s *Server) handleGetIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	record, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}

	type FindingSummary struct {
		Title    string   `json:"title"`
		Severity string   `json:"severity,omitempty"`
		Evidence []string `json:"evidence,omitempty"`
	}

	type InvestigationSummary struct {
		Total      int              `json:"total"`
		Completed  int              `json:"completed"`
		InProgress int              `json:"in_progress"`
		Findings   []FindingSummary `json:"findings,omitempty"`
	}

	var summary InvestigationSummary
	if s.incidentInvestigationStore != nil {
		investigations, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(r.Context(), mustParseIncidentNumber(incidentID))
		if err == nil {
			summary.Total = len(investigations)
			for _, inv := range investigations {
				switch inv.Status {
				case "complete", "resolved":
					summary.Completed++
				case "in_progress", "investigating", "assigned", "pending":
					summary.InProgress++
				}
				if inv.Findings != nil {
					for _, f := range inv.Findings {
						summary.Findings = append(summary.Findings, FindingSummary{
							Title:    f.Title,
							Severity: f.Severity,
							Evidence: f.Evidence,
						})
					}
				}
			}
		}
	}

	response := map[string]any{
		"incident":              record,
		"investigation_summary": summary,
	}
	writeData(w, http.StatusOK, response)
}

func (s *Server) handlePatchIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	var req struct {
		Title          *string  `json:"title"`
		Description    *string  `json:"description"`
		Summary        *string  `json:"summary"`
		Severity       *string  `json:"severity"`
		ImpactLevel    *string  `json:"impact_level"`
		Priority       *string  `json:"priority"`
		IncidentType   *string  `json:"incident_type"`
		CommanderID    *string  `json:"commander_id"`
		CommunicatorID *string  `json:"communicator_id"`
		ServiceID      *string  `json:"service_id"`
		ConferenceURL  *string  `json:"conference_url"`
		Tags           []string `json:"tags"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	current, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}

	if req.Severity != nil && !incident.ValidSeverity(*req.Severity) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid severity: must be critical, high, warning, or info")
		return
	}
	if req.ImpactLevel != nil && !incident.ValidImpact(*req.ImpactLevel) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid impact_level: must be high, medium, or low")
		return
	}
	if req.Priority != nil && *req.Priority != "" && !incident.ValidPriority(*req.Priority) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid priority: must be P1, P2, P3, P4, or P5")
		return
	}
	if req.IncidentType != nil && !incident.ValidIncidentType(*req.IncidentType) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid incident_type: must be real, alert, or degradation")
		return
	}
	if (req.Severity != nil || req.ImpactLevel != nil) && (req.Priority == nil || *req.Priority == "") {
		sev := current.Severity
		if req.Severity != nil && *req.Severity != "" {
			sev = *req.Severity
		}
		impact := current.ImpactLevel
		if req.ImpactLevel != nil && *req.ImpactLevel != "" {
			impact = *req.ImpactLevel
		}
		computed := incident.ComputePriority(sev, impact)
		current.Priority = computed
	}

	if req.Title != nil {
		current.Title = *req.Title
	}
	if req.Description != nil {
		current.Description = *req.Description
	}
	if req.Summary != nil {
		current.Summary = *req.Summary
	}
	if req.Severity != nil {
		current.Severity = *req.Severity
	}
	if req.ImpactLevel != nil {
		current.ImpactLevel = *req.ImpactLevel
	}
	if req.Priority != nil && *req.Priority != "" {
		current.Priority = *req.Priority
	}
	if req.IncidentType != nil {
		current.IncidentType = *req.IncidentType
	}
	if req.CommanderID != nil {
		if uid, err := uuid.Parse(*req.CommanderID); err == nil {
			current.CommanderID = &uid
		} else {
			current.CommanderID = nil
		}
	}
	if req.CommunicatorID != nil {
		if uid, err := uuid.Parse(*req.CommunicatorID); err == nil {
			current.CommunicatorID = &uid
		} else {
			current.CommunicatorID = nil
		}
	}
	if req.ServiceID != nil {
		if uid, err := uuid.Parse(*req.ServiceID); err == nil {
			current.ServiceID = &uid
		} else {
			current.ServiceID = nil
		}
	}
	if req.ConferenceURL != nil {
		current.ConferenceURL = *req.ConferenceURL
	}
	if req.Tags != nil {
		current.Tags = req.Tags
	}

	updated, err := s.incidentStore.UpdateIncident(r.Context(), mustParseIncidentNumber(incidentID), current)
	if err != nil {
		writeInternalError(w, err, "failed to update incident")
		return
	}

	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditIncidentUpdated, map[string]any{
		"incident_number": incidentID,
	})

	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsDelete) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	incidentNumber := mustParseIncidentNumber(incidentID)

	// Snapshot the incident and its investigations BEFORE the delete: the
	// tombstone makes post-delete lookups (investigations, metric state) come
	// back empty, which previously turned the agent abort signal into dead code.
	current, err := s.incidentStore.GetIncident(r.Context(), incidentNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get incident")
		return
	}
	if current == nil || current.DeletedAt != nil {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}
	var linked []store.IncidentInvestigationRecord
	if s.incidentInvestigationStore != nil {
		invs, listErr := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(r.Context(), incidentNumber)
		if listErr != nil {
			logger.WarnCtx(r.Context(), "failed to list incident investigations before delete", "incident_number", incidentID, "error", listErr)
		} else {
			linked = invs
		}
	}

	// Stop any pending escalation before the tombstone so the sweep's pending
	// entry and the cancel marker agree (the escalation_cancelled timeline
	// entry also needs a live row to attach to).
	s.cancelEscalationForIncident(r.Context(), incidentID, "incident deleted")

	if err := s.incidentStore.DeleteIncident(r.Context(), incidentNumber); err != nil {
		if errors.Is(err, store.ErrIncidentNotFound) {
			writeError(w, ErrorCodeNotFound, "incident not found")
			return
		}
		writeInternalError(w, err, "failed to delete incident")
		return
	}
	if !escalation.IsTerminalIncidentStatus(current.Status) {
		metrics.IncidentsActive.Add(-1)
	}

	deleteCtx, deleteCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer deleteCancel()
	s.onIncidentDeleted(deleteCtx, incidentNumber, linked)

	s.publishIncidentEvent("incident_deleted", map[string]string{"incident_number": incidentID})
	s.audit(r, store.AuditIncidentDeleted, map[string]any{
		"incident_number": incidentID,
	})

	s.invalidateDashboardCache(r)
	writeStatus(w, "deleted")
}

// onIncidentDeleted records a cancel marker for the deleted incident and
// best-effort pushes investigation_abort to agents with an in-flight incident
// investigation. The DB guard (hard-deleted rows) is the durable backstop.
// linked must be captured before DeleteIncident runs: once the incident is
// tombstoned, ListIncidentInvestigationsByIncident can no longer see its rows.
func (s *Server) onIncidentDeleted(ctx context.Context, incidentNumber int64, linked []store.IncidentInvestigationRecord) {
	if s.cancelSet != nil {
		_ = s.cancelSet.Add(ctx, valkey.CancelKeyIncident(incidentNumber))
	}
	for _, inv := range linked {
		if inv.AgentID == "" {
			continue
		}
		if s.investigationForwarder != nil && s.investigationForwarder.AgentOnline(inv.AgentID) {
			if s.cancelSet != nil {
				_ = s.cancelSet.Add(ctx, valkey.CancelKeyInvestigation(inv.IncidentInvestigationID))
			}
			s.forwardInvestigationSignal(inv.AgentID, inv.IncidentInvestigationID, "investigation_abort", "incident deleted", "system")
		}
	}
}

// parseIncidentNumber parses the URL-derived incident id into its numeric
// form. It returns 0 for empty/invalid input; callers must guard the 0 case
// (most downstream stores treat incident_number = 0 as a no-op). The name
// intentionally avoids the `must...` prefix because this function does not
// panic — silent failure is the historical contract.
func parseIncidentNumber(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		logger.Warn("parseIncidentNumber: invalid incident id", "component", "api", "input", s)
		return 0
	}
	return n
}

// mustParseIncidentNumber is kept as a backwards-compatible alias; new code
// should call parseIncidentNumber directly.
func mustParseIncidentNumber(s string) int64 { return parseIncidentNumber(s) }

func extractIncidentIDBeforePostMortem(suffix string) string {
	idx := strings.Index(suffix, "/post-mortem")
	if idx <= 0 {
		return ""
	}
	return suffix[:idx]
}
