package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"alga-agent/internal/tools"
)

// RemoteServerConfig describes a single external MCP server the agent should
// consume tools from. Exactly one of Command (stdio transport) or URL
// (Streamable HTTP transport) must be set.
type RemoteServerConfig struct {
	// Name is used as a prefix for the imported tools to avoid collisions
	// (e.g. "github" → "github_list_repos"). Must be lowercase alphanumeric
	// + underscore.
	Name string `yaml:"name" json:"name"`
	// Command + Args launch a subprocess that speaks MCP over stdin/stdout.
	// Use this for stdio-based MCP servers (e.g. the official filesystem,
	// sqlite, git servers).
	Command string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
	// Env is passed to the subprocess.
	Env []string `yaml:"env,omitempty" json:"env,omitempty"`
	// URL connects to a Streamable HTTP MCP server. Mutually exclusive with
	// Command.
	URL string `yaml:"url,omitempty" json:"url,omitempty"`
	// ToolPrefix overrides the default "<name>_" prefix for imported tools.
	ToolPrefix string `yaml:"tool_prefix,omitempty" json:"tool_prefix,omitempty"`
	// InitTimeout bounds the initial ListTools round-trip. Default 10s.
	InitTimeout time.Duration `yaml:"init_timeout,omitempty" json:"init_timeout,omitempty"`
	// Disabled, when true, skips this server at startup.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// Client connects to one or more external MCP servers and imports their
// tools into the agent's tool registry. Each imported tool is exposed under
// a namespaced name (e.g. "github_list_repos") to avoid collisions with
// Alga tools.
//
// The client maintains long-lived connections to each server and tracks
// per-server lifecycle so a single misbehaving server does not bring down
// the agent. On Disconnect, all subprocess / HTTP sessions are closed.
type Client struct {
	logger  *slog.Logger
	mu      sync.Mutex
	servers map[string]*managedServer
}

// managedServer tracks one external MCP connection.
type managedServer struct {
	cfg     RemoteServerConfig
	session *mcp.ClientSession
	cancel  context.CancelFunc
	done    chan struct{}
	tools   []managedTool // tools registered on behalf of this server
}

// managedTool captures the original MCP tool name so we can route calls.
type managedTool struct {
	originalName string
	agentName    string
}

// NewClient constructs an MCP client. Pass WithClientLogger to inject a
// structured logger.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		logger:  slog.Default(),
		servers: make(map[string]*managedServer),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// ClientOption configures the MCP client.
type ClientOption func(*Client)

// WithClientLogger sets the structured logger used for connection lifecycle
// events.
func WithClientLogger(l *slog.Logger) ClientOption {
	return func(c *Client) {
		if l != nil {
			c.logger = l
		}
	}
}

// Connect dials each configured server, lists its tools, and registers
// each as an agent tool under a namespaced name. Returns the count of tools
// imported across all servers. Servers that fail to connect are logged and
// skipped; the agent keeps the successfully connected ones.
func (c *Client) Connect(ctx context.Context, registry *tools.Registry, configs []RemoteServerConfig) (int, error) {
	var imported int
	var firstErr error
	for _, cfg := range configs {
		if cfg.Disabled {
			c.logger.Info("mcp server disabled, skipping", "name", cfg.Name)
			continue
		}
		if err := c.connectOne(ctx, registry, cfg); err != nil {
			c.logger.Error("mcp server connect failed, skipping",
				"name", cfg.Name, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		imported += len(c.servers[cfg.Name].tools)
	}
	return imported, firstErr
}

func (c *Client) connectOne(parentCtx context.Context, registry *tools.Registry, cfg RemoteServerConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("mcp server config: name is required")
	}
	if cfg.Command == "" && cfg.URL == "" {
		return fmt.Errorf("mcp server %q: command or url is required", cfg.Name)
	}
	if cfg.Command != "" && cfg.URL != "" {
		return fmt.Errorf("mcp server %q: command and url are mutually exclusive", cfg.Name)
	}
	prefix := cfg.ToolPrefix
	if prefix == "" {
		// Default: <name>_. Replace non-alphanumeric so tool names stay
		// valid across MCP and OpenAI's tool-name regex.
		prefix = sanitizeName(cfg.Name) + "_"
	}
	timeout := cfg.InitTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	impl := &mcp.Implementation{Name: "alga-agent-mcp-client", Version: "1.0.0"}
	client := mcp.NewClient(impl, nil)

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	var transport mcp.Transport
	switch {
	case cfg.Command != "":
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		if len(cfg.Env) > 0 {
			cmd.Env = append(cmd.Environ(), cfg.Env...)
		}
		transport = &mcp.CommandTransport{Command: cmd}
	case cfg.URL != "":
		transport = &mcp.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: &http.Client{Timeout: 60 * time.Second},
		}
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	listCtx, listCancel := context.WithTimeout(parentCtx, timeout)
	defer listCancel()
	listResult, err := session.ListTools(listCtx, nil)
	if err != nil {
		_ = session.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	ms := &managedServer{
		cfg:     cfg,
		session: session,
		done:    make(chan struct{}),
	}

	// Keep the session alive in the background. session.Close() is called on
	// Disconnect; this goroutine just waits.
	ms.cancel = func() {
		_ = session.Close()
		close(ms.done)
	}

	for _, t := range listResult.Tools {
		original := t.Name
		agentName := prefix + sanitizeName(original)
		imported := &importedTool{
			original:   original,
			session:    session,
			serverName: cfg.Name,
		}
		registry.Register(imported.asTool(agentName, t.Description, t.InputSchema))
		ms.tools = append(ms.tools, managedTool{
			originalName: original,
			agentName:    agentName,
		})
	}

	c.mu.Lock()
	c.servers[cfg.Name] = ms
	c.mu.Unlock()
	c.logger.Info("mcp server connected",
		"name", cfg.Name, "tools", len(ms.tools), "transport", transportKind(cfg))
	return nil
}

// Disconnect closes every external MCP session. Safe to call multiple times.
func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, ms := range c.servers {
		if ms.cancel != nil {
			ms.cancel()
		}
		c.logger.Info("mcp server disconnected", "name", name)
		delete(c.servers, name)
	}
}

// Servers returns the names of currently connected servers. Primarily for
// status / metrics.
func (c *Client) Servers() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.servers))
	for name := range c.servers {
		out = append(out, name)
	}
	return out
}

