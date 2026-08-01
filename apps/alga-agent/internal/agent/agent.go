package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"alga-agent/internal/config"
	"alga-agent/internal/llm"
	"alga-agent/internal/tools"
)

// ErrMaxIterations is returned when the conversation loop exceeds the max
// iterations without producing a final text response.
var ErrMaxIterations = errors.New("agent: max iterations reached without final response")

// StreamSink receives incremental text during the final streaming turn. It is
// optional; when nil, the final turn is not streamed. Channels implement this
// to deliver progressive message edits (SPEC §5.2.1, §6.3).
type StreamSink interface {
	// OnDelta is called with the accumulated text so far and the new delta.
	// Return false to stop streaming early.
	OnDelta(accumulated, delta string) bool
}

// AgentCore runs the conversation loop: receive message → load session → build
// prompt → call LLM → execute tools → loop until text response.
type AgentCore struct {
	llm        *llm.Client
	tools      *tools.Registry
	store      *SessionStore
	cfg        config.AgentBehaviorConfig
	agentCfg   config.AgentConfig
	promptFile string
	logger     *slog.Logger
}

// Options configure an AgentCore.
type Options struct {
	LLM        *llm.Client
	Tools      *tools.Registry
	Store      *SessionStore
	Behavior   config.AgentBehaviorConfig
	Agent      config.AgentConfig
	PromptFile string
	Logger     *slog.Logger
}

