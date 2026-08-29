package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/capability"
	"alga/config"
	"alga/ics"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

// agentTokenContext is the canonical agent identity type propagated through
// request context. Aliased to platform.AgentTokenContext so existing handler
// signatures read unchanged.
type agentTokenContext = platform.AgentTokenContext

// agentFromContext retrieves the agent identity from the request context.
func agentFromContext(ctx context.Context) *agentTokenContext {
	return platform.AgentFromContext(ctx)
}

// agentActorFromRequest builds an EventActor for audit/timeline entries from
// the agent identity stored in the request context.
func agentActorFromRequest(r *http.Request) *store.EventActor {
	a := agentFromContext(r.Context())
	if a == nil {
		return nil
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = "agent"
	}
	return &store.EventActor{Username: name, Source: "agent"}
}

// agentAlertActor wraps AlertActionActor around the agent identity in the
// request context. Returns nil when no agent is set.
func agentAlertActor(r *http.Request) *AlertActionActor {
	actor := agentActorFromRequest(r)
	if actor == nil {
		return nil
	}
	return &AlertActionActor{
		Actor:     actor,
		IsAgent:   true,
		AgentName: actor.Username,
	}
}

// agentOnline reports whether an agent token (by hex id) currently has an SSE
// subscriber or is reachable via presence.
func (s *Service) agentOnline(idHex string) bool {
	if s.sse == nil {
		return false
	}
	return s.sse.AgentOnline(idHex)
}

// validateTokenName is the agent-package copy of api.validateTokenName so this
// package does not depend on package api for the /api/v1/agent-tokens handler.
func validateTokenName(w http.ResponseWriter, name string) bool {
	if name == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "name is required")
		return false
	}
	if len(name) > maxTokenNameLength {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, fmt.Sprintf("name must be at most %d characters", maxTokenNameLength))
		return false
	}
	return true
}

// parseAndValidateExpiry is the agent-package copy of api.parseAndValidateExpiry.
func parseAndValidateExpiry(w http.ResponseWriter, raw string) (*time.Time, bool) {
	expPtr, err := parseOptionalExpiry(raw)
	if err != nil {
		if err == errExpiryInPast {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "expires_at must be in the future")
		} else {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid expires_at (use RFC3339)")
		}
		return nil, false
	}
	return expPtr, true
}

type expiryError struct{}

func (e *expiryError) Error() string { return "expires_at must be in the future" }

var errExpiryInPast = &expiryError{}

func parseOptionalExpiry(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	if !t.After(timeNow()) {
		return nil, errExpiryInPast
	}
	return &t, nil
}

// serializeAgentToken mirrors api.serializeAgentToken so the agent-tokens
// endpoint does not need package api. Returns the same JSON shape, but only
// includes the plaintext Token when includeToken is true (create/regenerate).
// List responses must pass includeToken=false so persisted hashes never leak.
func serializeAgentToken(t store.AgentTokenRecord, idHex string, online bool, scope string, labelSelectors any, capabilities []string) map[string]any {
	return serializeAgentTokenOpts(t, idHex, online, scope, labelSelectors, capabilities, true)
}

// serializeAgentTokenSummary is the list-response shape: it omits the Token
// field so a GET /agent-tokens never exposes persisted token material.
func serializeAgentTokenSummary(t store.AgentTokenRecord, idHex string, online bool, scope string, labelSelectors any, capabilities []string) map[string]any {
	return serializeAgentTokenOpts(t, idHex, online, scope, labelSelectors, capabilities, false)
}

func serializeAgentTokenOpts(t store.AgentTokenRecord, idHex string, online bool, scope string, labelSelectors any, capabilities []string, includeToken bool) map[string]any {
	row := map[string]any{
		"id":         idHex,
		"name":       t.Name,
		"agent_type": t.AgentType,
		"created_at": t.CreatedAt,
		"last_used":  t.LastUsedAt,
		"revoked":    t.Revoked,
		"enabled":    t.Enabled,
		"online":     online,
		"scope":      scope,
	}
	if includeToken {
		// Newly created / regenerated tokens are shown once; persisted records
		// only carry a hash, so list responses omit the field entirely.
		row["token"] = t.Token
	}
	if labelSelectors != nil {
		row["label_selectors"] = labelSelectors
	}
	if len(capabilities) > 0 {
		row["capabilities"] = capabilities
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.IsZero() {
		row["expires_at"] = t.ExpiresAt.UTC().Format(time.RFC3339)
		row["expired"] = timeNow().After(*t.ExpiresAt)
	}
	return row
}

func (s *Service) handleAgentAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	if !capability.HasAny(act.Capabilities, capability.Investigate, capability.Command) {
		platform.WriteError(w, platform.ErrorCodeForbidden, fmt.Sprintf("agent %q lacks required capability (one of: %s, %s)", act.Name, capability.Investigate, capability.Command))
		return
	}
	if s.writeAlertsQueryResponse == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "alert store not configured")
		return
	}
	s.writeAlertsQueryResponse(w, r)
}

func (s *Service) handleAgentTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tokens, err := s.agentTokenStore.ListTokens()
		if err != nil {
			platform.WriteInternalError(w, err, "failed to list agent tokens")
			return
		}
		out := make([]map[string]any, 0, len(tokens))
		for _, t := range tokens {
			idHex := t.ID.String()
			out = append(out, serializeAgentTokenSummary(t, idHex, s.agentOnline(idHex), t.Scope, t.LabelSelectors, t.Capabilities))
		}
		platform.WriteData(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name           string                  `json:"name"`
			ExpiresAt      string                  `json:"expires_at"`
			AgentType      string                  `json:"agent_type"`
			Scope          string                  `json:"scope"`
			Capabilities   []string                `json:"capabilities"`
			LabelSelectors []config.RouteCondition `json:"label_selectors"`
		}
		if !platform.DecodeJSON(w, r, &req) {
			return
		}
		if !validateTokenName(w, req.Name) {
			return
		}
		expPtr, ok := parseAndValidateExpiry(w, req.ExpiresAt)
		if !ok {
			return
		}
		caps := capability.Normalize(req.Capabilities)
		scope := req.Scope
		if scope == "" {
			scope = "all"
		}
		if scope != "all" && scope != "labels" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "scope must be \"all\" or \"labels\"")
			return
		}
		record, err := s.agentTokenStore.CreateToken(req.Name, expPtr, req.AgentType, caps)
		if err != nil {
			if errors.Is(err, store.ErrInvalidAgentType) {
				platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, err.Error())
				return
			}
			platform.WriteInternalError(w, err, "failed to create agent token")
			return
		}
		s.audit(r, store.AuditTokenCreated, map[string]any{
			"token_id":   record.ID.String(),
			"token_name": record.Name,
			"token_kind": "agent",
		})
		if scope == "labels" || len(req.LabelSelectors) > 0 {
			if err := s.agentTokenStore.UpdateAgentConfig(record.ID, scope, req.LabelSelectors, caps); err != nil {
				logger.Warn("Failed to set agent config on creation", "error", err)
			}
			record.Scope = scope
			record.LabelSelectors = req.LabelSelectors
		}
		idHex := record.ID.String()
		platform.WriteJSON(w, http.StatusCreated, serializeAgentToken(*record, idHex, s.agentOnline(idHex), scope, req.LabelSelectors, caps))
	default:
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
	}
}

