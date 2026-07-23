package escalation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/oncall"
	"alga/store"
)

type stubEscalationStore struct {
	policies map[uuid.UUID]*store.EscalationPolicyRecord
}

func (s *stubEscalationStore) CreatePolicy(_ context.Context, _ *store.EscalationPolicyRecord) (*store.EscalationPolicyRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubEscalationStore) GetPolicy(_ context.Context, id uuid.UUID) (*store.EscalationPolicyRecord, error) {
	p, ok := s.policies[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

func (s *stubEscalationStore) UpdatePolicy(_ context.Context, _ uuid.UUID, _ *store.EscalationPolicyRecord) (*store.EscalationPolicyRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubEscalationStore) DeletePolicy(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *stubEscalationStore) ListPolicies(_ context.Context, _, _ int) ([]store.EscalationPolicyRecord, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

type stubTeamStore struct {
	members    map[uuid.UUID][]store.TeamMemberRecord
	teamsByNme map[string]*store.TeamRecord
}

func (s *stubTeamStore) CreateTeam(_ context.Context, _ *store.TeamRecord) (*store.TeamRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubTeamStore) GetTeam(_ context.Context, _ uuid.UUID) (*store.TeamRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubTeamStore) GetTeamByName(_ context.Context, name string) (*store.TeamRecord, error) {
	if t, ok := s.teamsByNme[name]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("not found")
}

func (s *stubTeamStore) GetTeamName(_ context.Context, _ uuid.UUID) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (s *stubTeamStore) SeedOpsTeam(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (s *stubTeamStore) UpdateTeam(_ context.Context, _ uuid.UUID, _ *store.TeamRecord) (*store.TeamRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubTeamStore) DeleteTeam(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *stubTeamStore) ListTeams(_ context.Context, _, _ int) ([]store.TeamRecord, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *stubTeamStore) AddMember(_ context.Context, _, _ uuid.UUID, _ string) (*store.TeamMemberRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubTeamStore) UpdateMemberRole(_ context.Context, _, _ uuid.UUID, _ string) error {
	return fmt.Errorf("not implemented")
}

func (s *stubTeamStore) RemoveMember(_ context.Context, _, _ uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *stubTeamStore) GetMembers(_ context.Context, teamID uuid.UUID) ([]store.TeamMemberRecord, error) {
	return s.members[teamID], nil
}

type stubOnCallStore struct {
	schedules map[uuid.UUID]*store.OnCallScheduleRecord
}

func (s *stubOnCallStore) CreateSchedule(_ context.Context, _ *store.OnCallScheduleRecord) (*store.OnCallScheduleRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubOnCallStore) GetSchedule(_ context.Context, id uuid.UUID) (*store.OnCallScheduleRecord, error) {
	if sched, ok := s.schedules[id]; ok {
		return sched, nil
	}
	return nil, fmt.Errorf("not found")
}

func (s *stubOnCallStore) GetScheduleByTeam(_ context.Context, teamID uuid.UUID) (*store.OnCallScheduleRecord, error) {
	for _, sched := range s.schedules {
		if sched.TeamID != nil && *sched.TeamID == teamID {
			return sched, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (s *stubOnCallStore) UpdateSchedule(_ context.Context, _ uuid.UUID, _ *store.OnCallScheduleRecord) (*store.OnCallScheduleRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubOnCallStore) ListSchedules(_ context.Context, _, _ int) ([]store.OnCallScheduleRecord, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *stubOnCallStore) CreateOverride(_ context.Context, _ *store.ScheduleOverrideRecord) (*store.ScheduleOverrideRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubOnCallStore) DeleteOverride(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *stubOnCallStore) ListOverrides(_ context.Context, _ uuid.UUID) ([]store.ScheduleOverrideRecord, error) {
	return nil, nil
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func mustUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

func TestPolicyEngine_EvaluatePolicy_UserTargets(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000001")
	user1 := mustUUID("30000000-0000-0000-0000-000000000001")
	user2 := mustUUID("30000000-0000-0000-0000-000000000002")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "user-targets-policy",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber: 1,
						Targets: []store.EscalationTargetRecord{
							{TargetType: "user", TargetUserID: &user1},
							{TargetType: "user", TargetUserID: &user2},
						},
						NotifyChannels: []string{"email", "voice"},
					},
				},
			},
		},
	}

	engine := NewPolicyEngine(escStore, nil, nil, nil)
	userIDs, channels, _, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}

	if len(userIDs) != 2 {
		t.Fatalf("len(userIDs) = %d, want 2", len(userIDs))
	}
	if userIDs[0] != user1 || userIDs[1] != user2 {
		t.Errorf("userIDs = %v, want [%s, %s]", userIDs, user1, user2)
	}
	if len(channels) != 2 || channels[0] != "email" || channels[1] != "voice" {
		t.Errorf("channels = %v, want [email voice]", channels)
	}
}

func TestPolicyEngine_EvaluatePolicy_TeamTargets(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000002")
	teamID := mustUUID("40000000-0000-0000-0000-000000000001")
	scheduleID := mustUUID("50000000-0000-0000-0000-000000000010")
	onCallUser := mustUUID("30000000-0000-0000-0000-000000000010")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "team-policy",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber: 1,
						Targets: []store.EscalationTargetRecord{
							{TargetType: "team", TargetTeamID: &teamID},
						},
						NotifyChannels: []string{"slack"},
					},
				},
			},
		},
	}

	onCallStore := &stubOnCallStore{
		schedules: map[uuid.UUID]*store.OnCallScheduleRecord{
			scheduleID: {
				ID:     scheduleID,
				TeamID: &teamID,
				Layers: []store.ScheduleLayerRecord{
					{
						UserIds:          []string{onCallUser.String()},
						RotationType:     "weekly",
						RotationInterval: 1,
						StartDate:        mustTime("2025-01-01T00:00:00Z"),
					},
				},
			},
		},
	}

	resolver := oncall.NewResolver(onCallStore)

	engine := NewPolicyEngine(escStore, onCallStore, resolver, nil)
	userIDs, channels, _, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}

	if len(userIDs) != 1 {
		t.Fatalf("len(userIDs) = %d, want 1 (the team's on-call user)", len(userIDs))
	}
	if userIDs[0] != onCallUser {
		t.Errorf("userIDs = %v, want [%s]", userIDs, onCallUser)
	}
	if len(channels) != 1 || channels[0] != "slack" {
		t.Errorf("channels = %v, want [slack]", channels)
	}
}

