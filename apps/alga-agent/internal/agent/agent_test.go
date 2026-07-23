package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"alga-agent/internal/config"
	"alga-agent/internal/llm"
	"alga-agent/internal/tools"
)

// recordingSink captures streamed deltas for verification.
type recordingSink struct {
	deltas []string
	final  string
}

func (r *recordingSink) OnDelta(accumulated, delta string) bool {
	r.deltas = append(r.deltas, delta)
	return true
}

// --- Session & prompt unit tests (no LLM server needed) ---

func TestSessionStore_GetCreatesAndReuses(t *testing.T) {
	store := NewSessionStore(10)
	s1 := store.Get("telegram:1")
	s2 := store.Get("telegram:1")
	if s1 != s2 {
		t.Error("Get should return the same session instance")
	}
	if !store.Has("telegram:1") {
		t.Error("Has should report true")
	}
}

func TestSessionStore_Clear(t *testing.T) {
	store := NewSessionStore(10)
	store.Get("telegram:1")
	store.Clear("telegram:1")
	if store.Has("telegram:1") {
		t.Error("session should be cleared")
	}
}

func TestSession_AppendAndTrim(t *testing.T) {
	s := &Session{maxTurns: 4}
	s.AppendMessage(llm.Message{Role: "system", Content: "sys"})
	s.AppendMessage(llm.Message{Role: "user", Content: "u1"})
	s.AppendMessage(llm.Message{Role: "assistant", Content: "a1"})
	s.AppendMessage(llm.Message{Role: "user", Content: "u2"})
	s.AppendMessage(llm.Message{Role: "assistant", Content: "a2"})
	// Now we have 5 messages, maxTurns=4. System is preserved; oldest non-system dropped.
	msgs := s.Messages()
	if len(msgs) > 5 {
		t.Errorf("trim failed: len = %d", len(msgs))
	}
	// System must always be first.
	if msgs[0].Role != "system" {
		t.Errorf("first message role = %q, want system", msgs[0].Role)
	}
}

func TestSession_TrimPreservesToolCallGroups(t *testing.T) {
	// Regression: trim() must never orphan tool result messages from their
	// preceding assistant tool_call. If an assistant+tool group is dropped,
	// all its tool messages must be dropped together. Otherwise the LLM API
	// rejects the malformed history.
	s := &Session{maxTurns: 5}
	s.AppendMessage(llm.Message{Role: "system", Content: "sys"})
	s.AppendMessage(llm.Message{Role: "user", Content: "u1"})
	// assistant with tool_calls + two tool results (a complete group).
	s.AppendMessage(llm.Message{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.ToolFunction{Name: "x"}}},
	})
	s.AppendMessage(llm.Message{Role: "tool", ToolCallID: "c1", Content: "r1"})
	s.AppendMessage(llm.Message{Role: "tool", ToolCallID: "c1", Content: "r2"})
	// Add more messages to force trimming past maxTurns=5.
	s.AppendMessage(llm.Message{Role: "user", Content: "u2"})
	s.AppendMessage(llm.Message{Role: "assistant", Content: "a2"})
	s.AppendMessage(llm.Message{Role: "user", Content: "u3"})

	msgs := s.Messages()
	// Validate: no tool message may lack a preceding assistant with matching tool_call.
	for i, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		// Find the assistant that issued this tool call earlier in the history.
		found := false
		for j := 0; j < i; j++ {
			if msgs[j].Role != "assistant" {
				continue
			}
			for _, tc := range msgs[j].ToolCalls {
				if tc.ID == m.ToolCallID {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("orphaned tool message at index %d: tool_call_id=%q has no preceding assistant", i, m.ToolCallID)
		}
	}
}

func TestSession_PerSessionLocking(t *testing.T) {
	s := &Session{maxTurns: 10}
	// Verify lock/unlock don't deadlock.
	s.Lock()
	s.Unlock()
	// Concurrent appends shouldn't panic.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			s.AppendMessage(llm.Message{Role: "user", Content: "x"})
		}
	}()
	s.AppendMessage(llm.Message{Role: "user", Content: "main"})
	<-done
}

