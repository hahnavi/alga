package alga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- REST: idempotency ---

// TestIdempotencyKeyAutoInjected verifies the SDK stamps an Idempotency-Key
// header on every state-changing request even when the caller doesn't supply
// one. Without this, a retry of a transient 503 would double-fire the
// mutation.
func TestIdempotencyKeyAutoInjected(t *testing.T) {
	var gotKey string
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		seenPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	_, _ = c.SendCommand(context.Background(), "investigation_x", ResolveAlert("fp"))

	if !strings.HasPrefix(gotKey, "alga-") {
		t.Errorf("Idempotency-Key = %q, want prefix alga-", gotKey)
	}
	if seenPath != "/api/v1/agent/messages" {
		t.Errorf("path = %q, want /api/v1/agent/messages", seenPath)
	}
}

// TestIdempotencyKeyCallerSupplied verifies a caller-provided key is forwarded
// unchanged (not overwritten by the SDK's auto-key).
func TestIdempotencyKeyCallerSupplied(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	_, _ = c.SendCommandWithKey(context.Background(), "chat", ResolveAlert("fp"), "my-outbox-key-1")

	if gotKey != "my-outbox-key-1" {
		t.Errorf("Idempotency-Key = %q, want my-outbox-key-1", gotKey)
	}
}

// TestIdempotencyKeyNotInjectedOnGET verifies GETs are left untouched so the
// backend doesn't waste cache slots on idempotent reads.
func TestIdempotencyKeyNotInjectedOnGET(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[]}`))
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	if _, err := c.ListAlerts(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if gotKey != "" {
		t.Errorf("Idempotency-Key on GET = %q, want empty", gotKey)
	}
}

// --- REST: retries ---

// TestRESTRetriesTransient verifies the SDK retries 503 and 429 responses
// when MaxRESTRetries > 0.
func TestRESTRetriesTransient(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[]}`))
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(3))
	_, err := c.ListAlerts(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server was hit %d times, want 3", got)
	}
}

// TestRESTRetriesAuthNotRetried verifies auth failures fail fast.
func TestRESTRetriesAuthNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "stale-tok", WithMaxRESTRetries(3))
	_, err := c.ListAlerts(context.Background(), nil)
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
	if !IsAuthError(err) {
		t.Errorf("expected IsAuthError to be true, got %T: %v", err, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server was hit %d times, want 1 (auth not retried)", got)
	}
}

// TestRESTRetriesPermanentNotRetried verifies 4xx (non-429) failures fail
// immediately without consuming the retry budget.
func TestRESTRetriesPermanentNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(3))
	_, err := c.ListAlerts(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server was hit %d times, want 1", got)
	}
}

// TestRetryAfterHeaderHonored verifies a 429 with Retry-After: 0 (immediate)
// is retried and the small backoff doesn't slow the test down.
func TestRetryAfterHeaderHonored(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[]}`))
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(2))
	_, err := c.ListAlerts(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected success after honoring retry-after, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server was hit %d times, want 2", got)
	}
}

// --- REST: list normalization at call sites ---

func TestListAlertsReturnsParsedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"fingerprint":"fp"}],"total":1}`))
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	resp, err := c.ListAlerts(context.Background(), map[string]string{"status": "active"})
	if err != nil {
		t.Fatal(err)
	}
	all := resp.All()
	if len(all) != 1 || all[0].Fingerprint != "fp" {
		t.Errorf("All() = %+v", all)
	}
}

// --- SSE: parsing ---

