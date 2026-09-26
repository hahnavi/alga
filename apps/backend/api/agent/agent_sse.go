package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/logger"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

type AgentSSEHandler struct {
	broker     *sse.Broker
	vkClient   *valkey.Client
	presence   *valkey.Presence
	tokenStore store.AgentTokenStore
	executor   *AgentToolExecutor

	allowedOrigins []string

	// allowQueryToken re-enables the legacy `?token=` fallback for pure-
	// EventSource consumers; deny-by-default.
	allowQueryToken bool
}

func NewAgentSSEHandler(
	broker *sse.Broker,
	vkClient *valkey.Client,
	presence *valkey.Presence,
	tokenStore store.AgentTokenStore,
	executor *AgentToolExecutor,
) *AgentSSEHandler {
	return &AgentSSEHandler{
		broker:     broker,
		vkClient:   vkClient,
		presence:   presence,
		tokenStore: tokenStore,
		executor:   executor,
	}
}

func (h *AgentSSEHandler) SetAllowedOrigins(origins []string) {
	h.allowedOrigins = slices.Clone(origins)
}

// SetAllowQueryToken re-enables the legacy `?token=` SSE fallback (escape
// hatch for EventSource-only consumers; deny-by-default).
func (h *AgentSSEHandler) SetAllowQueryToken(v bool) {
	h.allowQueryToken = v
}

func (h *AgentSSEHandler) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			platform.WriteError(w, platform.ErrorCodeMethodNotAllowed, "method not allowed")
			return
		}

		if !h.checkOrigin(r) {
			platform.WriteError(w, platform.ErrorCodeForbidden, "origin not allowed")
			return
		}

		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" && h.allowQueryToken {
			// Legacy escape hatch only (AGENT_SSE_ALLOW_QUERY_TOKEN=true):
			// never log the URL or token here — query strings land in proxy
			// and access logs.
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			platform.WriteError(w, platform.ErrorCodeUnauthorized, "missing authorization token")
			return
		}

		rec, err := h.tokenStore.ValidateToken(token)
		if err != nil {
			platform.WriteInternalError(w, errors.New("token validation"), "failed to validate token")
			return
		}
		if rec == nil {
			platform.WriteError(w, platform.ErrorCodeUnauthorized, "invalid or expired token")
			return
		}

		agentKey := rec.ID.String()

		flusher, ok := w.(http.Flusher)
		if !ok {
			platform.WriteError(w, platform.ErrorCodeInternal, "streaming not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Clear the server-wide WriteTimeout for this long-lived stream;
		// the request context is the cancellation source for SSE.
		sse.DisableWriteDeadline(w)

		clientID := uuid.New().String()
		ch := make(chan sse.Event, 64)
		h.broker.SubscribeAgent(agentKey, clientID, ch)

		ctx := r.Context()

		if h.presence != nil {
			if err := h.presence.Register(ctx, agentKey, clientID, string(store.NormalizeAgentType(rec.AgentType))); err != nil {
				logger.WarnCtx(ctx, "SSE agent presence register failed", "agent_id", agentKey, "error", err)
			}
		}

		h.executor.PublishAgentPresence(agentKey, true)
		if h.presence != nil {
			_ = h.presence.PublishEvent(ctx, valkey.AgentEvent{
				Type:      valkey.AgentEventOnline,
				AgentID:   agentKey,
				SessionID: clientID,
			})
		}

		logger.InfoCtx(r.Context(), "Agent SSE connected", "agent_id", agentKey, "agent_name", rec.Name, "session_id", clientID)

		defer func() {
			h.broker.UnsubscribeAgent(agentKey, clientID)

			// The request context is already canceled by the time cleanup runs;
			// detach but bound so presence/valkey calls cannot hang forever.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()

			var presenceEmpty bool
			if h.presence != nil {
				empty, err := h.presence.Unregister(cleanupCtx, agentKey, clientID)
				if err != nil {
					logger.Warn("SSE agent presence unregister failed", "agent_id", agentKey, "error", err)
				}
				presenceEmpty = empty
				_ = h.presence.PublishEvent(cleanupCtx, valkey.AgentEvent{
					Type:      valkey.AgentEventSessionEnded,
					AgentID:   agentKey,
					SessionID: clientID,
				})
			}

			if !h.broker.AgentOnline(agentKey) && (h.presence == nil || presenceEmpty) {
				h.executor.PublishAgentPresence(agentKey, false)
				if h.presence != nil {
					_ = h.presence.PublishEvent(cleanupCtx, valkey.AgentEvent{
						Type:      valkey.AgentEventOffline,
						AgentID:   agentKey,
						SessionID: clientID,
					})
				}
			}

			logger.Info("Agent SSE disconnected", "agent_id", agentKey, "session_id", clientID)
		}()

		if err := sse.WriteEvent(w, sse.Event{Type: "connected", Data: map[string]string{"client_id": clientID, "agent_id": agentKey}}); err != nil {
			logger.Warn("failed to write agent sse connected frame", "component", "agent-sse", "error", err)
			return
		}
		flusher.Flush()

		keepalive := time.NewTicker(sse.KeepaliveInterval)
		defer keepalive.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				if err := sse.WriteEvent(w, event); err != nil {
					logger.Error("Failed to write agent SSE event", "error", err)
					continue
				}
				flusher.Flush()
			case <-keepalive.C:
				_, _ = fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()

				if h.presence != nil {
					_ = h.presence.Renew(r.Context(), agentKey, clientID)
				}
			}
		}
	}
}