func TestPolicyEngine_EvaluatePolicy_TeamTargetSkippedWhenNoStore(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000006")
	teamID := mustUUID("40000000-0000-0000-0000-000000000003")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "team-no-store",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber: 1,
						Targets: []store.EscalationTargetRecord{
							{TargetType: "team", TargetTeamID: &teamID},
						},
					},
				},
			},
		},
	}

	// nil onCallStore: the engine cannot resolve a team's schedule, so the
	// target is skipped and no users are paged.
	engine := NewPolicyEngine(escStore, nil, nil, nil)
	userIDs, _, _, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}
	if len(userIDs) != 0 {
		t.Errorf("expected 0 users when onCallStore is nil, got %d", len(userIDs))
	}
}

func TestPolicyEngine_EvaluatePolicy_MixedTargets(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000004")
	user1 := mustUUID("30000000-0000-0000-0000-000000000030")
	teamID := mustUUID("40000000-0000-0000-0000-000000000002")
	scheduleID := mustUUID("50000000-0000-0000-0000-000000000002")
	onCallUser := mustUUID("30000000-0000-0000-0000-000000000032")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "mixed-policy",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber: 1,
						Targets: []store.EscalationTargetRecord{
							{TargetType: "user", TargetUserID: &user1},
							{TargetType: "team", TargetTeamID: &teamID},
						},
						NotifyChannels: []string{"email", "slack", "email"},
					},
				},
			},
		},
	}

	onCallStore := &stubOnCallStore{
		schedules: map[uuid.UUID]*store.OnCallScheduleRecord{
			scheduleID: {
				ID:     scheduleID,
				TeamID: &teamID,
				Layers: []store.ScheduleLayerRecord{
					{
						UserIds:          []string{onCallUser.String()},
						RotationType:     "weekly",
						RotationInterval: 1,
						StartDate:        mustTime("2025-01-01T00:00:00Z"),
					},
				},
			},
		},
	}

	resolver := oncall.NewResolver(onCallStore)

	engine := NewPolicyEngine(escStore, onCallStore, resolver, nil)
	userIDs, channels, _, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}

	if len(userIDs) != 2 {
		t.Fatalf("len(userIDs) = %d, want 2 (user + team on-call)", len(userIDs))
	}

	wantIDs := map[uuid.UUID]bool{user1: true, onCallUser: true}
	for _, id := range userIDs {
		if !wantIDs[id] {
			t.Errorf("unexpected userID %s", id)
		}
	}

	if len(channels) != 2 {
		t.Errorf("channels should be deduplicated, got %d: %v", len(channels), channels)
	}
}

