package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entoncallschedule "alga/ent/oncallschedule"
	entschedulelayer "alga/ent/schedulelayer"
	entscheduleoverride "alga/ent/scheduleoverride"
	entteam "alga/ent/team"
	entteammember "alga/ent/teammember"
	entuser "alga/ent/user"
	"alga/logger"
)

type TeamRecord struct {
	ID          uuid.UUID          `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Members     []TeamMemberRecord `json:"members,omitempty"`
}

type TeamMemberRecord struct {
	ID        uuid.UUID `json:"id"`
	TeamID    uuid.UUID `json:"team_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UserName  string    `json:"user_name,omitempty"`
	UserEmail string    `json:"user_email,omitempty"`
}

type TeamStore interface {
	CreateTeam(ctx context.Context, record *TeamRecord) (*TeamRecord, error)
	GetTeam(ctx context.Context, id uuid.UUID) (*TeamRecord, error)
	GetTeamByName(ctx context.Context, name string) (*TeamRecord, error)
	// GetTeamName returns just the team's display name. Use it for cheap
	// dynamic name resolution (e.g. on-call schedule titles derived from a
	// team) without loading members or the full record.
	GetTeamName(ctx context.Context, id uuid.UUID) (string, error)
	UpdateTeam(ctx context.Context, id uuid.UUID, record *TeamRecord) (*TeamRecord, error)
	DeleteTeam(ctx context.Context, id uuid.UUID) error
	ListTeams(ctx context.Context, limit, skip int) ([]TeamRecord, int64, error)
	AddMember(ctx context.Context, teamID, userID uuid.UUID, role string) (*TeamMemberRecord, error)
	UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error
	GetMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMemberRecord, error)
	SeedOpsTeam(ctx context.Context, adminUserID uuid.UUID) error
}

type pgTeamStore struct {
	pgStoreBase
}

func newPGTeamStore(client *ent.Client) TeamStore {
	return &pgTeamStore{pgStoreBase{client: client}}
}

func (s *pgTeamStore) CreateTeam(ctx context.Context, record *TeamRecord) (*TeamRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	b := s.client.Team.Create().
		SetName(record.Name).
		SetDescription(record.Description).
		SetCreatedAt(time.Now().UTC()).
		SetUpdatedAt(time.Now().UTC())

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}
	record.ID = saved.ID
	record.CreatedAt = saved.CreatedAt
	record.UpdatedAt = saved.UpdatedAt
	return record, nil
}

func (s *pgTeamStore) GetTeam(ctx context.Context, id uuid.UUID) (*TeamRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	t, err := s.client.Team.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*TeamRecord](err, "team")
	}

	rec := s.toTeamRecord(t)

	members, err := s.GetMembers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}
	rec.Members = members

	return rec, nil
}

func (s *pgTeamStore) GetTeamByName(ctx context.Context, name string) (*TeamRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	t, err := s.client.Team.Query().Where(entteam.NameEQ(name)).Only(ctx)
	if err != nil {
		return handleQueryErr[*TeamRecord](err, "team")
	}
	return s.toTeamRecord(t), nil
}

func (s *pgTeamStore) GetTeamName(ctx context.Context, id uuid.UUID) (string, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	name, err := s.client.Team.Query().Where(entteam.IDEQ(id)).Select(entteam.FieldName).String(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get team name: %w", err)
	}
	return name, nil
}

func (s *pgTeamStore) UpdateTeam(ctx context.Context, id uuid.UUID, record *TeamRecord) (*TeamRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	b := s.client.Team.UpdateOneID(id).
		SetName(record.Name).
		SetDescription(record.Description).
		SetUpdatedAt(time.Now().UTC())

	updated, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update team: %w", err)
	}
	return s.toTeamRecord(updated), nil
}

func (s *pgTeamStore) DeleteTeam(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer rollbackTx(tx)

	// Cascade-delete the team's schedule (and its layers/overrides) so a
	// deleted team never leaves an orphan schedule behind.
	schedules, err := tx.OnCallSchedule.Query().Where(entoncallschedule.TeamIDEQ(id)).All(ctx)
	if err != nil {
		return fmt.Errorf("failed to load team schedules: %w", err)
	}
	for _, sched := range schedules {
		if _, err := tx.ScheduleOverride.Delete().Where(entscheduleoverride.ScheduleIDEQ(sched.ID)).Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete schedule overrides: %w", err)
		}
		if _, err := tx.ScheduleLayer.Delete().Where(entschedulelayer.ScheduleIDEQ(sched.ID)).Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete schedule layers: %w", err)
		}
		if err := tx.OnCallSchedule.DeleteOneID(sched.ID).Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("failed to delete schedule: %w", err)
		}
	}

	_, err = tx.TeamMember.Delete().
		Where(entteammember.TeamIDEQ(id)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete team members: %w", err)
	}

	err = tx.Team.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("team not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete team: %w", err)
	}

	return tx.Commit()
}

