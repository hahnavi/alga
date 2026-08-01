package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"alga-agent/internal/agent"
)

// Router dispatches inbound channel messages to the agent core and routes
// responses back to the originating channel. Sessions are namespaced per
// channel ("alga:<id>") so messages never bleed across.
type Router struct {
	agent  *agent.AgentCore
	logger *slog.Logger
	// channels is the set of response sinks keyed by channel name.
	sinks map[string]ResponseSink
	mu    sync.RWMutex
	// wg tracks in-flight dispatch goroutines so graceful shutdown can drain.
	wg sync.WaitGroup
}

// NewRouter constructs a Router.
func NewRouter(a *agent.AgentCore, logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		agent:  a,
		logger: logger,
		sinks:  make(map[string]ResponseSink),
	}
}

// RegisterSink associates a response sink with a channel name.
func (r *Router) RegisterSink(name string, sink ResponseSink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sinks[name] = sink
}

// Dispatch processes an inbound message: notifies the channel ("thinking"),
// runs the agent loop, and delivers the response. It is safe to call from
// any goroutine. Each session is serialized so concurrent messages from the
// same chat queue rather than interleave.
func (r *Router) Dispatch(ctx context.Context, msg InboundMessage) {
	start := time.Now()
	logger := r.logger.With("session", msg.SessionID, "channel", msg.ChannelName)
	logger.Info("message received", "sender", msg.SenderName, "len", len(msg.Text))

	sink := r.lookupSink(msg.ChannelName)

	// Notify the channel that we're processing (renders a placeholder).
	if sink != nil {
		if err := sink.OnThinking(ctx, msg.ChatID); err != nil {
			logger.Warn("OnThinking failed", "err", err)
		}
	}

	// Run the agent loop with the channel's sink (for streaming deltas).
	var agentSink agent.StreamSink
	if sink != nil {
		agentSink = asAgentSink(ctx, sink, msg.ChatID)
	}
	result, err := r.agent.Process(ctx, agent.ProcessRequest{
		SessionID:     msg.SessionID,
		ChatID:        msg.ChatID,
		Text:          msg.Text,
		SenderName:    msg.SenderName,
		AlgaCtx:       msg.AlgaCtx,
		SystemContext: msg.SystemContext,
		Sink:          agentSink,
	})
	if err != nil {
		// Context cancellation (graceful shutdown) is not a user-facing error.
		// Don't spam the chat with "I'm having trouble" on the way out.
		if ctx.Err() != nil {
			logger.Info("agent processing cancelled by shutdown", "elapsed", time.Since(start).String())
			return
		}
		logger.Error("agent processing failed", "err", err, "elapsed", time.Since(start).String())
		r.deliverError(ctx, sink, msg.ChatID, err)
		return
	}

	// Deliver the final response.
	if sink != nil {
		if err := sink.OnFinal(ctx, msg.ChatID, result.Text); err != nil {
			logger.Warn("OnFinal failed", "err", err)
		}
	}
	logger.Info("message processed",
		"elapsed", time.Since(start).String(),
		"iterations", result.Iterations,
		"tool_calls", result.ToolCalls,
		"response_len", len(result.Text))
}

// DispatchAsync runs Dispatch in a tracked goroutine so the caller doesn't
// block. The goroutine is registered with the router's WaitGroup so graceful
// shutdown can drain in-flight messages.
func (r *Router) DispatchAsync(ctx context.Context, msg InboundMessage) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.Dispatch(ctx, msg)
	}()
}

// Wait blocks until all in-flight dispatch goroutines finish. Used during
// graceful shutdown to drain messages before exit.
func (r *Router) Wait() {
	r.wg.Wait()
}

func (r *Router) lookupSink(channelName string) ResponseSink {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sinks[channelName]
}

func (r *Router) deliverError(ctx context.Context, sink ResponseSink, chatID string, err error) {
	if sink == nil {
		return
	}
	text := friendlyError(err)
	if cerr := sink.OnError(ctx, chatID, text); cerr != nil {
		r.logger.Warn("OnError failed", "err", cerr)
	}
}

// friendlyError maps known agent errors to user-friendly messages per SPEC §8.3.
func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, agent.ErrMaxIterations.Error()) {
		return "I've reached my iteration limit while working on this. Can you narrow the request or break it into smaller steps?"
	}
	// Generic LLM/transient failure.
	return "I'm having trouble thinking right now. Please try again in a moment."
}

// SessionIDFor constructs a channel-namespaced session id.
func SessionIDFor(channel string, chatID any) string {
	return fmt.Sprintf("%s:%v", channel, chatID)
}
