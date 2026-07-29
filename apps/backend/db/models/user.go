package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users"`

	ID                      uuid.UUID      `bun:"id,pk"`
	Email                   string         `bun:"email,notnull,unique"`
	Password                string         `bun:"password,notnull"`
	Role                    string         `bun:"role,notnull,default:'viewer'"`
	FullName                string         `bun:"full_name,default:''"`
	Phone                   string         `bun:"phone,default:''"`
	PhoneCountry            string         `bun:"phone_country,default:''"`
	FailedLoginAttempts     int            `bun:"failed_login_attempts,notnull,default:0"`
	LockedUntil             *time.Time     `bun:"locked_until"`
	LastFailedLogin         *time.Time     `bun:"last_failed_login"`
	LastLoginAt             *time.Time     `bun:"last_login_at"`
	LastLoginIP             string         `bun:"last_login_ip,default:''"`
	GoogleID                string         `bun:"google_id,default:''"`
	SlackUserID             string         `bun:"slack_user_id,default:''"`
	SlackDisplayName        string         `bun:"slack_display_name,default:''"`
	NotificationPreferences map[string]any `bun:"notification_preferences,type:jsonb"`
	VoiceOptOut             bool           `bun:"voice_opt_out,notnull,default:false"`
	CreatedAt               time.Time      `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt               time.Time      `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*User) TableName() string { return "users" }
