package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"alga-agent/internal/tools"
)

// TestServerExposesRegistryTools spins up an in-process MCP server backed by
// a tiny registry, connects a real MCP client to it over Streamable HTTP,
// and verifies the tool round-trips end to end. This is the integration test
// that proves the MCP server wiring is correct.
func TestServerExposesRegistryTools(t *testing.T) {
	reg := NewTestRegistry(t)
	mcpSrv := NewServer(reg, WithServerImplementation(&mcp.Implementation{
		Name:    "test-alga-mcp",
		Version: "v1.0.0-test",
	}))

	// Bind the MCP server to a random httptest port instead of starting it
	// directly, so we can drive it with a real MCP client.
	streamHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpSrv.buildMCPServer()
	}, nil)
	httpSrv := httptest.NewServer(streamHandler)
	defer httpSrv.Close()

	// Connect a real MCP client.
	impl := &mcp.Implementation{Name: "test-client", Version: "v1.0.0"}
	client := mcp.NewClient(impl, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             httpSrv.URL + "/mcp",
		HTTPClient:           &http.Client{Timeout: 10 * time.Second},
		DisableStandaloneSSE: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	wantNames := map[string]bool{
		"alga_list_alerts":   true,
		"alga_resolve_alert": true,
		"echo_simple":        true,
	}
	gotNames := make(map[string]bool)
	for _, tool := range list.Tools {
		gotNames[tool.Name] = true
	}
	for want := range wantNames {
		if !gotNames[want] {
			t.Errorf("tool %q not advertised by MCP server (got %v)", want, keys(gotNames))
		}
	}

	// Call a tool through MCP and verify the result.
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "echo_simple",
		Arguments: map[string]any{"msg": "hello"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("no content in result")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	if !strings.Contains(text.Text, `"ok":true`) || !strings.Contains(text.Text, "hello") {
		t.Errorf("unexpected result text: %s", text.Text)
	}
}

// TestServerStartAndShutdown exercises the HTTP server start/shutdown cycle.
func TestServerStartAndShutdown(t *testing.T) {
	reg := NewTestRegistry(t)
	mcpSrv := NewServer(reg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mcpSrv.Start(ctx, "127.0.0.1:0", "/mcp")
	}()

	// Give it a moment to bind, then shut down. The address is chosen by the
	// listener; we can't easily probe it here without extra plumbing, but
	// Shutdown should cleanly stop whatever's running.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return within 2s of cancel")
	}
}

// TestClientImportsExternalServer starts an MCP server with one tool, then
// uses our Client to import that tool into a fresh registry. Verifies the
// imported tool is callable through the agent's Tool interface.
func TestClientImportsExternalServer(t *testing.T) {
	externalSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "external-fs",
		Version: "v1.0.0",
	}, nil)
	mcp.AddTool(externalSrv,
		&mcp.Tool{Name: "read_file", Description: "read a file by path"},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Path string `json:"path"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "contents of " + in.Path}},
			}, nil, nil
		},
	)
	streamHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return externalSrv
	}, nil)
	httpSrv := httptest.NewServer(streamHandler)
	defer httpSrv.Close()

	// Now connect our client.
	reg := tools.NewRegistry()
	client := NewClient(WithClientLogger(testLogger()))
	_, err := client.Connect(context.Background(), reg, []RemoteServerConfig{{
		Name:        "fs",
		URL:         httpSrv.URL + "/mcp",
		InitTimeout: 5 * time.Second,
	}})
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer client.Disconnect()

	// Verify the imported tool is registered.
	imported, ok := reg.Get("fs_read_file")
	if !ok {
		t.Fatalf("expected fs_read_file in registry; tools = %+v", toolNames(reg))
	}

	// Call it via the agent Tool interface.
	out, err := imported.Execute(context.Background(), json.RawMessage(`{"path":"/etc/hosts"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "contents of /etc/hosts") {
		t.Errorf("unexpected output: %s", out)
	}
}

