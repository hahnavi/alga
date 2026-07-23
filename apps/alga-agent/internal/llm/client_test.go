package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// mockLLMServer is a configurable OpenAI-compatible test server.
type mockLLMServer struct {
	t           *testing.T
	responses   []string // queued non-streaming responses (JSON)
	streamTexts []string // queued streaming responses (plain text)
	failStatus  int      // if >0, return this status code
	failTimes   int32    // number of times to fail before succeeding
	callCount   int32
}

func newMockServer(t *testing.T) *mockLLMServer {
	return &mockLLMServer{t: t}
}

func (m *mockLLMServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&m.callCount, 1)
		if m.failStatus > 0 && atomic.LoadInt32(&m.failTimes) > 0 {
			atomic.AddInt32(&m.failTimes, -1)
			w.WriteHeader(m.failStatus)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Parse the request to detect stream vs non-stream.
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		stream, _ := req["stream"].(bool)

		w.Header().Set("Content-Type", "application/json")
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			text := ""
			if len(m.streamTexts) > 0 {
				text = m.streamTexts[0]
				m.streamTexts = m.streamTexts[1:]
			}
			// Emit text in word-by-word chunks.
			words := strings.Fields(text)
			for _, word := range words {
				chunk := map[string]any{
					"choices": []map[string]any{
						{"delta": map[string]any{"content": word + " "}},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		// Non-streaming: emit the first queued response or a default.
		resp := `{"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`
		if len(m.responses) > 0 {
			resp = m.responses[0]
			m.responses = m.responses[1:]
		}
		_, _ = w.Write([]byte(resp))
	})
}

func TestClient_Complete_Success(t *testing.T) {
	mock := newMockServer(t)
	mock.responses = []string{`{"choices":[{"message":{"role":"assistant","content":"hi there"},"finish_reason":"stop"}]}`}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	c := New(srv.URL, "test-key", "test-model")
	resp, err := c.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "hi there" {
		t.Errorf("content = %q", resp.Choices[0].Message.Content)
	}
}

func TestClient_Complete_Retries(t *testing.T) {
	mock := newMockServer(t)
	mock.failStatus = http.StatusServiceUnavailable
	atomic.StoreInt32(&mock.failTimes, 2) // fail twice, then succeed
	mock.responses = []string{`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	c := New(srv.URL, "test-key", "test-model",
		WithMaxRetries(3),
	)
	// Use a short retry base for test speed.
	c.RetryBaseDelay = 0

	resp, err := c.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete after retries: %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Errorf("content = %q", resp.Choices[0].Message.Content)
	}
	if calls := atomic.LoadInt32(&mock.callCount); calls != 3 {
		t.Errorf("call count = %d, want 3 (1 initial + 2 retries)", calls)
	}
}

func TestClient_Complete_NonRetryableFails(t *testing.T) {
	mock := newMockServer(t)
	mock.failStatus = http.StatusBadRequest
	atomic.StoreInt32(&mock.failTimes, 5) // would retry many times, but 400 is not retryable
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	c := New(srv.URL, "test-key", "test-model", WithMaxRetries(3))
	c.RetryBaseDelay = 0

	_, err := c.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", apiErr.StatusCode)
	}
	if calls := atomic.LoadInt32(&mock.callCount); calls != 1 {
		t.Errorf("call count = %d, want 1 (no retries for 400)", calls)
	}
}

func TestClient_Complete_ToolCallsParsed(t *testing.T) {
	mock := newMockServer(t)
	mock.responses = []string{`{
		"choices":[{
			"message":{
				"role":"assistant",
				"content":"",
				"tool_calls":[
					{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}
				]
			},
			"finish_reason":"tool_calls"
		}]
	}`}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	c := New(srv.URL, "test-key", "test-model", WithMaxRetries(0))
	resp, err := c.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "weather?"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d", len(resp.Choices[0].Message.ToolCalls))
	}
	tc := resp.Choices[0].Message.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("tool call id = %q", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("function name = %q", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"city":"SF"}` {
		t.Errorf("arguments = %q", tc.Function.Arguments)
	}
	if !HasToolCalls(resp.Choices[0].Message) {
		t.Error("HasToolCalls returned false")
	}
}

func TestClient_Stream_Success(t *testing.T) {
	mock := newMockServer(t)
	mock.streamTexts = []string{"Hello world from stream"}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	c := New(srv.URL, "test-key", "test-model")
	var accumulated string
	var deltas []string
	text, err := c.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(acc, delta string) bool {
		accumulated = acc
		deltas = append(deltas, delta)
		return true
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !strings.Contains(text, "Hello") {
		t.Errorf("streamed text = %q", text)
	}
	if len(deltas) == 0 {
		t.Error("no deltas received")
	}
	// accumulated should match text.
	if accumulated != text {
		t.Errorf("accumulated != final text")
	}
}

func TestClient_Stream_CallbackStopsEarly(t *testing.T) {
	mock := newMockServer(t)
	mock.streamTexts = []string{"one two three four five"}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	c := New(srv.URL, "test-key", "test-model")
	callCount := 0
	text, err := c.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(acc, delta string) bool {
		callCount++
		return callCount < 2 // stop after 2 deltas
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if callCount != 2 {
		t.Errorf("callback invoked %d times, want 2", callCount)
	}
	if text == "" {
		t.Error("text should contain partial stream")
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		code      int
		retryable bool
	}{
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
	}
	for _, tt := range tests {
		e := &APIError{StatusCode: tt.code}
		if e.IsRetryable() != tt.retryable {
			t.Errorf("code %d: retryable = %v, want %v", tt.code, e.IsRetryable(), tt.retryable)
		}
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "5")
	d := parseRetryAfter(h)
	if d != 5_000_000_000 {
		t.Errorf("retry after = %v, want 5s", d)
	}
}

func TestParseRetryAfter_Empty(t *testing.T) {
	h := http.Header{}
	d := parseRetryAfter(h)
	if d != 0 {
		t.Errorf("retry after = %v, want 0", d)
	}
}

func TestUserFacingMessage(t *testing.T) {
	msg := UserFacingMessage(fmt.Errorf("some error"))
	if msg == "" {
		t.Error("UserFacingMessage should not be empty")
	}
}
