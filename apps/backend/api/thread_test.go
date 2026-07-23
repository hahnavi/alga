package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/capability"
	"alga/config"
	"alga/rabbitmq"
	"alga/routing"
	"alga/store"
)

type memoryInvestigationThreadStore struct {
	mu      sync.Mutex
	threads map[string]*store.InvestigationThreadRecord
}

func newMemoryInvestigationThreadStore() *memoryInvestigationThreadStore {
	return &memoryInvestigationThreadStore{threads: map[string]*store.InvestigationThreadRecord{}}
}

func (m *memoryInvestigationThreadStore) EnsureThread(ctx context.Context, ownerType string, ownerID string) (*store.InvestigationThreadRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := ownerType + ":" + ownerID
	if thread, ok := m.threads[key]; ok {
		return cloneInvestigationThread(thread), nil
	}

	now := time.Now().UTC()
	thread := &store.InvestigationThreadRecord{
		ID:        uuid.New(),
		ThreadID:  uuid.NewString(),
		OwnerType: ownerType,
		OwnerID:   ownerID,
		Messages:  []store.InvestigationThreadMessage{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.threads[key] = thread
	return cloneInvestigationThread(thread), nil
}

func (m *memoryInvestigationThreadStore) GetThreadByOwner(ctx context.Context, ownerType string, ownerID string, limit int64, skip int64) (*store.InvestigationThreadRecord, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	thread, ok := m.threads[ownerType+":"+ownerID]
	if !ok {
		return nil, 0, store.ErrNotFound
	}
	clone := cloneInvestigationThread(thread)
	total := int64(len(clone.Messages))
	if skip > total {
		clone.Messages = []store.InvestigationThreadMessage{}
		return clone, total, nil
	}
	clone.Messages = clone.Messages[int(skip):]
	if limit >= 0 && int64(len(clone.Messages)) > limit {
		clone.Messages = clone.Messages[:int(limit)]
	}
	return clone, total, nil
}

func (m *memoryInvestigationThreadStore) AddMessage(ctx context.Context, threadID string, message store.InvestigationThreadMessage) (*store.InvestigationThreadMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, thread := range m.threads {
		if thread.ThreadID != threadID {
			continue
		}
		now := time.Now().UTC()
		message.ID = uuid.New()
		message.ThreadID = threadID
		message.CreatedAt = now
		message.UpdatedAt = now
		thread.Messages = append(thread.Messages, message)
		thread.UpdatedAt = now
		return &message, nil
	}
	return nil, store.ErrNotFound
}

func (m *memoryInvestigationThreadStore) UpdateMessage(ctx context.Context, ownerType string, ownerID string, messageID string, message string, markEdited bool) (*store.InvestigationThreadMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	thread, ok := m.threads[ownerType+":"+ownerID]
	if !ok {
		return nil, store.ErrNotFound
	}
	for i := range thread.Messages {
		if thread.Messages[i].ID.String() != messageID {
			continue
		}
		now := time.Now().UTC()
		thread.Messages[i].Message = message
		thread.Messages[i].Edited = markEdited
		thread.Messages[i].UpdatedAt = now
		thread.UpdatedAt = now
		msg := thread.Messages[i]
		return &msg, nil
	}
	return nil, store.ErrNotFound
}

func (m *memoryInvestigationThreadStore) DeleteMessage(ctx context.Context, ownerType string, ownerID string, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	thread, ok := m.threads[ownerType+":"+ownerID]
	if !ok {
		return store.ErrNotFound
	}
	for i := range thread.Messages {
		if thread.Messages[i].ID.String() != messageID {
			continue
		}
		thread.Messages = append(thread.Messages[:i], thread.Messages[i+1:]...)
		thread.UpdatedAt = time.Now().UTC()
		return nil
	}
	return store.ErrNotFound
}

func cloneInvestigationThread(thread *store.InvestigationThreadRecord) *store.InvestigationThreadRecord {
	if thread == nil {
		return nil
	}
	clone := *thread
	clone.Messages = slices.Clone(thread.Messages)
	return &clone
}

func newThreadTestServer(threads store.InvestigationThreadStore) (*Server, *http.ServeMux) {
	if threads == nil {
		threads = newMemoryInvestigationThreadStore()
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		threads,
		nil,
		nil,
		nil,
	)
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func TestAlertThreadRoutesAreOwnerScoped(t *testing.T) {
	_, mux := newThreadTestServer(newMemoryInvestigationThreadStore())

	postReq := authRequest(http.MethodPost, "/api/v1/alerts/42/thread/messages", bytes.NewBufferString(`{"message":"checking alert"}`))
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("POST thread message: expected 201, got %d, body: %s", postRec.Code, postRec.Body.String())
	}

	getReq := authRequest(http.MethodGet, "/api/v1/alerts/42/thread", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET thread: expected 200, got %d, body: %s", getRec.Code, getRec.Body.String())
	}

	var got struct {
		Items []struct {
			Message string `json:"message"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := decodeResponse(t, getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode thread response: %v", err)
	}
	if got.Total != 1 {
		t.Fatalf("expected total 1, got %d", got.Total)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected one message, got %d", len(got.Items))
	}
	if got.Items[0].Message != "checking alert" {
		t.Fatalf("expected message %q, got %q", "checking alert", got.Items[0].Message)
	}
}

func TestAlertThreadRoutePaginatesMessages(t *testing.T) {
	_, mux := newThreadTestServer(newMemoryInvestigationThreadStore())

	for _, message := range []string{"first", "second", "third"} {
		body, err := json.Marshal(map[string]string{"message": message})
		if err != nil {
			t.Fatalf("marshal message: %v", err)
		}
		req := authRequest(http.MethodPost, "/api/v1/alerts/42/thread/messages", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST thread message: expected 201, got %d, body: %s", rec.Code, rec.Body.String())
		}
	}

	req := authRequest(http.MethodGet, "/api/v1/alerts/42/thread?limit=2&skip=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET thread: expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Items []struct {
			Message string `json:"message"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode thread response: %v", err)
	}
	if got.Total != 3 {
		t.Fatalf("total = %d, want 3", got.Total)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(got.Items))
	}
	if got.Items[0].Message != "second" || got.Items[1].Message != "third" {
		t.Fatalf("items = %#v, want second/third", got.Items)
	}
}

func TestAgentIncomingMessageToAlertChatOnlyWritesOwnerThread(t *testing.T) {
	threads := newMemoryInvestigationThreadStore()
	agentRec := &store.AgentTokenRecord{ID: uuid.New(), Name: "Hermes"}
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-42": {
				ID:                   uuid.New(),
				AlertInvestigationID: "AINV-42",
				Status:               "investigating",
				AgentID:              agentRec.ID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-42", AlertNumber: 42}},
			},
		},
	}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetThreadStore(threads)

	messageID, err := executor.HandleIncomingMessage(
		agentRec,
		"alert_42",
		"checking alert",
		"",
		"",
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("HandleIncomingMessage: %v", err)
	}
	if messageID == "" {
		t.Fatal("expected thread message id")
	}

	thread, _, err := threads.GetThreadByOwner(context.Background(), store.ThreadOwnerAlert, "42", 50, 0)
	if err != nil {
		t.Fatalf("GetThreadByOwner: %v", err)
	}
	if len(thread.Messages) != 1 {
		t.Fatalf("expected one thread message, got %d", len(thread.Messages))
	}
	if thread.Messages[0].Message != "checking alert" {
		t.Fatalf("message = %q, want %q", thread.Messages[0].Message, "checking alert")
	}
}

