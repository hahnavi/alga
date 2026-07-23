package oncall

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/store"
)

type stubServiceStore struct {
	service *store.ServiceRecord
	svcErr  error
}

func (s *stubServiceStore) CreateService(ctx context.Context, record *store.ServiceRecord) (*store.ServiceRecord, error) {
	return nil, nil
}
func (s *stubServiceStore) GetService(ctx context.Context, id string) (*store.ServiceRecord, error) {
	return s.service, s.svcErr
}
func (s *stubServiceStore) GetServiceByName(ctx context.Context, name string) (*store.ServiceRecord, error) {
	return nil, nil
}
func (s *stubServiceStore) UpdateService(ctx context.Context, id string, record *store.ServiceRecord) (*store.ServiceRecord, error) {
	return nil, nil
}
func (s *stubServiceStore) DeleteService(ctx context.Context, id string) error { return nil }
func (s *stubServiceStore) ListServices(ctx context.Context, filter store.ListServicesFilter) ([]store.ServiceRecord, int, error) {
	return nil, 0, nil
}
func (s *stubServiceStore) UpdateServiceStatus(ctx context.Context, id string, status string) error {
	return nil
}
func (s *stubServiceStore) AddDependency(ctx context.Context, serviceID, dependsOnID uuid.UUID, depType string) error {
	return nil
}
func (s *stubServiceStore) RemoveDependency(ctx context.Context, serviceID, targetID uuid.UUID) error {
	return nil
}
func (s *stubServiceStore) GetDependencies(ctx context.Context, serviceID uuid.UUID) ([]store.ServiceDependencyRecord, error) {
	return nil, nil
}
func (s *stubServiceStore) GetDependents(ctx context.Context, serviceID uuid.UUID) ([]store.ServiceDependencyRecord, error) {
	return nil, nil
}
func (s *stubServiceStore) HasCircularDependency(ctx context.Context, serviceID, dependsOnID uuid.UUID) (bool, error) {
	return false, nil
}

type stubEscalationStore struct {
	policy *store.EscalationPolicyRecord
	polErr error
}

func (s *stubEscalationStore) CreatePolicy(ctx context.Context, record *store.EscalationPolicyRecord) (*store.EscalationPolicyRecord, error) {
	return nil, nil
}
func (s *stubEscalationStore) GetPolicy(ctx context.Context, id uuid.UUID) (*store.EscalationPolicyRecord, error) {
	return s.policy, s.polErr
}
func (s *stubEscalationStore) UpdatePolicy(ctx context.Context, id uuid.UUID, record *store.EscalationPolicyRecord) (*store.EscalationPolicyRecord, error) {
	return nil, nil
}
func (s *stubEscalationStore) DeletePolicy(ctx context.Context, id uuid.UUID) error { return nil }
func (s *stubEscalationStore) ListPolicies(ctx context.Context, limit, skip int) ([]store.EscalationPolicyRecord, int64, error) {
	return nil, 0, nil
}

func TestResolveOnCallUserForIncident(t *testing.T) {
	t.Parallel()

	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	user2 := makeUUID("00000000-0000-0000-0000-000000000002")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")
	teamID := makeUUID("50000000-0000-0000-0000-000000000001")
	policyID := makeUUID("30000000-0000-0000-0000-000000000001")
	serviceID := makeUUID("40000000-0000-0000-0000-000000000001")

	onCallStore := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID:     schedID,
			TeamID: &teamID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        time.Now(),
					UserIds:          []string{user1.String(), user2.String()},
				},
			},
		},
	}

	resolver := NewResolver(onCallStore)

	policyWithSchedule := &store.EscalationPolicyRecord{
		ID: policyID,
		Levels: []store.EscalationLevelRecord{
			{
				LevelNumber: 1,
				Targets: []store.EscalationTargetRecord{
					{TargetType: "team", TargetTeamID: &teamID},
				},
			},
		},
	}

	cases := []struct {
		name     string
		incident *store.IncidentRecord
		service  *store.ServiceRecord
		svcErr   error
		policy   *store.EscalationPolicyRecord
		polErr   error
		wantNil  bool
		wantErr  bool
		wantUser *uuid.UUID
	}{
		{
			name:     "resolves_on_call_user",
			incident: &store.IncidentRecord{ServiceID: &serviceID},
			service:  &store.ServiceRecord{ID: serviceID, EscalationPolicyID: &policyID},
			policy:   policyWithSchedule,
			wantUser: &user1,
		},
		{
			name:     "no_service_on_incident",
			incident: &store.IncidentRecord{},
			wantNil:  true,
		},
		{
			name:     "nil_incident",
			incident: nil,
			wantNil:  true,
		},
		{
			name:     "service_without_policy",
			incident: &store.IncidentRecord{ServiceID: &serviceID},
			service:  &store.ServiceRecord{ID: serviceID},
			wantNil:  true,
		},
		{
			name:     "policy_without_on_call_target",
			incident: &store.IncidentRecord{ServiceID: &serviceID},
			service:  &store.ServiceRecord{ID: serviceID, EscalationPolicyID: &policyID},
			policy: &store.EscalationPolicyRecord{
				ID: policyID,
				Levels: []store.EscalationLevelRecord{
					{LevelNumber: 1, Targets: []store.EscalationTargetRecord{
						{TargetType: "user", TargetUserID: &user2},
					}},
				},
			},
			wantNil: true,
		},
		{
			name:     "service_store_error_propagates",
			incident: &store.IncidentRecord{ServiceID: &serviceID},
			svcErr:   errors.New("boom"),
			wantErr:  true,
		},
		{
			name:     "policy_store_error_propagates",
			incident: &store.IncidentRecord{ServiceID: &serviceID},
			service:  &store.ServiceRecord{ID: serviceID, EscalationPolicyID: &policyID},
			polErr:   errors.New("boom"),
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveOnCallUserForIncident(
				context.Background(),
				tc.incident,
				&stubServiceStore{service: tc.service, svcErr: tc.svcErr},
				&stubEscalationStore{policy: tc.policy, polErr: tc.polErr},
				onCallStore,
				resolver,
			)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (user=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil && got != nil {
				t.Fatalf("expected nil user, got %v", got)
			}
			if tc.wantUser != nil {
				if got == nil {
					t.Fatalf("expected user %v, got nil", *tc.wantUser)
				}
				if *got != *tc.wantUser {
					t.Fatalf("got user %v, want %v", *got, *tc.wantUser)
				}
			}
		})
	}
}
