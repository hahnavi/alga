package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	algacrypto "alga/crypto"
	"alga/db/models"
)

var (
	ErrAccountLocked      = errors.New("account is locked due to too many failed login attempts")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSlackIdentityTaken = errors.New("slack identity is already linked to another user")
)

type UserRecord struct {
	ID           uuid.UUID `json:"id"`
	FullName     string    `json:"full_name,omitempty"`
	Phone        string    `json:"phone,omitempty"`
	PhoneCountry string    `json:"phone_country,omitempty"`
	Email        string    `json:"email"`
	Password     string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	GoogleID            string     `json:"-"`
	SlackUserID         string     `json:"-"`
	SlackDisplayName    string     `json:"-"`
	FailedLoginAttempts int        `json:"-"`
	LockedUntil         *time.Time `json:"-"`
	LastFailedLogin     *time.Time `json:"-"`
	LastLoginAt         *time.Time `json:"last_login_at"`
	LastLoginIP         string     `json:"-"`
	VoiceOptOut         bool       `json:"voice_opt_out"`
}

func (u *UserRecord) DisplayName() string {
	if u.FullName != "" {
		return u.FullName
	}
	return u.Email
}

type UserStore interface {
	CreateUser(email, password, role string) (*UserRecord, error)
	GetByEmail(email string) (*UserRecord, error)
	GetByID(id uuid.UUID) (*UserRecord, error)
	ListUsers() ([]UserRecord, error)
	UpdateUser(id uuid.UUID, updates map[string]any) error
	DeleteUser(id uuid.UUID) error
	CountAdmins() (int64, error)
	CountUsers() (int64, error)
	Authenticate(email, password string) (*UserRecord, error)
	RecordFailedLogin(email string) error
	RecordSuccessfulLogin(userID uuid.UUID, ip string) error
	UnlockAccount(userID uuid.UUID) error
	GetNotificationPreferences(ctx context.Context, userID string) (map[string]any, error)
	UpdateNotificationPreferences(ctx context.Context, userID string, prefs map[string]any) error
	GetByGoogleID(googleID string) (*UserRecord, error)
	GetBySlackUserID(slackUserID string) (*UserRecord, error)
	UpdateGoogleID(userID uuid.UUID, googleID string) error
	ClearGoogleID(userID uuid.UUID) error
	SetSlackIdentity(ctx context.Context, userID uuid.UUID, slackUserID, slackDisplayName string) error
	ClearSlackIdentity(ctx context.Context, userID uuid.UUID) error
}

type pgUserStore struct {
	pgStoreBase
}

func newPGUserStore(db *bun.DB) UserStore {
	return &pgUserStore{pgStoreBase{db: db}}
}

