package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"alga/api/platform"
	"alga/capability"
	"alga/config"
	"alga/logger"
	"alga/rbac"
	"alga/store"
)

// knowledgeNoteRequest is the wire shape used for both POST and PUT on
// /api/v1/knowledge. Fields that are unset on PUT are left unchanged.
type knowledgeNoteRequest struct {
	Kind                  string                   `json:"kind,omitempty"`
	Title                 string                   `json:"title,omitempty"`
	BodyMarkdown          string                   `json:"body_markdown,omitempty"`
	Tags                  *[]string                `json:"tags,omitempty"`
	Selectors             *[]config.RouteCondition `json:"selectors,omitempty"`
	SourceInvestigationID string                   `json:"source_investigation_id,omitempty"`
	Confidence            *float64                 `json:"confidence,omitempty"`
	ExpiresAt             *time.Time               `json:"expires_at,omitempty"`
}

// handleKnowledge handles /api/v1/knowledge (list + create). Reads are
// available to any authenticated user; writes require admin role.
func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.knowledgeStore, "knowledge store") {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listKnowledge(w, r)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.KnowledgeWrite) {
			return
		}
		s.createKnowledge(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

// handleKnowledgeByID handles /api/v1/knowledge/{id} (get + update + delete).
func (s *Server) handleKnowledgeByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.knowledgeStore, "knowledge store") {
		return
	}
	id := pathID(r, "/api/v1/knowledge/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		note, err := s.knowledgeStore.Get(r.Context(), id)
		if err != nil {
			writeInternalError(w, err, "failed to get knowledge note")
			return
		}
		if note == nil {
			writeError(w, ErrorCodeNotFound, "knowledge note not found")
			return
		}
		writeData(w, http.StatusOK, note)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.KnowledgeWrite) {
			return
		}
		var req knowledgeNoteRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		patch := knowledgeNoteFromRequest(req)
		out, err := s.knowledgeStore.Update(r.Context(), id, patch)
		if err != nil {
			writeInternalError(w, err, "failed to update knowledge note")
			return
		}
		logger.InfoCtx(r.Context(), "knowledge note updated", "component", "api", "knowledge_id", id, "kind", out.Kind)
		s.audit(r, store.AuditKnowledgeUpdated, map[string]any{
			"knowledge_id": id,
			"kind":         out.Kind,
		})
		writeData(w, http.StatusOK, out)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.KnowledgeDelete) {
			return
		}
		if err := s.knowledgeStore.Delete(r.Context(), id); err != nil {
			writeInternalError(w, err, "failed to delete knowledge note")
			return
		}
		logger.InfoCtx(r.Context(), "knowledge note deleted", "component", "api", "knowledge_id", id)
		s.audit(r, store.AuditKnowledgeDeleted, map[string]any{
			"knowledge_id": id,
		})
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) listKnowledge(w http.ResponseWriter, r *http.Request) {
	q := store.KnowledgeQuery{
		Kind:       r.URL.Query().Get("kind"),
		Tag:        r.URL.Query().Get("tag"),
		Text:       r.URL.Query().Get("q"),
		AuthorType: r.URL.Query().Get("author_type"),
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.knowledgeStore.List(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list knowledge notes")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) createKnowledge(w http.ResponseWriter, r *http.Request) {
	var req knowledgeNoteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	note := knowledgeNoteFromRequest(req)
	note.AuthorType = store.KnowledgeAuthorUser
	if user := userFromContext(r.Context()); user != nil {
		uid := user.ID
		note.AuthorID = &uid
		note.AuthorName = user.Email
	}
	out, err := s.knowledgeStore.Create(r.Context(), note)
	if err != nil {
		writeInternalError(w, err, "failed to create knowledge note")
		return
	}
	logger.InfoCtx(r.Context(), "knowledge note created", "component", "api", "knowledge_id", out.ID.String(), "kind", out.Kind)
	s.audit(r, store.AuditKnowledgeCreated, map[string]any{
		"knowledge_id": out.ID.String(),
		"kind":         out.Kind,
	})
	writeData(w, http.StatusCreated, out)
}

// handleAgentKnowledge handles /api/v1/agent/knowledge. GET is read-only
// listing (same shape as admin list). POST lets an authenticated agent
// write back a new note (used in Phase 4 / agent-authored KB); the agent
// must supply a source_investigation_id and confidence.
//
// Capability gates per WP-B7: reads need `investigate` OR `command`
// (commanders legitimately consult runbooks); authoring needs `investigate`
// — KB notes are ingested into other agents' prompts, so a minimal
// communicate-only token must not be able to plant content.
func (s *Server) handleAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.knowledgeStore, "knowledge store") {
		return
	}
	agent := platform.AgentFromContext(r.Context())
	if agent == nil {
		writeError(w, ErrorCodeUnauthorized, "missing agent context")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !capability.HasAny(agent.Capabilities, capability.Investigate, capability.Command) {
			writeErrorStatus(w, http.StatusForbidden, ErrorCodeForbidden,
				fmt.Sprintf("agent %q lacks required capability (one of: %s, %s)",
					agent.Name, capability.Investigate, capability.Command))
			return
		}
		s.listKnowledge(w, r)
	case http.MethodPost:
		if !capability.Has(agent.Capabilities, capability.Investigate) {
			writeErrorStatus(w, http.StatusForbidden, ErrorCodeForbidden,
				fmt.Sprintf("agent %q lacks required capability %q", agent.Name, capability.Investigate))
			return
		}
		s.createAgentKnowledge(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

// handleAgentKnowledgeByID handles /api/v1/agent/knowledge/{id} (GET only).
// The list/search endpoint returns truncated previews; this route gives
// agents the full note body so they can read complete runbooks without a
// human paste. Read-only and bearer-authenticated via the route middleware;
// requires `investigate` OR `command` (WP-B7).
func (s *Server) handleAgentKnowledgeByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.knowledgeStore, "knowledge store") {
		return
	}
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	agent := platform.AgentFromContext(r.Context())
	if agent == nil {
		writeError(w, ErrorCodeUnauthorized, "missing agent context")
		return
	}
	if !capability.HasAny(agent.Capabilities, capability.Investigate, capability.Command) {
		writeErrorStatus(w, http.StatusForbidden, ErrorCodeForbidden,
			fmt.Sprintf("agent %q lacks required capability (one of: %s, %s)",
				agent.Name, capability.Investigate, capability.Command))
		return
	}
	id := pathID(r, "/api/v1/agent/knowledge/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing id")
		return
	}
	note, err := s.knowledgeStore.Get(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "failed to get knowledge note")
		return
	}
	if note == nil {
		writeError(w, ErrorCodeNotFound, "knowledge note not found")
		return
	}
	writeData(w, http.StatusOK, note)
}