func (s *Service) handleAgentTokenByID(w http.ResponseWriter, r *http.Request) {
	suffix := platform.PathID(r, "/api/v1/agent-tokens/")
	if suffix == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "missing token id")
		return
	}
	if strings.Contains(suffix, "/chat/") {
		s.handleAgentTokenChat(w, r, suffix)
		return
	}
	const regenerateTail = "/regenerate"
	if strings.HasSuffix(suffix, regenerateTail) {
		if r.Method != http.MethodPost {
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
			return
		}
		idHex := strings.TrimSuffix(suffix, regenerateTail)
		if idHex == "" || strings.Contains(idHex, "/") {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
			return
		}
		id, err := uuid.Parse(idHex)
		if err != nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
			return
		}
		record, err := s.agentTokenStore.RegenerateToken(id)
		if err != nil {
			if errors.Is(err, store.ErrAgentNotFoundInactive) {
				platform.WriteError(w, platform.ErrorCodeNotFound, err.Error())
				return
			}
			platform.WriteInternalError(w, err, "failed to regenerate agent token")
			return
		}
		s.audit(r, store.AuditTokenRegenerated, map[string]any{
			"token_id":   idHex,
			"token_name": record.Name,
			"token_kind": "agent",
		})
		recIDHex := record.ID.String()
		platform.WriteData(w, http.StatusOK, serializeAgentToken(*record, recIDHex, s.agentOnline(recIDHex), "", nil, nil))
		return
	}
	if r.Method == http.MethodPut {
		if strings.Contains(suffix, "/") {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
			return
		}
		id, err := uuid.Parse(suffix)
		if err != nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
			return
		}
		var req struct {
			Scope          string                  `json:"scope"`
			LabelSelectors []config.RouteCondition `json:"label_selectors"`
			Capabilities   []string                `json:"capabilities"`
			Enabled        *bool                   `json:"enabled"`
		}
		if !platform.DecodeJSON(w, r, &req) {
			return
		}
		scope := req.Scope
		if scope == "" {
			scope = "all"
		}
		if scope != "all" && scope != "labels" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "scope must be \"all\" or \"labels\"")
			return
		}
		selectors := platform.EnsureSlice(req.LabelSelectors)
		caps := capability.Normalize(req.Capabilities)
		if err := s.agentTokenStore.UpdateAgentConfig(id, scope, selectors, caps); err != nil {
			if errors.Is(err, store.ErrAgentNotFoundInactive) {
				platform.WriteErrorStatus(w, http.StatusNotFound, platform.ErrorCodeNotFound, "agent not found")
				return
			}
			platform.WriteInternalError(w, err, "failed to update agent config")
			return
		}
		if req.Enabled != nil {
			if err := s.agentTokenStore.SetAgentEnabled(id, *req.Enabled); err != nil {
				if errors.Is(err, store.ErrAgentNotFoundInactive) {
					platform.WriteErrorStatus(w, http.StatusNotFound, platform.ErrorCodeNotFound, "agent not found")
					return
				}
				platform.WriteInternalError(w, err, "failed to set agent enabled")
				return
			}
			evt := store.AuditAgentDisabled
			if *req.Enabled {
				evt = store.AuditAgentEnabled
			}
			s.audit(r, evt, map[string]any{
				"token_id":   id.String(),
				"token_kind": "agent",
			})
		}
		platform.WriteJSON(w, http.StatusOK, map[string]any{
			"status":          "ok",
			"scope":           scope,
			"label_selectors": selectors,
			"capabilities":    caps,
		})
		return
	}
	if r.Method != http.MethodDelete {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	if strings.Contains(suffix, "/") {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
		return
	}
	if _, err := uuid.Parse(suffix); err != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
		return
	}
	if s.revokeTokenByID == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "token revocation not configured")
		return
	}
	s.revokeTokenByID(w, r, suffix, s.agentTokenStore.RevokeToken, "agent")
}

// authorizeAgentAlertMutation gates alert resolve/reopen. Investigate-capable
// agents may mutate any alert; if linked to an active incident, only the
// assigned incident commander is authorized.
// Read access (list/get) is intentionally not scoped here.
func (s *Service) authorizeAgentAlertMutation(ctx context.Context, agent agentTokenContext, fingerprint string) error {
	if s.alertStore == nil {
		return errors.New("unable to authorize alert mutation")
	}
	record, err := s.alertStore.GetByFingerprint(fingerprint)
	if err != nil || record == nil {
		return fmt.Errorf("alert %q not found", fingerprint)
	}

	// If the alert is linked to an active incident, only the assigned incident commander of that incident is authorized.
	// Otherwise, any investigate-capable agent is allowed.
	if s.exec != nil && s.exec.alertInvestigationStore != nil {
		inv, _ := s.exec.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, record.AlertNumber)
		if inv != nil && inv.PromotedIncidentID != nil {
			return s.exec.CommanderOwnsAlertIncident(ctx, agent.ID, record.AlertNumber)
		}
	}

	if capability.Has(agent.Capabilities, capability.Investigate) {
		return nil
	}
	return errors.New("commander agents are not authorized to resolve or reopen alerts")
}

