package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/capability"
	"alga/logger"
	"alga/memory"
	"alga/store"
)

// ---- stubs -----------------------------------------------------------------

type b7MemoryStore struct {
	rec     *store.AgentMemoryRecord
	deleted []uuid.UUID
}

func (m *b7MemoryStore) Get(ctx context.Context, id uuid.UUID) (*store.AgentMemoryRecord, error) {
	if m.rec != nil && m.rec.ID == id {
		return m.rec, nil
	}
	return nil, nil
}

func (m *b7MemoryStore) List(ctx context.Context, f store.MemoryFilters) ([]store.AgentMemoryRecord, int, error) {
	return nil, 0, nil
}

func (m *b7MemoryStore) CreateMemory(ctx context.Context, input memory.CreateMemoryInput) (*store.AgentMemoryRecord, error) {
	return nil, nil
}

func (m *b7MemoryStore) Delete(ctx context.Context, id uuid.UUID) error {
	m.deleted = append(m.deleted, id)
	return nil
}

type b7AuditStore struct {
	events []store.AuditEvent
	actors []string
}

func (a *b7AuditStore) Log(event store.AuditEvent, userID *uuid.UUID, username, ip, ua string, success bool, details map[string]any) {
	a.events = append(a.events, event)
	a.actors = append(a.actors, username)
}

func (a *b7AuditStore) LogEntity(event store.AuditEvent, userID *uuid.UUID, username, ip, ua string, success bool, details map[string]any, entityType string, entityID *uuid.UUID) {
	a.Log(event, userID, username, ip, ua, success, details)
}

func (a *b7AuditStore) LogRecord(rec store.AuditRecord) {
	a.events = append(a.events, rec.Event)
	a.actors = append(a.actors, rec.Username)
}

func (a *b7AuditStore) Query(filter map[string]any) ([]store.AuditRecord, int64, error) {
	return nil, 0, nil
}

func (a *b7AuditStore) GetRecentEvents(limit int) ([]store.AuditRecord, error) { return nil, nil }

func (a *b7AuditStore) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) { return 0, nil }

func b7Service(mem *b7MemoryStore, audit *b7AuditStore) *Service {
	logger.Init("error", "")
	return &Service{memorySvc: mem, auditStore: audit}
}

func b7Request(method, target string, agentID uuid.UUID, caps []string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	if len(target) > len("/api/v1/agent/memories/") {
		req.SetPathValue("id", target[len("/api/v1/agent/memories/"):])
	}
	return req.WithContext(platform.WithAgent(context.Background(), &platform.AgentTokenContext{
		ID:           agentID,
		Name:         "probe-agent",
		Capabilities: caps,
	}))
}

// ---- tests -----------------------------------------------------------------

// TestB7AgentMemoryCapabilityGates covers the WP-B7 matrix:
//
//   - GET requires investigate OR command
//   - POST/DELETE require investigate
//   - DELETE of a memory owned by another agent → 403
//   - DELETE of an owner-less (nil AgentID) memory → ALWAYS 403
//   - permitted delete writes an audit row attributed to the agent
func TestB7AgentMemoryCapabilityGates(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	memoryID := uuid.New()

	newHarness := func(rec *store.AgentMemoryRecord) (*Service, *b7MemoryStore, *b7AuditStore) {
		mem := &b7MemoryStore{rec: rec}
		audit := &b7AuditStore{}
		return b7Service(mem, audit), mem, audit
	}

	foreignID := uuid.New()
	ownMemory := &store.AgentMemoryRecord{ID: memoryID, AgentID: &ownerID}
	foreignMemory := &store.AgentMemoryRecord{ID: memoryID, AgentID: &foreignID}
	ownerLessMemory := &store.AgentMemoryRecord{ID: memoryID, AgentID: nil}

	t.Run("communicate-only token denied on read and write", func(t *testing.T) {
		svc, mem, _ := newHarness(ownMemory)

		w := httptest.NewRecorder()
		svc.handleAgentMemories(w, b7Request(http.MethodGet, "/api/v1/agent/memories", ownerID, []string{capability.Communicate}))
		if w.Code != http.StatusForbidden {
			t.Fatalf("GET status = %d (%s), want 403", w.Code, w.Body.String())
		}

		w = httptest.NewRecorder()
		svc.handleAgentMemories(w, b7Request(http.MethodPost, "/api/v1/agent/memories", ownerID, []string{capability.Communicate}))
		if w.Code != http.StatusForbidden {
			t.Fatalf("POST status = %d, want 403", w.Code)
		}

		if mem.deleted != nil {
			t.Fatal("denied token reached the store")
		}
	})

	t.Run("command-only token reads but cannot author", func(t *testing.T) {
		svc, _, _ := newHarness(ownMemory)

		w := httptest.NewRecorder()
		svc.handleAgentMemories(w, b7Request(http.MethodGet, "/api/v1/agent/memories", ownerID, []string{capability.Command}))
		if w.Code == http.StatusForbidden {
			t.Fatal("command-only token should be allowed to READ")
		}

		w = httptest.NewRecorder()
		svc.handleAgentMemories(w, b7Request(http.MethodPost, "/api/v1/agent/memories", ownerID, []string{capability.Command}))
		if w.Code != http.StatusForbidden {
			t.Fatalf("POST with command-only status = %d, want 403", w.Code)
		}
	})

	t.Run("investigate deletes own but not foreign or owner-less", func(t *testing.T) {
		caps := []string{capability.Investigate}
		target := "/api/v1/agent/memories/" + memoryID.String()

		svc, _, audit := newHarness(ownMemory)
		w := httptest.NewRecorder()
		svc.handleAgentMemoryByID(w, b7Request(http.MethodDelete, target, ownerID, caps))
		if w.Code != http.StatusOK {
			t.Fatalf("own-memory delete status = %d (%s), want 200", w.Code, w.Body.String())
		}
		if len(audit.events) != 1 || audit.events[0] != store.AuditMemoryDeleted {
			t.Fatalf("audit events = %v, want [memory_deleted]", audit.events)
		}
		if len(audit.actors) != 1 || audit.actors[0] != "agent:probe-agent" {
			t.Fatalf("audit actor = %v, want agent attribution", audit.actors)
		}

		svc, mem, _ := newHarness(foreignMemory)
		w = httptest.NewRecorder()
		svc.handleAgentMemoryByID(w, b7Request(http.MethodDelete, target, ownerID, caps))
		if w.Code != http.StatusForbidden {
			t.Fatalf("foreign delete status = %d, want 403", w.Code)
		}
		if mem.deleted != nil {
			t.Fatal("foreign memory was deleted")
		}

		svc, mem, _ = newHarness(ownerLessMemory)
		w = httptest.NewRecorder()
		svc.handleAgentMemoryByID(w, b7Request(http.MethodDelete, target, ownerID, caps))
		if w.Code != http.StatusForbidden {
			t.Fatalf("owner-less delete status = %d, want 403", w.Code)
		}
		if mem.deleted != nil {
			t.Fatal("owner-less memory was deleted by an agent")
		}
	})
}
