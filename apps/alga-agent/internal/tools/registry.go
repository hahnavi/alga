// Package tools defines the tool registry and the Tool interface implemented by
// every agent tool (Alga platform tools, shell, web search, etc.). Tools
// self-register via init() so new tools only need a new file in the package.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Tool is the interface every agent tool implements. The typed-tool
// framework (TypedTool) satisfies this interface for any handler signature
// `func(ctx, I) Result[O]`; legacy tools can keep using BaseTool.
type Tool interface {
	// Name is the tool identifier passed to the LLM (e.g. "alga_list_alerts").
	Name() string
	// Description is the human/LLM-readable summary of what the tool does.
	Description() string
	// Schema returns the JSON Schema (OpenAI function parameters format)
	// describing the tool's input arguments.
	Schema() map[string]any
	// Execute runs the tool with the JSON arguments from the LLM and returns
	// the result string (typically JSON) or an error. Errors are returned to
	// the LLM as tool-result text, per SPEC §8.3.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// CallContext carries per-turn shared state into tool executions. It is the
// mechanism by which channels inject Alga investigation context (investigation
// id, incident id, alerts) so that tools can resolve IDs without inference,
// per SPEC §6.1.
type CallContext struct {
	// ChatID is the channel-local chat identifier (e.g. telegram chat id or
	// Alga investigation chat id).
	ChatID string
	// SessionID is the fully-qualified session id ("telegram:<id>" | "alga:<id>").
	SessionID string
	// AlgaInvestigationID is the investigation id when responding inside an
	// Alga thread; empty for Telegram.
	AlgaInvestigationID string
	// AlgaIncidentID is the incident id (if any) associated with the current
	// investigation context.
	AlgaIncidentID string
	// SenderName is the human-readable name of the message sender.
	SenderName string
	// TimeoutHint, when non-zero, overrides the agent's default per-tool
	// timeout for this call.
	TimeoutHint int64
}

// callCtxKey is the context key for CallContext.
type callCtxKey struct{}

// WithCallContext returns ctx with cc attached, for retrieval by tools.
func WithCallContext(ctx context.Context, cc CallContext) context.Context {
	return context.WithValue(ctx, callCtxKey{}, cc)
}

// CallContextFrom extracts the CallContext from ctx, if present.
func CallContextFrom(ctx context.Context) (CallContext, bool) {
	cc, ok := ctx.Value(callCtxKey{}).(CallContext)
	return cc, ok
}

// Registry is a thread-safe collection of tools keyed by name. It supports
// both legacy Tool implementations and TypedTool wrappers, plus capability
// filtering for agents whose token grants a subset of capabilities.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Panics on duplicate names so that
// misconfiguration fails loudly at startup.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name()]; exists {
		panic(fmt.Sprintf("tools: duplicate registration for %q", t.Name()))
	}
	r.tools[t.Name()] = t
}

// Get returns the named tool and whether it was found.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools, sorted by name for deterministic ordering.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ListForCapabilities returns the subset of tools whose required capability
// is in the agent's capability set. Tools that don't self-describe a
// capability (the legacy default) are always included. capabilities may be
// empty to include all tools (treat as unrestricted).
func (r *Registry) ListForCapabilities(capabilities []string) []Tool {
	all := r.List()
	if len(capabilities) == 0 {
		return all
	}
	allowed := make(map[string]struct{}, len(capabilities))
	for _, c := range capabilities {
		allowed[c] = struct{}{}
	}
	out := all[:0]
	for _, t := range all {
		var required string
		if cp, ok := t.(CapabilityProvider); ok {
			required = cp.Capability()
		}
		if required == "" {
			out = append(out, t)
			continue
		}
		if _, ok := allowed[required]; ok {
			out = append(out, t)
		}
	}
	return out
}

// Definitions returns the OpenAI "tools" payload (array of function
// definitions) for all registered tools, sorted by name.
func (r *Registry) Definitions() []map[string]any {
	tools := r.List()
	return definitionsFor(tools)
}

// DefinitionsFor is like Definitions but restricted to a capability set.
// Useful for exposing a per-agent view of available tools.
func (r *Registry) DefinitionsFor(capabilities []string) []map[string]any {
	return definitionsFor(r.ListForCapabilities(capabilities))
}

func definitionsFor(tools []Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Schema(),
			},
		})
	}
	return out
}

// DecodeArgs unmarshals args into out, returning a helpful error string.
func DecodeArgs(args json.RawMessage, out any) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	if err := json.Unmarshal(args, out); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

// JSONString serializes v to a compact JSON string, panicking only on truly
// unexpected encoding failures (used for tool results that must be JSON).
func JSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":"marshal: %s"}`, err.Error())
	}
	return string(b)
}

// BaseTool is a convenience struct that supplies static Name/Description/Schema
// values and delegates Execute to a function field. Embed it to avoid boilerplate.
//
// Deprecated: prefer TypedTool, which auto-generates the schema from struct
// tags. BaseTool is retained for the existing shell / web-search tools and
// for any tool that needs hand-tuned JSON Schema.
type BaseTool struct {
	NameField        string
	DescriptionField string
	SchemaField      map[string]any
	Run              func(ctx context.Context, args json.RawMessage) (string, error)
}

func (b *BaseTool) Name() string           { return b.NameField }
func (b *BaseTool) Description() string    { return b.DescriptionField }
func (b *BaseTool) Schema() map[string]any { return b.SchemaField }
func (b *BaseTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return b.Run(ctx, args)
}

// noArgsSchema is the JSON Schema for tools that take no arguments.
func noArgsSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
