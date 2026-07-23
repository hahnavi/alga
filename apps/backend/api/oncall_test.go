package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alga/config"
	"alga/routing"
	"alga/store"

	"github.com/google/uuid"
)

// mockOnCallStore is a minimal store.OnCallStore for on-call handler tests.
// Only GetSchedule is populated; UpdateSchedule/CreateOverride echo back.
type mockOnCallStore struct {
	schedule *store.OnCallScheduleRecord
	updated  *store.OnCallScheduleRecord
	override *store.ScheduleOverrideRecord
}

func (m *mockOnCallStore) CreateSchedule(_ context.Context, record *store.OnCallScheduleRecord) (*store.OnCallScheduleRecord, error) {
	return record, nil
}
func (m *mockOnCallStore) GetSchedule(_ context.Context, _ uuid.UUID) (*store.OnCallScheduleRecord, error) {
	return m.schedule, nil
}
func (m *mockOnCallStore) GetScheduleByTeam(_ context.Context, _ uuid.UUID) (*store.OnCallScheduleRecord, error) {
	return m.schedule, nil
}
func (m *mockOnCallStore) UpdateSchedule(_ context.Context, _ uuid.UUID, record *store.OnCallScheduleRecord) (*store.OnCallScheduleRecord, error) {
	m.updated = record
	return record, nil
}
func (m *mockOnCallStore) ListSchedules(_ context.Context, _, _ int) ([]store.OnCallScheduleRecord, int64, error) {
	return nil, 0, nil
}
func (m *mockOnCallStore) CreateOverride(_ context.Context, record *store.ScheduleOverrideRecord) (*store.ScheduleOverrideRecord, error) {
	m.override = record
	return record, nil
}
func (m *mockOnCallStore) DeleteOverride(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockOnCallStore) ListOverrides(_ context.Context, _ uuid.UUID) ([]store.ScheduleOverrideRecord, error) {
	return nil, nil
}

// mockTeamStore is a minimal store.TeamStore; only GetMembers is meaningful.
type mockTeamStore struct {
	members []store.TeamMemberRecord
}

func (m *mockTeamStore) CreateTeam(_ context.Context, record *store.TeamRecord) (*store.TeamRecord, error) {
	return record, nil
}
func (m *mockTeamStore) GetTeam(_ context.Context, _ uuid.UUID) (*store.TeamRecord, error) {
	return nil, nil
}
func (m *mockTeamStore) GetTeamByName(_ context.Context, _ string) (*store.TeamRecord, error) {
	return nil, nil
}
func (m *mockTeamStore) GetTeamName(_ context.Context, id uuid.UUID) (string, error) {
	return "Test Team", nil
}
func (m *mockTeamStore) UpdateTeam(_ context.Context, _ uuid.UUID, record *store.TeamRecord) (*store.TeamRecord, error) {
	return record, nil
}
func (m *mockTeamStore) DeleteTeam(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockTeamStore) ListTeams(_ context.Context, _, _ int) ([]store.TeamRecord, int64, error) {
	return nil, 0, nil
}
func (m *mockTeamStore) AddMember(_ context.Context, _, _ uuid.UUID, _ string) (*store.TeamMemberRecord, error) {
	return &store.TeamMemberRecord{}, nil
}
func (m *mockTeamStore) UpdateMemberRole(_ context.Context, _, _ uuid.UUID, _ string) error {
	return nil
}
func (m *mockTeamStore) RemoveMember(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockTeamStore) GetMembers(_ context.Context, _ uuid.UUID) ([]store.TeamMemberRecord, error) {
	return m.members, nil
}
func (m *mockTeamStore) SeedOpsTeam(_ context.Context, _ uuid.UUID) error { return nil }