func (s *pgUserStore) CreateUser(email, password, role string) (*UserRecord, error) {
	hash, err := algacrypto.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()

	m := &models.User{
		ID:        models.NewUUID(),
		Email:     email,
		Password:  hash,
		Role:      role,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.db.NewInsert().Model(m).Exec(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return pgUserToRecord(m), nil
}

func (s *pgUserStore) GetByEmail(email string) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u := new(models.User)
	err := s.db.NewSelect().Model(u).Where("email = ?", email).Scan(ctx)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user by email")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) GetByID(id uuid.UUID) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u := new(models.User)
	err := s.db.NewSelect().Model(u).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) ListUsers() ([]UserRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var users []models.User
	err := s.db.NewSelect().Model(&users).Limit(500).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	records := make([]UserRecord, 0, len(users))
	for i := range users {
		records = append(records, *pgUserToRecord(&users[i]))
	}
	return records, nil
}

func (s *pgUserStore) UpdateUser(id uuid.UUID, updates map[string]any) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	q := s.db.NewUpdate().Model((*models.User)(nil)).Where("id = ?", id)
	hasSet := false

	if v, ok := updates["email"].(string); ok {
		q = q.Set("email = ?", v)
		hasSet = true
	}
	if v, ok := updates["password"].(string); ok {
		q = q.Set("password = ?", v)
		hasSet = true
	}
	if v, ok := updates["role"].(string); ok {
		q = q.Set("role = ?", v)
		hasSet = true
	}
	if v, ok := updates["full_name"].(string); ok {
		q = q.Set("full_name = ?", v)
		hasSet = true
	}
	if v, ok := updates["phone"].(string); ok {
		q = q.Set("phone = ?", v)
		hasSet = true
	}
	if v, ok := updates["phone_country"].(string); ok {
		q = q.Set("phone_country = ?", v)
		hasSet = true
	}
	if v, ok := updates["failed_login_attempts"].(int); ok {
		q = q.Set("failed_login_attempts = ?", v)
		hasSet = true
	}
	if v, ok := updates["locked_until"]; ok {
		if t, ok := v.(*time.Time); ok && t != nil {
			q = q.Set("locked_until = ?", *t)
		} else {
			q = q.Set("locked_until = NULL")
		}
		hasSet = true
	}
	if v, ok := updates["last_failed_login"]; ok {
		if t, ok := v.(*time.Time); ok && t != nil {
			q = q.Set("last_failed_login = ?", *t)
		} else {
			q = q.Set("last_failed_login = NULL")
		}
		hasSet = true
	}
	if v, ok := updates["last_login_at"]; ok {
		if t, ok := v.(*time.Time); ok && t != nil {
			q = q.Set("last_login_at = ?", *t)
		} else {
			q = q.Set("last_login_at = NULL")
		}
		hasSet = true
	}
	if v, ok := updates["last_login_ip"].(string); ok {
		q = q.Set("last_login_ip = ?", v)
		hasSet = true
	}
	if v, ok := updates["slack_user_id"].(string); ok {
		q = q.Set("slack_user_id = ?", v)
		hasSet = true
	}
	if v, ok := updates["slack_display_name"].(string); ok {
		q = q.Set("slack_display_name = ?", v)
		hasSet = true
	}
	if v, ok := updates["voice_opt_out"].(bool); ok {
		q = q.Set("voice_opt_out = ?", v)
		hasSet = true
	}

	if !hasSet {
		return nil
	}

	q = q.Set("updated_at = ?", time.Now().UTC())

	res, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user not found: %w", ErrUserNotFound)
	}
	return nil
}

func (s *pgUserStore) DeleteUser(id uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.User)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user not found: %w", ErrUserNotFound)
	}
	return nil
}

func (s *pgUserStore) CountAdmins() (int64, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	count, err := s.db.NewSelect().Model((*models.User)(nil)).Where("role = ?", "admin").Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count admins: %w", err)
	}
	return int64(count), nil
}

func (s *pgUserStore) Authenticate(email, password string) (*UserRecord, error) {
	u, err := s.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	if u.LockedUntil != nil && time.Now().Before(*u.LockedUntil) {
		return nil, ErrAccountLocked
	}

	ok, needsRehash, err := algacrypto.VerifyPassword(u.Password, password)
	if err != nil {
		return nil, fmt.Errorf("failed to verify password: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	if needsRehash {
		if newHash, hashErr := algacrypto.HashPassword(password); hashErr == nil {
			ctx, cancel := pgctx(context.Background())
			defer cancel()
			_, _ = s.db.NewUpdate().Model((*models.User)(nil)).
				Set("password = ?", newHash).
				Set("updated_at = ?", time.Now().UTC()).
				Where("id = ?", u.ID).
				Exec(ctx)
		}
	}

	return u, nil
}

func (s *pgUserStore) RecordFailedLogin(email string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	now := time.Now().UTC()

	u := new(models.User)
	err := s.db.NewSelect().Model(u).Where("email = ?", email).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to record failed login: %w", err)
	}

	newAttempts := u.FailedLoginAttempts + 1

	q := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("failed_login_attempts = ?", newAttempts).
		Set("last_failed_login = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", u.ID)

	if newAttempts >= 5 {
		lockoutEnd := now.Add(30 * time.Minute)
		q = q.Set("locked_until = ?", lockoutEnd)
	}

	_, err = q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to record failed login: %w", err)
	}
	return nil
}