func TestAgentEditMessageToAlertChatUpdatesOwnerThread(t *testing.T) {
	threads := newMemoryInvestigationThreadStore()
	thread, err := threads.EnsureThread(context.Background(), store.ThreadOwnerAlert, "42")
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	msg, err := threads.AddMessage(context.Background(), thread.ThreadID, store.InvestigationThreadMessage{
		Type:     "comment",
		Source:   "agent",
		Message:  "before",
		Username: "Hermes",
	})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	agentRec := &store.AgentTokenRecord{ID: uuid.New(), Name: "Hermes"}
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-42": {
				ID:                   uuid.New(),
				AlertInvestigationID: "AINV-42",
				Status:               "investigating",
				AgentID:              agentRec.ID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-42", AlertNumber: 42}},
			},
		},
	}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetThreadStore(threads)

	err = executor.HandleEditMessage("alert_42", msg.ID.String(), "after", agentRec)
	if err != nil {
		t.Fatalf("HandleEditMessage: %v", err)
	}

	got, _, err := threads.GetThreadByOwner(context.Background(), store.ThreadOwnerAlert, "42", 50, 0)
	if err != nil {
		t.Fatalf("GetThreadByOwner: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(got.Messages))
	}
	if got.Messages[0].Message != "after" || got.Messages[0].Edited {
		t.Fatalf("message = %q edited=%v, want after/false (agent edits do not mark edited)", got.Messages[0].Message, got.Messages[0].Edited)
	}
}

func TestAgentDeleteMessageToIncidentChatUpdatesOwnerThread(t *testing.T) {
	threads := newMemoryInvestigationThreadStore()
	thread, err := threads.EnsureThread(context.Background(), store.ThreadOwnerIncidentInvestigation, "1")
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	msg, err := threads.AddMessage(context.Background(), thread.ThreadID, store.InvestigationThreadMessage{
		Type:     "comment",
		Source:   "agent",
		Message:  "remove me",
		Username: "Hermes",
	})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	agentID := uuid.New()
	executor := agent.NewAgentToolExecutor(&mockAlertInvestigationStore{}, nil, nil, nil, nil)
	executor.SetThreadStore(threads)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentNumber: 1,
		AgentID:        agentID.String(),
		Status:         store.IncidentInvestigationStatusInvestigating,
	}})

	err = executor.HandleDeleteMessage("incident_inv_1", msg.ID.String(), &store.AgentTokenRecord{ID: agentID, Name: "Hermes"})
	if err != nil {
		t.Fatalf("HandleDeleteMessage: %v", err)
	}

	got, _, err := threads.GetThreadByOwner(context.Background(), store.ThreadOwnerIncidentInvestigation, "1", 50, 0)
	if err != nil {
		t.Fatalf("GetThreadByOwner: %v", err)
	}
	if len(got.Messages) != 0 {
		t.Fatalf("expected no messages, got %d", len(got.Messages))
	}
}