func TestSessionStore_EvictIdle(t *testing.T) {
	store := NewSessionStore(10)
	s := store.Get("old-session")
	s.lastActive = time.Now().Add(-2 * time.Hour) // very old
	n := store.EvictIdle(time.Hour)
	if n != 1 {
		t.Errorf("evicted = %d, want 1", n)
	}
	if store.Has("old-session") {
		t.Error("old session should be evicted")
	}
}

func TestBuildSystemPrompt_BaseIdentity(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{
		AgentName: "TestBot",
		ToolNames: []string{"alga_list_alerts", "shell"},
	})
	if !strings.Contains(prompt, "TestBot") {
		t.Error("prompt missing agent name")
	}
	if !strings.Contains(prompt, "alga_list_alerts") {
		t.Error("prompt missing tool name")
	}
	if !strings.Contains(prompt, "shell") {
		t.Error("prompt missing shell tool")
	}
}

func TestBuildSystemPrompt_AlgaContextInjected(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{
		AlgaCtx: AlgaContext{
			InvestigationID: "inv_123",
			IncidentID:      "inc_456",
			Severity:        "SEV1",
		},
	})
	if !strings.Contains(prompt, "inv_123") {
		t.Error("prompt missing investigation id")
	}
	if !strings.Contains(prompt, "inc_456") {
		t.Error("prompt missing incident id")
	}
	if !strings.Contains(prompt, "SEV1") {
		t.Error("prompt missing severity")
	}
}

func TestBuildSystemPrompt_EmptyAlgaContextOmitted(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{})
	// The "## Current Context" header should not appear when AlgaCtx is empty.
	// (The base identity text mentions "Current Context" in prose, so we check
	// for the markdown header specifically.)
	if strings.Contains(prompt, "## Current Context") {
		t.Error("empty Alga context should not emit the Current Context header")
	}
}

func TestBuildSystemPrompt_CustomPromptFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/prompt.txt"
	if err := writeFile(path, "CUSTOM PROMPT BODY"); err != nil {
		t.Fatal(err)
	}
	prompt := BuildSystemPrompt(SystemPromptOptions{
		CustomPromptFile: path,
	})
	if !strings.Contains(prompt, "CUSTOM PROMPT BODY") {
		t.Errorf("custom prompt not used: %q", prompt)
	}
}

// --- Agent loop tests using a real httptest LLM server ---

