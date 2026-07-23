package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/memory"
	"alga/rbac"
	"alga/store"
)

type memoryRequest struct {
	Content         string            `json:"content"`
	MemoryType      string            `json:"memory_type,omitempty"`
	InvestigationID string            `json:"investigation_id,omitempty"`
	CorrelationKey  string            `json:"correlation_key,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Confidence      *float64          `json:"confidence,omitempty"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
}

func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.memorySvc, "memory service") {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listMemories(w, r)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.MemoriesWrite) {
			return
		}
		s.createMemory(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleMemoryByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.memorySvc, "memory service") {
		return
	}
	idStr := pathID(r, "/api/v1/memories/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid memory id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, err := s.memorySvc.Get(r.Context(), id)
		if err != nil {
			writeInternalError(w, err, "failed to get memory")
			return
		}
		if rec == nil {
			writeError(w, ErrorCodeNotFound, "memory not found")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.MemoriesWrite) {
			return
		}
		var req memoryRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.Content) == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "content is required")
			return
		}
		out, err := s.memorySvc.Update(r.Context(), id, req.Content)
		if err != nil {
			writeInternalError(w, err, "failed to update memory")
			return
		}
		writeData(w, http.StatusOK, out)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.MemoriesDelete) {
			return
		}
		if err := s.memorySvc.Delete(r.Context(), id); err != nil {
			writeInternalError(w, err, "failed to delete memory")
			return
		}
		logger.InfoCtx(r.Context(), "memory deleted", "component", "api", "memory_id", id.String())
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	f := store.MemoryFilters{
		Query: r.URL.Query().Get("q"),
	}
	if v := r.URL.Query().Get("memory_type"); v != "" {
		mt := strings.ToLower(strings.TrimSpace(v))
		f.MemoryType = &mt
	}
	if v := r.URL.Query().Get("agent_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.AgentID = &id
		}
	}
	if v := r.URL.Query().Get("investigation_id"); v != "" {
		f.InvestigationID = &v
	}
	limit, skip := parseLimitSkip(r, 50)
	f.Limit = int(limit)
	f.Offset = int(skip)

	items, total, err := s.memorySvc.List(r.Context(), f)
	if err != nil {
		writeInternalError(w, err, "failed to list memories")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), int64(total))
}

func (s *Server) createMemory(w http.ResponseWriter, r *http.Request) {
	var req memoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "content is required")
		return
	}

	input := memory.CreateMemoryInput{
		Content:         req.Content,
		MemoryType:      req.MemoryType,
		InvestigationID: req.InvestigationID,
		CorrelationKey:  req.CorrelationKey,
		Labels:          req.Labels,
		Confidence:      req.Confidence,
		ExpiresAt:       req.ExpiresAt,
	}

	out, err := s.memorySvc.CreateMemory(r.Context(), input)
	if err != nil {
		writeInternalError(w, err, "failed to create memory")
		return
	}
	logger.InfoCtx(r.Context(), "memory created", "component", "api", "memory_id", out.ID.String())
	writeData(w, http.StatusCreated, out)
}

// Agent-scoped memory handlers (handleAgentMemories, handleAgentMemoryByID,
// createAgentMemory) live in package agent (see api/agent/memory.go). They
// reuse listMemories/createMemory/memoryRequest above via the
// agent.Service.memorySvc field and the agent.MemoryStore interface.