// --- imported tool adapter ---

// importedTool wraps an external MCP tool as an agent tools.Tool. It records
// the original MCP name and the session to call it on; the per-instance
// Execute call marshals args, invokes session.CallTool, and unwraps the
// result text.
type importedTool struct {
	original   string
	session    *mcp.ClientSession
	serverName string

	// cached metadata populated at construction time
	name        string
	description string
	schema      map[string]any
}

func (i *importedTool) asTool(name, description string, inputSchema any) tools.Tool {
	i.name = name
	i.description = description
	if i.description == "" {
		i.description = "MCP tool " + i.original + " (from " + i.serverName + ")"
	}
	i.schema = normalizeSchema(inputSchema)
	return i
}

func (i *importedTool) Name() string           { return i.name }
func (i *importedTool) Description() string    { return i.description }
func (i *importedTool) Schema() map[string]any { return i.schema }

func (i *importedTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	// MCP expects Arguments as a JSON object; pass through whatever the LLM
	// produced. Empty args become an empty object so the SDK doesn't reject
	// the call.
	var decoded any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &decoded); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	res, err := i.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      i.original,
		Arguments: decoded,
	})
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", i.original, err)
	}
	return unwrapMCPResult(res), nil
}

// unwrapMCPResult flattens an MCP CallToolResult into a JSON string for the
// agent loop. Text content is concatenated; structured content is preserved
// verbatim. IsError is surfaced via the result envelope so the LLM can
// self-correct.
func unwrapMCPResult(res *mcp.CallToolResult) string {
	if res == nil {
		return `{"ok":false,"error":"empty mcp result"}`
	}
	// Prefer structured content if the server provided it.
	if res.StructuredContent != nil {
		if b, err := json.Marshal(res.StructuredContent); err == nil {
			if res.IsError {
				return wrapEnvelope(false, "", string(b))
			}
			return wrapEnvelope(true, string(b), "")
		}
	}
	// Otherwise concatenate text content.
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc != nil {
			sb.WriteString(tc.Text)
		}
	}
	text := sb.String()
	if res.IsError {
		return wrapEnvelope(false, "", text)
	}
	return wrapEnvelope(true, text, "")
}

func wrapEnvelope(ok bool, data, errText string) string {
	if ok {
		return fmt.Sprintf(`{"ok":true,"data":%s}`, mustQuoteOrRaw(data))
	}
	return fmt.Sprintf(`{"ok":false,"error":%s}`, mustQuoteOrRaw(errText))
}

// mustQuoteOrRaw returns the input as a JSON string literal if it doesn't
// already parse as JSON, otherwise returns it verbatim. This lets us embed
// either a raw JSON payload or a plain-text error message uniformly.
func mustQuoteOrRaw(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	var any any
	if err := json.Unmarshal([]byte(s), &any); err == nil {
		return s
	}
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// normalizeSchema converts the MCP tool's inputSchema (which can be
// json.RawMessage, a struct, or a map) to the map[string]any shape the
// agent registry expects.
func normalizeSchema(s any) map[string]any {
	if s == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	switch v := s.(type) {
	case map[string]any:
		if _, ok := v["type"]; !ok {
			v["type"] = "object"
		}
		return v
	case json.RawMessage:
		var m map[string]any
		if err := json.Unmarshal(v, &m); err == nil {
			if _, ok := m["type"]; !ok {
				m["type"] = "object"
			}
			return m
		}
	}
	// Fall back: marshal + unmarshal as a generic object.
	b, err := json.Marshal(s)
	if err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return m
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else if r == '-' || r == ' ' || r == '.' {
			b.WriteRune('_')
		}
	}
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}

func transportKind(cfg RemoteServerConfig) string {
	if cfg.Command != "" {
		return "stdio"
	}
	return "http"
}