func TestPolicyEngine_EvaluatePolicy_DeduplicateNotifyChannels(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000007")
	user1 := mustUUID("30000000-0000-0000-0000-000000000040")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "dedup-channels",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber:    1,
						NotifyChannels: []string{"email", "slack", "email", "voice", "slack"},
						Targets: []store.EscalationTargetRecord{
							{TargetType: "user", TargetUserID: &user1},
						},
					},
				},
			},
		},
	}

	engine := NewPolicyEngine(escStore, nil, nil, nil)
	_, channels, _, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}

	if len(channels) != 3 {
		t.Fatalf("len(channels) = %d, want 3 (deduplicated)", len(channels))
	}
	want := []string{"email", "slack", "voice"}
	for i, ch := range want {
		if channels[i] != ch {
			t.Errorf("channels[%d] = %q, want %q", i, channels[i], ch)
		}
	}
}

func TestPolicyEngine_EvaluatePolicy_PolicyNotFound(t *testing.T) {
	t.Parallel()
	engine := NewPolicyEngine(&stubEscalationStore{policies: map[uuid.UUID]*store.EscalationPolicyRecord{}}, nil, nil, nil)
	_, _, _, err := engine.EvaluatePolicy(context.Background(), uuid.New(), 1)
	if err == nil {
		t.Fatal("expected error for missing policy")
	}
}

func TestPolicyEngine_EvaluatePolicy_LevelNotFound(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000005")
	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:     policyID,
				Name:   "test",
				Levels: []store.EscalationLevelRecord{{LevelNumber: 1}},
			},
		},
	}

	engine := NewPolicyEngine(escStore, nil, nil, nil)
	_, _, _, err := engine.EvaluatePolicy(context.Background(), policyID, 99)
	if err == nil {
		t.Fatal("expected error for missing level")
	}
}

func TestPolicyEngine_EvaluatePolicy_UserTargetWithNilID(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000009")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "nil-user-id",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber: 1,
						Targets: []store.EscalationTargetRecord{
							{TargetType: "user", TargetUserID: nil},
						},
					},
				},
			},
		},
	}

	engine := NewPolicyEngine(escStore, nil, nil, nil)
	userIDs, _, _, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}
	if len(userIDs) != 0 {
		t.Errorf("expected 0 users when TargetUserID is nil, got %d", len(userIDs))
	}
}

func TestPolicyEngine_EvaluatePolicy_EmptyTargetsAndChannels(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000010")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "empty-level",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber:    1,
						Targets:        nil,
						NotifyChannels: nil,
					},
				},
			},
		},
	}

	engine := NewPolicyEngine(escStore, nil, nil, nil)
	userIDs, channels, _, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}
	if len(userIDs) != 0 {
		t.Errorf("expected 0 users, got %d", len(userIDs))
	}
	if len(channels) != 0 {
		t.Errorf("expected 0 channels, got %d", len(channels))
	}
}

