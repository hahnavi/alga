package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
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

func newPGTeamStore(db *bun.DB) TeamStore {
	return &pgTeamStore{pgStoreBase{db: db}}
}

func (s *pgTeamStore) CreateTeam(ctx context.Context, record *TeamRecord) (*TeamRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	m := &models.Team{
		ID:          models.NewUUID(),
		Name:        record.Name,
		Description: record.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}
	record.ID = m.ID
	record.CreatedAt = m.CreatedAt
	record.UpdatedAt = m.UpdatedAt
	return record, nil
}

func (s *pgTeamStore) GetTeam(ctx context.Context, id uuid.UUID) (*TeamRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var t models.Team
	err := s.db.NewSelect().Model(&t).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*TeamRecord](err, "team")
	}

	rec := s.toTeamRecord(&t)

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

	var t models.Team
	err := s.db.NewSelect().Model(&t).Where("name = ?", name).Scan(ctx)
	if err != nil {
		return handleQueryErr[*TeamRecord](err, "team")
	}
	return s.toTeamRecord(&t), nil
}

func (s *pgTeamStore) GetTeamName(ctx context.Context, id uuid.UUID) (string, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var name string
	err := s.db.NewSelect().Model((*models.Team)(nil)).
		Column("name").
		Where("id = ?", id).
		Scan(ctx, &name)
	if err != nil {
		return "", fmt.Errorf("failed to get team name: %w", err)
	}
	return name, nil
}

func (s *pgTeamStore) UpdateTeam(ctx context.Context, id uuid.UUID, record *TeamRecord) (*TeamRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.Team)(nil)).
		Set("name = ?", record.Name).
		Set("description = ?", record.Description).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update team: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("team not found: %w", ErrNotFound)
	}

	var updated models.Team
	if err := s.db.NewSelect().Model(&updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to re-fetch updated team: %w", err)
	}
	return s.toTeamRecord(&updated), nil
}

func (s *pgTeamStore) DeleteTeam(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.Team)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("team not found: %w", ErrNotFound)
	}
	return nil
}

func (s *pgTeamStore) ListTeams(ctx context.Context, limit, skip int) ([]TeamRecord, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)

	total, err := s.db.NewSelect().Model((*models.Team)(nil)).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count teams: %w", err)
	}

	var teams []models.Team
	err = s.db.NewSelect().Model(&teams).
		Order("created_at DESC").
		Limit(limit).
		Offset(skip).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list teams: %w", err)
	}

	teamIDs := make([]uuid.UUID, 0, len(teams))
	for i := range teams {
		teamIDs = append(teamIDs, teams[i].ID)
	}

	membersByTeam := make(map[uuid.UUID][]TeamMemberRecord)
	if len(teamIDs) > 0 {
		var allMembers []models.TeamMember
		err = s.db.NewSelect().Model(&allMembers).
			Where("team_id IN (?)", bun.In(teamIDs)).
			Scan(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to load team members: %w", err)
		}

		userIDs := make([]uuid.UUID, 0, len(allMembers))
		seenUsers := make(map[uuid.UUID]struct{})
		for i := range allMembers {
			if _, ok := seenUsers[allMembers[i].UserID]; !ok {
				userIDs = append(userIDs, allMembers[i].UserID)
				seenUsers[allMembers[i].UserID] = struct{}{}
			}
		}

		usersByID := make(map[uuid.UUID]*models.User)
		if len(userIDs) > 0 {
			var users []models.User
			err = s.db.NewSelect().Model(&users).
				Where("id IN (?)", bun.In(userIDs)).
				Scan(ctx)
			if err != nil {
				logger.WarnCtx(ctx, "failed to batch-load users for team members", "component", "store", "error", err)
			}
			for i := range users {
				usersByID[users[i].ID] = &users[i]
			}
		}

		for i := range allMembers {
			m := &allMembers[i]
			u := usersByID[m.UserID]
			membersByTeam[m.TeamID] = append(membersByTeam[m.TeamID], buildTeamMemberRecord(m, u))
		}
	}

	records := make([]TeamRecord, 0, len(teams))
	for i := range teams {
		rec := s.toTeamRecord(&teams[i])
		rec.Members = membersByTeam[teams[i].ID]
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

	m := &models.TeamMember{
		ID:        models.NewUUID(),
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to add team member: %w", err)
	}

	var u models.User
	if err := s.db.NewSelect().Model(&u).Where("id = ?", userID).Scan(ctx); err != nil {
		logger.WarnCtx(ctx, "failed to get user for team member", "component", "store", "user_id", userID, "error", err)
	}
	rec := buildTeamMemberRecord(m, &u)
	return &rec, nil
}

func (s *pgTeamStore) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.TeamMember)(nil)).
		Set("role = ?", role).
		Where("team_id = ?", teamID).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("team member not found")
	}
	return nil
}

func (s *pgTeamStore) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.TeamMember)(nil)).
		Where("team_id = ?", teamID).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("team member not found")
	}
	return nil
}

func (s *pgTeamStore) GetMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMemberRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var members []models.TeamMember
	err := s.db.NewSelect().Model(&members).Where("team_id = ?", teamID).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}

	records := make([]TeamMemberRecord, 0, len(members))
	for i := range members {
		m := &members[i]
		var u models.User
		if err := s.db.NewSelect().Model(&u).Where("id = ?", m.UserID).Scan(ctx); err != nil {
			logger.WarnCtx(ctx, "failed to get user for team member", "component", "store", "user_id", m.UserID, "error", err)
		}
		records = append(records, buildTeamMemberRecord(m, &u))
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

	var existing models.Team
	err := s.db.NewSelect().Model(&existing).Where("name = ?", "ops-team").Scan(ctx)
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to look up ops-team: %w", err)
	}
	if isNotFound(err) {
		now := time.Now().UTC()
		existing = models.Team{
			ID:          models.NewUUID(),
			Name:        "ops-team",
			Description: "Operations team for agent escalations and confirmations",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, cerr := s.db.NewInsert().Model(&existing).Exec(ctx); cerr != nil {
			return fmt.Errorf("failed to create ops-team: %w", cerr)
		}
	}

	n, err := s.db.NewSelect().Model((*models.TeamMember)(nil)).
		Where("team_id = ?", existing.ID).
		Where("user_id = ?", adminUserID).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to check ops-team membership: %w", err)
	}
	if n == 0 {
		m := &models.TeamMember{
			ID:        models.NewUUID(),
			TeamID:    existing.ID,
			UserID:    adminUserID,
			Role:      "lead",
			CreatedAt: time.Now().UTC(),
		}
		if _, merr := s.db.NewInsert().Model(m).Exec(ctx); merr != nil {
			return fmt.Errorf("failed to add admin to ops-team: %w", merr)
		}
	}
	return nil
}

func (s *pgTeamStore) toTeamRecord(t *models.Team) *TeamRecord {
	return &TeamRecord{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func buildTeamMemberRecord(m *models.TeamMember, u *models.User) TeamMemberRecord {
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
