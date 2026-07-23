package incidentchannel

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"alga/store"
)

type fakeUserStore struct {
	users map[uuid.UUID]store.UserRecord
}

func (f *fakeUserStore) CreateUser(email, password, role string) (*store.UserRecord, error) {
	return nil, nil
}
func (f *fakeUserStore) GetByEmail(email string) (*store.UserRecord, error) { return nil, nil }
func (f *fakeUserStore) GetByID(id uuid.UUID) (*store.UserRecord, error) {
	if u, ok := f.users[id]; ok {
		return &u, nil
	}
	return nil, nil
}
func (f *fakeUserStore) ListUsers() ([]store.UserRecord, error)                { return nil, nil }
func (f *fakeUserStore) UpdateUser(id uuid.UUID, updates map[string]any) error { return nil }
func (f *fakeUserStore) DeleteUser(id uuid.UUID) error                         { return nil }
func (f *fakeUserStore) CountAdmins() (int64, error)                           { return 0, nil }
func (f *fakeUserStore) Authenticate(email, password string) (*store.UserRecord, error) {
	return nil, nil
}
func (f *fakeUserStore) RecordFailedLogin(email string) error                    { return nil }
func (f *fakeUserStore) RecordSuccessfulLogin(userID uuid.UUID, ip string) error { return nil }
func (f *fakeUserStore) UnlockAccount(userID uuid.UUID) error                    { return nil }
func (f *fakeUserStore) CountUsers() (int64, error)                              { return int64(len(f.users)), nil }
func (f *fakeUserStore) GetNotificationPreferences(ctx context.Context, userID string) (map[string]any, error) {
	return nil, nil
}
func (f *fakeUserStore) UpdateNotificationPreferences(ctx context.Context, userID string, prefs map[string]any) error {
	return nil
}
func (f *fakeUserStore) GetByGoogleID(googleID string) (*store.UserRecord, error) { return nil, nil }
func (f *fakeUserStore) GetBySlackUserID(slackUserID string) (*store.UserRecord, error) {
	return nil, nil
}
func (f *fakeUserStore) UpdateGoogleID(userID uuid.UUID, googleID string) error { return nil }
func (f *fakeUserStore) ClearGoogleID(userID uuid.UUID) error                   { return nil }
func (f *fakeUserStore) SetSlackIdentity(ctx context.Context, userID uuid.UUID, slackUserID, slackDisplayName string) error {
	return nil
}
func (f *fakeUserStore) ClearSlackIdentity(ctx context.Context, userID uuid.UUID) error { return nil }

type fakeOnCallResolver struct {
	user *uuid.UUID
	err  error
}

func (f *fakeOnCallResolver) ResolveOnCallUser(ctx context.Context, incident *store.IncidentRecord) (*uuid.UUID, error) {
	return f.user, f.err
}

func makeUser(id, slackID string) store.UserRecord {
	return store.UserRecord{ID: makeUUID(id), SlackUserID: slackID}
}

func makeUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

func TestCollectSlackUserIDs_AutoInvitesOnCallHuman(t *testing.T) {
	t.Parallel()
	commander := makeUUID("00000000-0000-0000-0000-000000000001")
	onCall := makeUUID("00000000-0000-0000-0000-000000000002")
	noSlack := makeUUID("00000000-0000-0000-0000-000000000003")

	users := &fakeUserStore{users: map[uuid.UUID]store.UserRecord{
		commander: makeUser("00000000-0000-0000-0000-000000000001", "U_COMMANDER"),
		onCall:    makeUser("00000000-0000-0000-0000-000000000002", "U_ONCALL"),
		noSlack:   {ID: noSlack},
	}}

	m := &Manager{
		userStore:      users,
		onCallResolver: &fakeOnCallResolver{user: &onCall},
	}

	incident := &store.IncidentRecord{
		IncidentNumber:    42,
		CommanderID:       &commander,
		OnCallResponderID: &noSlack,
	}

	got := m.collectSlackUserIDs(context.Background(), incident)
	want := map[string]bool{"U_COMMANDER": true, "U_ONCALL": true}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected slack id %q in %v", id, got)
		}
	}
}