func (s *Service) handleAgentAlertByFingerprint(w http.ResponseWriter, r *http.Request) {
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	if !capability.HasAny(act.Capabilities, capability.Investigate, capability.Command) {
		platform.WriteError(w, platform.ErrorCodeForbidden, fmt.Sprintf("agent %q lacks required capability (one of: %s, %s)", act.Name, capability.Investigate, capability.Command))
		return
	}

	suffix := platform.PathID(r, "/api/v1/agent/alerts/")
	if suffix == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "missing fingerprint")
		return
	}

	if strings.HasSuffix(suffix, "/resolve") {
		if r.Method != http.MethodPost {
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
			return
		}
		fingerprint := strings.TrimSuffix(suffix, "/resolve")
		if fingerprint == "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "missing fingerprint")
			return
		}
		if err := s.authorizeAgentAlertMutation(r.Context(), *act, fingerprint); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		if s.resolveAlert == nil {
			platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "alert store not configured")
			return
		}
		s.resolveAlert(w, r, fingerprint, agentAlertActor(r))
		return
	}

	if strings.HasSuffix(suffix, "/reopen") {
		if r.Method != http.MethodPost {
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
			return
		}
		fingerprint := strings.TrimSuffix(suffix, "/reopen")
		if fingerprint == "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "missing fingerprint")
			return
		}
		if err := s.authorizeAgentAlertMutation(r.Context(), *act, fingerprint); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		if s.reopenAlert == nil {
			platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "alert store not configured")
			return
		}
		s.reopenAlert(w, r, fingerprint, agentAlertActor(r))
		return
	}

	if r.Method != http.MethodGet {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	fingerprint := suffix
	record, err := s.alertStore.GetByFingerprint(fingerprint)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to get alert")
		return
	}
	if record == nil {
		platform.WriteError(w, platform.ErrorCodeNotFound, "not found")
		return
	}
	platform.WriteData(w, http.StatusOK, record)
}

// handleAgentTokenChat serves /api/v1/agent-tokens/{id}/chat/messages and .../chat/typing (admin).
func (s *Service) handleAgentTokenChat(w http.ResponseWriter, r *http.Request, suffix string) {
	const sep = "/chat/"
	idx := strings.Index(suffix, sep)
	if idx <= 0 {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid chat path")
		return
	}
	idHex := suffix[:idx]
	if strings.Contains(idHex, "/") {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
		return
	}
	tail := strings.TrimPrefix(suffix[idx:], sep)
	tokenOID, err := uuid.Parse(idHex)
	if err != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid token id")
		return
	}
	if s.agentDMStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "agent dm store not configured")
		return
	}
	tok, err := s.agentTokenStore.GetActiveAgentTokenByID(tokenOID)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to load agent token")
		return
	}
	if tok == nil {
		platform.WriteError(w, platform.ErrorCodeNotFound, "agent token not found")
		return
	}

	chatID := store.AlgaAgentDMChatID()

	switch tail {
	case "messages":
		switch r.Method {
		case http.MethodGet:
			beforeQ := r.URL.Query().Get("before")
			var beforePtr *uuid.UUID
			if strings.TrimSpace(beforeQ) != "" {
				oid, err := uuid.Parse(strings.TrimSpace(beforeQ))
				if err != nil {
					platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid before cursor")
					return
				}
				beforePtr = &oid
			}
			limit, _ := platform.ParseLimitSkip(r, 50)
			items, hasMore, err := s.agentDMStore.ListMessages(idHex, beforePtr, int(limit))
			if err != nil {
				platform.WriteInternalError(w, err, "failed to list messages")
				return
			}
			platform.WriteJSON(w, http.StatusOK, map[string]any{
				"items":    items,
				"has_more": hasMore,
				"chat_id":  chatID,
			})
		case http.MethodPost:
			var req struct {
				Message string `json:"message"`
			}
			if !platform.DecodeJSON(w, r, &req) {
				return
			}
			body := strings.TrimSpace(req.Message)
			if body == "" {
				platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "message is required")
				return
			}
			user := platform.UserFromContext(r.Context())
			if user == nil {
				platform.WriteError(w, platform.ErrorCodeUnauthorized, "unauthorized")
				return
			}
			uid := user.ID.String()
			senderName := user.DisplayName()
			if strings.TrimSpace(senderName) == "" {
				senderName = user.Email
			}
			uidPtr := uid
			namePtr := senderName
			msg, err := s.agentDMStore.AddMessage(idHex, store.AgentDMRoleUser, body, &uidPtr, &namePtr)
			if err != nil {
				platform.WriteInternalError(w, err, "failed to save message")
				return
			}
			s.publishAgentDMEvent("agent_dm_message", map[string]any{
				"agent_token_id": idHex,
				"chat_id":        chatID,
				"message":        msg,
			})
			if s.sse != nil {
				s.sse.PublishToAgentAllowDrop(idHex, sse.Event{
					Type: "message",
					Data: map[string]any{
						"type":        "message",
						"chat_id":     chatID,
						"message_id":  msg.ID.String(),
						"text":        body,
						"sender_id":   uid,
						"sender_name": senderName,
						"trigger":     "dispatch",
					},
				})
			}
			platform.WriteData(w, http.StatusOK, msg)
		default:
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		}
	case "typing":
		if r.Method != http.MethodPost {
			platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
			return
		}
		if s.sse != nil {
			s.sse.PublishToAgentAllowDrop(idHex, sse.Event{
				Type: "typing",
				Data: map[string]any{
					"type":    "typing",
					"chat_id": chatID,
					"active":  true,
				},
			})
		}
		platform.WriteStatus(w, "ok")
	default:
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "unknown chat resource")
	}
}

func (s *Service) handleAgentSSE(w http.ResponseWriter, r *http.Request) {
	if s.sse == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "agent SSE not available")
		return
	}
	s.sse.Handler().ServeHTTP(w, r)
}

// agentMessagePayload is the discriminated request body for
// POST /api/v1/agent/messages. The `kind` field selects between a plain text
// message (`kind="text"`, requires `text`) and an investigation tool
// (`kind="inv_tool"`, requires `command`).
type agentMessagePayload struct {
	ChatID           string   `json:"chat_id"`
	Kind             string   `json:"kind"`
	Text             string   `json:"text,omitempty"`
	Command          *InvTool `json:"command,omitempty"`
	SenderID         string   `json:"sender_id,omitempty"`
	SenderName       string   `json:"sender_name,omitempty"`
	Mentions         []string `json:"mentions,omitempty"`
	ReplyToMessageID string   `json:"reply_to_message_id,omitempty"`
}

