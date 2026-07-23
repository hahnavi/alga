package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	alga "github.com/alga/agent-sdk-go"
)

// fakeAlgaClient is an in-memory AlgaClient for exercising the typed Alga
// tools without spinning up the backend. Each method records its calls so
// tests can assert on the wire shape.
type fakeAlgaClient struct {
	// alerts
	alerts   []alga.Alert
	gotAlert *alga.Alert
	// inv
	invs       []alga.Investigation
	gotInv     *alga.Investigation
	postedType string
	// messaging
	sentText string
	lastCmd  alga.InvestigationCommand
	cmdResp  alga.CommandResponse
	cmdErr   error
	// knowledge / memory
	notes    []alga.KnowledgeNote
	memories []alga.Memory
	// incident / tasks
	incident *alga.Incident
	tasks    []alga.CoordinationTask
	// catalog
	services []alga.Service
	oncall   map[string]any

	// captures
	capturedChatID string
}

func (f *fakeAlgaClient) ListAlerts(_ context.Context, params map[string]string) (*alga.AlertListResponse, error) {
	return &alga.AlertListResponse{Alerts: f.alerts, Total: len(f.alerts)}, nil
}
func (f *fakeAlgaClient) GetAlert(_ context.Context, fp string) (*alga.Alert, error) {
	if f.gotAlert != nil {
		return f.gotAlert, nil
	}
	return &alga.Alert{Fingerprint: fp}, nil
}
func (f *fakeAlgaClient) ListInvestigations(_ context.Context, _ map[string]string) (*alga.InvestigationListResponse, error) {
	return &alga.InvestigationListResponse{Investigations: f.invs, Total: len(f.invs)}, nil
}
func (f *fakeAlgaClient) GetInvestigation(_ context.Context, id string) (*alga.Investigation, error) {
	if f.gotInv != nil {
		return f.gotInv, nil
	}
	return &alga.Investigation{ID: id, InvestigationID: id}, nil
}
func (f *fakeAlgaClient) PostUpdate(_ context.Context, id, t, _ string) (*alga.Investigation, error) {
	f.postedType = t
	return &alga.Investigation{ID: id, InvestigationID: id, Status: "investigating"}, nil
}
func (f *fakeAlgaClient) SendMessage(_ context.Context, chatID, text string, _ []string) (*alga.SendMessageResponse, error) {
	f.capturedChatID = chatID
	f.sentText = text
	return &alga.SendMessageResponse{Status: "ok", MessageID: "m1"}, nil
}
func (f *fakeAlgaClient) SendCommand(_ context.Context, chatID string, cmd alga.InvestigationCommand) (*alga.CommandResponse, error) {
	f.capturedChatID = chatID
	f.lastCmd = cmd
	if f.cmdErr != nil {
		return nil, f.cmdErr
	}
	resp := f.cmdResp
	if resp.Op == "" {
		resp.Op = cmd.Op
	}
	resp.Ok = true
	return &resp, nil
}
func (f *fakeAlgaClient) ListKnowledge(_ context.Context, _ map[string]string) (*alga.KnowledgeListResponse, error) {
	return &alga.KnowledgeListResponse{Notes: f.notes, Total: len(f.notes)}, nil
}
func (f *fakeAlgaClient) CreateKnowledge(_ context.Context, _ map[string]any) (*alga.KnowledgeNote, error) {
	return &alga.KnowledgeNote{ID: "k1"}, nil
}
func (f *fakeAlgaClient) ListMemories(_ context.Context, _ map[string]string) (*alga.MemoryListResponse, error) {
	return &alga.MemoryListResponse{Memories: f.memories, Total: len(f.memories)}, nil
}
func (f *fakeAlgaClient) CreateMemory(_ context.Context, _ map[string]any) (*alga.Memory, error) {
	return &alga.Memory{ID: "mem1"}, nil
}
func (f *fakeAlgaClient) DeleteMemory(_ context.Context, _ string) error { return nil }
func (f *fakeAlgaClient) GetIncident(_ context.Context, id string) (*alga.Incident, error) {
	if f.incident != nil {
		return f.incident, nil
	}
	return &alga.Incident{ID: id}, nil
}
func (f *fakeAlgaClient) AddIncidentTimeline(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeAlgaClient) ListServices(_ context.Context) ([]alga.Service, error) {
	return f.services, nil
}
func (f *fakeAlgaClient) WhoIsOnCall(_ context.Context) (map[string]any, error) {
	return f.oncall, nil
}
func (f *fakeAlgaClient) ListIncidentTasks(_ context.Context, _ int64) ([]alga.CoordinationTask, error) {
	return f.tasks, nil
}

