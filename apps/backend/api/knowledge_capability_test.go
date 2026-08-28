package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/capability"
	"alga/logger"
	"alga/store"
)

// ---- stubs -----------------------------------------------------------------

type b7KnowledgeStore struct {
	store.KnowledgeStore
	note *store.KnowledgeNote
}

func (k *b7KnowledgeStore) List(ctx context.Context, q store.KnowledgeQuery) ([]store.KnowledgeNote, int64, error) {
	return nil, 0, nil
}

func (k *b7KnowledgeStore) Get(ctx context.Context, id string) (*store.KnowledgeNote, error) {
	if k.note != nil && k.note.ID.String() == id {
		return k.note, nil
	}
	return nil, nil
}

// ---- tests -----------------------------------------------------------------

// TestAgentKnowledgeCapabilityGates covers the matrix for knowledge:
//
//   - GET list + GET by id require `investigate` OR `command`
//   - POST requires `investigate` (prompt-poisoning guard: a communicate-only
//     token must not author content other agents ingest)
func TestAgentKnowledgeCapabilityGates(t *testing.T) {
	t.Parallel()
	logger.Init("error", "")

	s := &Server{knowledgeStore: &b7KnowledgeStore{}}

	reqWithAgent := func(method, target string, caps []string) *http.Request {
		req := httptest.NewRequest(method, target, nil)
		if len(target) > len("/api/v1/agent/knowledge/") {
			req.SetPathValue("id", target[len("/api/v1/agent/knowledge/"):])
		}
		return req.WithContext(platform.WithAgent(req.Context(), &platform.AgentTokenContext{
			ID:           uuid.New(),
			Name:         "probe-agent",
			Capabilities: caps,
		}))
	}

	t.Run("communicate-only denied everywhere", func(t *testing.T) {
		caps := []string{capability.Communicate}

		w := httptest.NewRecorder()
		s.handleAgentKnowledge(w, reqWithAgent(http.MethodGet, "/api/v1/agent/knowledge", caps))
		if w.Code != http.StatusForbidden {
			t.Fatalf("GET status = %d (%s), want 403", w.Code, w.Body.String())
		}

		w = httptest.NewRecorder()
		s.handleAgentKnowledge(w, reqWithAgent(http.MethodPost, "/api/v1/agent/knowledge", caps))
		if w.Code != http.StatusForbidden {
			t.Fatalf("POST status = %d, want 403", w.Code)
		}

		w = httptest.NewRecorder()
		s.handleAgentKnowledgeByID(w, reqWithAgent(http.MethodGet, "/api/v1/agent/knowledge/note-1", caps))
		if w.Code != http.StatusForbidden {
			t.Fatalf("GET by id status = %d, want 403", w.Code)
		}
	})

	t.Run("command-only reads but cannot author", func(t *testing.T) {
		caps := []string{capability.Command}

		w := httptest.NewRecorder()
		s.handleAgentKnowledge(w, reqWithAgent(http.MethodGet, "/api/v1/agent/knowledge", caps))
		if w.Code == http.StatusForbidden {
			t.Fatal("command-only token should be allowed to READ the KB")
		}

		w = httptest.NewRecorder()
		s.handleAgentKnowledge(w, reqWithAgent(http.MethodPost, "/api/v1/agent/knowledge", caps))
		if w.Code != http.StatusForbidden {
			t.Fatalf("POST with command-only status = %d, want 403", w.Code)
		}

		noteID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
		kStore := &b7KnowledgeStore{note: &store.KnowledgeNote{ID: noteID}}
		s2 := &Server{knowledgeStore: kStore}
		w = httptest.NewRecorder()
		s2.handleAgentKnowledgeByID(w, reqWithAgent(http.MethodGet, "/api/v1/agent/knowledge/"+noteID.String(), caps))
		if w.Code != http.StatusOK {
			t.Fatalf("GET by id with command-only status = %d (%s), want 200", w.Code, w.Body.String())
		}
	})
}