func (s *pgTeamStore) ListTeams(ctx context.Context, limit, skip int) ([]TeamRecord, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)

	total, err := s.client.Team.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count teams: %w", err)
	}

	teams, err := s.client.Team.Query().
		Order(ent.Desc(entteam.FieldCreatedAt)).
		Limit(limit).
		Offset(skip).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list teams: %w", err)
	}

	teamIDs := make([]uuid.UUID, 0, len(teams))
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
	}

	membersByTeam := make(map[uuid.UUID][]TeamMemberRecord)
	if len(teamIDs) > 0 {
		allMembers, err := s.client.TeamMember.Query().
			Where(entteammember.TeamIDIn(teamIDs...)).
			All(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to load team members: %w", err)
		}

		userIDs := make([]uuid.UUID, 0, len(allMembers))
		seenUsers := make(map[uuid.UUID]struct{})
		for _, m := range allMembers {
			if _, ok := seenUsers[m.UserID]; !ok {
				userIDs = append(userIDs, m.UserID)
				seenUsers[m.UserID] = struct{}{}
			}
		}

		usersByID := make(map[uuid.UUID]*ent.User)
		if len(userIDs) > 0 {
			users, err := s.client.User.Query().
				Where(entuser.IDIn(userIDs...)).
				All(ctx)
			if err != nil {
				logger.WarnCtx(ctx, "failed to batch-load users for team members", "component", "store", "error", err)
			}
			for _, u := range users {
				usersByID[u.ID] = u
			}
		}

		for _, m := range allMembers {
			u := usersByID[m.UserID]
			membersByTeam[m.TeamID] = append(membersByTeam[m.TeamID], buildTeamMemberRecord(m, u))
		}
	}

	records := make([]TeamRecord, 0, len(teams))
	for _, t := range teams {
		rec := s.toTeamRecord(t)
		rec.Members = membersByTeam[t.ID]
		records = append(records, *rec)
	}
	return records, int64(total), nil
}

func (s *pgTeamStore) AddMember(ctx context.Context, teamID, userID uuid.UUID, role string) (*TeamMemberRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if role == "" {
		role = "member"
	}

	saved, err := s.client.TeamMember.Create().
		SetTeamID(teamID).
		SetUserID(userID).
		SetRole(role).
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to add team member: %w", err)
	}

	u, err := s.client.User.Get(ctx, userID)
	if err != nil {
		logger.WarnCtx(ctx, "failed to get user for team member", "component", "store", "user_id", userID, "error", err)
	}
	rec := buildTeamMemberRecord(saved, u)
	return &rec, nil
}

func (s *pgTeamStore) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	n, err := s.client.TeamMember.Update().
		Where(
			entteammember.TeamIDEQ(teamID),
			entteammember.UserIDEQ(userID),
		).
		SetRole(role).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}
	if n == 0 {
		return errors.New("team member not found")
	}
	return nil
}

func (s *pgTeamStore) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	n, err := s.client.TeamMember.Delete().
		Where(
			entteammember.TeamIDEQ(teamID),
			entteammember.UserIDEQ(userID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}
	if n == 0 {
		return errors.New("team member not found")
	}
	return nil
}

func (s *pgTeamStore) GetMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMemberRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	members, err := s.client.TeamMember.Query().
		Where(entteammember.TeamIDEQ(teamID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}

	records := make([]TeamMemberRecord, 0, len(members))
	for _, m := range members {
		u, err := s.client.User.Get(ctx, m.UserID)
		if err != nil {
			logger.WarnCtx(ctx, "failed to get user for team member", "component", "store", "user_id", m.UserID, "error", err)
		}
		records = append(records, buildTeamMemberRecord(m, u))
	}
	return records, nil
}

// SeedOpsTeam ensures an "ops-team" team exists with the admin user as a lead
// member. It is idempotent: a missing team is created, an existing team keeps
// its members, and the admin is added if absent. It replaces the former
// group-based escalation seed and backs the investigation scheduler's
// human-escalation hook.
func (s *pgTeamStore) SeedOpsTeam(ctx context.Context, adminUserID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	existing, err := s.client.Team.Query().Where(entteam.NameEQ("ops-team")).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("failed to look up ops-team: %w", err)
	}
	if ent.IsNotFound(err) {
		created, cerr := s.client.Team.Create().
			SetName("ops-team").
			SetDescription("Operations team for agent escalations and confirmations").
			SetCreatedAt(time.Now().UTC()).
			SetUpdatedAt(time.Now().UTC()).
			Save(ctx)
		if cerr != nil {
			return fmt.Errorf("failed to create ops-team: %w", cerr)
		}
		existing = created
	}

	if existing != nil {
		n, _ := s.client.TeamMember.Query().
			Where(entteammember.TeamIDEQ(existing.ID), entteammember.UserIDEQ(adminUserID)).
			Count(ctx)
		if n == 0 {
			if _, merr := s.client.TeamMember.Create().
				SetTeamID(existing.ID).
				SetUserID(adminUserID).
				SetRole("lead").
				SetCreatedAt(time.Now().UTC()).
				Save(ctx); merr != nil {
				return fmt.Errorf("failed to add admin to ops-team: %w", merr)
			}
		}
	}
	return nil
}

func (s *pgTeamStore) toTeamRecord(t *ent.Team) *TeamRecord {
	return &TeamRecord{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func buildTeamMemberRecord(m *ent.TeamMember, u *ent.User) TeamMemberRecord {
	rec := TeamMemberRecord{
		ID:        m.ID,
		TeamID:    m.TeamID,
		UserID:    m.UserID,
		Role:      m.Role,
		CreatedAt: m.CreatedAt,
	}
	if u != nil {
		rec.UserName = u.FullName
		rec.UserEmail = u.Email
	}
	return rec
}
