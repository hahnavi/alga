package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/logger"
	"alga/store"
	"alga/valkey"
)

// decodeJSON delegates to platform.DecodeJSON.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	return platform.DecodeJSON(w, r, target)
}

// parseLimitSkip delegates to platform.ParseLimitSkip.
func parseLimitSkip(r *http.Request, defaultLimit int) (limit, skip int64) {
	return platform.ParseLimitSkip(r, defaultLimit)
}

// writePaginatedJSON delegates to platform.WritePaginatedJSON.
func writePaginatedJSON(w http.ResponseWriter, items any, total int64) {
	platform.WritePaginatedJSON(w, items, total)
}

// writeRawJSON writes pre-marshaled JSON bytes directly to the response.
func writeRawJSON(w http.ResponseWriter, status int, data []byte) {
	platform.WriteRawJSON(w, status, data)
}

func parseOptionalExpiry(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	if !t.After(time.Now()) {
		return nil, errExpiryInPast
	}
	return &t, nil
}

var errExpiryInPast = &expiryError{}

type expiryError struct{}

func (e *expiryError) Error() string { return "expires_at must be in the future" }

// parseActionItemDueDate parses an action-item due date. Accepts either a
// full RFC3339 timestamp or a bare calendar date (`YYYY-MM-DD`, interpreted as
// 23:59:59 UTC so a due date picked "today" is not immediately overdue).
// Unlike token expiries, past dates are valid — an overdue due date is a
// legitimate state the sweep reports on. Empty input means "no due date".
func parseActionItemDueDate(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	if len(raw) == 10 {
		if d, err := time.Parse(time.DateOnly, raw); err == nil {
			endOfDay := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, time.UTC)
			return &endOfDay, nil
		}
	}
	return nil, fmt.Errorf("due_date must be RFC3339 or YYYY-MM-DD: %q", raw)
}

// requireAgent retrieves the agent from the request context (delegating to
// platform.AgentFromContext) or writes a 401.
func requireAgent(w http.ResponseWriter, r *http.Request) (*platform.AgentTokenContext, bool) {
	return platform.RequireAgent(w, r)
}

func validateTokenName(w http.ResponseWriter, name string) bool {
	if name == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return false
	}
	if len(name) > maxTokenNameLength {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, fmt.Sprintf("name must be at most %d characters", maxTokenNameLength))
		return false
	}
	return true
}

func parseAndValidateExpiry(w http.ResponseWriter, raw string) (*time.Time, bool) {
	expPtr, err := parseOptionalExpiry(raw)
	if err != nil {
		if err == errExpiryInPast {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "expires_at must be in the future")
		} else {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid expires_at (use RFC3339)")
		}
		return nil, false
	}
	return expPtr, true
}

type alertActionActor struct {
	actor     *store.EventActor
	isAgent   bool
	agentName string
}

func userAlertActor(r *http.Request) *alertActionActor {
	u := userFromContext(r.Context())
	if u == nil {
		return nil
	}
	return &alertActionActor{
		actor: &store.EventActor{UserID: u.ID.String(), Username: u.Email, DisplayName: u.DisplayName()},
	}
}

func (s *Server) auditAlertAction(r *http.Request, event store.AuditEvent, a *alertActionActor, details map[string]any) {
	if a.isAgent {
		s.auditAgent(r, event, a.agentName, details)
	} else {
		s.audit(r, event, details)
	}
}

func (s *Server) audit(r *http.Request, event store.AuditEvent, details map[string]any) {
	if user := userFromContext(r.Context()); user != nil {
		s.auditStore.LogRecord(store.AuditRecord{
			Event:     event,
			UserID:    &user.ID,
			Username:  user.Email,
			IP:        s.ipExtractor.clientIP(r),
			UserAgent: r.UserAgent(),
			Success:   true,
			Details:   details,
			RequestID: logger.RequestIDFrom(r.Context()),
		})
	}
}

func (s *Server) auditAgent(r *http.Request, event store.AuditEvent, agentName string, details map[string]any) {
	if s.auditStore != nil {
		s.auditStore.LogRecord(store.AuditRecord{
			Event:     event,
			Username:  "agent:" + agentName,
			IP:        s.ipExtractor.clientIP(r),
			UserAgent: r.UserAgent(),
			Success:   true,
			Details:   details,
			RequestID: logger.RequestIDFrom(r.Context()),
		})
	}
}

// auditVoiceCallback records an audit event from a voice provider webhook
// (Telnyx/Twilio DTMF ack or silence). These callbacks have no authenticated
// request, so the actor is derived from the user that was paged (resolved by
// the caller) and falls back to the provider name when unknown. Fire-and-forget
// via a recovered goroutine so it can never block the webhook 200 response.
func (s *Server) auditVoiceCallback(event store.AuditEvent, userID *uuid.UUID, actor, provider string, details map[string]any) {
	if s.auditStore == nil {
		return
	}
	if actor == "" {
		actor = provider + ":voice"
	}
	go func() {
		defer func() { _ = recover() }()
		s.auditStore.Log(event, userID, actor, "", "", true, details)
	}()
}

func (s *Server) requireStore(w http.ResponseWriter, storeRef any, name string) bool {
	if storeRef == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, name+" not configured")
		return false
	}
	return true
}

// ensureSlice delegates to platform.EnsureSlice.
func ensureSlice[T any](s []T) []T {
	return platform.EnsureSlice(s)
}

// pathID delegates to platform.PathID.
func pathID(r *http.Request, prefix string) string {
	return platform.PathID(r, prefix)
}

// parseTimeQuery delegates to platform.ParseTimeQuery.
func parseTimeQuery(raw string) (time.Time, bool) {
	return platform.ParseTimeQuery(raw)
}

func (s *Server) invalidateDashboardCache(r *http.Request) {
	if s.cache != nil {
		_ = s.cache.Invalidate(r.Context(), valkey.PrefixDashboardStats)
	}
}