func (s *Service) handleAgentSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	if s.exec == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "agent executor not configured")
		return
	}
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}

	var body agentMessagePayload
	if !platform.DecodeJSON(w, r, &body) {
		return
	}

	chatID := strings.TrimSpace(body.ChatID)
	kind := strings.TrimSpace(body.Kind)
	if chatID == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "chat_id is required")
		return
	}
	if kind == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "kind is required (use \"text\", \"tool_call\", \"inv_tool\", \"incident_summary\", \"triage_response\", \"command_decision\", or \"status_update\")")
		return
	}

	agentRec := &store.AgentTokenRecord{ID: act.ID, Name: act.Name, AgentType: act.AgentType, Capabilities: act.Capabilities}

	switch kind {
	case "text":
		if !store.IsAlgaAgentDMChatID(chatID) {
			ownerType, _ := parseOwnerFromChatID(chatID)
			var err error
			if store.IsIncidentThreadOwner(ownerType) {
				err = s.exec.requireAnyCapability(*act, capability.Investigate, capability.Command, capability.Communicate)
			} else {
				err = s.exec.requireCapability(*act, capability.Investigate)
			}
			if err != nil {
				platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
				return
			}
		}
		if body.Command != nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "command must be omitted when kind is \"text\"")
			return
		}
		if strings.TrimSpace(body.Text) == "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "text is required when kind is \"text\"")
			return
		}
		updateID, err := s.exec.HandleIncomingMessage(agentRec, chatID, body.Text, body.SenderID, body.SenderName, body.Mentions, body.ReplyToMessageID)
		if err != nil {
			logger.Error("Failed to handle agent message", "error", err)
			platform.WriteInternalError(w, err, "failed to handle message")
			return
		}
		resp := map[string]string{"status": "ok"}
		if updateID != "" {
			resp["message_id"] = updateID
		}
		platform.WriteData(w, http.StatusOK, resp)

	case "inv_tool":
		var err error
		op := ""
		if body.Command != nil {
			op = strings.TrimSpace(strings.ToLower(body.Command.Op))
		}
		if body.Command != nil && strings.HasPrefix(chatID, "incident_") && isIncidentToolOp(op) {
			err = s.exec.requireAnyCapability(*act, capability.Investigate, capability.Command, capability.Communicate)
		} else if body.Command != nil && (op == "resolve_alert" || op == "reopen_alert") && strings.HasPrefix(chatID, "alert_") {
			alertNumberStr := strings.TrimPrefix(chatID, "alert_")
			var isCommander bool
			if alertNum, errParse := strconv.ParseInt(alertNumberStr, 10, 64); errParse == nil {
				if s.exec.CommanderOwnsAlertIncident(r.Context(), act.ID, alertNum) == nil {
					isCommander = true
				}
			}
			if isCommander {
				err = s.exec.requireCapability(*act, capability.Command)
			} else {
				err = s.exec.requireCapability(*act, capability.Investigate)
			}
		} else if body.Command != nil && (op == "resolve_alert" || op == "reopen_alert") && strings.HasPrefix(chatID, "incident_") {
			var isCommander bool
			if parsedIncNum, ok := incidentNumberFromIncidentChatID(chatID); ok {
				roles := s.exec.activeAgentIncidentRoles(r.Context(), act.ID, parsedIncNum)
				if roles["incident_commander"] {
					isCommander = true
				}
			}
			if isCommander {
				err = s.exec.requireCapability(*act, capability.Command)
			} else {
				err = s.exec.requireCapability(*act, capability.Investigate)
			}
		} else {
			err = s.exec.requireCapability(*act, capability.Investigate)
		}
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "lacks required capability") {
				var capName string
				parts := strings.Split(msg, "lacks required capability")
				if len(parts) > 1 {
					capName = strings.Trim(parts[1], " \"`'")
				}
				if capName != "" {
					platform.WriteJSON(w, http.StatusForbidden, map[string]any{
						"error":               msg,
						"required_capability": capName,
					})
					return
				}
			}
			platform.WriteError(w, platform.ErrorCodeForbidden, msg)
			return
		}
		if body.Text != "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "text must be omitted when kind is \"inv_tool\"")
			return
		}
		if body.Command == nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "command is required when kind is \"inv_tool\"")
			return
		}
		if strings.TrimSpace(body.Command.Op) == "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "command.op is required")
			return
		}
		cmd := *body.Command
		cmd.ChatID = chatID

		out := s.exec.ExecuteInvTool(r.Context(), agentRec, cmd)

		resp := map[string]any{
			"ok":               out.Ok,
			"op":               out.Op,
			"chat_id":          chatID,
			"investigation_id": chatID, // deprecated alias of chat_id; kept for backward compatibility
		}
		if out.Error != "" {
			resp["error"] = out.Error
		}
		if out.IncidentNumber > 0 {
			resp["incident_number"] = out.IncidentNumber
		}
		switch {
		case out.Ok:
			platform.WriteData(w, http.StatusOK, resp)
		case out.Error == "investigation not found":
			platform.WriteJSON(w, http.StatusNotFound, resp)
		case out.Error == "failed to load investigation":
			platform.WriteJSON(w, http.StatusInternalServerError, resp)
		default:
			platform.WriteJSON(w, http.StatusUnprocessableEntity, resp)
		}

	case "incident_summary":
		if err := s.exec.requireCapability(*act, capability.Communicate); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		if strings.TrimSpace(body.Text) == "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "text is required when kind is \"incident_summary\"")
			return
		}
		incidentID, incidentChatOK := incidentNumberFromIncidentChatIDRaw(chatID)
		if !incidentChatOK {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "chat_id must use \"incident_coord_{id}\" or \"incident_inv_{id}\" format")
			return
		}
		if s.postIncidentSummaryFromAgent == nil {
			platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "incident channel manager not configured")
			return
		}
		if err := s.postIncidentSummaryFromAgent(r.Context(), agentRec, incidentID, strings.TrimSpace(body.Text)); err != nil {
			logger.Error("Failed to handle incident summary from agent", "error", err)
			platform.WriteInternalError(w, err, "failed to post incident summary")
			return
		}
		platform.WriteStatus(w, "ok")

	case "triage_response":
		if err := s.exec.requireCapability(*act, capability.Investigate); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		s.handleTriageResponse(w, r, *act, body)

	case "command_decision":
		if err := s.exec.requireCapability(*act, capability.Command); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		s.handleCommandDecision(w, r, *act, body)

	case "status_update":
		if err := s.exec.requireCapability(*act, capability.Communicate); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		s.handleStatusUpdate(w, r, *act, body)

	case "tool_call":
		if store.IsAlgaAgentDMChatID(chatID) {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "kind \"tool_call\" is not supported for DM chats")
			return
		}
		ownerType, _ := parseOwnerFromChatID(chatID)
		var capErr error
		if store.IsIncidentThreadOwner(ownerType) {
			capErr = s.exec.requireAnyCapability(*act, capability.Investigate, capability.Command, capability.Communicate)
		} else {
			capErr = s.exec.requireCapability(*act, capability.Investigate)
		}
		if capErr != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, capErr.Error())
			return
		}
		if body.Command != nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "command must be omitted when kind is \"tool_call\"")
			return
		}
		toolName := strings.TrimSpace(body.Text)
		if toolName == "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "text is required when kind is \"tool_call\" (the tool name)")
			return
		}
		updateID, err := s.exec.HandleToolCallMessage(agentRec, chatID, toolName)
		if err != nil {
			logger.Error("Failed to handle agent tool_call message", "error", err)
			platform.WriteInternalError(w, err, "failed to handle tool_call message")
			return
		}
		resp := map[string]string{"status": "ok"}
		if updateID != "" {
			resp["message_id"] = updateID
		}
		platform.WriteData(w, http.StatusOK, resp)

	default:
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, fmt.Sprintf("unsupported message kind: %q (use \"text\", \"tool_call\", \"inv_tool\", \"incident_summary\", \"triage_response\", \"command_decision\", or \"status_update\")", kind))
	}
}