// TestSSEDispatchMessage verifies event/data parsing and that the OnMessage
// callback fires with the right payload.
func TestSSEDispatchMessage(t *testing.T) {
	payload := "event: message\ndata: {\"type\":\"message\",\"chat_id\":\"investigation_1\",\"text\":\"hi\",\"message_id\":\"m1\"}\n\n"
	srv := newSSEServer(t, payload, http.StatusOK)

	got := make(chan MessageEvent, 1)
	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	c.OnMessage = func(ev MessageEvent) { got <- ev }

	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Disconnect()

	select {
	case ev := <-got:
		if ev.ChatID != "investigation_1" || ev.Text != "hi" {
			t.Errorf("got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnMessage did not fire within 2s")
	}
}

// TestSSEDispatchCoordinationTask verifies the new coordination_task_dispatched
// event is decoded and routed.
func TestSSEDispatchCoordinationTask(t *testing.T) {
	payload := "event: coordination_task_dispatched\ndata: {\"type\":\"coordination_task_dispatched\",\"task_id\":\"t1\",\"incident_number\":42,\"kind\":\"investigate\",\"goal\":\"find rc\",\"assignee_role\":\"responder\"}\n\n"
	srv := newSSEServer(t, payload, http.StatusOK)

	got := make(chan CoordinationTaskEvent, 1)
	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	c.OnCoordinationTask = func(ev CoordinationTaskEvent) { got <- ev }

	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Disconnect()

	select {
	case ev := <-got:
		if ev.TaskID != "t1" || ev.IncidentNumber != 42 || ev.Kind != TaskKindInvestigate || ev.AssigneeRole != "responder" {
			t.Errorf("got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnCoordinationTask did not fire within 2s")
	}
}

// TestSSEDedup verifies the dedup cache drops a replayed message_id.
func TestSSEDedup(t *testing.T) {
	once := "event: message\ndata: {\"type\":\"message\",\"chat_id\":\"c1\",\"text\":\"first\",\"message_id\":\"dup\"}\n\n"
	twice := "event: message\ndata: {\"type\":\"message\",\"chat_id\":\"c1\",\"text\":\"second\",\"message_id\":\"dup\"}\n\n"
	srv := newSSEServer(t, once+twice, http.StatusOK)

	var got []string
	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	c.OnMessage = func(ev MessageEvent) { got = append(got, ev.Text) }

	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Disconnect()

	deadline := time.Now().Add(2 * time.Second)
	for len(got) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) != 1 || got[0] != "first" {
		t.Errorf("expected only the first message delivered, got %v", got)
	}
}

// TestSSEAuthStopsLoop verifies auth errors terminate the reconnect loop and
// surface on ErrChan.
func TestSSEAuthStopsLoop(t *testing.T) {
	srv := newSSEServer(t, "", http.StatusUnauthorized)
	c := NewAlgaClient(srv.URL, "stale-tok", WithMaxRESTRetries(0))

	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Disconnect()

	select {
	case err := <-c.sse.ErrChan:
		if !IsAuthError(err) {
			t.Errorf("expected auth error on ErrChan, got %T: %v", err, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auth error did not surface on ErrChan within 2s")
	}
}

// --- heartbeat ---

// TestHeartbeatFired verifies the heartbeat goroutine posts to /heartbeat at
// the configured interval.
func TestHeartbeatFired(t *testing.T) {
	var beats int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/heartbeat" {
			atomic.AddInt32(&beats, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/v1/agent/events" {
			w.WriteHeader(http.StatusOK)
			// Hold the stream open briefly; the loop will reconnect on close.
			time.Sleep(300 * time.Millisecond)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithHeartbeatInterval(time.Second))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Disconnect()

	deadline := time.Now().Add(4 * time.Second)
	for atomic.LoadInt32(&beats) < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&beats); got < 2 {
		t.Errorf("expected >=2 heartbeats, got %d", got)
	}
}

// --- helpers ---

// newSSEServer returns an httptest server that responds to /events with the
// given status code, writing the payload once then closing the stream. The
// SDK's reconnect logic is exercised naturally on stream close.
func newSSEServer(t *testing.T, payload string, status int) *httptest.Server {
	t.Helper()
	var hits int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/events" && r.URL.Path != "/api/v1/agent/heartbeat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/api/v1/agent/heartbeat" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Only emit the payload once so tests don't loop forever replaying it.
		if atomic.AddInt32(&hits, 1) > 1 {
			// Hold the connection briefly to avoid hot-spinning, then close.
			time.Sleep(100 * time.Millisecond)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		if status == http.StatusOK && payload != "" {
			_, _ = io.Copy(w, strings.NewReader(payload))
			if !strings.HasSuffix(payload, "\n") {
				_, _ = w.Write([]byte("\n"))
			}
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
}

// TestCommandsRoundTrip ensures each builder round-trips through JSON without
// losing the Op field — guards against struct tag typos in commands.go.
func TestCommandsRoundTrip(t *testing.T) {
	ops := []InvestigationCommand{
		ResolveAlert("fp"),
		ReopenAlert("fp"),
		CancelInvestigation("r"),
		PauseInvestigation("r"),
		SetOutcome(nil, nil),
		PromoteToIncident("t", "sev", "p"),
		AssignInvestigation("a"),
		TriageFeedback("t", true, "d", "s", "n"),
		SetIncidentPriority(1, "p"),
		SetIncidentSeverity(1, "s"),
		TriggerEscalation(1),
		MitigateIncident(1, "r"),
		ResolveIncident(1, "r"),
		BeginTriage(1),
		PromoteIncident(1),
		AssignIncidentRole(1, "commander", "u", "t", "scope"),
		PostHandoff(1, "m", "a", "u"),
		PublishStatusUpdate(1, "m", "identified"),
		SetIncidentResolutionDocs(1, "s", "i", "a", "r", "res"),
		DispatchTask(1, TaskKindInvestigate, "g", "responder"),
		DispatchTaskToAgent(1, TaskKindVerify, "g", "a1"),
		ClaimTask("t1"),
		CompleteTask("t1", map[string]any{"k": "v"}),
		SynthesizeFindings(1, "s", nil),
		PostInvestigationThreadMessage("m", false),
	}
	for i, op := range ops {
		raw, err := json.Marshal(op)
		if err != nil {
			t.Fatalf("op %d marshal: %v", i, err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("op %d unmarshal: %v", i, err)
		}
		if _, ok := got["op"]; !ok {
			t.Errorf("op %d (%s) round-trip lost 'op' field: %s", i, op.Op, raw)
		}
		if got["op"] != op.Op {
			t.Errorf("op %d op field = %v, want %q", i, got["op"], op.Op)
		}
	}
}

// TestServerURLTrim verifies trailing slashes are stripped so path
// concatenation doesn't produce //api/v1/agent/...
func TestServerURLTrim(t *testing.T) {
	c := NewAlgaClient("https://example.com/alga/", "tok")
	if c.ServerURL() != "https://example.com/alga" {
		t.Errorf("ServerURL = %q", c.ServerURL())
	}
}

// TestContextCancelDuringCall verifies a canceled context aborts the request.
func TestContextCancelDuringCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the request is canceled; we expect the SDK to surface
		// the cancellation rather than hang.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled up front

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	_, err := c.ListAlerts(ctx, nil)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		// The SDK may wrap the error in AlgaConnectionError; the underlying
		// context error should still be reachable via errors.Is.
		var connErr *AlgaConnectionError
		if !errors.As(err, &connErr) || !errors.Is(connErr.Err, context.Canceled) {
			t.Errorf("expected context.Canceled to be reachable, got %T: %v", err, err)
		}
	}
}

// TestSendCommandResultShape verifies the CommandResponse JSON envelope is
// parsed correctly into the new struct fields (IncidentNumber, etc.).
func TestSendCommandResultShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Kind    string          `json:"kind"`
			Command json.RawMessage `json:"command"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		// Echo back a promotion outcome.
		_ = json.NewEncoder(w).Encode(CommandResponse{
			Ok:                      true,
			Op:                      "promote_to_incident",
			IncidentNumber:          42,
			IncidentInvestigationID: "inv-99",
		})
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	resp, err := c.SendCommand(context.Background(), "investigation_1", PromoteToIncident("t", "sev", "p"))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ok || resp.IncidentNumber != 42 || resp.IncidentInvestigationID != "inv-99" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

// TestUserAgentHeader verifies the User-Agent header is sent.
func TestUserAgentHeader(t *testing.T) {
	var seenUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[]}`))
	}))
	defer srv.Close()

	t.Run("default", func(t *testing.T) {
		c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
		_, _ = c.ListAlerts(context.Background(), nil)
		if seenUA != defaultUserAgent {
			t.Errorf("User-Agent = %q, want %q", seenUA, defaultUserAgent)
		}
	})
	t.Run("override", func(t *testing.T) {
		c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0), WithUserAgent("myagent/1.0"))
		_, _ = c.ListAlerts(context.Background(), nil)
		if seenUA != "myagent/1.0" {
			t.Errorf("User-Agent = %q, want myagent/1.0", seenUA)
		}
	})
}

