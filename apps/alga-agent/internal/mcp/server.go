// Package mcp exposes the agent's tool registry over the Model Context
// Protocol (https://modelcontextprotocol.io), letting external MCP-compatible
// clients (Claude Desktop, Cursor, custom MCP clients) drive the same Alga
// tools that the in-process agent uses.
//
// Two roles are supported:
//
//   - Server: registers every tool in a *tools.Registry as an MCP tool and
//     serves them over Streamable HTTP. External clients connect, list tools,
//     and call them.
//   - Client: connects to external MCP servers (e.g. a filesystem browser,
//     a database inspector, a GitHub client) and surfaces their tools as
//     agent tools, registered transparently into the agent's *tools.Registry.
//
// The dual role makes the agent both a producer and consumer of MCP tools,
// matching the protocol's intent: tools are reusable, framework-agnostic
// capabilities, not bespoke per-agent code.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"alga-agent/internal/tools"
)

// Implementation is re-exported from the upstream SDK so callers (main.go,
// setup wizard) don't need to import github.com/modelcontextprotocol/go-sdk
// directly. The fields are intentionally the same.
type Implementation = mcp.Implementation

// Server exposes a *tools.Registry as an MCP server over Streamable HTTP.
//
// Each agent tool is wrapped as an MCP tool whose InputSchema is the agent
// tool's JSON Schema (which is already OpenAI/JSON-Schema compatible). The
// MCP tool handler delegates to the agent tool's Execute method and wraps
// the JSON result string as an MCP TextContent.
//
// Tools that self-describe a capability (CapabilityProvider) carry an
// annotation so MCP clients can surface which require elevated privileges.
// The server does NOT enforce capability checks — MCP clients are
// untrusted-but-authenticated (mTLS or token-gated at the network edge), so
// the assumption is that whatever tools are registered are fair game for
// every connected MCP client.
type Server struct {
	registry      *tools.Registry
	impl          *mcp.Implementation
	logger        *slog.Logger
	httpServer    *http.Server
	streamHandler *mcp.StreamableHTTPHandler
	mu            sync.Mutex
	stopped       sync.Once
}

// ServerOption configures the MCP server.
type ServerOption func(*Server)

// WithServerLogger injects a structured logger.
func WithServerLogger(l *slog.Logger) ServerOption {
	return func(s *Server) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithServerImplementation overrides the default MCP Implementation info
// (name + version) advertised to clients.
func WithServerImplementation(impl *mcp.Implementation) ServerOption {
	return func(s *Server) {
		if impl != nil {
			s.impl = impl
		}
	}
}

// NewServer constructs an MCP server backed by the given tool registry. The
// server is not started; call Start to bind the HTTP listener.
func NewServer(reg *tools.Registry, opts ...ServerOption) *Server {
	s := &Server{
		registry: reg,
		logger:   slog.Default(),
		impl: &mcp.Implementation{
			Name:    "alga-agent",
			Version: "1.0.0",
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// buildMCPServer constructs a fresh *mcp.Server with every registry tool
// registered. We rebuild per request so session-scoped state in the SDK
// doesn't leak across MCP clients. Schema generation is cached on each tool.
func (s *Server) buildMCPServer() *mcp.Server {
	srv := mcp.NewServer(s.impl, &mcp.ServerOptions{
		Logger: s.logger.With("component", "mcp_server"),
	})
	for _, t := range s.registry.List() {
		s.registerOne(srv, t)
	}
	return srv
}

func (s *Server) registerOne(srv *mcp.Server, t tools.Tool) {
	name := t.Name()
	tool := &mcp.Tool{
		Name:        name,
		Description: t.Description(),
		InputSchema: json.RawMessage(mustMarshal(t.Schema())),
		Annotations: annotationsFor(t),
	}
	handler := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := json.Marshal(req.Params.Arguments)
		// Delegate to the agent tool. The agent tool already returns JSON
		// wrapped in the Result envelope, so we surface it verbatim as
		// TextContent. MCP clients that look for a typed StructuredContent
		// will fall back to the text representation, which is JSON-parseable.
		out, err := t.Execute(ctx, args)
		if err != nil {
			// Agent tool errors are in-band (the envelope carries them); the
			// few Go errors that escape (context cancellation, panics in
			// pre-validation) become MCP-level errors.
			return nil, fmt.Errorf("tool %s: %w", name, err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: out},
			},
		}, nil
	}
	srv.AddTool(tool, handler)
}

// annotationsFor returns MCP annotations describing the tool's intent (read
// vs. write) based on its category. Tools in the "Alga Platform" category
// that mutate state are flagged ReadOnlyHint=false; the heuristic is per-tool
// via the tools.CapabilityProvider interface (command/communicate/investigate
// capabilities all imply writes; pure reads like list_services get true).
func annotationsFor(t tools.Tool) *mcp.ToolAnnotations {
	readOnly := true
	if cp, ok := t.(tools.CapabilityProvider); ok {
		switch cp.Capability() {
		case "command", "communicate", "investigate":
			readOnly = false
		}
	}
	return &mcp.ToolAnnotations{
		Title:        t.Name(),
		ReadOnlyHint: readOnly,
	}
}

// Start binds the HTTP listener and serves MCP clients over Streamable HTTP
// at the given address (e.g. ":8085"). Blocks until Shutdown is called or
// the context is canceled. The path is the Streamable HTTP endpoint, by
// default "/mcp".
func (s *Server) Start(ctx context.Context, addr string, path string) error {
	if path == "" {
		path = "/mcp"
	}
	s.mu.Lock()
	if s.streamHandler != nil {
		s.mu.Unlock()
		return errors.New("mcp server already started")
	}
	s.streamHandler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.buildMCPServer()
	}, nil)
	mux := http.NewServeMux()
	mux.Handle(path, s.streamHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("mcp server listening",
			"addr", addr, "path", path, "tool_count", len(s.registry.List()))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the HTTP server. The Streamable HTTP handler
// does not expose a Close method in v1.6.x; closing the underlying HTTP
// server drops in-flight sessions, which is acceptable for a clean shutdown.
func (s *Server) Shutdown(ctx context.Context) error {
	var firstErr error
	s.stopped.Do(func() {
		s.mu.Lock()
		httpSrv := s.httpServer
		s.mu.Unlock()

		if httpSrv != nil {
			if err := httpSrv.Shutdown(ctx); err != nil {
				firstErr = fmt.Errorf("mcp http shutdown: %w", err)
			}
		}
	})
	return firstErr
}

// mustMarshal panics on a marshaling failure. Used only for already-validated
// schemas produced by GenerateSchema; a panic here indicates a programming
// bug rather than a runtime condition.
func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mcp: failed to marshal schema: %v", err))
	}
	return b
}
