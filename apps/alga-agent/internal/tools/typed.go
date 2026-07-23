package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Result is the standard tool-result envelope returned by typed tools. The
// agent loop and the MCP server both unwrap this into the format expected by
// their respective LLM consumers. Errors are encoded in-band so the LLM can
// reason about them (e.g. retry with different args), not as Go errors.
//
// Wire shape:
//
//	{"ok": true,  "data": {...}}            // success
//	{"ok": false, "error": "fingerprint..."} // failure
type Result[T any] struct {
	OK    bool   `json:"ok"`
	Data  T      `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	// Metadata carries optional diagnostics (latency, retries, source). Not
	// parsed by the LLM; surfaced for observability.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// OK builds a successful Result wrapping data.
func OK[T any](data T) Result[T] {
	return Result[T]{OK: true, Data: data}
}

// Err builds a failed Result from err. nil err returns a generic failure.
func Err[T any](err error) Result[T] {
	if err == nil {
		err = errors.New("tool failed")
	}
	return Result[T]{OK: false, Error: err.Error()}
}

// ErrMsg[T] builds a failed Result from a raw message string.
func ErrMsg[T any](msg string) Result[T] {
	return Result[T]{OK: false, Error: msg}
}

// String serializes the Result to JSON for the legacy Tool.Execute signature.
func (r Result[T]) String() string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"error":"marshal: %s"}`, err.Error())
	}
	return string(b)
}

// TypedHandler is the function signature of a typed tool: it accepts a
// decoded input and returns a typed Result. The framework handles all JSON
// marshaling and schema generation.
type TypedHandler[I, O any] func(ctx context.Context, in I) Result[O]

// TypedTool is a Tool implementation that derives its JSON Schema from the
// input type's struct tags and wraps a TypedHandler. It is the modern,
// less-verbose alternative to BaseTool: instead of writing a 40-line literal
// per tool with a hand-rolled schema, define an input struct + a handler.
//
// Example:
//
//	type listAlertsInput struct {
//	    Status   string `json:"status,omitempty" desc:"Filter by status"`
//	    Severity string `json:"severity,omitempty" desc:"Filter by severity"`
//	    Limit    int    `json:"limit,omitempty" desc:"Maximum alerts to return"`
//	}
//	tool := NewTypedTool("alga_list_alerts", "List alerts", listAlertsHandler)
type TypedTool[I, O any] struct {
	name        string
	description string
	capability  string // required RBAC capability ("", "investigate", "command")
	category    string // grouping for the system prompt ("Alga", "System", ...)
	handler     TypedHandler[I, O]
	schema      map[string]any // cached
}

// ToolOption configures a TypedTool.
type ToolOption[I, O any] func(*TypedTool[I, O])

// WithCapability declares the RBAC capability required to invoke this tool.
// The registry filters tools by the agent's capabilities at startup.
func WithCapability[I, O any](cap string) ToolOption[I, O] {
	return func(t *TypedTool[I, O]) { t.capability = cap }
}

// WithCategory groups the tool under a heading in the system prompt
// (e.g. "Alga Platform", "System").
func WithCategory[I, O any](cat string) ToolOption[I, O] {
	return func(t *TypedTool[I, O]) { t.category = cat }
}

// NewTypedTool constructs a TypedTool from the given name, description, and
// handler. The schema is generated lazily on first call.
func NewTypedTool[I, O any](name, description string, handler TypedHandler[I, O], opts ...ToolOption[I, O]) *TypedTool[I, O] {
	t := &TypedTool[I, O]{
		name:        name,
		description: description,
		handler:     handler,
		schema:      nil, // generated lazily so bad input types fail at first call, not registration
	}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

// Name implements Tool.
func (t *TypedTool[I, O]) Name() string { return t.name }

// Description implements Tool.
func (t *TypedTool[I, O]) Description() string { return t.description }

// Capability returns the RBAC capability required to invoke this tool
// ("" = unrestricted).
func (t *TypedTool[I, O]) Capability() string { return t.capability }

// Category returns the tool's grouping for the system prompt.
func (t *TypedTool[I, O]) Category() string { return t.category }

// Schema implements Tool. The schema is generated once and cached.
func (t *TypedTool[I, O]) Schema() map[string]any {
	if t.schema == nil {
		t.schema = GenerateSchema[I]()
	}
	return t.schema
}

// Execute implements Tool. It decodes args into I, runs the handler, and
// returns the Result as JSON. Handler panics are recovered and surfaced as
// failure results so the agent loop survives a buggy tool.
func (t *TypedTool[I, O]) Execute(ctx context.Context, args json.RawMessage) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = Err[O](fmt.Errorf("panic: %v", r)).String()
			err = nil
		}
	}()
	var in I
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &in); err != nil {
			return ErrMsg[O]("invalid arguments: " + err.Error()).String(), nil
		}
	}
	res := t.handler(ctx, in)
	return res.String(), nil
}

// CapabilityProvider returns the RBAC capability required to invoke a tool,
// or "" if the tool is unrestricted. Tools that don't self-describe a
// capability are always available. The TypedTool implementation declares
// capabilities via WithCapability; legacy tools return "".
type CapabilityProvider interface {
	Capability() string
}

// CategoryProvider returns the tool's grouping for the system prompt. The
// TypedTool implementation declares a category via WithCategory; legacy tools
// return "" and are bucketed by name prefix.
type CategoryProvider interface {
	Category() string
}

// timeBounded returns a handler that enforces a per-tool timeout. Tools that
// complete before the timeout are unaffected. The timeout is taken from the
// CallContext.TimeoutHint if non-zero, falling back to defaultTimeout.
func timeBounded[I, O any](defaultTimeout time.Duration, h TypedHandler[I, O]) TypedHandler[I, O] {
	if defaultTimeout <= 0 {
		return h
	}
	return func(ctx context.Context, in I) Result[O] {
		ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
		// Honor context cancellation from the timeout as an explicit failure
		// so the LLM sees a clean error rather than an empty Result.
		res := h(ctx, in)
		if ctx.Err() != nil && !res.OK && res.Error == "" {
			return ErrMsg[O]("tool timed out after " + defaultTimeout.String())
		}
		return res
	}
}

// logged returns a handler that emits a structured log line on completion.
// Used by the LoggingMiddleware below.
func logged[I, O any](logger *slog.Logger, name string, h TypedHandler[I, O]) TypedHandler[I, O] {
	if logger == nil {
		return h
	}
	return func(ctx context.Context, in I) Result[O] {
		start := time.Now()
		res := h(ctx, in)
		elapsed := time.Since(start)
		if res.OK {
			logger.Info("tool executed",
				"tool", name, "elapsed", elapsed.String(), "ok", true)
		} else {
			logger.Warn("tool executed with error",
				"tool", name, "elapsed", elapsed.String(), "ok", false, "err", res.Error)
		}
		return res
	}
}