func (s *Service) handleAgentMessageRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	if s.exec == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "agent executor not configured")
		return
	}
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}

	path := r.URL.Path
	messageID := strings.TrimPrefix(path, "/api/v1/agent/messages/")
	if messageID == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "missing message_id")
		return
	}

	agentRec := &store.AgentTokenRecord{ID: act.ID, Name: act.Name, AgentType: act.AgentType, Capabilities: act.Capabilities}

	if r.Method == http.MethodDelete {
		var body struct {
			ChatID string `json:"chat_id"`
		}
		if !platform.DecodeJSON(w, r, &body) {
			return
		}
		if body.ChatID == "" {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "chat_id is required")
			return
		}
		if err := s.exec.HandleDeleteMessage(body.ChatID, messageID, agentRec); err != nil {
			logger.Error("Failed to handle agent delete", "error", err)
			platform.WriteInternalError(w, err, "failed to delete message")
			return
		}
		platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message_id": messageID})
		return
	}

	// PUT (edit) — existing logic follows
	var body struct {
		ChatID string `json:"chat_id"`
		Kind   string `json:"kind"`
		Text   string `json:"text"`
	}
	if !platform.DecodeJSON(w, r, &body) {
		return
	}
	kind := strings.TrimSpace(body.Kind)
	if kind == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "kind is required (only \"text\" is supported)")
		return
	}
	if kind != "text" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, fmt.Sprintf("unsupported edit kind: %q (only \"text\" is supported)", kind))
		return
	}
	if body.ChatID == "" || body.Text == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "chat_id and text are required")
		return
	}

	if err := s.exec.HandleEditMessage(body.ChatID, messageID, body.Text, agentRec); err != nil {
		logger.Error("Failed to handle agent edit", "error", err)
		platform.WriteInternalError(w, err, "failed to edit message")
		return
	}

	platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message_id": messageID})
}

func (s *Service) handleAgentDraft(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	if s.exec == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "agent executor not configured")
		return
	}
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}

	var body struct {
		ChatID  string `json:"chat_id"`
		DraftID string `json:"draft_id"`
		Text    string `json:"text"`
	}
	if !platform.DecodeJSON(w, r, &body) {
		return
	}
	chatID := strings.TrimSpace(body.ChatID)
	draftID := strings.TrimSpace(body.DraftID)
	if chatID == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "chat_id is required")
		return
	}
	if draftID == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "draft_id is required")
		return
	}

	agentRec := &store.AgentTokenRecord{ID: act.ID, Name: act.Name, AgentType: act.AgentType, Capabilities: act.Capabilities}
	if !s.exec.HandleAgentDraft(agentRec, chatID, draftID, body.Text) {
		platform.WriteError(w, platform.ErrorCodeForbidden, "agent is not authorized for this chat")
		return
	}

	platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "draft_id": draftID})
}

func (s *Service) handleAgentTyping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	if s.exec == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "agent executor not configured")
		return
	}
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}

	var body struct {
		ChatID string `json:"chat_id"`
		Active *bool  `json:"active,omitempty"`
	}
	if !platform.DecodeJSON(w, r, &body) {
		return
	}
	if body.ChatID == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "chat_id is required")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	agentRec := &store.AgentTokenRecord{ID: act.ID, Name: act.Name, AgentType: act.AgentType, Capabilities: act.Capabilities}
	if !s.exec.HandleAgentTyping(agentRec, body.ChatID, active) {
		platform.WriteError(w, platform.ErrorCodeForbidden, "agent is not authorized for this chat")
		return
	}

	platform.WriteStatus(w, "ok")
}

func (s *Service) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	if _, ok := platform.RequireAgent(w, r); !ok {
		return
	}
	platform.WriteStatus(w, "ok")
}

var capabilityDescriptions = map[string]string{
	capability.Investigate: "Investigate alerts and produce root-cause analysis",
	capability.Communicate: "Send messages and updates to channels",
	capability.Command:     "Coordinate incident command decisions and escalation",
}

func (s *Service) handleAgentCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	out := make([]map[string]string, 0, len(capability.All))
	for _, id := range capability.All {
		desc := id
		if d, ok := capabilityDescriptions[id]; ok {
			desc = d
		}
		out = append(out, map[string]string{
			"id":          id,
			"name":        id,
			"description": desc,
		})
	}
	platform.WriteData(w, http.StatusOK, out)
}

