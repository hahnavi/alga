package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/rbac"
	"alga/store"
)

var validHeartbeatSeverities = map[string]bool{
	"critical": true,
	"high":     true,
	"warning":  true,
	"info":     true,
}

func heartbeatAlertFingerprint(id uuid.UUID) string {
	return "heartbeat:" + id.String()
}

func heartbeatActor() *store.EventActor {
	return &store.EventActor{
		UserID:      "heartbeat",
		Username:    "heartbeat",
		DisplayName: "Heartbeat Monitor",
		Source:      "heartbeat",
	}
}

type heartbeatRequest struct {
	Name            string            `json:"name,omitempty"`
	Description     string            `json:"description,omitempty"`
	IntervalSeconds *int              `json:"interval_seconds,omitempty"`
	GraceSeconds    *int              `json:"grace_seconds,omitempty"`
	Severity        string            `json:"severity,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	OwnerTeamID     string            `json:"owner_team_id,omitempty"`
	Enabled         *bool             `json:"enabled,omitempty"`
}

func (s *Server) handleListHeartbeats(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.heartbeatStore, "heartbeat store") {
		return
	}
	q := store.HeartbeatQuery{}
	if v := r.URL.Query().Get("enabled"); v != "" {
		enabled := strings.EqualFold(v, "true") || v == "1"
		q.Enabled = &enabled
	}
	if v := r.URL.Query().Get("status"); v != "" {
		q.Status = v
	}
	if v := r.URL.Query().Get("search"); v != "" {
		q.Search = v
	}
	if v := r.URL.Query().Get("owner_team_id"); v != "" {
		if uid, err := uuid.Parse(v); err == nil {
			q.OwnerTeamID = &uid
		}
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.heartbeatStore.List(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list heartbeats")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) handleCreateHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.heartbeatStore, "heartbeat store") {
		return
	}
	var req heartbeatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	if req.IntervalSeconds == nil || *req.IntervalSeconds <= 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "interval_seconds must be a positive integer")
		return
	}
	severity := strings.TrimSpace(req.Severity)
	if severity == "" {
		severity = "warning"
	}
	if !validHeartbeatSeverities[severity] {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid severity (expected critical, high, warning, or info)")
		return
	}
	grace := 60
	if req.GraceSeconds != nil && *req.GraceSeconds >= 0 {
		grace = *req.GraceSeconds
	}

	record := &store.HeartbeatRecord{
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		IntervalSeconds: *req.IntervalSeconds,
		GraceSeconds:    grace,
		Enabled:         true,
		Severity:        severity,
		Labels:          req.Labels,
	}
	if req.Enabled != nil {
		record.Enabled = *req.Enabled
	}
	if req.OwnerTeamID != "" {
		uid, err := uuid.Parse(req.OwnerTeamID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid owner_team_id")
			return
		}
		record.OwnerTeamID = &uid
	}
	if user := userFromContext(r.Context()); user != nil {
		record.CreatedBy = user.Email
	}

	out, err := s.heartbeatStore.Create(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create heartbeat")
		return
	}
	logger.InfoCtx(r.Context(), "heartbeat created", "component", "api", "heartbeat_id", out.ID.String(), "name", out.Name)
	s.audit(r, store.AuditHeartbeatCreated, map[string]any{
		"heartbeat_id": out.ID.String(),
		"name":         out.Name,
	})
	writeData(w, http.StatusCreated, out)
}

func (s *Server) handleHeartbeatByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.heartbeatStore, "heartbeat store") {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/heartbeats/")
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing id")
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id, err := uuid.Parse(parts[0])
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid id")
		return
	}

	// Sub-resource: POST {id}/regenerate-token.
	if len(parts) == 2 && parts[1] == "regenerate-token" {
		if r.Method != http.MethodPost {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		if !s.checkPermission(w, r, rbac.HeartbeatsWrite) {
			return
		}
		s.regenerateHeartbeatToken(w, r, id)
		return
	}
	if len(parts) >= 2 {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.HeartbeatsRead) {
			return
		}
		s.getHeartbeat(w, r, id)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.HeartbeatsWrite) {
			return
		}
		s.updateHeartbeat(w, r, id)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.HeartbeatsDelete) {
			return
		}
		s.deleteHeartbeat(w, r, id)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) getHeartbeat(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	rec, err := s.heartbeatStore.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "failed to get heartbeat")
		return
	}
	if rec == nil {
		writeError(w, ErrorCodeNotFound, "heartbeat not found")
		return
	}
	writeData(w, http.StatusOK, rec)
}

func (s *Server) updateHeartbeat(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req heartbeatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	severity := strings.TrimSpace(req.Severity)
	if severity != "" && !validHeartbeatSeverities[severity] {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid severity (expected critical, high, warning, or info)")
		return
	}

	patch := &store.HeartbeatRecord{
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Severity:    severity,
		Labels:      req.Labels,
	}
	if req.IntervalSeconds != nil {
		if *req.IntervalSeconds <= 0 {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "interval_seconds must be a positive integer")
			return
		}
		patch.IntervalSeconds = *req.IntervalSeconds
	}
	if req.GraceSeconds != nil {
		if *req.GraceSeconds < 0 {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "grace_seconds must be non-negative")
			return
		}
		patch.GraceSeconds = *req.GraceSeconds
	}
	if req.OwnerTeamID != "" {
		uid, err := uuid.Parse(req.OwnerTeamID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid owner_team_id")
			return
		}
		patch.OwnerTeamID = &uid
	}
	if req.Enabled != nil {
		patch.Enabled = *req.Enabled
		patch.EnabledSet = true
	}

	out, err := s.heartbeatStore.Update(r.Context(), id, patch)
	if err != nil {
		if errors.Is(err, store.ErrHeartbeatNotFound) {
			writeError(w, ErrorCodeNotFound, "heartbeat not found")
			return
		}
		writeInternalError(w, err, "failed to update heartbeat")
		return
	}
	s.audit(r, store.AuditHeartbeatUpdated, map[string]any{
		"heartbeat_id": id.String(),
		"enabled":      out.Enabled,
	})
	writeData(w, http.StatusOK, out)
}

func (s *Server) deleteHeartbeat(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if err := s.heartbeatStore.Delete(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrHeartbeatNotFound) {
			writeError(w, ErrorCodeNotFound, "heartbeat not found")
			return
		}
		writeInternalError(w, err, "failed to delete heartbeat")
		return
	}
	logger.InfoCtx(r.Context(), "heartbeat deleted", "component", "api", "heartbeat_id", id.String())
	s.audit(r, store.AuditHeartbeatDeleted, map[string]any{
		"heartbeat_id": id.String(),
	})
	writeStatus(w, "deleted")
}

func (s *Server) regenerateHeartbeatToken(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	out, err := s.heartbeatStore.RegenerateToken(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrHeartbeatNotFound) {
			writeError(w, ErrorCodeNotFound, "heartbeat not found")
			return
		}
		writeInternalError(w, err, "failed to regenerate heartbeat token")
		return
	}
	s.audit(r, store.AuditHeartbeatTokenRegenerated, map[string]any{
		"heartbeat_id": id.String(),
	})
	writeData(w, http.StatusOK, out)
}

// handleHeartbeatPing is a public, token-gated endpoint that external systems
// hit on a schedule to prove liveness. It is NOT behind authMiddleware; the
// ping token in the path is the capability. It is rate-limited to blunt brute
// force against the token namespace.
func (s *Server) handleHeartbeatPing(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.heartbeatStore, "heartbeat store") {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "heartbeat store not configured")
		return
	}
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	rec, err := s.heartbeatStore.GetByPingToken(r.Context(), token)
	if err != nil {
		logger.Warn("heartbeat ping lookup failed", "component", "api", "error", err)
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}
	if rec == nil || !rec.Enabled {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	now := time.Now().UTC()
	if _, err := s.heartbeatStore.RecordPing(r.Context(), rec.ID, now); err != nil {
		writeInternalError(w, err, "failed to record heartbeat ping")
		return
	}

	// Recovery: if this heartbeat had an open expired alert, resolve it so the
	// incident pipeline and on-call are notified that liveness is restored.
	s.resolveHeartbeatAlert(r, rec.ID)

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeStatus(w, "ok")
}

// resolveHeartbeatAlert best-effort resolves any open alert generated for the
// given heartbeat. Failures are logged, never returned to the caller, since a
// ping should always succeed even if alert resolution has a transient issue.
func (s *Server) resolveHeartbeatAlert(r *http.Request, id uuid.UUID) {
	if s.alertStore == nil {
		return
	}
	fp := heartbeatAlertFingerprint(id)
	existing, err := s.alertStore.GetOpenByFingerprint(fp)
	if err != nil {
		logger.Warn("heartbeat recovery: failed to look up open alert", "component", "api", "fingerprint", fp, "error", err)
		return
	}
	if existing == nil {
		return
	}
	if err := s.alertStore.ResolveAlertByUser(fp, heartbeatActor()); err != nil {
		if !errors.Is(err, store.ErrAlertNotFiring) {
			logger.Warn("heartbeat recovery: failed to resolve alert", "component", "api", "fingerprint", fp, "error", err)
		}
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.RemoveTracking(fp)
	}
	if rec, rerr := s.alertStore.GetByFingerprint(fp); rerr == nil && rec != nil {
		s.publishAlertUpdated(rec)
	}
	s.invalidateDashboardCache(r)
	logger.Info("heartbeat recovered: resolved alert", "component", "api", "heartbeat_id", id.String(), "fingerprint", fp)
}