func TestCollectSlackUserIDs_DedupesOnCallAndCommander(t *testing.T) {
	t.Parallel()
	commander := makeUUID("00000000-0000-0000-0000-000000000001")
	users := &fakeUserStore{users: map[uuid.UUID]store.UserRecord{
		commander: makeUser("00000000-0000-0000-0000-000000000001", "U_COMMANDER"),
	}}
	m := &Manager{
		userStore:      users,
		onCallResolver: &fakeOnCallResolver{user: &commander},
	}
	incident := &store.IncidentRecord{CommanderID: &commander}

	got := m.collectSlackUserIDs(context.Background(), incident)
	if len(got) != 1 || got[0] != "U_COMMANDER" {
		t.Fatalf("expected single U_COMMANDER, got %v", got)
	}
}

func TestCollectSlackUserIDs_ResolvesOnCallWithoutRoles(t *testing.T) {
	t.Parallel()
	onCall := makeUUID("00000000-0000-0000-0000-000000000009")
	users := &fakeUserStore{users: map[uuid.UUID]store.UserRecord{
		onCall: makeUser("00000000-0000-0000-0000-000000000009", "U_ONCALL"),
	}}
	m := &Manager{
		userStore:      users,
		onCallResolver: &fakeOnCallResolver{user: &onCall},
	}
	incident := &store.IncidentRecord{IncidentNumber: 7}

	got := m.collectSlackUserIDs(context.Background(), incident)
	if len(got) != 1 || got[0] != "U_ONCALL" {
		t.Fatalf("expected on-call human invited, got %v", got)
	}
}

func TestCollectSlackUserIDs_NoResolverOrErrorOnlyRoleUsers(t *testing.T) {
	t.Parallel()
	commander := makeUUID("00000000-0000-0000-0000-000000000001")
	users := &fakeUserStore{users: map[uuid.UUID]store.UserRecord{
		commander: makeUser("00000000-0000-0000-0000-000000000001", "U_COMMANDER"),
	}}

	incident := &store.IncidentRecord{CommanderID: &commander, IncidentNumber: 1}

	noResolver := &Manager{userStore: users}
	if got := noResolver.collectSlackUserIDs(context.Background(), incident); len(got) != 1 || got[0] != "U_COMMANDER" {
		t.Fatalf("no resolver: expected [U_COMMANDER], got %v", got)
	}

	errResolver := &Manager{userStore: users, onCallResolver: &fakeOnCallResolver{err: errors.New("boom")}}
	if got := errResolver.collectSlackUserIDs(context.Background(), incident); len(got) != 1 || got[0] != "U_COMMANDER" {
		t.Fatalf("resolver error: expected [U_COMMANDER], got %v", got)
	}
}

func TestGenerateChannelName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		incident *store.IncidentRecord
		want     string
	}{
		{
			name:     "title and number",
			incident: &store.IncidentRecord{IncidentNumber: 1234, Title: "Database Outage"},
			want:     "inc-1234-database-outage",
		},
		{
			name:     "title with special characters",
			incident: &store.IncidentRecord{IncidentNumber: 7, Title: "API Latency!! (us-east)"},
			want:     "inc-7-api-latency-us-east",
		},
		{
			name:     "long title is truncated",
			incident: &store.IncidentRecord{IncidentNumber: 1, Title: "this is a very long incident title that should be trimmed"},
			want:     "inc-1-this-is-a-very-long-incident-t",
		},
		{
			name:     "empty title falls back to number",
			incident: &store.IncidentRecord{IncidentNumber: 42, Title: ""},
			want:     "inc-42-42",
		},
		{
			name:     "title only punctuation falls back to number",
			incident: &store.IncidentRecord{IncidentNumber: 5, Title: "!!! ???"},
			want:     "inc-5-5",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := generateChannelName(tt.incident); got != tt.want {
				t.Fatalf("generateChannelName() = %q, want %q", got, tt.want)
			}
		})
	}
}
