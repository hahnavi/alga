// Code moved from http.go; see git history. Same-package code motion only.

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"alga/logger"
	"alga/rabbitmq"
	"alga/rbac"
	"alga/store"
	"alga/types"
	"alga/valkey"
	"alga/webhook"

	"github.com/google/uuid"
)

// searchAlertNumberQuery matches a plain numeric alert id or "#123" for list search.
var searchAlertNumberQuery = regexp.MustCompile(`^#?(\d+)$`)

// cancelFingerprintTTL is the TTL for the fingerprint-keyed cancel-set entry
// written by onAlertDeleted. Fingerprints are reused across alert lifecycles,
// so the key must expire quickly to avoid suppressing the next alert that
// reuses the fingerprint. alert_number remains the canonical identity with
// the default (long) TTL — see alga-domain-invariants.
const cancelFingerprintTTL = 60 * time.Second

// syncAlertChatPosts refreshes Mattermost/Slack messages for all delivery targets (best-effort; logs errors).
func (s *Server) syncAlertChatPosts(ctx context.Context, rec *store.AlertRecord) {
	if rec == nil {
		return
	}
	alert := webhook.AlertFromStoreRecord(rec)
	if err := webhook.UpdateChatPostsForAlert(ctx, s.chatRouter, rec, alert); err != nil {
		logger.Error("Failed to sync chat posts for alert", "fingerprint", rec.Fingerprint, "error", err)
	}
}

func (s *Server) finalizeAlertAction(w http.ResponseWriter, r *http.Request, fingerprint string) {
	record, err := s.alertStore.GetByFingerprint(fingerprint)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	s.syncAlertChatPosts(r.Context(), record)
	s.publishAlertUpdated(record)
	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, record)
}

func (s *Server) handleAlertResolve(w http.ResponseWriter, r *http.Request, fingerprint string, a *alertActionActor) {
	if a == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	if err := s.alertStore.ResolveAlertByUser(fingerprint, a.actor); err != nil {
		if errors.Is(err, store.ErrAlertNotFiring) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "alert is not firing or does not exist")
			return
		}
		writeInternalError(w, err, "failed to resolve alert")
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.RemoveTracking(fingerprint)
	}
	details := map[string]any{"fingerprint": fingerprint}
	if a.isAgent {
		details["agent_source"] = a.agentName
	}
	s.auditAlertAction(r, store.AuditAlertResolved, a, details)
	if s.alertInvestigationLifecycle != nil {
		if rec, rerr := s.alertStore.GetByFingerprint(fingerprint); rerr == nil && rec != nil && rec.AlertNumber > 0 {
			actorID := a.actor.UserID
			actorName := a.actor.DisplayName
			if actorName == "" {
				actorName = a.actor.Username
			}
			actorType := store.InvestigationActorUser
			if a.isAgent {
				actorType = store.InvestigationActorAgent
			}
			if err := s.alertInvestigationLifecycle.CompleteIfAllAlertsResolved(r.Context(), store.AlertInvestigationLifecycleCompletionRequest{
				AlertNumber: rec.AlertNumber,
				Reason:      store.AlertInvestigationCompletedReasonAlertsResolved,
				ActorType:   actorType,
				ActorID:     actorID,
				ActorName:   actorName,
			}); err != nil {
				logger.WarnCtx(r.Context(), "resolve: linked investigation completion failed", "alert_number", rec.AlertNumber, "error", err)
			}
		}
	}
	s.finalizeAlertAction(w, r, fingerprint)
}

func (s *Server) handleAlertReopen(w http.ResponseWriter, r *http.Request, fingerprint string, a *alertActionActor) {
	if a == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	ev := store.AlertEventWithActor("reopened", time.Now(), a.actor)
	if err := s.alertStore.ReopenAlert(fingerprint, ev); err != nil {
		if errors.Is(err, store.ErrAlertNotResolved) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "alert is not resolved or does not exist")
			return
		}
		writeInternalError(w, err, "failed to reopen alert")
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.MarkTracked(fingerprint)
	}
	details := map[string]any{"fingerprint": fingerprint}
	if a.isAgent {
		details["agent_source"] = a.agentName
	}
	s.auditAlertAction(r, store.AuditAlertReopened, a, details)
	s.reopenLinkedInvestigations(fingerprint)
	s.finalizeAlertAction(w, r, fingerprint)
}

