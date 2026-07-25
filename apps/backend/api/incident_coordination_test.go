package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/api/platform"
	"alga/capability"
	"alga/config"
	"alga/ics"
	"alga/sse"
	"alga/store"
)

type fakeIncidentCoordinationStore struct {
	messages []store.IncidentCoordinationMessageRecord
}

func (f *fakeIncidentCoordinationStore) CreateMessage(_ context.Context, record *store.IncidentCoordinationMessageRecord) (*store.IncidentCoordinationMessageRecord, error) {
	record.ID = uuid.New()
	f.messages = append(f.messages, *record)
	return record, nil
}

func (f *fakeIncidentCoordinationStore) ListMessages(_ context.Context, incidentNumber int64, _ int, _ int) ([]store.IncidentCoordinationMessageRecord, error) {
	out := make([]store.IncidentCoordinationMessageRecord, 0, len(f.messages))
	for _, msg := range f.messages {
		if msg.IncidentNumber == incidentNumber {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (f *fakeIncidentCoordinationStore) FindByProviderMessageID(_ context.Context, _ string) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeIncidentCoordinationStore) SetSlackMessageTS(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}

func (f *fakeIncidentCoordinationStore) ListMessagesByKind(_ context.Context, incidentNumber int64, kind string, _ int, _ int) ([]store.IncidentCoordinationMessageRecord, error) {
	out := make([]store.IncidentCoordinationMessageRecord, 0, len(f.messages))
	for _, msg := range f.messages {
		if msg.IncidentNumber == incidentNumber && msg.Kind == kind {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (f *fakeIncidentCoordinationStore) NewestStatusUpdate(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeIncidentCoordinationStore) NewestAgentCoordinationReply(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeIncidentCoordinationStore) CreateStatusUpdate(_ context.Context, incidentNumber int64, statusLevel string, body string, internal bool, actorID uuid.UUID, actorDisplayName string) (*store.IncidentCoordinationMessageRecord, error) {
	return f.CreateMessage(context.Background(), &store.IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             store.IncidentCoordinationKindStatusUpdate,
		ActorType:        store.IncidentCoordinationActorUser,
		ActorID:          &actorID,
		ActorDisplayName: actorDisplayName,
		Body:             body,
		Internal:         internal,
		Source:           store.IncidentCoordinationSourceAlga,
		Metadata:         map[string]any{"status_level": statusLevel},
	})
}

type noopAuditStore struct{}

func (noopAuditStore) Log(_ store.AuditEvent, _ *uuid.UUID, _, _, _ string, _ bool, _ map[string]any) {
}
func (noopAuditStore) LogEntity(_ store.AuditEvent, _ *uuid.UUID, _, _, _ string, _ bool, _ map[string]any, _ string, _ *uuid.UUID) {
}
func (noopAuditStore) Query(_ map[string]any) ([]store.AuditRecord, error) { return nil, nil }
func (noopAuditStore) GetRecentEvents(_ int) ([]store.AuditRecord, error)  { return nil, nil }

func TestIncidentCoordinationMessagesRequireMessage(t *testing.T) {
	server := NewServer(&config.Config{}, nil, nil, nil, nil, nil, noopAuditStore{}, nil, nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	server.SetIncidentCoordinationStore(&fakeIncidentCoordinationStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/1/coordination/messages", bytes.NewBufferString(`{"body":""}`))
	req = req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{ID: uuid.New(), Email: "operator@example.com", Role: "operator"}))
	rec := httptest.NewRecorder()

	server.handleIncidentCoordinationMessages(rec, req, "1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestIncidentCoordinationMessagesCreateAndList(t *testing.T) {
	coord := &fakeIncidentCoordinationStore{}
	server := NewServer(&config.Config{}, nil, nil, nil, nil, nil, noopAuditStore{}, nil, nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	server.SetIncidentCoordinationStore(coord)

	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/1/coordination/messages", strings.NewReader(`{"body":"Bridge update posted","kind":"chat"}`))
	postReq = postReq.WithContext(platform.WithUser(postReq.Context(), &store.UserRecord{ID: uuid.New(), Email: "operator@example.com", Role: "operator"}))
	postRec := httptest.NewRecorder()
	server.handleIncidentCoordinationMessages(postRec, postReq, "1")
	if postRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/1/coordination/messages", nil)
	getReq = getReq.WithContext(platform.WithUser(getReq.Context(), &store.UserRecord{ID: uuid.New(), Email: "viewer@example.com", Role: "viewer"}))
	getRec := httptest.NewRecorder()
	server.handleIncidentCoordinationMessages(getRec, getReq, "1")
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Bridge update posted") {
		t.Fatalf("expected response to contain message, got %s", getRec.Body.String())
	}
}

func TestIncidentCoordinationMessageStoresMentions(t *testing.T) {
	coord := &fakeIncidentCoordinationStore{}
	server := NewServer(&config.Config{}, nil, nil, nil, nil, nil, noopAuditStore{}, nil, nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	server.SetIncidentCoordinationStore(coord)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/1/coordination/messages", strings.NewReader(`{"body":"@agent summarize impact","mentions":["agent:agent-1"]}`))
	req = req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{ID: uuid.New(), Email: "operator@example.com", Role: "operator"}))
	rec := httptest.NewRecorder()

	server.handleIncidentCoordinationMessages(rec, req, "1")

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	mentions, ok := coord.messages[0].Metadata["mentions"].([]string)
	if !ok || len(mentions) != 1 || mentions[0] != "agent:agent-1" {
		t.Fatalf("expected mention metadata, got %#v", coord.messages[0].Metadata)
	}
}

func TestIncidentCoordinationMessageForwardsExplicitAgentMention(t *testing.T) {
	incidentID := "1"
	assignedAgentID := uuid.NewString()
	unassignedAgentID := uuid.NewString()
	coord := &fakeIncidentCoordinationStore{}
	broker := sse.NewBroker()
	assignedCh := make(chan sse.Event, 1)
	unassignedCh := make(chan sse.Event, 1)
	broker.SubscribeAgent(assignedAgentID, "assigned-client", assignedCh)
	broker.SubscribeAgent(unassignedAgentID, "unassigned-client", unassignedCh)

	server := NewServer(&config.Config{}, nil, nil, nil, nil, nil, noopAuditStore{}, nil, nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	server.SetIncidentCoordinationStore(coord)
	server.SetAgentSSE(agent.NewAgentSSEHandler(broker, nil, nil, nil, nil))
	server.incidentInvestigationStore = &spyIncidentInvestigationStore{list: []store.IncidentInvestigationRecord{
		{IncidentNumber: mustParseIncidentNumber(incidentID), AgentID: assignedAgentID, Status: store.IncidentInvestigationStatusInvestigating},
	}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/coordination/messages", strings.NewReader(`{"body":"sensitive incident update","mentions":["agent:`+unassignedAgentID+`"]}`))
	req = req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{ID: uuid.New(), Email: "operator@example.com", Role: "operator"}))
	rec := httptest.NewRecorder()

	server.handleIncidentCoordinationMessages(rec, req, incidentID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case event := <-assignedCh:
		data, ok := event.Data.(map[string]any)
		if !ok {
			t.Fatalf("assigned event data = %#v, want map[string]any", event.Data)
		}
		if data["trigger"] != "observe" {
			t.Fatalf("assigned trigger = %#v, want observe", data["trigger"])
		}
	case <-time.After(time.Second):
		t.Fatal("expected assigned incident investigation agent to receive observe event")
	}
	select {
	case event := <-unassignedCh:
		data, ok := event.Data.(map[string]any)
		if !ok {
			t.Fatalf("mentioned event data = %#v, want map[string]any", event.Data)
		}
		if data["trigger"] != "mention" {
			t.Fatalf("mentioned trigger = %#v, want mention", data["trigger"])
		}
	case <-time.After(time.Second):
		t.Fatal("expected explicitly mentioned agent to receive mention event")
	}
}

func TestCreateInvestigationSummaryCoordinationMessage(t *testing.T) {
	coord := &fakeIncidentCoordinationStore{}
	server := NewServer(&config.Config{}, nil, nil, nil, nil, nil, noopAuditStore{}, nil, nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	server.SetIncidentCoordinationStore(coord)

	created, err := server.createInvestigationSummaryCoordinationMessage(context.Background(), "1", "inv_1", "Database failover completed")
	if err != nil {
		t.Fatalf("create summary: %v", err)
	}
	if created.Kind != store.IncidentCoordinationKindInvestigationSummary || created.LinkedInvestigationID != "inv_1" {
		t.Fatalf("unexpected summary message: %#v", created)
	}
}

func TestCoordinationAgentRecipientsIncludesExplicitAgentMention(t *testing.T) {
	randomUUID := uuid.NewString()
	recipients := coordinationAgentRecipients(
		nil,
		nil,
		[]string{"agent:" + randomUUID},
		"",
	)
	if len(recipients) != 1 || recipients[0].AgentID != randomUUID || recipients[0].Trigger != "mention" {
		t.Fatalf("recipients = %+v, want explicit mentioned agent %s", recipients, randomUUID)
	}
}

func TestCoordinationAgentRecipientsIgnoreTerminalInvestigations(t *testing.T) {
	activeAgentID := uuid.NewString()
	completeAgentID := uuid.NewString()
	cancelledAgentID := uuid.NewString()
	recipients := coordinationAgentRecipients(
		[]store.IncidentInvestigationRecord{
			{AgentID: completeAgentID, Status: store.IncidentInvestigationStatusComplete},
			{AgentID: cancelledAgentID, Status: store.IncidentInvestigationStatusCancelled},
			{AgentID: activeAgentID, Status: store.IncidentInvestigationStatusInvestigating},
		},
		nil,
		nil,
		"",
	)
	if len(recipients) != 1 || recipients[0].AgentID != activeAgentID {
		t.Fatalf("recipients = %+v, want only active investigation agent %s", recipients, activeAgentID)
	}
}

func TestCoordinationAgentRecipientsCarryRoleType(t *testing.T) {
	commanderID := uuid.New()
	responderID := uuid.New()
	recipients := coordinationAgentRecipients(
		[]store.IncidentInvestigationRecord{
			{AgentID: responderID.String(), Status: store.IncidentInvestigationStatusInvestigating},
		},
		[]store.ICSRoleRecord{
			{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
			{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		},
		[]string{"agent:" + commanderID.String()},
		"",
	)
	if len(recipients) != 2 {
		t.Fatalf("expected 2 recipients, got %d: %+v", len(recipients), recipients)
	}
	byAgent := make(map[string]string, len(recipients))
	for _, r := range recipients {
		byAgent[r.AgentID] = r.RoleType
	}
	if byAgent[commanderID.String()] != string(ics.RoleIncidentCommander) {
		t.Fatalf("commander role type = %q, want %q", byAgent[commanderID.String()], string(ics.RoleIncidentCommander))
	}
}

func TestCoordinationMessageEventIncludesIncidentContext(t *testing.T) {
	event := coordinationMessageEvent("incident_coord_22", "resolve this", "user-1", "Admin", "mention", "22", string(ics.RoleIncidentCommander), "active")
	data, ok := event.Data.(map[string]any)
	if !ok {
		t.Fatalf("event data is not map[string]any: %#v", event.Data)
	}
	if data["incident_number"] != "22" {
		t.Fatalf("incident_id = %#v, want 22", data["incident_number"])
	}
	if data["incident_role"] != string(ics.RoleIncidentCommander) {
		t.Fatalf("incident_role = %#v, want %q", data["incident_role"], string(ics.RoleIncidentCommander))
	}
	if data["incident_status"] != "active" {
		t.Fatalf("incident_status = %#v, want active", data["incident_status"])
	}
}

// newAgentTestService wires a *Server with an *agent.Service so the agent
// HTTP handlers can be driven via the registered mux. Mirrors the pattern in
// http_test.go (e.g. TestCommandOnlyCommanderCanResolveIncidentThroughAgentMessages).
// The returned server uses a per-test bearer token that resolves to agentTok.
func newAgentTestService(t *testing.T, agentTok *testAgentTokenStore, incidentStore store.IncidentStore, coordStore store.IncidentCoordinationStore, roleStore store.ICSRoleStore) (*Server, *http.ServeMux) {
	t.Helper()
	srv := NewServer(&config.Config{}, nil, nil, agentTok, nil, nil, noopAuditStore{}, nil, nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	srv.SetIncidentStore(incidentStore)
	if coordStore != nil {
		srv.SetIncidentCoordinationStore(coordStore)
	}
	srv.SetICSRoleStore(roleStore)
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)
	srv.SetAgentService(agent.NewService(
		nil, executor, nil,
		srv.agentTokenStore,
		nil, nil, nil, nil,
		platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil,
		agent.WithICSRoles(roleStore),
		agent.WithAlertStores(nil, incidentStore, coordStore, nil, nil),
	))
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func TestAgentStatusUpdateCreatesCoordinationStatusUpdate(t *testing.T) {
	incidentID := "99"
	agentID := uuid.New()
	agentTok := &testAgentTokenStore{
		validToken:   "communicator-token",
		agentID:      agentID,
		name:         "communicator",
		capabilities: []string{capability.Communicate},
	}
	coordStore := &fakeIncidentCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &agentID, AgentName: "communicator"},
	}}
	_, mux := newAgentTestService(t, agentTok, &trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "active", SlackChannelID: ""},
	}}, coordStore, roleStore)

	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/messages", bytes.NewBufferString(`{"chat_id":"incident_coord_99","kind":"status_update","text":"We are investigating elevated latency for checkout."}`), agentTok.validToken)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rr.Code, rr.Body.String())
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected 1 coordination message, got %d", len(coordStore.messages))
	}
	createdMsg := coordStore.messages[0]
	if createdMsg.Kind != store.IncidentCoordinationKindStatusUpdate {
		t.Fatalf("created coordination message kind = %q, want status_update", createdMsg.Kind)
	}
	if createdMsg.ActorType != store.IncidentCoordinationActorAgent {
		t.Fatalf("actor type = %q, want agent", createdMsg.ActorType)
	}
}

func TestAgentIncidentContextAllowsAssignedCommunicator(t *testing.T) {
	incidentID := "99"
	agentID := uuid.New()
	agentTok := &testAgentTokenStore{
		validToken:   "communicator-token",
		agentID:      agentID,
		name:         "communicator",
		capabilities: []string{capability.Communicate},
	}
	commanderID := uuid.New()
	responderID := uuid.New()
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commanderID, AgentName: "commander"},
		{RoleType: string(ics.RoleCommunicationsLead), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &agentID, AgentName: "communicator"},
		{RoleType: string(ics.RoleResponder), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &responderID, AgentName: "responder"},
	}}
	_, mux := newAgentTestService(t, agentTok, &trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "active"},
	}}, nil, roleStore)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/incidents/"+incidentID, nil, agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body struct {
		Incident store.IncidentRecord `json:"incident"`
		Roles    []struct {
			RoleType string `json:"role_type"`
			AgentID  string `json:"agent_token_id"`
			Name     string `json:"agent_name"`
		} `json:"roles"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if body.Incident.IncidentNumber != mustParseIncidentNumber(incidentID) {
		t.Fatalf("incident number = %d, want %s", body.Incident.IncidentNumber, incidentID)
	}
	wantRoles := map[string]string{
		string(ics.RoleIncidentCommander):  "commander",
		string(ics.RoleCommunicationsLead): "communicator",
		string(ics.RoleResponder):          "responder",
	}
	if len(body.Roles) != len(wantRoles) {
		t.Fatalf("roles = %#v, want %d roles", body.Roles, len(wantRoles))
	}
	for _, role := range body.Roles {
		if wantRoles[role.RoleType] != role.Name || role.AgentID == "" {
			t.Fatalf("unexpected role in response: %#v", role)
		}
		delete(wantRoles, role.RoleType)
	}
	if len(wantRoles) != 0 {
		t.Fatalf("missing roles: %#v", wantRoles)
	}
}

func TestHandleStatusUpdateAllowsCommanderAndResponder(t *testing.T) {
	incidentID := "1"
	agentID := uuid.New()
	agentTok := &testAgentTokenStore{
		validToken:   "responder-token",
		agentID:      agentID,
		name:         "responder",
		capabilities: []string{capability.Communicate},
	}
	coordStore := &fakeIncidentCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &agentID, AgentName: "responder"},
	}}
	_, mux := newAgentTestService(t, agentTok, &trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "active"},
	}}, coordStore, roleStore)

	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/messages", bytes.NewBufferString(`{"chat_id":"incident_coord_1","kind":"status_update","text":"allowed status update"}`), agentTok.validToken)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for responder, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleStatusUpdateRejectsUnauthorizedAgent(t *testing.T) {
	incidentID := "1"
	agentID := uuid.New()
	agentTok := &testAgentTokenStore{
		validToken:   "unauthorized-token",
		agentID:      agentID,
		name:         "unauthorized",
		capabilities: []string{capability.Investigate},
	}
	coordStore := &fakeIncidentCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{}}
	_, mux := newAgentTestService(t, agentTok, &trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "active"},
	}}, coordStore, roleStore)

	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/messages", bytes.NewBufferString(`{"chat_id":"incident_coord_1","kind":"status_update","text":"blocked"}`), agentTok.validToken)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized agent, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCoordinationBareMentionHasNoContentAfterMentions(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		expected bool
	}{
		{"only mention link", "[@wad1D4w](agent:3c3cb236-e409-481d-90e1-1466c1e1bbda)", true},
		{"only mention link with whitespace", "  [@wad1D4w](agent:3c3cb236-e409-481d-90e1-1466c1e1bbda) \n", true},
		{"user mention only", "[@alice](user:abc)", true},
		{"multiple mentions no body", "[@a](agent:1) [@b](agent:2)", true},
		{"substantive body present", "Great work, [@wad1D4w](agent:1). Cluster is healthy.", false},
		{"substantive body with thank-you", "Thanks for the update. Investigating now.", false},
		{"empty", "", false},
		{"whitespace only", "   \n\t  ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coordinationMessageHasNoContentAfterMentions(tc.message); got != tc.expected {
				t.Fatalf("coordinationMessageHasNoContentAfterMentions(%q) = %v, want %v", tc.message, got, tc.expected)
			}
		})
	}
}