// --- Tool tests ---

// TestAlgaToolRegistryShape verifies every Alga tool registers, has a unique
// name, declares a category, and emits a non-empty JSON schema.
func TestAlgaToolRegistryShape(t *testing.T) {
	reg := NewRegistry()
	RegisterAlgaTools(reg, &fakeAlgaClient{})

	wantTools := []string{
		"alga_list_alerts", "alga_get_alert", "alga_resolve_alert", "alga_reopen_alert",
		"alga_list_investigations", "alga_get_investigation", "alga_post_update",
		"alga_send_message", "alga_set_outcome", "alga_cancel_investigation",
		"alga_triage_feedback", "alga_promote_to_incident",
		"alga_get_incident", "alga_add_incident_timeline",
		"alga_trigger_escalation", "alga_mitigate_incident", "alga_resolve_incident",
		"alga_search_knowledge", "alga_create_knowledge",
		"alga_list_memories", "alga_create_memory", "alga_delete_memory",
		"alga_dispatch_task", "alga_claim_task", "alga_complete_task",
		"alga_synthesize_findings", "alga_list_tasks",
		"alga_list_services", "alga_who_is_on_call",
	}
	for _, name := range wantTools {
		t.Run(name, func(t *testing.T) {
			tool, ok := reg.Get(name)
			if !ok {
				t.Errorf("tool %q not registered", name)
				return
			}
			if tool.Description() == "" {
				t.Errorf("tool %q has empty description", name)
			}
			schema := tool.Schema()
			if schema["type"] != "object" {
				t.Errorf("tool %q schema type = %v, want object", name, schema["type"])
			}
			if _, ok := schema["properties"]; !ok {
				t.Errorf("tool %q schema missing properties", name)
			}
			if cp, ok := tool.(CategoryProvider); !ok || cp.Category() != algaCategory {
				t.Errorf("tool %q missing Alga category", name)
			}
		})
	}
}

// TestAlgaListAlerts exercises a successful list + the result envelope shape.
func TestAlgaListAlerts(t *testing.T) {
	reg := NewRegistry()
	fc := &fakeAlgaClient{alerts: []alga.Alert{{Fingerprint: "fp1", Status: "active"}}}
	RegisterAlgaTools(reg, fc)

	tool, _ := reg.Get("alga_list_alerts")
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	var res Result[listAlertsOutput]
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Data.Count != 1 || res.Data.Alerts[0].Fingerprint != "fp1" {
		t.Errorf("result = %+v", res)
	}
}

// TestAlgaListAlertsDualKey verifies the .All() normalization surfaces
// alerts populated under either JSON key.
func TestAlgaListAlertsDualKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Note the "items" key, not "alerts" — verifies normalization.
		_, _ = w.Write([]byte(`{"items":[{"fingerprint":"fp-from-items"}]}`))
	}))
	defer srv.Close()
	client := alga.NewAlgaClient(srv.URL, "tok", alga.WithMaxRESTRetries(0))

	reg := NewRegistry()
	RegisterAlgaTools(reg, client)

	tool, _ := reg.Get("alga_list_alerts")
	out, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !strings.Contains(out, "fp-from-items") {
		t.Errorf("expected normalized output to include fp-from-items: %s", out)
	}
}