// TestSendCommandSerializesChatID verifies the wire payload includes chat_id
// and kind=inv_tool.
func TestSendCommandSerializesChatID(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"op":"resolve_alert"}`))
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	_, err := c.SendCommand(context.Background(), "investigation_42", ResolveAlert("fp"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(gotBody, &got)
	if got["chat_id"] != "investigation_42" {
		t.Errorf("chat_id = %v, want investigation_42", got["chat_id"])
	}
	if got["kind"] != "inv_tool" {
		t.Errorf("kind = %v, want inv_tool", got["kind"])
	}
	cmd, _ := got["command"].(map[string]any)
	if cmd == nil {
		t.Fatalf("missing command field: %s", gotBody)
	}
	if cmd["op"] != "resolve_alert" {
		t.Errorf("command.op = %v, want resolve_alert", cmd["op"])
	}
}

// TestListIncidentTasks verifies the coordination task listing parses.
func TestListIncidentTasks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/incidents/42/tasks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tasks":[{"task_id":"t1","kind":"investigate","goal":"g","status":"claimed"}],"total":1}`))
	}))
	defer srv.Close()

	c := NewAlgaClient(srv.URL, "tok", WithMaxRESTRetries(0))
	tasks, err := c.ListIncidentTasks(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "t1" || tasks[0].Kind != TaskKindInvestigate {
		t.Errorf("tasks = %+v", tasks)
	}
}

// ensure bytes is referenced even if future edits remove the only use.
var _ = bytes.NewReader

// suppress unused-import in case a future trim drops one of these.
var _ = fmt.Sprintf