func (s *Service) handleTriageResponse(w http.ResponseWriter, r *http.Request, agent agentTokenContext, payload agentMessagePayload) {
	if s.incidentStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "incident store not configured")
		return
	}
	chatID := payload.ChatID
	incidentID, incidentChatOK := incidentNumberFromIncidentChatIDRaw(chatID)
	if !incidentChatOK {
		if payload.Command != nil && payload.Command.IncidentNumber != 0 {
			incidentID = strconv.FormatInt(payload.Command.IncidentNumber, 10)
		} else {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "incident_number is required (via chat_id or command)")
			return
		}
	}
	incidentNumber, parseErr := strconv.ParseInt(incidentID, 10, 64)
	if parseErr != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid incident number")
		return
	}
	inc, err := s.incidentStore.GetIncident(r.Context(), incidentNumber)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to fetch incident")
		return
	}
	if inc == nil {
		platform.WriteErrorStatus(w, http.StatusNotFound, platform.ErrorCodeNotFound, "incident not found")
		return
	}
	if inc.Status == "detected" {
		if err := s.incidentStore.TransitionIncidentStatus(r.Context(), incidentNumber, []string{"detected"}, "triaging"); err != nil {
			logger.Warn("Failed to transition incident to triaging", "incident_number", incidentID, "error", err)
		}
		_ = s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "triage_started",
			ActorType:      "agent",
			Message:        fmt.Sprintf("Agent %s began triage", agent.Name),
		})
	}
	decision := ""
	if payload.Command != nil {
		decision = payload.Command.Op
	}
	reasoning := payload.Text
	if reasoning == "" && payload.Command != nil {
		reasoning = payload.Command.Reason
	}
	_ = s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "triage_response",
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s triage response: decision=%s reasoning=%s", agent.Name, decision, reasoning),
	})
	s.auditAgent(r, store.AuditIncidentUpdated, agent.Name, map[string]any{
		"incident_number": incidentNumber,
		"action":          "triage_response",
		"decision":        decision,
	})
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID, "action": "triage_response"})
	platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "accepted", "incident_number": incidentID})
}

func (s *Service) handleCommandDecision(w http.ResponseWriter, r *http.Request, agent agentTokenContext, payload agentMessagePayload) {
	chatID := payload.ChatID
	incidentID, incidentChatOK := incidentNumberFromIncidentChatIDRaw(chatID)
	if !incidentChatOK {
		if payload.Command != nil && payload.Command.IncidentNumber != 0 {
			incidentID = strconv.FormatInt(payload.Command.IncidentNumber, 10)
		} else {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "incident_number is required")
			return
		}
	}
	incidentNumber, parseErr := strconv.ParseInt(incidentID, 10, 64)
	if parseErr != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid incident number")
		return
	}
	decision := ""
	if payload.Command != nil {
		decision = payload.Command.Op
	}
	reasoning := payload.Text
	if reasoning == "" && payload.Command != nil {
		reasoning = payload.Command.Reason
	}
	if s.incidentStore != nil {
		_ = s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "command_decision",
			ActorType:      "agent",
			Message:        fmt.Sprintf("Agent %s command decision: %s — %s", agent.Name, decision, reasoning),
		})
	}
	s.auditAgent(r, store.AuditIncidentUpdated, agent.Name, map[string]any{
		"incident_number": incidentNumber,
		"action":          "command_decision",
		"decision":        decision,
	})
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID, "action": "command_decision", "decision": decision})
	platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "accepted", "incident_number": incidentID})
}

func (s *Service) handleStatusUpdate(w http.ResponseWriter, r *http.Request, agent agentTokenContext, payload agentMessagePayload) {
	chatID := payload.ChatID
	incidentID, incidentChatOK := incidentNumberFromIncidentChatIDRaw(chatID)
	if !incidentChatOK {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "chat_id must use incident_coord_{id} or incident_inv_{id} format")
		return
	}
	incidentNumber, parseErr := strconv.ParseInt(incidentID, 10, 64)
	if parseErr != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid incident number")
		return
	}
	if s.icsRoleStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "ICS role store not configured")
		return
	}
	commRoles, err := s.icsRoleStore.GetActiveRoles(r.Context(), incidentNumber)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to load incident roles")
		return
	}
	communicatorAllowed := false
	for _, role := range commRoles {
		if role.Status == string(ics.RoleStatusActive) && role.AssigneeType == "agent" && role.AgentTokenID != nil && *role.AgentTokenID == agent.ID &&
			(role.RoleType == string(ics.RoleCommunicationsLead) || role.RoleType == string(ics.RoleIncidentCommander) || role.RoleType == string(ics.RoleResponder)) {
			communicatorAllowed = true
			break
		}
	}
	if !communicatorAllowed && s.incidentInvestigationStore != nil {
		if inv, err := s.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(r.Context(), incidentNumber); err == nil && inv != nil && inv.AgentID == agent.ID.String() {
			communicatorAllowed = true
		}
	}
	if !communicatorAllowed {
		platform.WriteError(w, platform.ErrorCodeForbidden, "only authorized incident roles (commander, responder, communicator) or active investigators may post status updates via this endpoint")
		return
	}
	text := strings.TrimSpace(payload.Text)
	if text == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "text is required for status_update")
		return
	}
	if s.incidentStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "incident store not configured")
		return
	}
	inc, err := s.incidentStore.GetIncident(r.Context(), incidentNumber)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to fetch incident")
		return
	}
	if inc == nil {
		platform.WriteErrorStatus(w, http.StatusNotFound, platform.ErrorCodeNotFound, "incident not found")
		return
	}
	agentName := agent.Name
	if agentName == "" {
		agentName = "Agent"
	}
	if s.incidentChannelManager != nil && inc.SlackChannelID != "" {
		if postErr := s.incidentChannelManager.PostAgentSummary(r.Context(), inc, agentName, text); postErr != nil {
			logger.Warn("Failed to post agent status update to Slack", "incident_number", incidentID, "error", postErr)
		}
	}
	if s.incidentCoordinationStore != nil {
		_, err := s.incidentCoordinationStore.CreateMessage(r.Context(), &store.IncidentCoordinationMessageRecord{
			IncidentNumber:   incidentNumber,
			Kind:             store.IncidentCoordinationKindStatusUpdate,
			ActorType:        store.IncidentCoordinationActorAgent,
			ActorID:          &agent.ID,
			ActorDisplayName: agentName,
			Body:             text,
			Internal:         false,
			Source:           store.IncidentCoordinationSourceAgent,
			Metadata:         map[string]any{"status_level": statusLevelForIncidentStatus(inc.Status)},
		})
		if err != nil {
			platform.WriteInternalError(w, err, "failed to create status update")
			return
		}
	}
	_ = s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "status_update",
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s posted a status update", agentName),
	})
	if s.vkClient != nil {
		_ = s.vkClient.Do(r.Context(), s.vkClient.Builder().Set().Key("alga:summary:last:"+incidentID).Value("1").ExSeconds(int64(15*time.Minute/time.Second)).Build())
		_ = s.vkClient.Del(r.Context(), "alga:summary:pending:"+incidentID)
	}
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID, "action": "status_update"})
	platform.WriteStatus(w, "ok")
}

