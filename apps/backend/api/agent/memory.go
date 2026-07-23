package agent

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/memory"
	"alga/store"
)

// memoryRequest mirrors the admin memoryRequest in package api so the agent
// memory endpoints accept the same JSON shape.
type memoryRequest struct {
	Content         string            `json:"content"`
	MemoryType      string            `json:"memory_type,omitempty"`
	InvestigationID string            `json:"investigation_id,omitempty"`
	CorrelationKey  string            `json:"correlation_key,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Confidence      *float64          `json:"confidence,omitempty"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
}

func (s *Service) handleAgentMemories(w http.ResponseWriter, r *http.Request) {
	if s.memorySvc == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "memory service not configured")
		return
	}
	if agentFromContext(r.Context()) == nil {
		platform.WriteError(w, platform.ErrorCodeUnauthorized, "missing agent context")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listMemories(w, r)
	case http.MethodPost:
		s.createAgentMemory(w, r)
	default:
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
	}
}

func (s *Service) handleAgentMemoryByID(w http.ResponseWriter, r *http.Request) {
	if s.memorySvc == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "memory service not configured")
		return
	}
	agent := agentFromContext(r.Context())
	if agent == nil {
		platform.WriteError(w, platform.ErrorCodeUnauthorized, "missing agent context")
		return
	}
	idStr := platform.PathID(r, "/api/v1/agent/memories/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid memory id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, err := s.memorySvc.Get(r.Context(), id)
		if err != nil {
			platform.WriteInternalError(w, err, "failed to get memory")
			return
		}
		if rec == nil {
			platform.WriteError(w, platform.ErrorCodeNotFound, "memory not found")
			return
		}
		platform.WriteData(w, http.StatusOK, rec)
	case http.MethodDelete:
		rec, err := s.memorySvc.Get(r.Context(), id)
		if err != nil || rec == nil {
			platform.WriteError(w, platform.ErrorCodeNotFound, "memory not found")
			return
		}
		if rec.AgentID != nil && *rec.AgentID != agent.ID {
			platform.WriteError(w, platform.ErrorCodeForbidden, "can only delete own memories")
			return
		}
		if err := s.memorySvc.Delete(r.Context(), id); err != nil {
			platform.WriteInternalError(w, err, "failed to delete memory")
			return
		}
		platform.WriteStatus(w, "deleted")
	default:
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
	}
}

func (s *Service) listMemories(w http.ResponseWriter, r *http.Request) {
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
	limit, skip := platform.ParseLimitSkip(r, 50)
	f.Limit = int(limit)
	f.Offset = int(skip)

	items, total, err := s.memorySvc.List(r.Context(), f)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to list memories")
		return
	}
	platform.WritePaginatedJSON(w, platform.EnsureSlice(items), int64(total))
}

func (s *Service) createAgentMemory(w http.ResponseWriter, r *http.Request) {
	agent := agentFromContext(r.Context())
	if agent == nil {
		platform.WriteError(w, platform.ErrorCodeUnauthorized, "missing agent context")
		return
	}
	var req memoryRequest
	if !platform.DecodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "content is required")
		return
	}

	agentID := agent.ID
	input := memory.CreateMemoryInput{
		Content:         req.Content,
		MemoryType:      req.MemoryType,
		AgentID:         &agentID,
		AgentName:       agent.Name,
		AgentType:       agent.AgentType,
		InvestigationID: req.InvestigationID,
		CorrelationKey:  req.CorrelationKey,
		Labels:          req.Labels,
		Confidence:      req.Confidence,
		ExpiresAt:       req.ExpiresAt,
	}

	out, err := s.memorySvc.CreateMemory(r.Context(), input)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to create memory")
		return
	}
	platform.WriteJSON(w, http.StatusCreated, out)
}
