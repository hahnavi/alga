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
	"alga/ics"
	"alga/store"
)

// stubEndRoleICSStore records the EndRole reason; every other method panics
// via the nil embedded interface, which is the desired signal if a test
// reaches it.
type stubEndRoleICSStore struct {
	store.ICSRoleStore
	gotReason ics.EndReason
	called    bool
}

func (s *stubEndRoleICSStore) EndRole(_ context.Context, _ uuid.UUID, reason ics.EndReason) error {
	s.gotReason = reason
	s.called = true
	return nil
}

// stubTimelineIncidentStore satisfies only the timeline write that
// handleEndICSRole performs after a successful end; everything else panics.
type stubTimelineIncidentStore struct {
	store.IncidentStore
}

func (s *stubTimelineIncidentStore) AddTimelineEntry(_ context.Context, _ *store.IncidentTimelineEntryRecord) error {
	return nil
}

// TestHandleEndICSRoleValidatesEndedReason pins the edge validation:
// an unknown ended_reason returns 400 before reaching the store (previously it
// surfaced as a CHECK-constraint 500), while the empty default and the enum
// values still pass through.
func TestHandleEndICSRoleValidatesEndedReason(t *testing.T) {
	t.Parallel()

	run := func(st *stubEndRoleICSStore, body string) *httptest.ResponseRecorder {
		s := &Server{
			cfg:           &config.Config{},
			incidentStore: &stubTimelineIncidentStore{},
			icsRoleStore:  st,
			auditStore:    &recordingAuditStore{},
			ipExtractor:   newIPExtractor(&config.Config{}),
		}
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/incidents/1/ics-roles/"+uuid.NewString(), strings.NewReader(body))
		req = req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{Role: "admin"}))
		w := httptest.NewRecorder()
		s.handleEndICSRole(w, req, "1", uuid.New())
		return w
	}

	t.Run("unknown reason rejected with 400 without touching the store", func(t *testing.T) {
		t.Parallel()
		st := &stubEndRoleICSStore{}
		w := run(st, `{"ended_reason":"bogus"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
		}
		if st.called {
			t.Fatal("EndRole must not be called for an unknown reason")
		}
	})

	t.Run("empty reason defaults to replaced", func(t *testing.T) {
		t.Parallel()
		st := &stubEndRoleICSStore{}
		w := run(st, `{}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		if st.gotReason != ics.EndReasonReplaced {
			t.Fatalf("reason = %q, want %q", st.gotReason, ics.EndReasonReplaced)
		}
	})

	t.Run("enum reasons accepted", func(t *testing.T) {
		t.Parallel()
		for _, reason := range []ics.EndReason{ics.EndReasonReplaced, ics.EndReasonIncidentResolved, ics.EndReasonAssigned, ics.EndReasonAgentOffline, ics.EndReasonIncidentClosed} {
			st := &stubEndRoleICSStore{}
			w := run(st, `{"ended_reason":"`+string(reason)+`"}`)
			if w.Code != http.StatusOK {
				t.Fatalf("reason %q: status = %d, want 200 (body=%s)", reason, w.Code, w.Body.String())
			}
			if st.gotReason != reason {
				t.Fatalf("reason = %q, want %q", st.gotReason, reason)
			}
		}
	})
}

// compile-time guards that the stubs satisfy the interfaces
var (
	_ store.ICSRoleStore  = (*stubEndRoleICSStore)(nil)
	_ store.IncidentStore = (*stubTimelineIncidentStore)(nil)
)
