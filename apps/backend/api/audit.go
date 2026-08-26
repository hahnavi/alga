// Package api: audit.go exposes the read-only audit-events review endpoint
// described by spec 09_identity_access/05 (finding C9: `audit:read` was granted
// to admin/operator yet gated no route). The route is classified as an
// RBAC-protected operator/frontend GET: registered through authMiddleware with
// rbac.AuditRead, no request body, no mutation, and therefore nothing to audit.
// Audit data is global system data; cross-tenant scoping is not applicable —
// the permission gate itself is the boundary.
package api

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	if s.auditStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "audit store not configured")
		return
	}

	q := r.URL.Query()
	limit, skip := parseLimitSkip(r, 100)
	filter := map[string]any{
		"$limit": limit,
		"$skip":  skip,
	}
	if v := strings.TrimSpace(q.Get("event")); v != "" {
		filter["event"] = v
	}
	if v := strings.TrimSpace(q.Get("entity_type")); v != "" {
		filter["entity_type"] = v
	}
	if v := strings.TrimSpace(q.Get("entity_id")); v != "" {
		if _, err := uuid.Parse(v); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid entity_id (use a UUID)")
			return
		}
		filter["entity_id"] = v
	}

	events, total, err := s.auditStore.Query(filter)
	if err != nil {
		writeInternalError(w, err, "failed to query audit events")
		return
	}
	writePaginatedJSON(w, ensureSlice(events), total)
}
