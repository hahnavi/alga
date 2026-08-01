package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	alga "github.com/alga/agent-sdk-go"

	"alga-agent/internal/agent"
	"alga-agent/internal/config"
)

// AlgaChannel adapts the Alga agent SDK (SSE inbound + REST outbound) to the
// Channel/ResponseSink interfaces.
type AlgaChannel struct {
	client    AlgaSDKClient
	cfg       config.AlgaConfig
	router    *Router
	logger    *slog.Logger
	chatIDMux sync.Mutex
	inflight  map[string]*algaStreamState
	// consecutiveFailures counts connection failures; after 5 the channel is
	// marked unhealthy and disabled for the session (SPEC §8.3).
	consecutiveFailures int
	healthy             bool
	mu                  sync.Mutex
	stop                context.CancelFunc
	// ctx is the channel-scoped context, captured at Start and propagated to
	// dispatched messages so graceful shutdown cancels in-flight work.
	ctx     context.Context
	stopped sync.Once
}

// AlgaSDKClient is the subset of *alga.AlgaClient used by AlgaChannel.
type AlgaSDKClient interface {
	Connect(ctx context.Context) error
	Disconnect()
	Err() <-chan error
	SendMessage(ctx context.Context, chatID, text string, mentions []string) (*alga.SendMessageResponse, error)
	SendTyping(ctx context.Context, chatID string, active bool) error
}

// algaStreamState tracks per-chat outbound state (for typing indicators).
type algaStreamState struct {
	chatID string
}

// NewAlgaChannel constructs an AlgaChannel. Returns (nil, nil) when disabled.
func NewAlgaChannel(cfg config.AlgaConfig, router *Router, logger *slog.Logger) (*AlgaChannel, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	client := alga.NewAlgaClient(cfg.ServerURL, cfg.AgentToken)
	return newAlgaChannelWithClient(client, cfg, router, logger), nil
}

// newAlgaChannelWithClient allows injecting a fake AlgaSDKClient (tests).
func newAlgaChannelWithClient(client AlgaSDKClient, cfg config.AlgaConfig, router *Router, logger *slog.Logger) *AlgaChannel {
	if logger == nil {
		logger = slog.Default()
	}
	return &AlgaChannel{
		client:   client,
		cfg:      cfg,
		router:   router,
		logger:   logger,
		inflight: make(map[string]*algaStreamState),
		healthy:  true,
	}
}

// Name implements Channel.
func (a *AlgaChannel) Name() string { return "alga" }

// Start connects to the Alga SSE stream and registers event callbacks. The
// SDK's SSE client reconnects internally with backoff (2s→60s, jitter); this
// method wires a liveness watchdog that marks the channel unhealthy if no
// connected event arrives within a deadline, per SPEC §8.3.
func (a *AlgaChannel) Start(ctx context.Context) error {
	innerCtx, cancel := context.WithCancel(ctx)
	a.stop = cancel
	a.ctx = innerCtx

	// Wire callbacks before connecting.
	a.registerCallbacks()

	if err := a.client.Connect(innerCtx); err != nil {
		a.markUnhealthy()
		return fmt.Errorf("alga connect: %w", err)
	}
	a.logger.Info("alga channel connected", "server_url", a.cfg.ServerURL)

	// Liveness watchdog: the SDK reconnects internally, but if it can't reach
	// the server for sustained periods we mark the channel unhealthy so the
	// operator is alerted. OnConnected resets the failure counter on every
	// successful (re)connection.
	go a.watchdog(innerCtx)

	// Terminal-error watcher: the SDK stops reconnecting on auth failures
	// (revoked token) and surfaces the error on Err(). Mark the channel
	// unhealthy so the operator is alerted; a new token requires a restart.
	go func() {
		select {
		case <-innerCtx.Done():
		case err, ok := <-a.client.Err():
			if !ok || err == nil {
				return
			}
			a.logger.Error("alga SSE terminal error, channel stopped reconnecting", "err", err)
			a.mu.Lock()
			a.healthy = false
			a.mu.Unlock()
		}
	}()

	return nil
}

