package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/config"
	"alga/ics"
	"alga/store"
)

type stubMeetClient struct {
	result *ics.SpaceResult
	err    error
}

func (s *stubMeetClient) CreateSpace(context.Context) (*ics.SpaceResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func newMeetTestServer(t *testing.T, inc *store.IncidentRecord, meet ics.MeetSpaceCreator) *Server {
	t.Helper()
	srv := NewServer(&config.Config{}, nil, nil, nil, nil, nil, noopAuditStore{}, nil, nil, 0, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	srv.SetIncidentStore(&trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{inc.IncidentNumber: inc}})
	if meet != nil {
		srv.SetGoogleMeetClient(meet)
	}
	return srv
}

func operatorReq(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{ID: uuid.New(), Email: "op@example.com", Role: "operator"}))
}

func TestHandleCreateGoogleMeetSuccess(t *testing.T) {
	inc := &store.IncidentRecord{IncidentNumber: 5, Title: "x", Status: "active"}
	srv := newMeetTestServer(t, inc, &stubMeetClient{result: &ics.SpaceResult{SpaceName: "spaces/s", MeetingURI: "https://meet.google.com/s"}})
	rec := httptest.NewRecorder()
	srv.handleCreateGoogleMeet(rec, operatorReq(http.MethodPost, "/api/v1/incidents/5/google-meet"), "5")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	got, _ := srv.incidentStore.GetIncident(context.Background(), 5)
	if got.GoogleMeetSpaceName != "spaces/s" {
		t.Fatalf("google_meet_space_name = %q", got.GoogleMeetSpaceName)
	}
}

func TestHandleCreateGoogleMeetConflict(t *testing.T) {
	inc := &store.IncidentRecord{IncidentNumber: 5, GoogleMeetSpaceName: "spaces/exists"}
	srv := newMeetTestServer(t, inc, &stubMeetClient{})
	rec := httptest.NewRecorder()
	srv.handleCreateGoogleMeet(rec, operatorReq(http.MethodPost, "/api/v1/incidents/5/google-meet"), "5")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestHandleCreateGoogleMeetUnavailable(t *testing.T) {
	inc := &store.IncidentRecord{IncidentNumber: 5}
	srv := newMeetTestServer(t, inc, nil) // no meet client configured
	rec := httptest.NewRecorder()
	srv.handleCreateGoogleMeet(rec, operatorReq(http.MethodPost, "/api/v1/incidents/5/google-meet"), "5")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleUnlinkGoogleMeet(t *testing.T) {
	inc := &store.IncidentRecord{IncidentNumber: 5, GoogleMeetSpaceName: "spaces/s", ConferenceURL: "https://meet.google.com/s"}
	srv := newMeetTestServer(t, inc, nil)
	rec := httptest.NewRecorder()
	srv.handleUnlinkGoogleMeet(rec, operatorReq(http.MethodDelete, "/api/v1/incidents/5/google-meet"), "5")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	got, _ := srv.incidentStore.GetIncident(context.Background(), 5)
	if got.GoogleMeetSpaceName != "" {
		t.Fatalf("expected cleared google_meet_space_name, got %q", got.GoogleMeetSpaceName)
	}
}
