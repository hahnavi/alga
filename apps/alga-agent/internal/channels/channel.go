// Package channels implements the messaging channel adapters (Alga)
// and the message router that dispatches inbound messages to the agent core.
package channels

import (
	"context"

	"alga-agent/internal/agent"
)

// Channel is the interface every messaging adapter implements.
type Channel interface {
	// Name returns the channel identifier ("alga").
	Name() string
	// Start begins receiving messages. Blocks until ctx is cancelled or the
	// channel stops. Must be safe to call in a goroutine.
	Start(ctx context.Context) error
	// Stop shuts down the channel, draining in-flight work. Called during
	// graceful shutdown.
	Stop() error
}

// InboundMessage is a message received from a channel, to be routed to the
// agent core.
type InboundMessage struct {
	// SessionID is the fully-qualified session id ("alga:<chat_id>").
	// Constructed by the channel adapter.
	SessionID string
	// ChatID is the channel-local chat identifier.
	ChatID string
	// Text is the message body.
	Text string
	// SenderID is the channel-local user identifier.
	SenderID string
	// SenderName is a human-readable name for the sender.
	SenderName string
	// ChannelName is "alga".
	ChannelName string
	// AlgaCtx carries Alga investigation context (alga channel only).
	AlgaCtx agent.AlgaContext
	// SystemContext carries behavioral rules from the backend dispatch that
	// should be injected into the LLM system prompt (alga channel only).
	SystemContext string
}

// ResponseSink is implemented by channels to receive the agent's response.
// The router uses it to deliver the final text and, for streaming channels,
// progressive deltas during the final LLM turn.
type ResponseSink interface {
	// OnThinking is called once at the start of processing, so the channel
	// can render a "thinking..." placeholder before the first token.
	OnThinking(ctx context.Context, chatID string) error
	// OnDelta delivers an incremental text delta during the final streaming
	// turn. Return false to stop streaming early.
	OnDelta(ctx context.Context, chatID, accumulated, delta string) bool
	// OnFinal delivers the completed response text.
	OnFinal(ctx context.Context, chatID, text string) error
	// OnError delivers an error message to display to the user.
	OnError(ctx context.Context, chatID, text string) error
}

// sinkAdapter adapts a ResponseSink to the agent.StreamSink interface.
type sinkAdapter struct {
	ctx    context.Context
	chatID string
	sink   ResponseSink
}

func (s *sinkAdapter) OnDelta(accumulated, delta string) bool {
	return s.sink.OnDelta(s.ctx, s.chatID, accumulated, delta)
}

// asAgentSink returns an agent.StreamSink backed by sink for chatID.
func asAgentSink(ctx context.Context, sink ResponseSink, chatID string) agent.StreamSink {
	return &sinkAdapter{ctx: ctx, chatID: chatID, sink: sink}
}
