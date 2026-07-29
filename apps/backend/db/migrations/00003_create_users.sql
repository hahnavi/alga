-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'operator', 'viewer')),
    full_name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    phone_country TEXT NOT NULL DEFAULT '',
    failed_login_attempts INT NOT NULL DEFAULT 0 CHECK (failed_login_attempts >= 0),
    locked_until TIMESTAMPTZ NULL,
    last_failed_login TIMESTAMPTZ NULL,
    last_login_at TIMESTAMPTZ NULL,
    last_login_ip TEXT NOT NULL DEFAULT '',
    google_id TEXT NOT NULL DEFAULT '',
    slack_user_id TEXT NOT NULL DEFAULT '',
    slack_display_name TEXT NOT NULL DEFAULT '',
    notification_preferences JSONB NULL,
    voice_opt_out BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX users_google_id ON users (google_id);
CREATE UNIQUE INDEX users_slack_user_id ON users (slack_user_id) WHERE slack_user_id <> '';

-- +goose Down
DROP TABLE IF EXISTS users;