// New constructs an AgentCore.
func New(opts Options) *AgentCore {
	if opts.Store == nil {
		opts.Store = NewSessionStore(opts.Behavior.ContextWindow)
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &AgentCore{
		llm:        opts.LLM,
		tools:      opts.Tools,
		store:      opts.Store,
		cfg:        opts.Behavior,
		agentCfg:   opts.Agent,
		promptFile: opts.PromptFile,
		logger:     opts.Logger,
	}
}

// Store returns the session store (used by channels to clear sessions, etc.).
func (a *AgentCore) Store() *SessionStore { return a.store }

// ProcessRequest is a single inbound message to process. ChatID and SessionID
// are channel-local identifiers; Text is the user message.
type ProcessRequest struct {
	SessionID  string
	ChatID     string
	Text       string
	SenderName string
	AlgaCtx    AlgaContext
	// SystemContext carries behavioral rules from the backend dispatch to be
	// injected into the LLM system prompt. Empty for non-dispatch messages.
	SystemContext string
	// Sink, if non-nil, receives streamed deltas from the final turn.
	Sink StreamSink
}

// Result is the outcome of processing a message.
type Result struct {
	Text string
	// Iterations is the number of LLM round-trips performed.
	Iterations int
	// ToolCalls is the number of tool calls executed.
	ToolCalls int
	// Latency is the total processing time.
	Latency time.Duration
}

// Process runs the conversation loop for a single inbound message. The session
// is locked for the duration so concurrent messages from the same chat are
// serialized (preventing interleaved tool executions).
func (a *AgentCore) Process(ctx context.Context, req ProcessRequest) (Result, error) {
	start := time.Now()
	session := a.store.Get(req.SessionID)
	session.Lock()
	defer session.Unlock()

	// Refresh Alga context on the session for this turn.
	if req.AlgaCtx.InvestigationID != "" || req.AlgaCtx.IncidentID != "" || len(req.AlgaCtx.AlertFingerprints) > 0 {
		session.SetAlgaCtx(req.AlgaCtx)
	}
	algaCtx := session.AlgaContext()

	// Store dispatch behavioral rules on the session so they persist across
	// turns and are injected into the system prompt on every LLM call.
	if req.SystemContext != "" {
		session.SetDispatchContext(req.SystemContext)
	}
	dispatchCtx := session.DispatchContext()

	// Build/refresh system prompt if missing or context changed.
	systemPrompt := a.buildPrompt(algaCtx, dispatchCtx)

	// Assemble the messages array: system + history + new user message.
	history := session.Messages()
	msgs := make([]llm.Message, 0, len(history)+2)

	// Ensure exactly one leading system message.
	if len(history) > 0 && history[0].Role == "system" {
		// Replace the stored system message with the fresh one (context may
		// have changed).
		msgs = append(msgs, llm.Message{Role: "system", Content: systemPrompt})
		msgs = append(msgs, history[1:]...)
	} else {
		msgs = append(msgs, llm.Message{Role: "system", Content: systemPrompt})
		msgs = append(msgs, history...)
	}
	// Append the new user message. When dispatch behavioral rules are present
	// in the system prompt, strip them from the user message to avoid
	// redundant token usage.
	userText := req.Text
	if dispatchCtx != "" {
		userText = strings.Replace(userText, "\n"+dispatchCtx, "", 1)
	}
	userMsg := llm.Message{Role: "user", Content: userText}
	msgs = append(msgs, userMsg)

	toolDefs := a.tools.Definitions()

	var result Result
	var lastAssistantText string

	for iter := 1; iter <= a.cfg.MaxIterations; iter++ {
		result.Iterations = iter

		// Detect shutdown early so we don't deliver spurious error messages to
		// users during graceful shutdown (SPEC §9).
		if err := ctx.Err(); err != nil {
			a.persistOnExit(req.SessionID, session, msgs)
			result.Latency = time.Since(start)
			return result, err
		}

		// Non-streaming call with tools enabled (SPEC §5.2.1).
		completeReq := llm.Request{
			Messages: msgs,
			Tools:    toolDefs,
		}
		resp, err := a.llm.Complete(ctx, completeReq)
		if err != nil {
			// Context cancellation during an LLM call is expected on shutdown;
			// persist progress and return without wrapping so the router treats
			// it as a clean exit, not a user-facing error.
			if ctxErr := ctx.Err(); ctxErr != nil {
				a.persistOnExit(req.SessionID, session, msgs)
				result.Latency = time.Since(start)
				return result, ctxErr
			}
			a.persistOnExit(req.SessionID, session, msgs)
			result.Latency = time.Since(start)
			return result, fmt.Errorf("llm iteration %d: %w", iter, err)
		}
		if len(resp.Choices) == 0 {
			a.persistOnExit(req.SessionID, session, msgs)
			result.Latency = time.Since(start)
			return result, errors.New("llm returned no choices")
		}
		choice := resp.Choices[0]
		assistantMsg := choice.Message

		if len(assistantMsg.ToolCalls) > 0 {
			// Append the assistant turn (with tool_calls) to the message list.
			msgs = append(msgs, assistantMsg)

			// Execute each tool call and append the result.
			for _, tc := range assistantMsg.ToolCalls {
				result.ToolCalls++
				toolResult := a.executeToolCall(ctx, tc, req)
				msgs = append(msgs, llm.Message{
					Role:       "tool",
					Content:    toolResult,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				})
			}
			// Continue the loop: the LLM will see the tool results and either
			// call more tools or produce a final text response.
			continue
		}

		// No tool calls: this is the final turn. If a stream sink is provided,
		// re-issue the final turn as a streaming request to deliver tokens
		// progressively (SPEC §5.2.1). Otherwise use the text we already have.
		if req.Sink != nil {
			streamed, err := a.streamFinal(ctx, msgs, req.Sink)
			if err != nil {
				// Fall back to the non-streamed text on streaming failure.
				a.logger.Warn("streaming final turn failed, using non-streamed text", "err", err)
				lastAssistantText = assistantMsg.Content
			} else {
				lastAssistantText = streamed
			}
		} else {
			lastAssistantText = assistantMsg.Content
		}

		// Persist the conversation: the user message and the final assistant
		// message. We replace the in-memory history with msgs + final assistant.
		finalMsgs := make([]llm.Message, 0, len(msgs)+1)
		finalMsgs = append(finalMsgs, msgs...)
		finalMsgs = append(finalMsgs, llm.Message{Role: "assistant", Content: lastAssistantText})
		session.ReplaceMessages(finalMsgs)
		a.persistToDisk(req.SessionID)

		result.Text = lastAssistantText
		result.Latency = time.Since(start)
		return result, nil
	}

	// Max iterations exceeded. Persist the full conversation (including tool
	// results) and return an error so the router can notify the user.
	a.persistOnExit(req.SessionID, session, msgs)
	result.Latency = time.Since(start)
	return result, ErrMaxIterations
}

// persistOnExit saves the conversation progress to the session on an error or
// early-exit path. It persists the accumulated messages (system + history +
// user + any assistant/tool turns) so the next turn resumes with full context
// rather than losing the in-flight tool results.
func (a *AgentCore) persistOnExit(sessionID string, session *Session, msgs []llm.Message) {
	session.ReplaceMessages(msgs)
	a.persistToDisk(sessionID)
}

// persistToDisk writes the session to disk when persistence is enabled.
// Fire-and-forget: a failed write must never fail the user's turn.
func (a *AgentCore) persistToDisk(sessionID string) {
	if err := a.store.Persist(sessionID); err != nil {
		a.logger.Warn("session persist failed", "session_id", sessionID, "err", err)
	}
}

// streamFinal issues a streaming request for the final no-tool turn. The
// messages array already contains the latest user message and history.
func (a *AgentCore) streamFinal(ctx context.Context, msgs []llm.Message, sink StreamSink) (string, error) {
	// Remove tools to encourage a pure text response on the final turn.
	streamReq := llm.Request{
		Messages: msgs,
		// Tools intentionally omitted: we want a text response.
	}
	return a.llm.Stream(ctx, streamReq, func(accumulated, delta string) bool {
		return sink.OnDelta(accumulated, delta)
	})
}

// executeToolCall runs a single tool call with the configured timeout and call
// context, returning the result string (JSON) or an error message. Errors are
// returned as text to the LLM, not as Go errors (SPEC §8.3).
func (a *AgentCore) executeToolCall(ctx context.Context, tc llm.ToolCall, req ProcessRequest) string {
	tool, ok := a.tools.Get(tc.Function.Name)
	if !ok {
		return errorResult(fmt.Sprintf("unknown tool %q", tc.Function.Name))
	}

	// Derive tool timeout from agent config. Each tool gets a child context.
	toolCtx, cancel := context.WithTimeout(ctx, a.cfg.ToolTimeout)
	defer cancel()

	// Inject the call context (chat id, session id, Alga context) for tools
	// that need to resolve IDs (SPEC §6.1).
	callCtx := tools.CallContext{
		ChatID:              req.ChatID,
		SessionID:           req.SessionID,
		AlgaInvestigationID: req.AlgaCtx.InvestigationID,
		AlgaIncidentID:      req.AlgaCtx.IncidentID,
		SenderName:          req.SenderName,
	}
	toolCtx = tools.WithCallContext(toolCtx, callCtx)

	start := time.Now()
	args := llmArgsToRaw(tc.Function.Arguments)
	result, err := tool.Execute(toolCtx, args)
	elapsed := time.Since(start)
	if err != nil {
		a.logger.Warn("tool execution failed",
			"tool", tc.Function.Name, "elapsed", elapsed.String(), "err", err)
		return errorResult(fmt.Sprintf("tool %q failed: %s", tc.Function.Name, err.Error()))
	}
	a.logger.Debug("tool executed",
		"tool", tc.Function.Name, "elapsed", elapsed.String(), "result_len", len(result))
	return result
}

func (a *AgentCore) buildPrompt(algaCtx AlgaContext, dispatchCtx string) string {
	return BuildSystemPrompt(SystemPromptOptions{
		AgentName:        a.agentCfg.Name,
		AgentDescription: a.agentCfg.Description,
		AlgaCtx:          algaCtx,
		DispatchContext:  dispatchCtx,
		// MemoryContext intentionally empty (v0.2 slot).
		ToolNames:        a.toolNames(),
		CustomPromptFile: a.promptFile,
	})
}

func (a *AgentCore) toolNames() []string {
	var names []string
	for _, t := range a.tools.List() {
		names = append(names, t.Name())
	}
	return names
}

// errorResult formats a tool error as JSON for the LLM.
func errorResult(msg string) string {
	return fmt.Sprintf(`{"error":%q}`, strings.ReplaceAll(msg, `"`, `\"`))
}

// llmArgsToRaw converts the string arguments from a tool call to a raw JSON
// message. Empty/invalid args become nil, which DecodeArgs handles.
func llmArgsToRaw(args string) []byte {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	return []byte(args)
}