// publishIncidentEvent publishes an SSE event on the agent service's publisher
// (mirrors Server.publishIncidentEvent in package api). It is a no-op when no
// publisher is configured.
func (s *Service) publishIncidentEvent(eventType string, data any) {
	if s.ssePublisher == nil {
		return
	}
	s.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

// publishAgentDMEvent publishes an agent_dm SSE event (mirrors
// Server.publishAgentDMEvent in package api).
func (s *Service) publishAgentDMEvent(eventType string, data map[string]any) {
	if s.ssePublisher == nil {
		return
	}
	s.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

// auditAgent logs an audit event attributed to "agent:<name>".
func (s *Service) auditAgent(r *http.Request, event store.AuditEvent, agentName string, details map[string]any) {
	if s.auditStore == nil {
		return
	}
	s.auditStore.Log(event, nil, "agent:"+agentName, "", r.UserAgent(), true, details)
}

// audit logs an audit event for the authenticated operator (user) in context.
// Used by the operator-facing agent-token management endpoints.
func (s *Service) audit(r *http.Request, event store.AuditEvent, details map[string]any) {
	if s.auditStore == nil {
		return
	}
	if user := platform.UserFromContext(r.Context()); user != nil {
		s.auditStore.Log(event, &user.ID, user.Email, "", r.UserAgent(), true, details)
	}
}

func statusLevelForIncidentStatus(status string) string {
	switch status {
	case "detected", "triaging":
		return "investigating"
	case "active":
		return "identified"
	case "mitigated":
		return "monitoring"
	case "resolved", "closed":
		return "resolved"
	default:
		return "investigating"
	}
}

func (s *Service) handleAgentIncidentRoutes(w http.ResponseWriter, r *http.Request) {
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	if s.incidentStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "incident store not configured")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/agent/incidents/"), "/")
	incidentID := parts[0]
	if incidentID == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "incident ID is required")
		return
	}
	incidentNumber, parseErr := strconv.ParseInt(incidentID, 10, 64)
	if parseErr != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid incident number")
		return
	}
	if r.Method == http.MethodPost && len(parts) > 1 && parts[1] == "timeline" {
		if err := s.exec.requireCapability(*act, capability.Investigate); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		s.handleAgentAddIncidentTimeline(w, r, *act, incidentNumber)
		return
	}
	if r.Method == http.MethodPatch && len(parts) == 1 {
		if err := s.exec.requireCapability(*act, capability.Investigate); err != nil {
			platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
			return
		}
		s.handleAgentPatchIncident(w, r, *act, incidentNumber)
		return
	}
	if r.Method != http.MethodGet {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	inc, err := s.incidentStore.GetIncident(r.Context(), incidentNumber)
	if err != nil || inc == nil {
		platform.WriteError(w, platform.ErrorCodeNotFound, "incident not found")
		return
	}
	if !s.agentCanReadIncidentContext(r.Context(), *act, incidentNumber) {
		platform.WriteError(w, platform.ErrorCodeForbidden, "agent is not assigned or authorized to read this incident")
		return
	}
	if len(parts) > 1 && parts[1] == "timeline" {
		entries, err := s.incidentStore.GetTimeline(r.Context(), incidentNumber)
		if err != nil {
			platform.WriteInternalError(w, err, "failed to get timeline")
			return
		}
		platform.WriteData(w, http.StatusOK, platform.EnsureSlice(entries))
		return
	}
	platform.WriteData(w, http.StatusOK, s.buildAgentIncidentContext(r.Context(), inc))
}

type agentIncidentContextResponse struct {
	Incident *store.IncidentRecord      `json:"incident"`
	Roles    []agentIncidentRoleContext `json:"roles"`
}