func TestAgent_Process_SimpleResponse(t *testing.T) {
	// Use the script LLM via a test server wrapper.
	srv := newTestLLMServer(t, []string{
		// First (and only) turn: no tool calls, text response.
		`{"choices":[{"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}]}`,
	}, nil)
	defer srv.close()

	core := newTestAgent(t, srv, nil)
	result, err := core.Process(context.Background(), ProcessRequest{
		SessionID: "test:1",
		ChatID:    "1",
		Text:      "hi",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Text != "Hello!" {
		t.Errorf("text = %q, want Hello!", result.Text)
	}
	if result.Iterations != 1 {
		t.Errorf("iterations = %d, want 1", result.Iterations)
	}
}

func TestAgent_Process_ToolCallLoop(t *testing.T) {
	srv := newTestLLMServer(t, []string{
		// Turn 1: assistant requests a tool call.
		`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"msg\":\"ping\"}"}}]},"finish_reason":"tool_calls"}]}`,
		// Turn 2: assistant produces a final text response.
		`{"choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`,
	}, nil)
	defer srv.close()

	reg := tools.NewRegistry()
	reg.Register(&testTool{name: "echo"})

	core := newTestAgentWithTools(t, srv, reg)
	result, err := core.Process(context.Background(), ProcessRequest{
		SessionID: "test:tool",
		ChatID:    "1",
		Text:      "call echo",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Text != "pong" {
		t.Errorf("text = %q, want pong", result.Text)
	}
	if result.ToolCalls != 1 {
		t.Errorf("tool_calls = %d, want 1", result.ToolCalls)
	}
	if result.Iterations != 2 {
		t.Errorf("iterations = %d, want 2", result.Iterations)
	}
}

func TestAgent_Process_SessionIsolation(t *testing.T) {
	srv := newTestLLMServer(t, []string{
		`{"choices":[{"message":{"role":"assistant","content":"session-A"},"finish_reason":"stop"}]}`,
		`{"choices":[{"message":{"role":"assistant","content":"session-B"},"finish_reason":"stop"}]}`,
	}, nil)
	defer srv.close()

	core := newTestAgent(t, srv, nil)

	// Process message from session A.
	resA, err := core.Process(context.Background(), ProcessRequest{
		SessionID: "telegram:111",
		ChatID:    "111",
		Text:      "msg-A",
	})
	if err != nil {
		t.Fatalf("Process A: %v", err)
	}
	// Process message from session B.
	resB, err := core.Process(context.Background(), ProcessRequest{
		SessionID: "telegram:222",
		ChatID:    "222",
		Text:      "msg-B",
	})
	if err != nil {
		t.Fatalf("Process B: %v", err)
	}
	// Sessions should be distinct.
	if resA.Text == resB.Text {
		t.Errorf("sessions leaked: both got %q", resA.Text)
	}
	// Session A history must not contain B's message.
	msgsA := core.Store().Get("telegram:111").Messages()
	for _, m := range msgsA {
		if strings.Contains(m.Content, "msg-B") {
			t.Error("session A contains session B message")
		}
	}
}

func TestAgent_Process_AlgaContextPreserved(t *testing.T) {
	srv := newTestLLMServer(t, []string{
		`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`,
	}, nil)
	defer srv.close()

	core := newTestAgent(t, srv, nil)
	_, err := core.Process(context.Background(), ProcessRequest{
		SessionID: "alga:inv_99",
		ChatID:    "investigation_99",
		Text:      "status?",
		AlgaCtx:   AlgaContext{InvestigationID: "inv_99", Severity: "SEV2"},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	// The session should retain the Alga context.
	sess := core.Store().Get("alga:inv_99")
	ac := sess.AlgaContext()
	if ac.InvestigationID != "inv_99" {
		t.Errorf("investigation id = %q, want inv_99", ac.InvestigationID)
	}
	if ac.Severity != "SEV2" {
		t.Errorf("severity = %q, want SEV2", ac.Severity)
	}
}

// --- helpers ---

type testTool struct{ name string }

func (t *testTool) Name() string        { return t.name }
func (t *testTool) Description() string { return "test tool" }
func (t *testTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"msg": map[string]any{"type": "string"},
	}}
}
func (t *testTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Msg string `json:"msg"`
	}
	_ = json.Unmarshal(args, &in)
	return `{"echo":"` + in.Msg + `"}`, nil
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// newTestLLMServer creates an httptest server that replays non-streaming
// responses in order.
type testLLMServer struct {
	server    *httptest.Server
	responses []string
	mu        sync.Mutex
	idx       int
	t         *testing.T
}

func newTestLLMServer(t *testing.T, responses []string, _ []string) *testLLMServer {
	srv := &testLLMServer{responses: responses, t: t}
	srv.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		srv.mu.Lock()
		resp := `{"choices":[{"message":{"role":"assistant","content":"default"},"finish_reason":"stop"}]}`
		if srv.idx < len(srv.responses) {
			resp = srv.responses[srv.idx]
			srv.idx++
		}
		srv.mu.Unlock()
		_, _ = w.Write([]byte(resp))
	}))
	return srv
}

func (s *testLLMServer) close() { s.server.Close() }

func newTestAgent(t *testing.T, srv *testLLMServer, _ []string) *AgentCore {
	return newTestAgentWithTools(t, srv, tools.NewRegistry())
}

func newTestAgentWithTools(t *testing.T, srv *testLLMServer, reg *tools.Registry) *AgentCore {
	c := llm.New(srv.server.URL, "test-key", "test-model",
		llm.WithMaxRetries(0),
	)
	return New(Options{
		LLM:      c,
		Tools:    reg,
		Behavior: config.AgentBehaviorConfig{MaxIterations: 10, ToolTimeout: 5 * time.Second, ContextWindow: 20},
		Agent:    config.AgentConfig{Name: "TestAgent"},
	})
}