func newOnCallTestServer(t *testing.T, oncall store.OnCallStore, team store.TeamStore, users []store.UserRecord) (*Server, *http.ServeMux) {
	t.Helper()
	userStore := &mockUserStore{users: users}
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
		&allowAllLoginLimiter{},
		&allowAllRateLimiter{},
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.SetOnCallStore(oncall)
	if team != nil {
		srv.SetTeamStore(team)
	}
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func TestPatchScheduleRejectsNonTeamMember(t *testing.T) {
	teamID := uuid.New()
	scheduleID := uuid.New()
	memberUser := store.UserRecord{ID: uuid.New(), Email: "member@alga.local", Phone: "+15550001", Role: "responder"}
	outsider := store.UserRecord{ID: uuid.New(), Email: "outsider@alga.local", Phone: "+15550002", Role: "responder"}
	oncall := &mockOnCallStore{schedule: &store.OnCallScheduleRecord{ID: scheduleID, TeamID: &teamID}}
	team := &mockTeamStore{members: []store.TeamMemberRecord{{UserID: memberUser.ID, TeamID: teamID, Role: "member"}}}
	_, mux := newOnCallTestServer(t, oncall, team, []store.UserRecord{testAdminUser, memberUser, outsider})

	body := bytes.NewBufferString(`{"layers":[{"name":"L1","rotation_type":"weekly","start_date":"2026-01-01T00:00:00Z","user_ids":["` + outsider.ID.String() + `"]}]}`)
	req := authRequest(http.MethodPatch, "/api/v1/on-call/schedules/"+scheduleID.String(), body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-member in rotation, got %d: %s", rec.Code, rec.Body.String())
	}
	if oncall.updated != nil {
		t.Fatalf("expected schedule not to be updated when validation fails")
	}
}

func TestPatchScheduleAcceptsTeamMember(t *testing.T) {
	teamID := uuid.New()
	scheduleID := uuid.New()
	memberUser := store.UserRecord{ID: uuid.New(), Email: "member@alga.local", Phone: "+15550001", Role: "responder"}
	oncall := &mockOnCallStore{schedule: &store.OnCallScheduleRecord{ID: scheduleID, TeamID: &teamID}}
	team := &mockTeamStore{members: []store.TeamMemberRecord{{UserID: memberUser.ID, TeamID: teamID, Role: "member"}}}
	_, mux := newOnCallTestServer(t, oncall, team, []store.UserRecord{testAdminUser, memberUser})

	body := bytes.NewBufferString(`{"layers":[{"name":"L1","rotation_type":"weekly","start_date":"2026-01-01T00:00:00Z","user_ids":["` + memberUser.ID.String() + `"]}]}`)
	req := authRequest(http.MethodPatch, "/api/v1/on-call/schedules/"+scheduleID.String(), body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for team member in rotation, got %d: %s", rec.Code, rec.Body.String())
	}
	if oncall.updated == nil || len(oncall.updated.Layers) != 1 {
		t.Fatalf("expected schedule updated with one layer")
	}
}

func TestCreateOverrideRejectsNonTeamMember(t *testing.T) {
	teamID := uuid.New()
	scheduleID := uuid.New()
	memberUser := store.UserRecord{ID: uuid.New(), Email: "member@alga.local", Phone: "+15550001", Role: "responder"}
	outsider := store.UserRecord{ID: uuid.New(), Email: "outsider@alga.local", Phone: "+15550002", Role: "responder"}
	oncall := &mockOnCallStore{schedule: &store.OnCallScheduleRecord{ID: scheduleID, TeamID: &teamID}}
	team := &mockTeamStore{members: []store.TeamMemberRecord{{UserID: memberUser.ID, TeamID: teamID, Role: "member"}}}
	_, mux := newOnCallTestServer(t, oncall, team, []store.UserRecord{testAdminUser, memberUser, outsider})

	body := bytes.NewBufferString(`{"user_id":"` + outsider.ID.String() + `","start_at":"2026-01-01T00:00:00Z","end_at":"2026-01-02T00:00:00Z"}`)
	req := authRequest(http.MethodPost, "/api/v1/on-call/schedules/"+scheduleID.String()+"/overrides", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for override on non-member, got %d: %s", rec.Code, rec.Body.String())
	}
	if oncall.override != nil {
		t.Fatalf("expected override not to be created when validation fails")
	}
}

func TestCreateOverrideAcceptsTeamMember(t *testing.T) {
	teamID := uuid.New()
	scheduleID := uuid.New()
	memberUser := store.UserRecord{ID: uuid.New(), Email: "member@alga.local", Phone: "+15550001", Role: "responder"}
	oncall := &mockOnCallStore{schedule: &store.OnCallScheduleRecord{ID: scheduleID, TeamID: &teamID}}
	team := &mockTeamStore{members: []store.TeamMemberRecord{{UserID: memberUser.ID, TeamID: teamID, Role: "member"}}}
	_, mux := newOnCallTestServer(t, oncall, team, []store.UserRecord{testAdminUser, memberUser})

	body := bytes.NewBufferString(`{"user_id":"` + memberUser.ID.String() + `","start_at":"2026-01-01T00:00:00Z","end_at":"2026-01-02T00:00:00Z"}`)
	req := authRequest(http.MethodPost, "/api/v1/on-call/schedules/"+scheduleID.String()+"/overrides", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for override on team member, got %d: %s", rec.Code, rec.Body.String())
	}
}
