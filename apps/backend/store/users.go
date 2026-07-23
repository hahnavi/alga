package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	algacrypto "alga/crypto"
	"alga/ent"
	"alga/ent/user"
)

var (
	ErrAccountLocked      = errors.New("account is locked due to too many failed login attempts")
	ErrInvalidCredentials = errors.New("invalid credentials")
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

func newPGUserStore(client *ent.Client) UserStore {
	return &pgUserStore{pgStoreBase{client: client}}
}

func (s *pgUserStore) CreateUser(email, password, role string) (*UserRecord, error) {
	hash, err := algacrypto.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()

	saved, err := s.client.User.Create().
		SetEmail(email).
		SetPassword(hash).
		SetRole(role).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return pgUserToRecord(saved), nil
}

func (s *pgUserStore) GetByEmail(email string) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u, err := s.client.User.Query().Where(user.Email(email)).Only(ctx)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user by email")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) GetByID(id uuid.UUID) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u, err := s.client.User.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) ListUsers() ([]UserRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	users, err := s.client.User.Query().Limit(500).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	records := make([]UserRecord, 0, len(users))
	for _, u := range users {
		records = append(records, *pgUserToRecord(u))
	}
	return records, nil
}

func (s *pgUserStore) UpdateUser(id uuid.UUID, updates map[string]any) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	b := s.client.User.UpdateOneID(id)

	if v, ok := updates["email"].(string); ok {
		b.SetEmail(v)
	}
	if v, ok := updates["password"].(string); ok {
		b.SetPassword(v)
	}
	if v, ok := updates["role"].(string); ok {
		b.SetRole(v)
	}
	if v, ok := updates["full_name"].(string); ok {
		b.SetFullName(v)
	}
	if v, ok := updates["phone"].(string); ok {
		b.SetPhone(v)
	}
	if v, ok := updates["phone_country"].(string); ok {
		b.SetPhoneCountry(v)
	}
	if v, ok := updates["failed_login_attempts"].(int); ok {
		b.SetFailedLoginAttempts(v)
	}
	if v, ok := updates["locked_until"]; ok {
		if t, ok := v.(*time.Time); ok && t != nil {
			b.SetLockedUntil(*t)
		} else {
			b.ClearLockedUntil()
		}
	}
	if v, ok := updates["last_failed_login"]; ok {
		if t, ok := v.(*time.Time); ok && t != nil {
			b.SetLastFailedLogin(*t)
		} else {
			b.ClearLastFailedLogin()
		}
	}
	if v, ok := updates["last_login_at"]; ok {
		if t, ok := v.(*time.Time); ok && t != nil {
			b.SetLastLoginAt(*t)
		} else {
			b.ClearLastLoginAt()
		}
	}
	if v, ok := updates["last_login_ip"].(string); ok {
		b.SetLastLoginIP(v)
	}
	if v, ok := updates["slack_user_id"].(string); ok {
		b.SetSlackUserID(v)
	}
	if v, ok := updates["slack_display_name"].(string); ok {
		b.SetSlackDisplayName(v)
	}
	if v, ok := updates["voice_opt_out"].(bool); ok {
		b.SetVoiceOptOut(v)
	}

	b.SetUpdatedAt(time.Now().UTC())

	_, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("user not found: %w", ErrUserNotFound)
		}
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

func (s *pgUserStore) DeleteUser(id uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	err := s.client.User.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("user not found: %w", ErrUserNotFound)
		}
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

func (s *pgUserStore) CountAdmins() (int64, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	count, err := s.client.User.Query().Where(user.Role("admin")).Count(ctx)
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
			_, _ = s.client.User.UpdateOneID(u.ID).
				SetPassword(newHash).
				SetUpdatedAt(time.Now().UTC()).
				Save(ctx)
		}
	}

	return u, nil
}

func (s *pgUserStore) RecordFailedLogin(email string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	now := time.Now().UTC()

	u, err := s.client.User.Query().Where(user.Email(email)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to record failed login: %w", err)
	}

	newAttempts := u.FailedLoginAttempts + 1

	b := s.client.User.UpdateOneID(u.ID).
		SetFailedLoginAttempts(newAttempts).
		SetLastFailedLogin(now).
		SetUpdatedAt(now)

	if newAttempts >= 5 {
		lockoutEnd := now.Add(30 * time.Minute)
		b.SetLockedUntil(lockoutEnd)
	}

	_, err = b.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to record failed login: %w", err)
	}
	return nil
}

func (s *pgUserStore) RecordSuccessfulLogin(userID uuid.UUID, ip string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	now := time.Now().UTC()

	_, err := s.client.User.UpdateOneID(userID).
		SetFailedLoginAttempts(0).
		ClearLockedUntil().
		SetLastLoginAt(now).
		SetLastLoginIP(ip).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to record successful login: %w", err)
	}
	return nil
}

func (s *pgUserStore) UnlockAccount(userID uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.client.User.UpdateOneID(userID).
		SetFailedLoginAttempts(0).
		ClearLockedUntil().
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("user not found: %w", ErrUserNotFound)
		}
		return fmt.Errorf("failed to unlock account: %w", err)
	}
	return nil
}

func (s *pgUserStore) CountUsers() (int64, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	count, err := s.client.User.Query().Count(ctx)
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
	u, err := s.client.User.Get(ctx, uid)
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
	_, err = s.client.User.UpdateOneID(uid).
		SetNotificationPreferences(prefs).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update notification preferences: %w", err)
	}
	return nil
}

func (s *pgUserStore) GetByGoogleID(googleID string) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u, err := s.client.User.Query().Where(user.GoogleID(googleID)).Only(ctx)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user by google id")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) GetBySlackUserID(slackUserID string) (*UserRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	u, err := s.client.User.Query().Where(user.SlackUserIDEQ(slackUserID)).Only(ctx)
	if err != nil {
		return handleQueryErr[*UserRecord](err, "user by slack id")
	}
	return pgUserToRecord(u), nil
}

func (s *pgUserStore) UpdateGoogleID(userID uuid.UUID, googleID string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	err := s.client.User.UpdateOneID(userID).SetGoogleID(googleID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update google_id: %w", err)
	}
	return nil
}

func (s *pgUserStore) ClearGoogleID(userID uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.client.User.UpdateOneID(userID).
		ClearGoogleID().
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear google_id: %w", err)
	}
	return nil
}

func (s *pgUserStore) SetSlackIdentity(ctx context.Context, userID uuid.UUID, slackUserID, slackDisplayName string) error {
	_, err := s.client.User.UpdateOneID(userID).
		SetSlackUserID(slackUserID).
		SetSlackDisplayName(slackDisplayName).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set slack identity: %w", err)
	}
	return nil
}

func (s *pgUserStore) ClearSlackIdentity(ctx context.Context, userID uuid.UUID) error {
	_, err := s.client.User.UpdateOneID(userID).
		SetSlackUserID("").
		SetSlackDisplayName("").
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear slack identity: %w", err)
	}
	return nil
}

func pgUserToRecord(u *ent.User) *UserRecord {
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
