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

// stubScopeICSStore serves a scripted GetAllRoles list and captures the
// UpdateRoleScope call; AssignRole and every other method panic via the nil
// embedded interface — reaching them would mean the handler re-inserts
// instead of updating in place (the bug).
type stubScopeICSStore struct {
	store.ICSRoleStore
	roles        []store.ICSRoleRecord
	updatedID    uuid.UUID
	updatedScope *string
	updateCalled bool
	updateErr    error
}

func (s *stubScopeICSStore) GetAllRoles(_ context.Context, _ int64) ([]store.ICSRoleRecord, error) {
	return s.roles, nil
}

func (s *stubScopeICSStore) UpdateRoleScope(_ context.Context, assignmentID uuid.UUID, scope *string) error {
	s.updateCalled = true
	s.updatedID = assignmentID
	s.updatedScope = scope
	return s.updateErr
}

// TestHandleUpdateICSRoleUpdatesScopeInPlace pins PATCH scope updates
// the active assignment in place (never re-inserts), audits the transition,
// and maps a missing/ended assignment to 404.
func TestHandleUpdateICSRoleUpdatesScopeInPlace(t *testing.T) {
	t.Parallel()

	roleID := uuid.New()
	userID := uuid.New()
	run := func(st *stubScopeICSStore, roleID uuid.UUID, body string) *httptest.ResponseRecorder {
		s := &Server{
			cfg:           &config.Config{},
			incidentStore: &stubTimelineIncidentStore{},
			icsRoleStore:  st,
			auditStore:    &recordingAuditStore{},
			ipExtractor:   newIPExtractor(&config.Config{}),
		}
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/incidents/1/ics/roles/"+roleID.String(), strings.NewReader(body))
		req = req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{ID: userID, Role: "admin"}))
		w := httptest.NewRecorder()
		s.handleUpdateICSRole(w, req, "1", roleID)
		return w
	}

	t.Run("scope updated in place and audited", func(t *testing.T) {
		t.Parallel()
		st := &stubScopeICSStore{roles: []store.ICSRoleRecord{{
			ID:       roleID,
			RoleType: "incident_commander",
			UserID:   &userID,
			Status:   "active",
		}}}
		w := run(st, roleID, `{"scope_description":"contain in us-east-1"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		if !st.updateCalled {
			t.Fatal("UpdateRoleScope not called")
		}
		if st.updatedID != roleID {
			t.Errorf("updated id = %s, want %s (in-place, no re-assign)", st.updatedID, roleID)
		}
		if st.updatedScope == nil || *st.updatedScope != "contain in us-east-1" {
			t.Errorf("scope = %v, want the request scope", st.updatedScope)
		}
	})

	t.Run("ended assignment maps to 404", func(t *testing.T) {
		t.Parallel()
		st := &stubScopeICSStore{
			roles:     []store.ICSRoleRecord{{ID: roleID, RoleType: "incident_commander", UserID: &userID, Status: "active"}},
			updateErr: store.ErrICSRoleNotFound,
		}
		w := run(st, roleID, `{"scope_description":"x"}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body=%s)", w.Code, w.Body.String())
		}
	})

	t.Run("unknown role maps to 404 before the store write", func(t *testing.T) {
		t.Parallel()
		st := &stubScopeICSStore{}
		w := run(st, roleID, `{"scope_description":"x"}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body=%s)", w.Code, w.Body.String())
		}
		if st.updateCalled {
			t.Error("UpdateRoleScope must not run for an unknown role")
		}
	})
}
