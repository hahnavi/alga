package agent

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/logger"
	"alga/store"
)

// peerAskCreateRequest is the JSON body for POST /api/v1/agent/peer-ask.
// At least one of to_agent_id / to_agent_type must be set.
type peerAskCreateRequest struct {
	ToAgentID       string `json:"to_agent_id,omitempty"`
	ToAgentType     string `json:"to_agent_type,omitempty"`
	InvestigationID string `json:"investigation_id,omitempty"`
	Question        string `json:"question"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
}

type peerAskReplyRequest struct {
	Reply string `json:"reply"`
}

// peerAskMinQuestion and peerAskMaxQuestion bound question length to avoid
// pathological payloads. Kept permissive on purpose.
const (
	peerAskMinQuestion = 3
	peerAskMaxQuestion = 4000
	peerAskMaxReply    = 8000
)

func (s *Service) handleAgentPeerAsk(w http.ResponseWriter, r *http.Request) {
	if s.agentAskStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "peer-ask not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listAgentPeerAsks(w, r)
	case http.MethodPost:
		s.createAgentPeerAsk(w, r)
	default:
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
	}
}

// handleAgentPeerAskByID handles /api/v1/agent/peer-ask/{id} and
// /api/v1/agent/peer-ask/{id}/reply|cancel.
func (s *Service) handleAgentPeerAskByID(w http.ResponseWriter, r *http.Request) {
	if s.agentAskStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "peer-ask not configured")
		return
	}
	path := platform.PathID(r, "/api/v1/agent/peer-ask/")
	path = strings.Trim(path, "/")
	if path == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "missing ask id")
		return
	}
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch action {
	case "":
		if r.Method != http.MethodGet {
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
			return
		}
		ask, err := s.agentAskStore.Get(r.Context(), id)
		if err != nil {
			platform.WriteInternalError(w, err, "failed to get peer ask")
			return
		}
		if ask == nil {
			platform.WriteError(w, platform.ErrorCodeNotFound, "peer ask not found")
			return
		}
		platform.WriteData(w, http.StatusOK, ask)
	case "reply":
		if r.Method != http.MethodPost {
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
			return
		}
		s.replyAgentPeerAsk(w, r, id)
	case "cancel":
		if r.Method != http.MethodPost {
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
			return
		}
		s.cancelAgentPeerAsk(w, r, id)
	default:
		platform.WriteError(w, platform.ErrorCodeNotFound, "not found")
	}
}

func (s *Service) listAgentPeerAsks(w http.ResponseWriter, r *http.Request) {
	agent, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	role := r.URL.Query().Get("role") // "inbox" (default) or "sent"
	q := store.AgentAskQuery{
		Status: r.URL.Query().Get("status"),
	}
	agentID := agent.ID
	switch role {
	case "sent":
		q.FromAgentID = &agentID
	default:
		q.ForAgentID = &agentID
		q.ForAgentType = agent.AgentType
	}
	limit, skip := platform.ParseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.agentAskStore.List(r.Context(), q)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to list peer asks")
		return
	}
	platform.WritePaginatedJSON(w, platform.EnsureSlice(items), total)
}

func (s *Service) createAgentPeerAsk(w http.ResponseWriter, r *http.Request) {
	agent, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	var req peerAskCreateRequest
	if !platform.DecodeJSON(w, r, &req) {
		return
	}
	question := strings.TrimSpace(req.Question)
	if len(question) < peerAskMinQuestion {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "question is too short")
		return
	}
	if len(question) > peerAskMaxQuestion {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "question is too long")
		return
	}
	toType := strings.TrimSpace(strings.ToLower(req.ToAgentType))
	toID := strings.TrimSpace(req.ToAgentID)
	if toID == "" && toType == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "either to_agent_id or to_agent_type is required")
		return
	}

	// Basic quota guard: cap pending asks per agent to 5 at once. This also
	// acts as a rate limit for noisy agents; the caller can cancel old asks
	// to free slots.
	if count := s.countPendingAsksForAgent(r, agent.ID); count >= 5 {
		platform.WriteError(w, platform.ErrorCodeRateLimited, "too many pending peer asks; cancel older asks first")
		return
	}

	rec := &store.AgentAskRecord{
		FromAgentID:     agent.ID,
		FromAgentName:   agent.Name,
		FromAgentType:   agent.AgentType,
		InvestigationID: strings.TrimSpace(req.InvestigationID),
		ToAgentType:     toType,
		Question:        question,
		Status:          store.AgentAskPending,
	}
	if toID != "" {
		oid, err := uuid.Parse(toID)
		if err != nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid to_agent_id")
			return
		}
		rec.ToAgentID = &oid
	}
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	if timeout > 30*time.Minute {
		timeout = 30 * time.Minute
	}
	rec.ExpiresAt = time.Now().UTC().Add(timeout)

	out, err := s.agentAskStore.Create(r.Context(), rec)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to create peer ask")
		return
	}

	s.pushPeerAskFrame(out)

	logger.InfoCtx(r.Context(), "peer ask created", "component", "api", "ask_id", out.ID.String(), "from_agent", agent.Name, "to_agent_id", toID, "to_agent_type", toType)
	s.auditAgent(r, store.AuditPeerAskCreated, agent.Name, map[string]any{
		"ask_id":           out.ID.String(),
		"investigation_id": out.InvestigationID,
		"to_agent_id":      toID,
		"to_agent_type":    toType,
	})
	platform.WriteJSON(w, http.StatusCreated, out)
}

func (s *Service) replyAgentPeerAsk(w http.ResponseWriter, r *http.Request, askID string) {
	agent, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	ask, err := s.agentAskStore.Get(r.Context(), askID)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to get peer ask")
		return
	}
	if ask == nil {
		platform.WriteError(w, platform.ErrorCodeNotFound, "peer ask not found")
		return
	}
	// Authorisation: only the target agent (or any agent of the target type
	// when no specific target was set) may reply.
	if ask.ToAgentID != nil {
		if ask.ToAgentID.String() != agent.ID.String() {
			platform.WriteError(w, platform.ErrorCodeForbidden, "not addressed to this agent")
			return
		}
	} else if ask.ToAgentType != "" && !strings.EqualFold(ask.ToAgentType, agent.AgentType) {
		platform.WriteError(w, platform.ErrorCodeForbidden, "not addressed to this agent type")
		return
	}

	var req peerAskReplyRequest
	if !platform.DecodeJSON(w, r, &req) {
		return
	}
	reply := strings.TrimSpace(req.Reply)
	if reply == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "reply is required")
		return
	}
	if len(reply) > peerAskMaxReply {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "reply is too long")
		return
	}

	out, err := s.agentAskStore.Reply(r.Context(), askID, reply, agent.ID, agent.Name)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to reply to peer ask")
		return
	}

	s.pushPeerReplyFrame(out)

	logger.InfoCtx(r.Context(), "peer ask replied", "component", "api", "ask_id", askID, "from_agent", agent.Name)
	s.auditAgent(r, store.AuditPeerAskReplied, agent.Name, map[string]any{
		"ask_id":        askID,
		"from_agent_id": out.FromAgentID.String(),
	})
	platform.WriteData(w, http.StatusOK, out)
}

func (s *Service) cancelAgentPeerAsk(w http.ResponseWriter, r *http.Request, askID string) {
	agent, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	if err := s.agentAskStore.Cancel(r.Context(), askID, agent.ID); err != nil {
		platform.WriteInternalError(w, err, "failed to cancel peer ask")
		return
	}
	logger.InfoCtx(r.Context(), "peer ask cancelled", "component", "api", "ask_id", askID, "agent", agent.Name)
	s.auditAgent(r, store.AuditPeerAskCancelled, agent.Name, map[string]any{
		"ask_id": askID,
	})
	platform.WriteStatus(w, "cancelled")
}

// countPendingAsksForAgent returns the number of currently pending asks this
// agent has authored. Used as a simple per-agent quota guard.
func (s *Service) countPendingAsksForAgent(r *http.Request, agentID uuid.UUID) int {
	_, total, err := s.agentAskStore.List(r.Context(), store.AgentAskQuery{
		FromAgentID: &agentID,
		Status:      store.AgentAskPending,
		Limit:       1,
	})
	if err != nil {
		return 0
	}
	return int(total)
}

// pushPeerAskFrame delivers a peer_ask SSE frame to the target agent if they
// are currently online (either Hermes or OpenClaw). Falls back to a
// type-level broadcast when only to_agent_type is set. Pure best effort.
func (s *Service) pushPeerAskFrame(ask *store.AgentAskRecord) {
	if ask == nil {
		return
	}
	frame := PeerAskFrame{
		Type:            "peer_ask",
		AskID:           ask.ID.String(),
		FromAgentID:     ask.FromAgentID.String(),
		FromAgentName:   ask.FromAgentName,
		FromAgentType:   ask.FromAgentType,
		InvestigationID: ask.InvestigationID,
		Question:        ask.Question,
		ExpiresAt:       ask.ExpiresAt,
		CreatedAt:       ask.CreatedAt,
	}
	if ask.ToAgentID != nil {
		key := ask.ToAgentID.String()
		PublishPeerAskToAgent(s.sse, key, frame)
		return
	}
	if ask.ToAgentType != "" {
		BroadcastPeerAskToType(s.sse, frame)
	}
}

func (s *Service) pushPeerReplyFrame(ask *store.AgentAskRecord) {
	if ask == nil {
		return
	}
	frame := PeerReplyFrame{
		Type:            "peer_reply",
		AskID:           ask.ID.String(),
		InvestigationID: ask.InvestigationID,
		Reply:           ask.Reply,
	}
	if ask.RepliedByAgentID != nil {
		frame.RepliedByAgentID = ask.RepliedByAgentID.String()
	}
	frame.RepliedByAgentName = ask.RepliedByAgentName
	if ask.AnsweredAt != nil {
		frame.AnsweredAt = *ask.AnsweredAt
	}
	key := ask.FromAgentID.String()
	PublishPeerReplyToAgent(s.sse, key, frame)
}