func (h *AgentSSEHandler) checkOrigin(r *http.Request) bool {
	if len(h.allowedOrigins) == 0 {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	for _, o := range h.allowedOrigins {
		if strings.EqualFold(origin, strings.TrimSpace(o)) {
			return true
		}
	}
	return origin == ""
}

// PublishToAgent delivers an event to every live session of the agent: the
// local broker plus the Valkey fan-out (peer replicas loop it back to their
// local sessions). It returns an error only when the event could not reach
// any session: no local subscriber, no Valkey, and presence confirms the
// agent has no session anywhere. Dispatchers rely on this to requeue work
// instead of crediting a phantom delivery to a disconnected agent.
func (h *AgentSSEHandler) PublishToAgent(agentTokenID string, event sse.Event) error {
	localErr := h.broker.PublishToAgent(agentTokenID, event)
	if h.vkClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sse.PublishToValkeyAgent(ctx, h.vkClient.Client(), agentTokenID, event); err != nil {
			logger.Error("Failed to publish SSE event to Valkey for agent", "agent_id", agentTokenID, "error", err)
		}
	}
	if localErr == nil {
		return nil
	}
	if h.presence != nil && h.presence.Available() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if !h.presence.IsAgentOnline(ctx, agentTokenID) {
			return fmt.Errorf("agent %s has no connected SSE session", agentTokenID)
		}
	}
	if h.vkClient == nil {
		return localErr
	}
	return nil
}

func (h *AgentSSEHandler) PublishToAgentAllowDrop(agentTokenID string, event sse.Event) {
	h.broker.PublishToAgentAllowDrop(agentTokenID, event)
	if h.vkClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sse.PublishToValkeyAgent(ctx, h.vkClient.Client(), agentTokenID, event); err != nil {
			logger.Error("Failed to publish SSE event to Valkey for agent (allow drop)", "agent_id", agentTokenID, "error", err)
		}
	}
}

func (h *AgentSSEHandler) BroadcastToAgents(event sse.Event, excludeAgentID string) {
	h.broker.BroadcastToAgents(event, excludeAgentID)
}

func (h *AgentSSEHandler) AgentOnline(agentTokenID string) bool {
	if h.broker.AgentOnline(agentTokenID) {
		return true
	}
	if h.presence != nil && h.presence.Available() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return h.presence.IsAgentOnline(ctx, agentTokenID)
	}
	return false
}

// boundedCtx removed: callers inline context.WithTimeout with defer cancel.