func (s *Server) reopenLinkedInvestigations(fingerprint string) {
	record, err := s.alertStore.GetByFingerprint(fingerprint)
	if err != nil {
		logger.Error("failed to get alert for linked investigation reopen", "fingerprint", fingerprint, "error", err)
		return
	}
	if s.alertInvestigationStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	linked, err := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(ctx, record.AlertNumber)
	if err != nil {
		logger.Error("failed to list linked alert investigations for alert reopen", "fingerprint", fingerprint, "error", err)
		return
	}
	if len(linked) == 0 {
		return
	}
	latest := linked[0]
	if !store.IsReopenableInvestigationStatus(latest.Status) {
		return
	}

	// Auto-acknowledge alerts because the investigation is resuming.
	if s.alertStore != nil {
		actor := &store.EventActor{Username: "System", Source: "system"}
		alertsToAck := latest.Alerts
		if len(alertsToAck) == 0 {
			alertsToAck = []rabbitmq.CorrelatedAlert{{Fingerprint: record.Fingerprint, AlertNumber: record.AlertNumber}}
		}
		for _, a := range alertsToAck {
			if err := s.alertStore.AcknowledgeAlert(a.Fingerprint, actor); err != nil {
				logger.Warn("failed to auto-acknowledge alert on investigation reopen", "fingerprint", a.Fingerprint, "error", err)
			} else if rec, err := s.alertStore.GetByFingerprint(a.Fingerprint); err == nil {
				s.publishAlertUpdated(rec)
			}
		}
	}

	alertName := strings.TrimSpace(record.Labels["alertname"])
	if alertName == "" {
		alertName = "Alert"
	}
	alertRef := fmt.Sprintf("[%s](/alerts/%s)", alertName, fingerprint)
	if record.AlertNumber > 0 {
		alertRef = fmt.Sprintf("%s (#%d)", alertRef, record.AlertNumber)
	}
	updateMsg := fmt.Sprintf("Investigation reopened because %s was reopened", alertRef)
	invID := latest.AlertInvestigationID
	invUUID := latest.ID.String()
	agentAvailable := latest.AgentID != "" && s.investigationForwarder != nil && s.investigationForwarder.AgentOnline(latest.AgentID)

	if agentAvailable {
		if err := s.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, invUUID, slices.Concat(store.InvestigationTerminalStatuses, []string{"paused"}), "investigating"); err != nil {
			logger.Error("failed to transition alert investigation on reopen", "investigation_id", invID, "error", err)
			return
		}
		_ = s.alertInvestigationStore.AddAlertInvestigationUpdate(ctx, invID, store.InvestigationUpdate{
			Type:     store.UpdateTypeComment,
			Message:  updateMsg,
			Source:   store.UpdateSourceSystem,
			Internal: true,
		})
		s.forwardInvestigationSignal(latest.AgentID, invID, "investigation_resume", "", "system")
		s.publishInvestigationEvent(invID, "investigation_started", map[string]any{
			"alert_investigation_id": invID,
			"status":                 "investigating",
			"agent_name":             latest.AgentName,
			"agent_type":             latest.AgentType,
		})
		s.publishInvestigationEvent(invID, "investigation_update", map[string]any{
			"alert_investigation_id": invID,
			"status":                 "investigating",
			"agent_name":             latest.AgentName,
			"agent_type":             latest.AgentType,
		})
	} else {
		previousAgentName := latest.AgentName
		if err := s.alertInvestigationStore.UpdateAlertInvestigationAgent(ctx, invUUID, "", "", ""); err != nil {
			logger.Error("failed to clear agent on alert investigation reopen", "investigation_id", invID, "error", err)
			return
		}
		if err := s.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, invUUID, slices.Concat(store.InvestigationTerminalStatuses, []string{"paused"}), "pending"); err != nil {
			logger.Error("failed to transition alert investigation to pending on reopen", "investigation_id", invID, "error", err)
			return
		}
		reassignMsg := updateMsg
		if previousAgentName != "" {
			reassignMsg = fmt.Sprintf("%s (reassigned: agent %s unavailable)", updateMsg, previousAgentName)
		}
		_ = s.alertInvestigationStore.AddAlertInvestigationUpdate(ctx, invID, store.InvestigationUpdate{
			Type:     store.UpdateTypeComment,
			Message:  reassignMsg,
			Source:   store.UpdateSourceSystem,
			Internal: true,
		})
		if s.pendingNotifier != nil {
			s.pendingNotifier.NotifyPending()
		}
		s.publishInvestigationEvent(invID, "investigation_update", map[string]any{
			"alert_investigation_id": invID,
			"status":                 "pending",
			"agent_name":             "",
			"agent_type":             "",
		})
	}
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeAlertsQueryResponse(w, r)
	case http.MethodPost:
		s.handleCreateAlert(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

type createAlertRequest struct {
	Alertname   string            `json:"alertname"`
	Severity    string            `json:"severity,omitempty"`
	Message     string            `json:"message,omitempty"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Source      string            `json:"source,omitempty"`
}

const (
	manualAlertMaxFieldLen   = 256
	manualAlertMaxEntries    = 32
	manualAlertMaxBodyLen    = 16 * 1024
	manualAlertDescriptionMx = 4096
)

var manualAlertLabelKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func sanitizeManualAlertMap(m map[string]string, allowMultilineValues bool) (map[string]string, error) {
	out := make(map[string]string, len(m))
	if len(m) > manualAlertMaxEntries {
		return nil, fmt.Errorf("too many entries (max %d)", manualAlertMaxEntries)
	}
	for k, v := range m {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if len(key) > manualAlertMaxFieldLen {
			return nil, fmt.Errorf("key too long: %q", key)
		}
		if !manualAlertLabelKeyRe.MatchString(key) {
			return nil, fmt.Errorf("invalid key %q: must match [A-Za-z_][A-Za-z0-9_]*", key)
		}
		val := v
		if !allowMultilineValues {
			val = strings.TrimSpace(val)
		}
		if len(val) > manualAlertDescriptionMx {
			return nil, fmt.Errorf("value for %q too long", key)
		}
		out[key] = val
	}
	return out, nil
}

func (s *Server) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.AlertsWrite) {
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	if s.alertIngestor == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "alert ingestor not configured")
		return
	}

	// Pre-cap the body to the manual-alert limit; decodeJSON will further wrap
	// with the standard 1 MiB cap, but the smaller limit here takes effect.
	r.Body = http.MaxBytesReader(w, r.Body, manualAlertMaxBodyLen)
	var req createAlertRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	alertname := strings.TrimSpace(req.Alertname)
	if alertname == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "alertname is required")
		return
	}
	if len(alertname) > manualAlertMaxFieldLen {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "alertname too long")
		return
	}

	severity := strings.TrimSpace(req.Severity)
	if len(severity) > manualAlertMaxFieldLen {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "severity too long")
		return
	}

	message := strings.TrimSpace(req.Message)
	description := req.Description
	if len(message) > manualAlertDescriptionMx {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "message too long")
		return
	}
	if len(description) > manualAlertDescriptionMx {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "description too long")
		return
	}

	labels, err := sanitizeManualAlertMap(req.Labels, false)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid labels: "+err.Error())
		return
	}
	annotations, err := sanitizeManualAlertMap(req.Annotations, true)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid annotations: "+err.Error())
		return
	}

	labels["alertname"] = alertname
	if severity != "" {
		labels["severity"] = severity
	}
	if message != "" {
		if _, exists := annotations["summary"]; !exists {
			annotations["summary"] = message
		}
	}
	if description != "" {
		if _, exists := annotations["description"]; !exists {
			annotations["description"] = description
		}
	}

	source := strings.TrimSpace(req.Source)
	if len(source) > 2048 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "source URL too long")
		return
	}

	now := time.Now().UTC()
	fp := "manual-" + uuid.NewString()
	alert := types.Alert{
		Status:       "firing",
		Labels:       labels,
		Annotations:  annotations,
		StartsAt:     now.Format(time.RFC3339),
		Fingerprint:  fp,
		GeneratorURL: source,
	}

	actor := &store.EventActor{
		UserID:      user.ID.String(),
		Username:    user.Email,
		DisplayName: user.DisplayName(),
		Source:      "user",
	}

	rec, err := s.alertIngestor.IngestManualAlert(r.Context(), alert, actor)
	if err != nil {
		writeInternalError(w, err, "failed to create alert")
		return
	}

	s.audit(r, store.AuditAlertCreated, map[string]any{
		"fingerprint":  rec.Fingerprint,
		"alert_number": rec.AlertNumber,
		"alertname":    alertname,
		"severity":     severity,
	})
	writeData(w, http.StatusCreated, rec)
	s.invalidateDashboardCache(r)
}

func alertsFilterFromRequest(r *http.Request) map[string]any {
	filter := map[string]any{}
	if v := r.URL.Query().Get("status"); v != "" {
		switch strings.ToLower(v) {
		case "open":
			filter["status"] = "firing"
		case "closed":
			filter["status"] = "resolved"
		case "unacknowledged":
			filter["status"] = "firing"
			filter["acknowledged"] = false
		case "acknowledged":
			filter["acknowledged"] = true
		case "all":
		default:
			filter["status"] = v
		}
	}
	if v := r.URL.Query().Get("channel"); v != "" {
		filter["channel"] = v
	}
	if v := r.URL.Query().Get("provider"); v != "" {
		filter["provider"] = v
	}
	if v := r.URL.Query().Get("severity"); v != "" {
		filter["severity"] = v
	}

	if v := r.URL.Query().Get("search"); v != "" {
		trimmed := strings.TrimSpace(v)
		if m := searchAlertNumberQuery.FindStringSubmatch(trimmed); len(m) == 2 {
			if n, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				filter["alert_number"] = n
			}
		} else {
			filter["search"] = trimmed
		}
	}
	if v := r.URL.Query().Get("start_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			filter["start_date"] = t
		}
	}
	if v := r.URL.Query().Get("end_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			filter["end_date"] = t
		}
	}
	return filter
}

func (s *Server) writeAlertsQueryResponse(w http.ResponseWriter, r *http.Request) {
	filter := alertsFilterFromRequest(r)

	limit, skip := parseLimitSkip(r, 50)
	filter["$limit"] = limit
	filter["$skip"] = skip

	if v := r.URL.Query().Get("sort"); v != "" {
		filter["$sort"] = v
	} else {
		filter["$sort"] = "-updated_at"
	}

	if _, hasSearch := filter["search"]; hasSearch {
		filter["summary"] = true
	}

	records, err := s.alertStore.QueryAlerts(filter)
	if err != nil {
		writeInternalError(w, err, "failed to query alerts")
		return
	}
	writeData(w, http.StatusOK, records)
}

func (s *Server) handleAlertByNumber(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/alerts/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing alert number")
		return
	}

	// POST /api/v1/alerts/{alert_number}/acknowledge
	if strings.HasSuffix(suffix, "/acknowledge") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		numStr := strings.TrimSuffix(suffix, "/acknowledge")
		alertNumber, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
			return
		}
		if !s.checkPermission(w, r, rbac.AlertsWrite) {
			return
		}
		s.handleAlertAcknowledgeByNumber(w, r, alertNumber, userAlertActor(r))
		return
	}

	// POST /api/v1/alerts/{alert_number}/resolve
	if strings.HasSuffix(suffix, "/resolve") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		numStr := strings.TrimSuffix(suffix, "/resolve")
		alertNumber, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
			return
		}
		if !s.checkPermission(w, r, rbac.AlertsWrite) {
			return
		}
		s.handleAlertResolveByNumber(w, r, alertNumber, userAlertActor(r))
		return
	}

	// POST /api/v1/alerts/{alert_number}/reopen
	if strings.HasSuffix(suffix, "/reopen") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		numStr := strings.TrimSuffix(suffix, "/reopen")
		alertNumber, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
			return
		}
		if !s.checkPermission(w, r, rbac.AlertsWrite) {
			return
		}
		s.handleAlertReopenByNumber(w, r, alertNumber, userAlertActor(r))
		return
	}

	// POST /api/v1/alerts/{alert_number}/investigate
	if strings.HasSuffix(suffix, "/investigate") {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		numStr := strings.TrimSuffix(suffix, "/investigate")
		alertNumber, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
			return
		}
		if !s.checkPermission(w, r, rbac.AlertsWrite) {
			return
		}
		s.handleAlertInvestigate(w, r, alertNumber)
		return
	}

	// DELETE /api/v1/alerts/{alert_number}
	if r.Method == http.MethodDelete {
		alertNumber, err := strconv.ParseInt(suffix, 10, 64)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
			return
		}
		user := userFromContext(r.Context())
		if user == nil {
			writeError(w, ErrorCodeUnauthorized, "unauthorized")
			return
		}
		if !s.checkPermission(w, r, rbac.AlertsDelete) {
			return
		}

		record, err := s.alertStore.GetByAlertNumber(alertNumber)
		if err != nil {
			writeInternalError(w, err, "failed to get alert")
			return
		}
		if record == nil {
			writeError(w, ErrorCodeNotFound, "not found")
			return
		}

		if err := s.alertStore.DeleteAlertByNumber(alertNumber); err != nil {
			if errors.Is(err, store.ErrAlertNotFound) {
				writeError(w, ErrorCodeNotFound, "not found")
				return
			}
			writeInternalError(w, err, "failed to delete alert")
			return
		}

		deleteCtx, deleteCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer deleteCancel()
		s.onAlertDeleted(deleteCtx, record)

		s.audit(r, store.AuditAlertDeleted, map[string]any{
			"fingerprint":  record.Fingerprint,
			"alert_number": record.AlertNumber,
		})

		if s.dedupCache != nil {
			s.dedupCache.RemoveTracking(record.Fingerprint)
		}

		if s.cooldownRemover != nil {
			s.cooldownRemover.RemoveCooldown(r.Context(), record.Labels)
		}

		s.publishAlertDeleted(record)
		s.invalidateDashboardCache(r)
		writeStatus(w, "deleted")
		return
	}

	// GET /api/v1/alerts/{alert_number}
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	alertNumber, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
		return
	}
	record, err := s.alertStore.GetByAlertNumber(alertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	response := map[string]any{
		"alert": record,
	}

	if s.alertInvestigationStore != nil {
		alertInv, err := s.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(r.Context(), alertNumber)
		if err == nil && alertInv != nil {
			response["alert_investigation"] = alertInv
		}
	}
	writeData(w, http.StatusOK, response)
}

// onAlertDeleted records cancel markers for the deleted alert and best-effort
// pushes investigation_abort to agents with an in-flight linked investigation.
// The DB guard (hard-deleted investigation rows) is the durable backstop; this
// cancel set is the fast path and the abort push makes agent halt prompt.
//
// alert_number is the canonical alert identity, so its cancel-set key uses the
// default (long) TTL. The fingerprint key is also written so the brief in-flight
// window — where a published message may carry only a fingerprint — still
// observes the cancel. Because fingerprints are reused across alert lifecycles,
// the fingerprint key is written with a short TTL to avoid poisoning the cancel
// set for the next alert that reuses the fingerprint (see alga-domain-invariants).
func (s *Server) onAlertDeleted(ctx context.Context, record *store.AlertRecord) {
	if s.cancelSet != nil {
		if record.AlertNumber > 0 {
			_ = s.cancelSet.Add(ctx, valkey.CancelKeyAlertNum(record.AlertNumber))
		}
		if record.Fingerprint != "" {
			_ = s.cancelSet.AddWithTTL(ctx, valkey.CancelKeyAlert(record.Fingerprint), cancelFingerprintTTL)
		}
	}
	if s.alertInvestigationStore == nil {
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	linked, err := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(listCtx, record.AlertNumber)
	if err != nil {
		logger.Warn("failed to list linked alert investigations for abort", "alert_number", record.AlertNumber, "error", err)
		return
	}
	for _, inv := range linked {
		if inv.AgentID == "" {
			continue
		}
		if s.investigationForwarder != nil && s.investigationForwarder.AgentOnline(inv.AgentID) {
			if s.cancelSet != nil {
				_ = s.cancelSet.Add(ctx, valkey.CancelKeyInvestigation(inv.AlertInvestigationID))
			}
			s.forwardInvestigationSignal(inv.AgentID, inv.AlertInvestigationID, "investigation_abort", "alert deleted", "system")
		}
	}
}

func (s *Server) handleAlertAcknowledgeByNumber(w http.ResponseWriter, r *http.Request, alertNumber int64, a *alertActionActor) {
	if a == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	if err := s.alertStore.AcknowledgeAlertByNumber(alertNumber, a.actor); err != nil {
		if errors.Is(err, store.ErrAlertNotFound) {
			writeError(w, ErrorCodeNotFound, "not found")
			return
		}
		writeInternalError(w, err, "failed to acknowledge alert")
		return
	}
	record, err := s.alertStore.GetByAlertNumber(alertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	details := map[string]any{"fingerprint": record.Fingerprint, "alert_number": alertNumber}
	if a.isAgent {
		details["agent_source"] = a.agentName
	}
	s.auditAlertAction(r, store.AuditAlertAcknowledged, a, details)
	s.syncAlertChatPosts(r.Context(), record)
	s.publishAlertUpdated(record)
	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, record)
}

func (s *Server) handleAlertResolveByNumber(w http.ResponseWriter, r *http.Request, alertNumber int64, a *alertActionActor) {
	if a == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	if err := s.alertStore.ResolveAlertByNumber(alertNumber, a.actor); err != nil {
		if errors.Is(err, store.ErrAlertNotFiring) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "alert is not firing or does not exist")
			return
		}
		writeInternalError(w, err, "failed to resolve alert")
		return
	}
	record, err := s.alertStore.GetByAlertNumber(alertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.RemoveTracking(record.Fingerprint)
	}
	details := map[string]any{"fingerprint": record.Fingerprint, "alert_number": alertNumber}
	if a.isAgent {
		details["agent_source"] = a.agentName
	}
	s.auditAlertAction(r, store.AuditAlertResolved, a, details)
	s.syncAlertChatPosts(r.Context(), record)
	s.publishAlertUpdated(record)
	s.invalidateDashboardCache(r)
	if s.alertInvestigationLifecycle != nil && alertNumber > 0 {
		actorID := a.actor.UserID
		actorName := a.actor.DisplayName
		if actorName == "" {
			actorName = a.actor.Username
		}
		actorType := store.InvestigationActorUser
		if a.isAgent {
			actorType = store.InvestigationActorAgent
		}
		if err := s.alertInvestigationLifecycle.CompleteIfAllAlertsResolved(r.Context(), store.AlertInvestigationLifecycleCompletionRequest{
			AlertNumber: alertNumber,
			Reason:      store.AlertInvestigationCompletedReasonAlertsResolved,
			ActorType:   actorType,
			ActorID:     actorID,
			ActorName:   actorName,
		}); err != nil {
			logger.WarnCtx(r.Context(), "resolve: linked investigation completion failed", "alert_number", alertNumber, "error", err)
		}
	}
	writeData(w, http.StatusOK, record)
}

func (s *Server) handleAlertReopenByNumber(w http.ResponseWriter, r *http.Request, alertNumber int64, a *alertActionActor) {
	if a == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	ev := store.AlertEventWithActor("reopened", time.Now(), a.actor)
	if err := s.alertStore.ReopenAlertByNumber(alertNumber, ev); err != nil {
		if errors.Is(err, store.ErrAlertNotFound) {
			// Same sentinel as ack/delete for this path (missing or not
			// resolved); 404 keeps the by-number action codes aligned.
			writeError(w, ErrorCodeNotFound, "not found")
			return
		}
		writeInternalError(w, err, "failed to reopen alert")
		return
	}
	record, err := s.alertStore.GetByAlertNumber(alertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.MarkTracked(record.Fingerprint)
	}
	details := map[string]any{"fingerprint": record.Fingerprint, "alert_number": alertNumber}
	if a.isAgent {
		details["agent_source"] = a.agentName
	}
	s.auditAlertAction(r, store.AuditAlertReopened, a, details)
	s.reopenLinkedInvestigations(record.Fingerprint)
	s.syncAlertChatPosts(r.Context(), record)
	s.publishAlertUpdated(record)
	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, record)
}

func (s *Server) handleAlertRelated(w http.ResponseWriter, r *http.Request) {
	alertNumberStr := r.PathValue("alert_number")
	if alertNumberStr == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing alert number")
		return
	}
	alertNumber, err := strconv.ParseInt(alertNumberStr, 10, 64)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
		return
	}

	record, err := s.alertStore.GetByAlertNumber(alertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	type relatedAlert struct {
		Fingerprint string            `json:"fingerprint"`
		AlertNumber int64             `json:"alert_number,omitempty"`
		Status      string            `json:"status"`
		Labels      map[string]string `json:"labels"`
		StartsAt    string            `json:"starts_at"`
	}

	response := map[string]any{
		"related_alerts": []relatedAlert{},
		"incident":       nil,
	}

	setIncident := func(inc *store.IncidentRecord) {
		if inc == nil {
			return
		}
		incidentMap := map[string]any{
			"incident_number": inc.IncidentNumber,
			"title":           inc.Title,
			"status":          inc.Status,
			"severity":        inc.Severity,
			"priority":        inc.Priority,
		}
		if inc.DeletedAt != nil {
			incidentMap["deleted_at"] = inc.DeletedAt
		}
		response["incident"] = incidentMap
	}

	loadDirectIncident := func() {
		if response["incident"] != nil {
			return
		}
		lister, ok := s.alertStore.(interface {
			GetIncidentsByAlertNumber(context.Context, int64) ([]store.IncidentRecord, error)
		})
		if !ok {
			return
		}
		incs, err := lister.GetIncidentsByAlertNumber(r.Context(), alertNumber)
		if err == nil && len(incs) > 0 {
			setIncident(&incs[0])
		}
	}

	if s.alertInvestigationStore == nil {
		loadDirectIncident()
		writeData(w, http.StatusOK, response)
		return
	}

	alertInvs, err := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(r.Context(), alertNumber)
	if err != nil {
		loadDirectIncident()
		writeData(w, http.StatusOK, response)
		return
	}

	if len(alertInvs) == 0 {
		loadDirectIncident()
		writeData(w, http.StatusOK, response)
		return
	}

	seen := make(map[string]struct{})
	related := make([]relatedAlert, 0)
	for _, inv := range alertInvs {
		for _, a := range inv.Alerts {
			if a.Fingerprint == record.Fingerprint {
				continue
			}
			if _, ok := seen[a.Fingerprint]; ok {
				continue
			}
			seen[a.Fingerprint] = struct{}{}

			status := a.Status
			if status == "" {
				actual, fetchErr := s.alertStore.GetByFingerprint(a.Fingerprint)
				if fetchErr == nil && actual != nil {
					status = actual.Status
				}
			}

			related = append(related, relatedAlert{
				Fingerprint: a.Fingerprint,
				AlertNumber: a.AlertNumber,
				Status:      status,
				Labels:      a.Labels,
				StartsAt:    a.StartsAt,
			})
		}
	}
	response["related_alerts"] = related

	primary := alertInvs[0]
	if primary.PromotedIncidentID != nil && s.incidentStore != nil {
		inc, err := s.incidentStore.GetIncidentByID(r.Context(), *primary.PromotedIncidentID)
		if err == nil && inc != nil {
			setIncident(inc)
		}
	}
	loadDirectIncident()
	writeData(w, http.StatusOK, response)
}

func (s *Server) handleAlertInvestigate(w http.ResponseWriter, r *http.Request, alertNumber int64) {
	if s.investigator == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "investigation pipeline not configured")
		return
	}

	record, err := s.alertStore.GetByAlertNumber(alertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "alert not found")
		return
	}
	if record.Status == "resolved" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "resolved alerts cannot be investigated")
		return
	}

	if s.alertInvestigationStore != nil {
		existing, listErr := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(r.Context(), alertNumber)
		if listErr == nil && len(existing) > 0 {
			writeError(w, ErrorCodeConflict, "investigation already exists for this alert")
			return
		}
	}

	values := make(map[string]float64)
	if record.Values != nil {
		for k, v := range record.Values {
			if f, ok := v.(float64); ok {
				values[k] = f
			}
		}
	}

	correlatedAlert := rabbitmq.CorrelatedAlert{
		Fingerprint:  record.Fingerprint,
		AlertNumber:  record.AlertNumber,
		Labels:       record.Labels,
		Annotations:  record.Annotations,
		Status:       record.Status,
		StartsAt:     record.StartsAt.Format(time.RFC3339),
		Values:       values,
		GeneratorURL: record.GeneratorURL,
	}

	if err := s.investigator.ProcessAlert(r.Context(), correlatedAlert); err != nil {
		writeInternalError(w, err, "failed to trigger investigation")
		return
	}

	s.audit(r, store.AuditAlertInvestigated, map[string]any{
		"fingerprint":  record.Fingerprint,
		"alert_number": alertNumber,
	})

	if s.pendingNotifier != nil {
		s.pendingNotifier.NotifyPending()
	}

	response := map[string]any{"alert": record}
	if s.alertInvestigationStore != nil {
		records, listErr := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(r.Context(), alertNumber)
		if listErr == nil {
			response["alert_investigations"] = records
			if len(records) > 0 {
				response["alert_investigation"] = records[0]
			}
		}
	}
	writeData(w, http.StatusOK, response)
}