// TestAlgaGetAlertMissingFingerprint verifies the typed tool surfaces a
// validation error in the envelope (not as a Go error).
func TestAlgaGetAlertMissingFingerprint(t *testing.T) {
	reg := NewRegistry()
	RegisterAlgaTools(reg, &fakeAlgaClient{})

	tool, _ := reg.Get("alga_get_alert")
	out, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Errorf("expected in-band error, got Go error: %v", err)
	}
	if !strings.Contains(out, "fingerprint is required") {
		t.Errorf("expected validation error, got %s", out)
	}
}

// TestAlgaResolveAlertChatIDFromContext verifies that when CallContext
// provides the chat_id (i.e. running inside an Alga thread), the tool uses
// it rather than requiring the LLM to supply one.
func TestAlgaResolveAlertChatIDFromContext(t *testing.T) {
	reg := NewRegistry()
	fc := &fakeAlgaClient{}
	RegisterAlgaTools(reg, fc)

	tool, _ := reg.Get("alga_resolve_alert")
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "investigation_42"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"fingerprint":"fp"}`))
	if err != nil {
		t.Fatal(err)
	}
	if fc.capturedChatID != "investigation_42" {
		t.Errorf("chat_id = %q, want investigation_42", fc.capturedChatID)
	}
	if fc.lastCmd.Op != "resolve_alert" || fc.lastCmd.Fingerprint != "fp" {
		t.Errorf("cmd = %+v", fc.lastCmd)
	}
}

// TestAlgaSendCommandSDKError verifies that an SDK error is surfaced via
// algaErr into the result envelope rather than crashing the agent.
func TestAlgaSendCommandSDKError(t *testing.T) {
	reg := NewRegistry()
	fc := &fakeAlgaClient{cmdErr: &alga.AlgaAuthError{StatusCode: 401, Message: "bad token"}}
	RegisterAlgaTools(reg, fc)

	tool, _ := reg.Get("alga_resolve_alert")
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "investigation_1"})
	out, _ := tool.Execute(ctx, json.RawMessage(`{"fingerprint":"fp"}`))
	if strings.Contains(out, `"ok":true`) {
		t.Errorf("expected failure envelope, got %s", out)
	}
	if !strings.Contains(out, "alga auth error") {
		t.Errorf("expected alga auth error in envelope, got %s", out)
	}
}

// TestAlgaDispatchTask verifies the new coordination task tools.
func TestAlgaDispatchTask(t *testing.T) {
	reg := NewRegistry()
	fc := &fakeAlgaClient{}
	RegisterAlgaTools(reg, fc)

	tool, _ := reg.Get("alga_dispatch_task")
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "incident_coord_42"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"incident_number":42,"kind":"investigate","goal":"find rc"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Errorf("expected success envelope, got %s", out)
	}
	if fc.lastCmd.Op != "dispatch_task" || fc.lastCmd.TaskKind != "investigate" || fc.lastCmd.Goal != "find rc" {
		t.Errorf("cmd = %+v", fc.lastCmd)
	}
}

// TestAlgaCompleteTaskRequiredFields verifies validation.
func TestAlgaCompleteTaskRequiredFields(t *testing.T) {
	reg := NewRegistry()
	RegisterAlgaTools(reg, &fakeAlgaClient{})

	tool, _ := reg.Get("alga_complete_task")
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "incident_coord_1"})

	// Missing task_id.
	out, _ := tool.Execute(ctx, json.RawMessage(`{}`))
	if !strings.Contains(out, "task_id is required") {
		t.Errorf("expected validation error, got %s", out)
	}
}

// TestAlgaToolCapabilityGating verifies the command-required tools are
// filtered out for investigate-only agents.
func TestAlgaToolCapabilityGating(t *testing.T) {
	reg := NewRegistry()
	RegisterAlgaTools(reg, &fakeAlgaClient{})

	investigateOnly := reg.ListForCapabilities([]string{"investigate"})
	for _, tool := range investigateOnly {
		if tool.Name() == "alga_dispatch_task" || tool.Name() == "alga_synthesize_findings" ||
			tool.Name() == "alga_resolve_incident" || tool.Name() == "alga_mitigate_incident" {
			t.Errorf("investigate-only agent should not see %s", tool.Name())
		}
	}

	// Commander should see all the command tools.
	commander := reg.ListForCapabilities([]string{"investigate", "command"})
	names := map[string]bool{}
	for _, tool := range commander {
		names[tool.Name()] = true
	}
	if !names["alga_dispatch_task"] || !names["alga_synthesize_findings"] {
		t.Errorf("commander should see dispatch_task and synthesize_findings")
	}
}

// TestAlgaToolDefinitionsAreValidJSON verifies the OpenAI tools payload
// generated for an LLM round-trips through JSON without losing structure.
func TestAlgaToolDefinitionsAreValidJSON(t *testing.T) {
	reg := NewRegistry()
	RegisterAlgaTools(reg, &fakeAlgaClient{})

	defs := reg.Definitions()
	if len(defs) < 20 {
		t.Errorf("expected >=20 tool definitions, got %d", len(defs))
	}
	for _, def := range defs {
		b, err := json.Marshal(def)
		if err != nil {
			t.Errorf("failed to marshal definition: %v", err)
			continue
		}
		var roundTrip map[string]any
		if err := json.Unmarshal(b, &roundTrip); err != nil {
			t.Errorf("definition did not round-trip: %v", err)
		}
		fn, _ := roundTrip["function"].(map[string]any)
		if fn == nil || fn["name"] == nil {
			t.Errorf("definition missing function.name: %s", b)
		}
	}
}

// TestAlgaSendCommandWithRealBackendHTTP verifies the typed tool correctly
// dispatches through the SDK client over HTTP, exercising the full wire
// path including idempotency-key injection.
func TestAlgaSendCommandWithRealBackendHTTP(t *testing.T) {
	var sawOp, sawChat, sawIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawIdem = r.Header.Get("Idempotency-Key")
		var req struct {
			ChatID  string          `json:"chat_id"`
			Kind    string          `json:"kind"`
			Command json.RawMessage `json:"command"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		sawChat = req.ChatID
		var cmd map[string]any
		_ = json.Unmarshal(req.Command, &cmd)
		sawOp, _ = cmd["op"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"op":"resolve_alert"}`))
	}))
	defer srv.Close()

	client := alga.NewAlgaClient(srv.URL, "tok", alga.WithMaxRESTRetries(0))
	reg := NewRegistry()
	RegisterAlgaTools(reg, client)

	tool, _ := reg.Get("alga_resolve_alert")
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "investigation_1"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"fingerprint":"fp"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Errorf("expected ok envelope, got %s", out)
	}
	if sawOp != "resolve_alert" {
		t.Errorf("op = %q", sawOp)
	}
	if sawChat != "investigation_1" {
		t.Errorf("chat_id = %q", sawChat)
	}
	if !strings.HasPrefix(sawIdem, "alga-") {
		t.Errorf("Idempotency-Key = %q, want prefix alga-", sawIdem)
	}
}

// TestAlgaListIncidentTasks verifies the new task listing tool.
func TestAlgaListIncidentTasks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/tasks") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tasks":[{"task_id":"t1","kind":"investigate","goal":"g","status":"claimed"}],"total":1}`))
	}))
	defer srv.Close()

	client := alga.NewAlgaClient(srv.URL, "tok", alga.WithMaxRESTRetries(0))
	reg := NewRegistry()
	RegisterAlgaTools(reg, client)

	tool, _ := reg.Get("alga_list_tasks")
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"incident_number":42}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "t1") || !strings.Contains(out, `"count":1`) {
		t.Errorf("expected t1 in result, got %s", out)
	}
}

// ensure errors import is used.
var _ = errors.New