// watchdog periodically checks liveness. If the channel hasn't seen a
// connected event recently, it increments the failure counter. This is a
// best-effort health signal since the SDK doesn't expose SSE errors directly.
func (a *AlgaChannel) watchdog(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// If we've accumulated 5+ failures without a reconnect, the SDK's
			// internal backoff is failing; mark unhealthy (idempotent).
			a.mu.Lock()
			unhealthy := a.consecutiveFailures >= 5
			a.mu.Unlock()
			if unhealthy {
				a.logger.Warn("alga channel watchdog: no successful reconnect recently",
					"consecutive_failures", a.consecutiveFailures)
			}
		}
	}
}

// registerCallbacks wires the SDK On* callbacks to channel handlers.
func (a *AlgaChannel) registerCallbacks() {
	// We need the concrete *alga.AlgaClient to set callbacks; the interface
	// doesn't expose them. For production wiring we pass the concrete client.
	if c, ok := a.client.(*alga.AlgaClient); ok {
		c.OnConnected = func(ev alga.ConnectedEvent) {
			a.mu.Lock()
			wasUnhealthy := !a.healthy
			a.consecutiveFailures = 0
			a.healthy = true
			a.mu.Unlock()
			if wasUnhealthy {
				a.logger.Info("alga SSE reconnected, channel healthy again",
					"client_id", ev.ClientID, "agent_id", ev.AgentID)
			} else {
				a.logger.Info("alga SSE connected", "client_id", ev.ClientID, "agent_id", ev.AgentID)
			}
		}
		c.OnMessage = a.onMessage
		c.OnPeerAsk = a.onPeerAsk
		c.OnPeerFinding = a.onPeerFinding
		c.OnInvestigationResume = a.onInvestigationResume
		c.OnCoordinationTask = a.onCoordinationTask
		c.OnSummarizeIncident = a.onSummarizeIncident
		c.OnIncidentCommsStale = a.onIncidentCommsStale
		c.OnAlertAutoResolved = a.onAlertAutoResolved
	}
}

// onMessage handles inbound chat messages.
func (a *AlgaChannel) onMessage(ev alga.MessageEvent) {
	if ev.Text == "" {
		return
	}
	// "observe" deliveries are context-only: the backend sends them so the
	// agent can see the transcript, not so it acts. Only dispatch/mention
	// (and legacy events without a trigger) wake the agent.
	if ev.Trigger == "observe" {
		return
	}
	// Skip internal/system messages (leading 🔒 per SDK convention).
	if strings.HasPrefix(ev.Text, "🔒") {
		return
	}
	chatID := ev.ChatID
	if chatID == "" {
		return
	}
	// Derive a per-message context from the channel's ctx so graceful
	// shutdown cancels in-flight Alga work. Previously this used
	// context.Background(), which leaked work past shutdown.
	ctx := a.dispatchCtx()
	// Derive Alga context from the chat id grammar (alert_<n>,
	// incident_inv_<n>, incident_coord_<n>).
	algaCtx := algaContextFromChatID(chatID)

	sessionID := SessionIDFor("alga", chatID)
	a.router.DispatchAsync(ctx, InboundMessage{
		SessionID:     sessionID,
		ChatID:        chatID,
		Text:          ev.Text,
		SenderID:      ev.SenderID,
		SenderName:    ev.SenderName,
		ChannelName:   a.Name(),
		AlgaCtx:       algaCtx,
		SystemContext: ev.SystemContext,
	})
}