// TestClientStdioTransport exercises the subprocess transport by building a
// tiny MCP echo server on the fly and connecting our client to it via stdio.
//
// This test needs network access to fetch github.com/modelcontextprotocol/go-sdk
// for the helper binary, so it's skipped in short mode or when the build
// cannot complete (offline CI, air-gapped dev environments). The HTTP
// transport variant (TestClientImportsExternalServer) covers the same client
// code path without those dependencies.
func TestClientStdioTransport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	if os.Getenv("ALGA_TEST_MCP_STDIO") != "1" {
		t.Skip("skipping stdio MCP test; set ALGA_TEST_MCP_STDIO=1 to enable")
	}
	if _, err := execLookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	src := `package main
import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)
type in struct {
	Msg string
}
func main() {
	s := mcp.NewServer(&mcp.Implementation{Name:"stdio-echo", Version:"v1"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name:"echo"}, func(ctx context.Context, _ *mcp.CallToolRequest, in in) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: in.Msg}}}, nil, nil
	})
	s.Run(context.Background(), &mcp.StdioTransport{})
}`
	dir := t.TempDir()
	if err := writeFile(dir+"/main.go", src); err != nil {
		t.Fatal(err)
	}
	if err := runGoModInit(dir); err != nil {
		t.Fatalf("go mod init: %v", err)
	}
	if err := runGoModTidy(dir); err != nil {
		t.Skipf("could not tidy helper module (likely offline): %v", err)
	}
	binPath := dir + "/echo"
	if err := runGoBuild(dir, binPath); err != nil {
		t.Skipf("could not build stdio helper: %v", err)
	}

	reg := tools.NewRegistry()
	client := NewClient(WithClientLogger(testLogger()))
	_, err := client.Connect(context.Background(), reg, []RemoteServerConfig{{
		Name:        "echo",
		Command:     binPath,
		InitTimeout: 15 * time.Second,
	}})
	if err != nil {
		t.Fatalf("stdio connect: %v", err)
	}
	defer client.Disconnect()

	imported, ok := reg.Get("echo_echo")
	if !ok {
		t.Fatalf("expected echo_echo in registry; tools = %+v", toolNames(reg))
	}
	out, err := imported.Execute(context.Background(), json.RawMessage(`{"Msg":"stdio-hello"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "stdio-hello") {
		t.Errorf("unexpected output: %s", out)
	}
}

// TestNormalizeSchema covers the schema normalization fallbacks.
func TestNormalizeSchema(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		s := normalizeSchema(nil)
		if s["type"] != "object" {
			t.Errorf("nil schema should default to object")
		}
	})
	t.Run("map", func(t *testing.T) {
		s := normalizeSchema(map[string]any{"properties": map[string]any{}})
		if s["type"] != "object" {
			t.Errorf("map schema should default type to object")
		}
	})
	t.Run("raw message", func(t *testing.T) {
		s := normalizeSchema(json.RawMessage(`{"properties":{"x":{"type":"string"}}}`))
		if s["type"] != "object" {
			t.Errorf("raw schema should default type to object")
		}
		props, _ := s["properties"].(map[string]any)
		if props == nil {
			t.Error("missing properties")
		}
	})
}

// TestSanitizeName covers name sanitization.
func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"GitHub Tools":  "github_tools",
		"db.query":      "db_query",
		"already_clean": "already_clean",
		"---weird---":   "weird",
		"UPPER":         "upper",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestImportedToolError verifies that an MCP server-side error is surfaced
// in the result envelope (not as a Go error).
func TestImportedToolError(t *testing.T) {
	externalSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "err-srv",
		Version: "v1.0.0",
	}, nil)
	mcp.AddTool(externalSrv,
		&mcp.Tool{Name: "fail"},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Msg string `json:"msg"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "boom: " + in.Msg}},
			}, nil, nil
		},
	)
	streamHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return externalSrv
	}, nil)
	httpSrv := httptest.NewServer(streamHandler)
	defer httpSrv.Close()

	reg := tools.NewRegistry()
	client := NewClient(WithClientLogger(testLogger()))
	_, err := client.Connect(context.Background(), reg, []RemoteServerConfig{{
		Name: "err",
		URL:  httpSrv.URL + "/mcp",
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect()

	imported, _ := reg.Get("err_fail")
	out, _ := imported.Execute(context.Background(), json.RawMessage(`{"msg":"x"}`))
	if !strings.Contains(out, `"ok":false`) || !strings.Contains(out, "boom") {
		t.Errorf("expected failure envelope with boom, got %s", out)
	}
}

// --- helpers ---

// NewTestRegistry returns a Registry prepopulated with a couple of fake Alga
// tools so the MCP server has something to expose.
func NewTestRegistry(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	alerts := []struct {
		name string
	}{
		{"alga_list_alerts"},
		{"alga_resolve_alert"},
	}
	for _, a := range alerts {
		a := a
		reg.Register(tools.NewTypedTool(a.name, "test tool "+a.name,
			func(ctx context.Context, _ struct{}) tools.Result[struct {
				Ok bool `json:"ok"`
			}] {
				return tools.OK(struct {
					Ok bool `json:"ok"`
				}{Ok: true})
			}))
	}
	reg.Register(tools.NewTypedTool("echo_simple", "echoes a message",
		func(ctx context.Context, in struct {
			Msg string `json:"msg" desc:"the message"`
		}) tools.Result[struct {
			Echo string `json:"echo"`
		}] {
			return tools.OK(struct {
				Echo string `json:"echo"`
			}{Echo: in.Msg})
		}))
	return reg
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func toolNames(r *tools.Registry) []string {
	out := []string{}
	for _, t := range r.List() {
		out = append(out, t.Name())
	}
	return out
}

// testLogger returns a logger that discards everything; tests don't need the
// noise unless they're debugging.
func testLogger() *slog.Logger { return slog.New(discardHandler{}) }

type discardHandler struct{}

func (discardHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }
func (discardHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (discardHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return discardHandler{} }
func (discardHandler) WithGroup(_ string) slog.Handler               { return discardHandler{} }

// writeFile, runGoModInit, runGoBuild, execLookPath wrap os/exec so we can
// stub them in tests if needed.
func writeFile(path, content string) error {
	return writeFileImpl(path, content)
}

func runGoModInit(dir string) error { return runGoCmd(dir, "mod", "init", "tempecho") }
func runGoModTidy(dir string) error { return runGoCmd(dir, "mod", "tidy") }
func runGoBuild(dir, out string) error {
	return runGoCmd(dir, "build", "-o", out, ".")
}