func TestAgentAlertToolToAlertChatUsesOwnerAlert(t *testing.T) {
	threads := newMemoryInvestigationThreadStore()
	alerts := &mockStore{
		byNumber: map[int64]store.AlertRecord{
			42: {
				Fingerprint: "fp-42",
				AlertNumber: 42,
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighLatency"},
			},
		},
	}
	executor := agent.NewAgentToolExecutor(&mockAlertInvestigationStore{}, nil, nil, nil, nil)
	executor.SetThreadStore(threads)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           uuid.New(),
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_42", Op: "resolve_alert"})
	if !out.Ok {
		t.Fatalf("ExecuteInvTool ok=false error=%q", out.Error)
	}

	thread, _, err := threads.GetThreadByOwner(context.Background(), store.ThreadOwnerAlert, "42", 50, 0)
	if err != nil {
		t.Fatalf("GetThreadByOwner: %v", err)
	}
	if len(thread.Messages) != 1 {
		t.Fatalf("expected one thread message, got %d", len(thread.Messages))
	}
	if thread.Messages[0].Type != "action" || thread.Messages[0].Source != "system" {
		t.Fatalf("message type/source = %s/%s, want action/system", thread.Messages[0].Type, thread.Messages[0].Source)
	}
}

func TestStandaloneInvestigationRoutesAreGone(t *testing.T) {
	_, mux := newThreadTestServer(newMemoryInvestigationThreadStore())

	req := authRequest(http.MethodGet, "/api/v1/investigations", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentIncomingMessagePersistsReplyToMessageID(t *testing.T) {
	threads := newMemoryInvestigationThreadStore()
	thread, err := threads.EnsureThread(context.Background(), store.ThreadOwnerAlert, "42")
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	original, err := threads.AddMessage(context.Background(), thread.ThreadID, store.InvestigationThreadMessage{
		Type:     "comment",
		Source:   "user",
		Message:  "What caused the spike?",
		Username: "Operator",
	})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	agentRec := &store.AgentTokenRecord{ID: uuid.New(), Name: "Hermes"}
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-42": {
				ID:                   uuid.New(),
				AlertInvestigationID: "AINV-42",
				Status:               "investigating",
				AgentID:              agentRec.ID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-42", AlertNumber: 42}},
			},
		},
	}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetThreadStore(threads)

	_, err = executor.HandleIncomingMessage(
		agentRec,
		"alert_42",
		"A memory leak in the worker process.",
		agentRec.ID.String(),
		"Hermes",
		nil,
		original.ID.String(),
	)
	if err != nil {
		t.Fatalf("HandleIncomingMessage: %v", err)
	}

	got, _, err := threads.GetThreadByOwner(context.Background(), store.ThreadOwnerAlert, "42", 50, 0)
	if err != nil {
		t.Fatalf("GetThreadByOwner: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("expected two thread messages, got %d", len(got.Messages))
	}
	reply := got.Messages[1]
	if reply.ReplyToMessageID != original.ID.String() {
		t.Fatalf("reply reply_to_message_id = %q, want %q", reply.ReplyToMessageID, original.ID.String())
	}
}

func TestResolveReplyToMessage(t *testing.T) {
	msgs := []store.InvestigationThreadMessage{
		{Message: "first", Username: "Alice"},
		{Message: "second", Username: "Bob"},
	}
	s := &Server{}

	text, author := s.resolveReplyToMessage(msgs, "nonexistent-id")
	if text != "" || author != "" {
		t.Fatalf("unknown id should resolve empty, got text=%q author=%q", text, author)
	}

	text, author = s.resolveReplyToMessage(msgs, "")
	if text != "" || author != "" {
		t.Fatalf("empty id should resolve empty, got text=%q author=%q", text, author)
	}

	// Re-resolve against a known id by reconstructing it: the memory store assigns
	// UUIDs at creation, so validate against a fresh store instead.
	threads := newMemoryInvestigationThreadStore()
	thread, err := threads.EnsureThread(context.Background(), store.ThreadOwnerAlert, "7")
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	created, err := threads.AddMessage(context.Background(), thread.ThreadID, store.InvestigationThreadMessage{
		Message: "reply body", Username: "Carol",
	})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	stored, _, err := threads.GetThreadByOwner(context.Background(), store.ThreadOwnerAlert, "7", 50, 0)
	if err != nil {
		t.Fatalf("GetThreadByOwner: %v", err)
	}
	text, author = s.resolveReplyToMessage(stored.Messages, created.ID.String())
	if text != "reply body" || author != "Carol" {
		t.Fatalf("resolve = %q/%q, want reply body/Carol", text, author)
	}
}