type agentIncidentRoleContext struct {
	RoleType     string `json:"role_type"`
	AssigneeType string `json:"assignee_type"`
	AgentTokenID string `json:"agent_token_id,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	UserName     string `json:"user_name,omitempty"`
	Status       string `json:"status"`
}

func (s *Service) buildAgentIncidentContext(ctx context.Context, inc *store.IncidentRecord) agentIncidentContextResponse {
	resp := agentIncidentContextResponse{Incident: inc, Roles: []agentIncidentRoleContext{}}
	if inc == nil || s.icsRoleStore == nil {
		return resp
	}
	roles, err := s.icsRoleStore.GetActiveRoles(ctx, inc.IncidentNumber)
	if err != nil {
		logger.WarnCtx(ctx, "failed to load active incident roles for agent context", "incident_number", inc.IncidentNumber, "error", err)
		return resp
	}
	for _, role := range roles {
		entry := agentIncidentRoleContext{
			RoleType:     role.RoleType,
			AssigneeType: role.AssigneeType,
			AgentName:    role.AgentName,
			UserName:     role.UserName,
			Status:       role.Status,
		}
		if role.AgentTokenID != nil {
			entry.AgentTokenID = role.AgentTokenID.String()
		}
		if role.UserID != nil {
			entry.UserID = role.UserID.String()
		}
		resp.Roles = append(resp.Roles, entry)
	}
	return resp
}

// agentCanReadIncidentContext reports whether the agent may read the incident
// context: either by holding the investigate capability or by holding an
// active ICS role on the incident.
func (s *Service) agentCanReadIncidentContext(ctx context.Context, agent agentTokenContext, incidentNumber int64) bool {
	if s.exec != nil && s.exec.requireCapability(agent, capability.Investigate) == nil {
		return true
	}
	if s.icsRoleStore != nil {
		roles, err := s.icsRoleStore.GetActiveRoles(ctx, incidentNumber)
		if err == nil {
			for _, role := range roles {
				if role.Status != string(ics.RoleStatusActive) || role.AssigneeType != "agent" || role.AgentTokenID == nil || *role.AgentTokenID != agent.ID {
					continue
				}
				switch role.RoleType {
				case string(ics.RoleIncidentCommander), string(ics.RoleCommunicationsLead), string(ics.RoleResponder):
					return true
				}
			}
		}
	}
	if s.incidentInvestigationStore != nil {
		investigations, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
		if err == nil {
			for _, inv := range investigations {
				if inv.AgentID == agent.ID.String() && isActiveIncidentInvestigationForCoordination(inv.Status) {
					return true
				}
			}
		}
	}
	return false
}

func (s *Service) handleAgentAddIncidentTimeline(w http.ResponseWriter, r *http.Request, agent agentTokenContext, incidentNumber int64) {
	var req struct {
		EventType string `json:"event_type"`
		Message   string `json:"message"`
	}
	if !platform.DecodeJSON(w, r, &req) {
		return
	}
	if req.Message == "" {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "message is required")
		return
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = "agent_note"
	}
	agentName := agent.Name
	if agentName == "" {
		agentName = "Agent"
	}
	invs, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(r.Context(), incidentNumber)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to list investigations")
		return
	}
	authorized := false
	for _, inv := range invs {
		if inv.AgentID == agent.ID.String() {
			authorized = true
			break
		}
	}
	if !authorized {
		platform.WriteError(w, platform.ErrorCodeForbidden, "agent is not assigned to any investigation in this incident")
		return
	}
	if err := s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      eventType,
		ActorType:      "agent",
		Message:        fmt.Sprintf("[%s] %s", agentName, req.Message),
	}); err != nil {
		platform.WriteInternalError(w, err, "failed to add timeline entry")
		return
	}
	s.auditAgent(r, store.AuditIncidentUpdated, agent.Name, map[string]any{
		"incident_number": incidentNumber,
		"action":          "timeline_entry",
		"event_type":      eventType,
	})
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": strconv.FormatInt(incidentNumber, 10), "action": "timeline_entry"})
	platform.WriteStatus(w, "ok")
}

func (s *Service) handleAgentPatchIncident(w http.ResponseWriter, r *http.Request, agent agentTokenContext, incidentNumber int64) {
	var req struct {
		Summary *string `json:"summary"`
	}
	if !platform.DecodeJSON(w, r, &req) {
		return
	}
	inc, err := s.incidentStore.GetIncident(r.Context(), incidentNumber)
	if err != nil || inc == nil {
		platform.WriteError(w, platform.ErrorCodeNotFound, "incident not found")
		return
	}
	agentIDHex := agent.ID.String()
	invs, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(r.Context(), incidentNumber)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to list investigations")
		return
	}
	authorized := false
	for _, inv := range invs {
		if inv.AgentID == agentIDHex {
			authorized = true
			break
		}
	}
	if !authorized {
		platform.WriteError(w, platform.ErrorCodeForbidden, "agent is not assigned to any investigation in this incident")
		return
	}
	if req.Summary != nil {
		inc.Summary = *req.Summary
	}
	updated, err := s.incidentStore.UpdateIncident(r.Context(), incidentNumber, inc)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to update incident")
		return
	}
	s.auditAgent(r, store.AuditIncidentUpdated, agent.Name, map[string]any{
		"incident_number": incidentNumber,
		"action":          "patch",
		"fields":          "summary",
	})
	s.publishIncidentEvent("incident_updated", updated)
	platform.WriteData(w, http.StatusOK, updated)
}

func (s *Service) handleAgentServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	if err := s.exec.requireCapability(*act, capability.Investigate); err != nil {
		platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
		return
	}
	if s.serviceStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "service store not configured")
		return
	}
	limit, skip := platform.ParseLimitSkip(r, 100)
	records, total, err := s.serviceStore.ListServices(r.Context(), store.ListServicesFilter{Limit: int(limit), Skip: int(skip)})
	if err != nil {
		platform.WriteInternalError(w, err, "failed to list services")
		return
	}
	platform.WritePaginatedJSON(w, platform.EnsureSlice(records), int64(total))
}

func (s *Service) handleAgentOnCallCurrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	if err := s.exec.requireCapability(*act, capability.Command); err != nil {
		platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
		return
	}
	if s.onCallStore == nil || s.onCallResolver == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "on-call store not configured")
		return
	}
	schedules, _, err := s.onCallStore.ListSchedules(r.Context(), 100, 0)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to list schedules")
		return
	}
	now := timeNow().UTC()
	type OnCallEntry struct {
		ScheduleID   string `json:"schedule_id"`
		ScheduleName string `json:"schedule_name"`
		UserID       string `json:"user_id,omitempty"`
		UserName     string `json:"user_name,omitempty"`
	}
	var results []OnCallEntry
	for _, sched := range schedules {
		userID, resolveErr := s.onCallResolver.ResolveWhoIsOnCall(r.Context(), sched.ID, now)
		if resolveErr != nil || userID == nil {
			continue
		}
		name := ""
		if s.userStore != nil {
			if u, userErr := s.userStore.GetByID(*userID); userErr == nil && u != nil {
				name = u.DisplayName()
				if name == "" {
					name = u.Email
				}
			}
		}
		scheduleName := "On-Call"
		if s.scheduleDisplayName != nil {
			scheduleName = s.scheduleDisplayName(r.Context(), &sched)
		}
		results = append(results, OnCallEntry{
			ScheduleID:   sched.ID.String(),
			ScheduleName: scheduleName,
			UserID:       userID.String(),
			UserName:     name,
		})
	}
	if results == nil {
		results = []OnCallEntry{}
	}
	platform.WriteData(w, http.StatusOK, results)
}

func (s *Service) handleAgentPlaybooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}

	// Require the Investigate capability, mirroring handleAgentServices.
	// Previously any valid agent token could list playbooks regardless of its
	// capabilities (ASVS V4.1, SPEC gap M6).
	act, ok := platform.RequireAgent(w, r)
	if !ok {
		return
	}
	if err := s.exec.requireCapability(*act, capability.Investigate); err != nil {
		platform.WriteError(w, platform.ErrorCodeForbidden, err.Error())
		return
	}

	if s.alertStore == nil || s.playbookStore == nil {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "service not available")
		return
	}

	alertFingerprint := r.URL.Query().Get("alert_fingerprint")

	var labels map[string]string
	if alertFingerprint != "" {
		alert, err := s.alertStore.GetByFingerprint(alertFingerprint)
		if err == nil && alert != nil {
			labels = alert.Labels
		}
	}

	playbooks, err := s.playbookStore.FindMatching(r.Context(), labels)
	if err != nil {
		platform.WriteInternalError(w, err, "failed to find matching playbooks")
		return
	}

	platform.WriteData(w, http.StatusOK, platform.EnsureSlice(playbooks))
}

// maxTokenNameLength bounds the length of agent-token names (mirrors
// api.maxTokenNameLength).
const maxTokenNameLength = 256

// isActiveIncidentInvestigationForCoordination is the agent-package copy of
// api.isActiveIncidentInvestigationForCoordination, used by the agent incident
// context read gate.
func isActiveIncidentInvestigationForCoordination(status string) bool {
	switch status {
	case store.IncidentInvestigationStatusPending, store.IncidentInvestigationStatusAssigned, store.IncidentInvestigationStatusInvestigating, store.IncidentInvestigationStatusPaused:
		return true
	default:
		return false
	}
}