func TestPolicyEngine_EvaluatePolicy_ForcesChannelsForOpsTeam(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000020")
	opsTeamID := mustUUID("40000000-0000-0000-0000-000000000020")
	scheduleID := mustUUID("50000000-0000-0000-0000-000000000020")
	onCallUser := mustUUID("30000000-0000-0000-0000-000000000050")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "ops-team-default",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber:    1,
						NotifyChannels: []string{"email"},
						Targets: []store.EscalationTargetRecord{
							{TargetType: "team", TargetTeamID: &opsTeamID},
						},
					},
				},
			},
		},
	}

	teamStore := &stubTeamStore{
		teamsByNme: map[string]*store.TeamRecord{
			"ops-team": {ID: opsTeamID, Name: "ops-team"},
		},
	}

	onCallStore := &stubOnCallStore{
		schedules: map[uuid.UUID]*store.OnCallScheduleRecord{
			scheduleID: {
				ID:     scheduleID,
				TeamID: &opsTeamID,
				Layers: []store.ScheduleLayerRecord{
					{
						UserIds:          []string{onCallUser.String()},
						RotationType:     "weekly",
						RotationInterval: 1,
						StartDate:        mustTime("2025-01-01T00:00:00Z"),
					},
				},
			},
		},
	}

	resolver := oncall.NewResolver(onCallStore)

	engine := NewPolicyEngineWithOpsTeam(escStore, onCallStore, resolver, teamStore, "ops-team")

	userIDs, channels, forced, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}
	if !forced {
		t.Errorf("forcedChannels = false, want true for ops-team target")
	}
	if len(userIDs) != 1 || userIDs[0] != onCallUser {
		t.Errorf("userIDs = %v, want [%s]", userIDs, onCallUser)
	}
	if len(channels) != 1 || channels[0] != "email" {
		t.Errorf("engine should still surface policy channels, got %v", channels)
	}
}

func TestPolicyEngine_EvaluatePolicy_NoForcedChannelsForNonOpsSchedule(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000021")
	opsTeamID := mustUUID("40000000-0000-0000-0000-000000000021")
	otherTeamID := mustUUID("40000000-0000-0000-0000-000000000022")
	scheduleID := mustUUID("50000000-0000-0000-0000-000000000021")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "other-team-policy",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber: 1,
						Targets: []store.EscalationTargetRecord{
							{TargetType: "team", TargetTeamID: &otherTeamID},
						},
					},
				},
			},
		},
	}

	teamStore := &stubTeamStore{
		teamsByNme: map[string]*store.TeamRecord{
			"ops-team": {ID: opsTeamID, Name: "ops-team"},
		},
	}

	engine := NewPolicyEngineWithOpsTeam(escStore, &stubOnCallStore{
		schedules: map[uuid.UUID]*store.OnCallScheduleRecord{
			scheduleID: {ID: scheduleID, TeamID: &otherTeamID},
		},
	}, nil, teamStore, "ops-team")

	_, _, forced, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}
	if forced {
		t.Errorf("forcedChannels = true, want false for non-ops-team target")
	}
}

func TestPolicyEngine_EvaluatePolicy_NoForcedWhenOpsTeamMissing(t *testing.T) {
	t.Parallel()

	policyID := mustUUID("20000000-0000-0000-0000-000000000022")
	teamID := mustUUID("40000000-0000-0000-0000-000000000023")
	scheduleID := mustUUID("50000000-0000-0000-0000-000000000022")

	escStore := &stubEscalationStore{
		policies: map[uuid.UUID]*store.EscalationPolicyRecord{
			policyID: {
				ID:   policyID,
				Name: "missing-ops-policy",
				Levels: []store.EscalationLevelRecord{
					{
						LevelNumber: 1,
						Targets: []store.EscalationTargetRecord{
							{TargetType: "team", TargetTeamID: &teamID},
						},
					},
				},
			},
		},
	}

	engine := NewPolicyEngineWithOpsTeam(escStore, &stubOnCallStore{
		schedules: map[uuid.UUID]*store.OnCallScheduleRecord{
			scheduleID: {ID: scheduleID, TeamID: &teamID},
		},
	}, nil, &stubTeamStore{}, "ops-team")

	_, _, forced, err := engine.EvaluatePolicy(context.Background(), policyID, 1)
	if err != nil {
		t.Fatalf("EvaluatePolicy() error: %v", err)
	}
	if forced {
		t.Errorf("forcedChannels = true, want false when ops-team is missing")
	}
}
