package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/config"
	"alga/store"
)

// stubOverrideTriageStore captures the override patch; every other method
// panics via the nil embedded interface.
type stubOverrideTriageStore struct {
	store.TriageResultStore
	gotID string
	patch *store.TriageResultRecord
}

func (s *stubOverrideTriageStore) Update(_ context.Context, id string, patch *store.TriageResultRecord) (*store.TriageResultRecord, error) {
	s.gotID = id
	s.patch = patch
	out := *patch
	out.ID = uuid.MustParse(id)
	return &out, nil
}

// TestHandleTriageResultOverridePersistsReason pins WP-A15: the override
// endpoint persists the reason it accepts (previously dropped), records who
// and when overrode, and audits the state transition. An unknown decision is
// rejected with 400 before touching the store.
func TestHandleTriageResultOverridePersistsReason(t *testing.T) {
	t.Parallel()

	resultID := uuid.NewString()
	userID := uuid.New()
	run := func(st *stubOverrideTriageStore, body string) *httptest.ResponseRecorder {
		s := &Server{
			cfg:               &config.Config{},
			triageResultStore: st,
			auditStore:        &recordingAuditStore{},
			ipExtractor:       newIPExtractor(&config.Config{}),
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/triage/results/"+resultID, strings.NewReader(body))
		req = req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{ID: userID, Role: "admin"}))
		w := httptest.NewRecorder()
		s.handleTriageResultByID(w, req)
		return w
	}

	t.Run("reason and override attribution reach the store", func(t *testing.T) {
		t.Parallel()
		st := &stubOverrideTriageStore{}
		w := run(st, `{"decision":"suppress","reason":"known-flaky probe"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		if st.gotID != resultID {
			t.Errorf("updated id = %q, want %q", st.gotID, resultID)
		}
		if st.patch == nil {
			t.Fatal("store.Update not called")
		}
		if st.patch.Outcome != store.TriageResultOutcomeOverridden {
			t.Errorf("outcome = %q, want %q", st.patch.Outcome, store.TriageResultOutcomeOverridden)
		}
		if st.patch.OverriddenTo != store.TriageDecisionSuppress {
			t.Errorf("overridden_to = %q, want %q", st.patch.OverriddenTo, store.TriageDecisionSuppress)
		}
		if st.patch.OverrideReason != "known-flaky probe" {
			t.Errorf("override_reason = %q, want the request reason", st.patch.OverrideReason)
		}
		if st.patch.OverriddenBy == nil || *st.patch.OverriddenBy != userID {
			t.Errorf("overridden_by = %v, want the authenticated user", st.patch.OverriddenBy)
		}
		if st.patch.OverriddenAt == nil {
			t.Error("overridden_at not set")
		}
	})

	t.Run("override is audited", func(t *testing.T) {
		t.Parallel()
		st := &stubOverrideTriageStore{}
		audits := &recordingAuditStore{}
		s := &Server{
			cfg:               &config.Config{},
			triageResultStore: st,
			auditStore:        audits,
			ipExtractor:       newIPExtractor(&config.Config{}),
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/triage/results/"+resultID, strings.NewReader(`{"decision":"investigate","reason":"false positive"}`))
		req = req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{ID: userID, Role: "admin"}))
		w := httptest.NewRecorder()
		s.handleTriageResultByID(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		found := false
		for _, ev := range audits.events {
			if ev == store.AuditTriageOverridden {
				found = true
			}
		}
		if !found {
			t.Errorf("audit events = %v, want %q", audits.events, store.AuditTriageOverridden)
		}
	})

	t.Run("unknown decision rejected without touching the store", func(t *testing.T) {
		t.Parallel()
		st := &stubOverrideTriageStore{}
		w := run(st, `{"decision":"page-everyone","reason":"panic"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
		}
		if st.patch != nil {
			t.Error("store.Update must not be called for an unknown decision")
		}
	})
}