func (s *pgUserStore) RecordSuccessfulLogin(userID uuid.UUID, ip string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	now := time.Now().UTC()

	_, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("failed_login_attempts = ?", 0).
		Set("locked_until = NULL").
		Set("last_login_at = ?", now).
		Set("last_login_ip = ?", ip).
		Set("updated_at = ?", now).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to record successful login: %w", err)
	}
	return nil
}

func (s *pgUserStore) UnlockAccount(userID uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("failed_login_attempts = ?", 0).
		Set("locked_until = NULL").
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to unlock account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to unlock account: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user not found: %w", ErrUserNotFound)
	}
	return nil
}

func (s *pgUserStore) CountUsers() (int64, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	count, err := s.db.NewSelect().Model((*models.User)(nil)).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return int64(count), nil
}

func (s *pgUserStore) GetNotificationPreferences(ctx context.Context, userID string) (map[string]any, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	u := new(models.User)
	err = s.db.NewSelect().Model(u).Where("id = ?", uid).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get user preferences: %w", err)
	}
	if u.NotificationPreferences == nil {
		return map[string]any{}, nil
	}
	return u.NotificationPreferences, nil
}

func (s *pgUserStore) UpdateNotificationPreferences(ctx context.Context, userID string, prefs map[string]any) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return err
	}
	_, err = s.db.NewUpdate().Model((*models.User)(nil)).
		Set("notification_preferences = ?", prefs).
		Where("id = ?", uid).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("update notification preferences: %w", err)
	}
	return nil
}

func (s *pgUserStore) GetByGoogleID(googleID string) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u := new(models.User)
	err := s.db.NewSelect().Model(u).Where("google_id = ?", googleID).Scan(ctx)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user by google id")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) GetBySlackUserID(slackUserID string) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u := new(models.User)
	err := s.db.NewSelect().Model(u).Where("slack_user_id = ?", slackUserID).Scan(ctx)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user by slack id")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) UpdateGoogleID(userID uuid.UUID, googleID string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("google_id = ?", googleID).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update google_id: %w", err)
	}
	return nil
}

func (s *pgUserStore) ClearGoogleID(userID uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("google_id = ?", "").
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear google_id: %w", err)
	}
	return nil
}

func (s *pgUserStore) SetSlackIdentity(ctx context.Context, userID uuid.UUID, slackUserID, slackDisplayName string) error {
	_, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("slack_user_id = ?", slackUserID).
		Set("slack_display_name = ?", slackDisplayName).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		// users_slack_user_id is a partial unique index over non-empty values,
		// so a second user claiming an identity already bound elsewhere lands here.
		if pgIsDuplicateKey(err) {
			return ErrSlackIdentityTaken
		}
		return fmt.Errorf("failed to set slack identity: %w", err)
	}
	return nil
}

func (s *pgUserStore) ClearSlackIdentity(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("slack_user_id = ?", "").
		Set("slack_display_name = ?", "").
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear slack identity: %w", err)
	}
	return nil
}

func pgUserToRecord(u *models.User) *UserRecord {
	return &UserRecord{
		ID:                  u.ID,
		FullName:            u.FullName,
		Phone:               u.Phone,
		PhoneCountry:        u.PhoneCountry,
		Email:               u.Email,
		Password:            u.Password,
		Role:                u.Role,
		CreatedAt:           u.CreatedAt,
		UpdatedAt:           u.UpdatedAt,
		GoogleID:            u.GoogleID,
		SlackUserID:         u.SlackUserID,
		SlackDisplayName:    u.SlackDisplayName,
		FailedLoginAttempts: u.FailedLoginAttempts,
		LockedUntil:         u.LockedUntil,
		LastFailedLogin:     u.LastFailedLogin,
		LastLoginAt:         u.LastLoginAt,
		LastLoginIP:         u.LastLoginIP,
		VoiceOptOut:         u.VoiceOptOut,
	}
}