func (s *Server) createAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	agent := platform.AgentFromContext(r.Context())
	if agent == nil {
		writeError(w, ErrorCodeUnauthorized, "missing agent context")
		return
	}
	var req knowledgeNoteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SourceInvestigationID) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "source_investigation_id is required for agent-authored notes")
		return
	}
	if req.Confidence == nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "confidence is required for agent-authored notes")
		return
	}
	note := knowledgeNoteFromRequest(req)
	note.AuthorType = store.KnowledgeAuthorAgent
	note.AuthorName = agent.Name
	out, err := s.knowledgeStore.Create(r.Context(), note)
	if err != nil {
		writeInternalError(w, err, "failed to create knowledge note")
		return
	}
	s.auditAgent(r, store.AuditKnowledgeCreated, agent.Name, map[string]any{
		"knowledge_id":            out.ID.String(),
		"kind":                    out.Kind,
		"source_investigation_id": out.SourceInvestigationID,
		"confidence":              out.Confidence,
	})
	writeData(w, http.StatusCreated, out)
}

func knowledgeNoteFromRequest(req knowledgeNoteRequest) *store.KnowledgeNote {
	note := &store.KnowledgeNote{
		Kind:                  strings.TrimSpace(req.Kind),
		Title:                 strings.TrimSpace(req.Title),
		BodyMarkdown:          req.BodyMarkdown,
		SourceInvestigationID: strings.TrimSpace(req.SourceInvestigationID),
		Confidence:            req.Confidence,
		ExpiresAt:             req.ExpiresAt,
	}
	if req.Tags != nil {
		note.Tags = *req.Tags
	}
	if req.Selectors != nil {
		note.Selectors = *req.Selectors
	}
	return note
}
