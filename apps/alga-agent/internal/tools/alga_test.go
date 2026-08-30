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
	// messaging
	sentText string
	lastCmd  alga.InvestigationCommand
	cmdResp  alga.CommandResponse
	cmdErr   error
	// knowledge / memory
	notes    []alga.KnowledgeNote
	memories []alga.Memory
	// incident
	incident *alga.IncidentContext
	// catalog
	services []alga.Service
	oncall   []alga.OnCallEntry

	// captures
	capturedChatID string
}

func (f *fakeAlgaClient) ListAlerts(_ context.Context, params map[string]string) ([]alga.Alert, error) {
	return f.alerts, nil
}
func (f *fakeAlgaClient) GetAlert(_ context.Context, fp string) (*alga.Alert, error) {
	if f.gotAlert != nil {
		return f.gotAlert, nil
	}
	return &alga.Alert{Fingerprint: fp}, nil
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
	return &alga.KnowledgeListResponse{Items: f.notes, Total: int64(len(f.notes))}, nil
}
func (f *fakeAlgaClient) CreateKnowledge(_ context.Context, _ map[string]any) (*alga.KnowledgeNote, error) {
	return &alga.KnowledgeNote{ID: "k1"}, nil
}
func (f *fakeAlgaClient) ListMemories(_ context.Context, _ map[string]string) (*alga.MemoryListResponse, error) {
	return &alga.MemoryListResponse{Items: f.memories, Total: int64(len(f.memories))}, nil
}
func (f *fakeAlgaClient) CreateMemory(_ context.Context, _ map[string]any) (*alga.Memory, error) {
	return &alga.Memory{ID: "mem1"}, nil
}
func (f *fakeAlgaClient) DeleteMemory(_ context.Context, _ string) error { return nil }
func (f *fakeAlgaClient) GetIncident(_ context.Context, n int64) (*alga.IncidentContext, error) {
	if f.incident != nil {
		return f.incident, nil
	}
	return &alga.IncidentContext{Incident: alga.Incident{IncidentNumber: n}}, nil
}
func (f *fakeAlgaClient) AddIncidentTimeline(_ context.Context, _ int64, _, _ string) error {
	return nil
}
func (f *fakeAlgaClient) ListServices(_ context.Context, _ map[string]string) (*alga.ServiceListResponse, error) {
	return &alga.ServiceListResponse{Items: f.services, Total: int64(len(f.services))}, nil
}
func (f *fakeAlgaClient) WhoIsOnCall(_ context.Context) ([]alga.OnCallEntry, error) {
	return f.oncall, nil
}

// --- Tool tests ---

// TestAlgaToolRegistryShape verifies every Alga tool registers, has a unique
// name, declares a category, and emits a non-empty JSON schema.
func TestAlgaToolRegistryShape(t *testing.T) {
	reg := NewRegistry()
	RegisterAlgaTools(reg, &fakeAlgaClient{})

	wantTools := []string{
		"alga_list_alerts", "alga_get_alert", "alga_resolve_alert", "alga_reopen_alert",
		"alga_send_message", "alga_set_outcome", "alga_cancel_investigation",
		"alga_triage_feedback", "alga_promote_to_incident",
		"alga_get_incident", "alga_add_incident_timeline",
		"alga_trigger_escalation", "alga_mitigate_incident", "alga_resolve_incident",
		"alga_search_knowledge", "alga_create_knowledge",
		"alga_list_memories", "alga_create_memory", "alga_delete_memory",
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

// TestAlgaListAlertsEnvelope verifies the SDK decodes the backend's
// {"data": [...]} success envelope end-to-end through the tool.
func TestAlgaListAlertsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"fingerprint":"fp-from-envelope"}]}`))
	}))
	defer srv.Close()
	client := alga.NewAlgaClient(srv.URL, "tok", alga.WithMaxRESTRetries(0))

	reg := NewRegistry()
	RegisterAlgaTools(reg, client)

	tool, _ := reg.Get("alga_list_alerts")
	out, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !strings.Contains(out, "fp-from-envelope") {
		t.Errorf("expected envelope-decoded output to include fp-from-envelope: %s", out)
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
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "alert_42"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"fingerprint":"fp"}`))
	if err != nil {
		t.Fatal(err)
	}
	if fc.capturedChatID != "alert_42" {
		t.Errorf("chat_id = %q, want alert_42", fc.capturedChatID)
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
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "alert_1"})
	out, _ := tool.Execute(ctx, json.RawMessage(`{"fingerprint":"fp"}`))
	if strings.Contains(out, `"ok":true`) {
		t.Errorf("expected failure envelope, got %s", out)
	}
	if !strings.Contains(out, "alga auth error") {
		t.Errorf("expected alga auth error in envelope, got %s", out)
	}
}

// TestAlgaToolCapabilityGating verifies the command-required tools are
// filtered out for investigate-only agents.
func TestAlgaToolCapabilityGating(t *testing.T) {
	reg := NewRegistry()
	RegisterAlgaTools(reg, &fakeAlgaClient{})

	investigateOnly := reg.ListForCapabilities([]string{"investigate"})
	for _, tool := range investigateOnly {
		if tool.Name() == "alga_resolve_incident" || tool.Name() == "alga_mitigate_incident" ||
			tool.Name() == "alga_add_incident_timeline" {
			t.Errorf("investigate-only agent should not see %s", tool.Name())
		}
	}

	// Commander should see all the command tools.
	commander := reg.ListForCapabilities([]string{"investigate", "command"})
	names := map[string]bool{}
	for _, tool := range commander {
		names[tool.Name()] = true
	}
	if !names["alga_resolve_incident"] || !names["alga_mitigate_incident"] {
		t.Errorf("commander should see resolve_incident and mitigate_incident")
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
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "alert_1"})
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
	if sawChat != "alert_1" {
		t.Errorf("chat_id = %q", sawChat)
	}
	if !strings.HasPrefix(sawIdem, "alga-") {
		t.Errorf("Idempotency-Key = %q, want prefix alga-", sawIdem)
	}
}

// ensure errors import is used.
var _ = errors.New
