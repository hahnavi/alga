package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"alga-agent/internal/config"
)

// echoTool is a test tool that echoes its arguments.
type echoTool struct {
	name string
}

func (e *echoTool) Name() string        { return e.name }
func (e *echoTool) Description() string { return "echo tool for testing" }
func (e *echoTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"msg": map[string]any{"type": "string"},
		},
	}
}
func (e *echoTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Msg string `json:"msg"`
	}
	_ = DecodeArgs(args, &in)
	if in.Msg == "fail" {
		return "", errors.New("requested failure")
	}
	return `{"echo":"` + in.Msg + `"}`, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{name: "test_echo"})
	got, ok := r.Get("test_echo")
	if !ok {
		t.Fatal("tool not found after registration")
	}
	if got.Name() != "test_echo" {
		t.Errorf("name = %q", got.Name())
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nope")
	if ok {
		t.Error("expected not found for unregistered tool")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	r.Register(&echoTool{name: "dup"})
}

func TestRegistry_ListSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{name: "zebra"})
	r.Register(&echoTool{name: "alpha"})
	r.Register(&echoTool{name: "mid"})
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	if list[0].Name() != "alpha" || list[1].Name() != "mid" || list[2].Name() != "zebra" {
		t.Errorf("not sorted: %s, %s, %s", list[0].Name(), list[1].Name(), list[2].Name())
	}
}

func TestRegistry_DefinitionsFormat(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{name: "test_def"})
	defs := r.Definitions()
	if len(defs) != 1 {
		t.Fatalf("len = %d, want 1", len(defs))
	}
	if defs[0]["type"] != "function" {
		t.Errorf("type = %v, want function", defs[0]["type"])
	}
	fn := defs[0]["function"].(map[string]any)
	if fn["name"] != "test_def" {
		t.Errorf("name = %v", fn["name"])
	}
	if fn["description"] != "echo tool for testing" {
		t.Errorf("description = %v", fn["description"])
	}
	if fn["parameters"] == nil {
		t.Error("parameters missing")
	}
}

func TestCallContextPropagation(t *testing.T) {
	ctx := context.Background()
	cc := CallContext{ChatID: "123", AlgaInvestigationID: "inv_1"}
	ctx = WithCallContext(ctx, cc)
	got, ok := CallContextFrom(ctx)
	if !ok {
		t.Fatal("CallContext not found")
	}
	if got.ChatID != "123" {
		t.Errorf("ChatID = %q", got.ChatID)
	}
	if got.AlgaInvestigationID != "inv_1" {
		t.Errorf("AlgaInvestigationID = %q", got.AlgaInvestigationID)
	}
}

func TestDecodeArgs_EmptyAndValid(t *testing.T) {
	var in struct {
		X int `json:"x"`
	}
	if err := DecodeArgs(nil, &in); err != nil {
		t.Errorf("nil args: %v", err)
	}
	if err := DecodeArgs(json.RawMessage(`{"x":42}`), &in); err != nil {
		t.Errorf("valid args: %v", err)
	}
	if in.X != 42 {
		t.Errorf("x = %d, want 42", in.X)
	}
}

func TestDecodeArgs_Invalid(t *testing.T) {
	var in struct {
		X int `json:"x"`
	}
	if err := DecodeArgs(json.RawMessage(`{bad json`), &in); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- Shell tool ---

func TestShellTool_AllowedCommand(t *testing.T) {
	cfg := config.ShellConfig{
		Enabled:         true,
		AllowedCommands: []string{"echo", "true"},
		MaxOutputBytes:  1024,
		Timeout:         5_000_000_000, // 5s
	}
	tool := NewShellTool(cfg)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Stdout != "hello\n" {
		t.Errorf("stdout = %q, want %q", out.Stdout, "hello\n")
	}
	if out.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", out.ExitCode)
	}
}

func TestShellTool_DisallowedCommand(t *testing.T) {
	cfg := config.ShellConfig{
		Enabled:         true,
		AllowedCommands: []string{"echo"},
		MaxOutputBytes:  1024,
		Timeout:         5_000_000_000,
	}
	tool := NewShellTool(cfg)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err == nil {
		t.Error("expected error for disallowed command")
	}
}

func TestShellTool_Timeout(t *testing.T) {
	cfg := config.ShellConfig{
		Enabled:         true,
		AllowedCommands: []string{"sleep"},
		MaxOutputBytes:  1024,
		Timeout:         1_000_000_000, // 1s
	}
	tool := NewShellTool(cfg)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"sleep 10"}`))
	if err != nil {
		t.Fatalf("execute returned go error (expected JSON result with timed_out): %v", err)
	}
	var out struct {
		ExitCode int  `json:"exit_code"`
		TimedOut bool `json:"timed_out"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.TimedOut {
		t.Error("expected timed_out=true")
	}
}

func TestShellTool_DisabledReturnsNil(t *testing.T) {
	tool := NewShellTool(config.ShellConfig{Enabled: false})
	if tool != nil {
		t.Error("expected nil for disabled shell")
	}
}

func TestShellTool_TruncatesOutput(t *testing.T) {
	cfg := config.ShellConfig{
		Enabled:         true,
		AllowedCommands: []string{"echo"},
		MaxOutputBytes:  10,
		Timeout:         5_000_000_000,
	}
	tool := NewShellTool(cfg)
	// echo with a long string
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo abcdefghijklmnopqrstuvwxyz"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out struct {
		Stdout string `json:"stdout"`
	}
	_ = json.Unmarshal([]byte(result), &out)
	if !contains(out.Stdout, "truncated") {
		t.Errorf("expected truncated marker in stdout, got %q", out.Stdout)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