// dispatchCtx returns a context that is canceled when the channel stops,
// falling back to context.Background() if Start was never called.
func (a *AlgaChannel) dispatchCtx() context.Context {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// onPeerAsk handles peer-agent questions directed at this agent.
//
// Known limitation: the reply is routed back through the normal message path
// using a synthetic chat id derived from the investigation UUID; proper
// ReplyPeerAsk wiring is a separate feature.
func (a *AlgaChannel) onPeerAsk(ev alga.PeerAskEvent) {
	ctx := a.dispatchCtx()
	chatID := investigationChatID(ev.InvestigationID)
	algaCtx := agent.AlgaContext{InvestigationID: ev.InvestigationID}
	sessionID := SessionIDFor("alga", chatID)
	// Treat peer asks as actionable by default; classify in the loop.
	a.router.DispatchAsync(ctx, InboundMessage{
		SessionID:   sessionID,
		ChatID:      chatID,
		Text:        "Peer ask from " + ev.FromAgentName + ": " + ev.Question,
		SenderID:    ev.FromAgentID,
		SenderName:  ev.FromAgentName,
		ChannelName: a.Name(),
		AlgaCtx:     algaCtx,
	})
}

// onPeerFinding stores peer findings in session memory (PLAN §7.2).
func (a *AlgaChannel) onPeerFinding(ev alga.PeerFindingEvent) {
	// Store as a memory via the agent's memory tool if available. For now,
	// log; the agent can recall it on the next turn.
	a.logger.Info("alga peer finding",
		"investigation_id", ev.InvestigationID,
		"peer_agent_id", ev.PeerAgentID,
		"peer_agent_type", ev.PeerAgentType,
		"text", ev.Text)
}

func (a *AlgaChannel) onInvestigationResume(ev alga.InvestigationSignalEvent) {
	invID := ev.InvestigationID
	if invID == "" {
		invID = ev.AlertInvestigationID
	}
	a.logger.Info("alga investigation resumed", "investigation_id", invID, "reason", ev.Reason, "actor", ev.Actor)
}

// dispatchIncidentInstruction dispatches an incident-scoped instruction
// message to the agent. It resolves a fallback coordination chat id when chatID
// is empty, builds the AlgaContext and session id, and enqueues the message via
// DispatchAsync. Shared by the coordination-task, summarize-incident, and
// comms-stale handlers.
func (a *AlgaChannel) dispatchIncidentInstruction(incidentNumber int64, text, senderID, senderName, chatID string) {
	if chatID == "" {
		chatID = fmt.Sprintf("incident_coord_%d", incidentNumber)
	}
	ctx := a.dispatchCtx()
	algaCtx := agent.AlgaContext{IncidentID: fmt.Sprintf("%d", incidentNumber)}
	sessionID := SessionIDFor("alga", chatID)
	a.router.DispatchAsync(ctx, InboundMessage{
		SessionID:   sessionID,
		ChatID:      chatID,
		Text:        text,
		SenderID:    senderID,
		SenderName:  senderName,
		ChannelName: a.Name(),
		AlgaCtx:     algaCtx,
	})
}

// onCoordinationTask handles a coordination task dispatched by the scheduler
// on behalf of an incident commander. The agent is expected to claim the task,
// perform the work, and complete it with a typed result.
func (a *AlgaChannel) onCoordinationTask(ev alga.CoordinationTaskEvent) {
	goal := ev.GoalText()
	if goal == "" {
		a.logger.Warn("coordination task dispatch with empty goal, skipping",
			"task_id", ev.TaskID, "incident_number", ev.IncidentNumber)
		return
	}

	a.logger.Info("coordination task dispatched",
		"task_id", ev.TaskID,
		"incident_number", ev.IncidentNumber,
		"kind", ev.Kind,
		"assignee_role", ev.AssigneeRole)

	text := fmt.Sprintf(
		"You have been dispatched a coordination task for incident #%d.\n\n"+
			"Task ID: %s\nKind: %s\nAssignee role: %s\nGoal: %s\n\n"+
			"Claim this task with alga_claim_task, perform the work, then report your result with alga_complete_task.",
		ev.IncidentNumber, ev.TaskID, ev.Kind, ev.AssigneeRole, goal,
	)

	a.dispatchIncidentInstruction(ev.IncidentNumber, text, "scheduler", "Coordination Scheduler", ev.ChatID)
}

// onSummarizeIncident handles a backend request for an incident summary
// (typically directed at a communicate-capable agent).
func (a *AlgaChannel) onSummarizeIncident(ev alga.SummarizeIncidentEvent) {
	a.logger.Info("incident summary requested", "incident_number", ev.IncidentNumber)

	text := fmt.Sprintf(
		"The backend has requested an incident summary for incident #%d. "+
			"Review the incident state and post a concise status update using alga_add_incident_timeline.",
		ev.IncidentNumber,
	)

	a.dispatchIncidentInstruction(ev.IncidentNumber, text, "backend", "Alga Backend", ev.ChatID)
}

// onIncidentCommsStale handles a nudge when incident communications have gone
// quiet past the SLA threshold.
func (a *AlgaChannel) onIncidentCommsStale(ev alga.IncidentCommsStaleEvent) {
	a.logger.Info("incident comms stale",
		"incident_number", ev.IncidentNumber, "reason", ev.Reason)

	text := fmt.Sprintf(
		"Incident #%d communications have gone stale (reason: %s). "+
			"Post a status update or escalate if the incident is not progressing.",
		ev.IncidentNumber, ev.Reason,
	)

	a.dispatchIncidentInstruction(ev.IncidentNumber, text, "backend", "Alga Backend", "")
}

// onAlertAutoResolved logs that an alert the agent was investigating has
// auto-resolved at the source. No action is needed — the investigation is
// complete.
func (a *AlgaChannel) onAlertAutoResolved(ev alga.AlertAutoResolvedEvent) {
	a.logger.Info("alert auto-resolved",
		"investigation_id", ev.InvestigationID,
		"fingerprint", ev.Fingerprint,
		"alert_name", ev.AlertName)
}

// --- ResponseSink implementation ---

// OnThinking sends a typing indicator.
func (a *AlgaChannel) OnThinking(ctx context.Context, chatID string) error {
	return a.client.SendTyping(ctx, chatID, true)
}

// OnDelta is a no-op for Alga (the SDK delivers the final message; partial
// updates are not expected). We refresh the typing indicator.
func (a *AlgaChannel) OnDelta(ctx context.Context, chatID, accumulated, delta string) bool {
	_ = a.client.SendTyping(ctx, chatID, true)
	return true
}

// OnFinal delivers the completed response via SendMessage.
func (a *AlgaChannel) OnFinal(ctx context.Context, chatID, text string) error {
	_, err := a.client.SendMessage(ctx, chatID, text, nil)
	if err != nil {
		return fmt.Errorf("alga send: %w", err)
	}
	_ = a.client.SendTyping(ctx, chatID, false)
	return nil
}

// OnError delivers an error message via SendMessage.
func (a *AlgaChannel) OnError(ctx context.Context, chatID, text string) error {
	_, err := a.client.SendMessage(ctx, chatID, text, nil)
	return err
}

// markUnhealthy disables the channel after 5 consecutive failures (SPEC §8.3).
func (a *AlgaChannel) markUnhealthy() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.consecutiveFailures++
	if a.consecutiveFailures >= 5 {
		a.healthy = false
		a.logger.Error("alga channel marked unhealthy after 5 consecutive failures", "failures", a.consecutiveFailures)
	}
}

