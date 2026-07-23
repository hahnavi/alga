// Package llm implements an OpenAI-compatible chat completions client with
// streaming support, tool-call parsing, and retry/backoff for transient errors.
package llm

import (
	"context"
)

// Provider is the backend-agnostic chat completions interface consumed by the
// agent loop. The default implementation, *Client, speaks OpenAI's
// chat-completions API (which Anthropic, Google, OpenRouter, Ollama, and most
// other gateways emulate).
//
// To add a native non-OpenAI backend (e.g. raw Anthropic Messages API for
// prompt caching, or Gemini GenerateContent), implement Provider and pass it
// to the agent. The loop is unaware of the wire format.
//
// The interface is deliberately minimal: Complete for tool-call turns (non-
// streaming) and Stream for the final text turn. Anything more (token
// accounting, prompt caching hints, fine-grained params) is exposed via the
// concrete type's Options.
type Provider interface {
	// Complete performs a non-streaming chat completion.
	Complete(ctx context.Context, req Request) (*CompletionResponse, error)
	// Stream performs a streaming chat completion, invoking cb for each text
	// delta. Returns the full accumulated text.
	Stream(ctx context.Context, req Request, cb StreamCallback) (string, error)
	// Model returns the configured model name. Used for logging/metrics.
	Model() string
}

// *Client satisfies Provider at compile time.
var _ Provider = (*Client)(nil)

// Event is a structured observation emitted by the agent loop. Listeners
// (telemetry exporters, debug UIs, replays) subscribe via AgentCore.Events.
// Events are non-blocking: a slow listener does not stall the loop.
type Event struct {
	Kind      EventKind
	SessionID string
	ChatID    string
	Iteration int
	ToolName  string
	Latency   string
	TokenIn   int
	TokenOut  int
	Model     string
	Err       error
}

// EventKind enumerates the structured events the agent loop emits.
type EventKind string

const (
	EventTurnStart    EventKind = "turn_start"
	EventTurnComplete EventKind = "turn_complete"
	EventLLMCall      EventKind = "llm_call"
	EventToolCall     EventKind = "tool_call"
	EventStreamDelta  EventKind = "stream_delta"
	EventTurnError    EventKind = "turn_error"
)

// EventEmitter fans out Events to non-blocking subscribers. The agent loop
// constructs one and calls Emit on every significant state transition.
// Subscribers that are interested in tool-call latency, token usage, model
// selection, or failure modes can register here without touching the loop
// internals.
type EventEmitter struct {
	subs []chan Event
}

// NewEventEmitter constructs an EventEmitter with capacity subs, each backed
// by a buffered channel of the given size. A subscriber that falls behind
// drops events once its buffer fills — the loop never blocks on telemetry.
func NewEventEmitter(subs, bufSize int) *EventEmitter {
	e := &EventEmitter{subs: make([]chan Event, subs)}
	for i := range e.subs {
		e.subs[i] = make(chan Event, bufSize)
	}
	return e
}

// Subscribe returns the i-th subscriber channel. Reads block until an Event
// arrives or the agent shuts down.
func (e *EventEmitter) Subscribe(i int) <-chan Event {
	if i < 0 || i >= len(e.subs) {
		return nil
	}
	return e.subs[i]
}

// Emit delivers ev to every subscriber. Non-blocking: a full subscriber's
// event is dropped (the loop must not stall on telemetry).
func (e *EventEmitter) Emit(ev Event) {
	for _, ch := range e.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}