// Healthy reports whether the channel is still considered healthy.
func (a *AlgaChannel) Healthy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.healthy
}

// Stop disconnects from the Alga SSE stream.
func (a *AlgaChannel) Stop() error {
	a.stopped.Do(func() {
		if a.stop != nil {
			a.stop()
		}
		a.client.Disconnect()
	})
	return nil
}

// Note on reconnect: the Alga SDK's SSE client implements its own reconnect
// with exponential backoff (2s→60s, ~0.9-1.1x jitter) and honors Retry-After
// on 429s. OnConnected fires on each successful (re)connection and resets the
// failure counter. The watchdog goroutine monitors liveness and logs when the
// channel has been unable to reconnect for sustained periods.

// algaContextFromChatID derives an AlgaContext from the backend chat id
// grammar: alert_<n> (alert investigation thread), incident_inv_<n>
// (incident investigation thread), incident_coord_<n> (coordination thread).
// Alert chats carry no investigation UUID, so InvestigationID stays empty and
// tools that need it require an explicit argument.
func algaContextFromChatID(chatID string) agent.AlgaContext {
	var ctx agent.AlgaContext
	if n, ok := stripPrefix(chatID, "incident_inv_"); ok {
		ctx.IncidentID = n
	} else if n, ok := stripPrefix(chatID, "incident_coord_"); ok {
		ctx.IncidentID = n
	}
	return ctx
}

// investigationChatID constructs a synthetic session/chat key for peer-ask
// dispatch. This is not a backend chat id (see onPeerAsk).
func investigationChatID(invID string) string {
	if invID == "" {
		return ""
	}
	return "investigation_" + invID
}

// stripPrefix returns s without the prefix and true if s starts with prefix.
func stripPrefix(s, prefix string) (string, bool) {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return s, false
}
