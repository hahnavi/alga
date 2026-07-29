-- +goose Up
CREATE TABLE team_members (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'lead')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT team_members_team_id_fk FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE CASCADE,
    CONSTRAINT team_members_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX team_members_team_id_user_id ON team_members (team_id, user_id);
CREATE INDEX team_members_user_id ON team_members (user_id);

-- +goose Down
DROP TABLE IF EXISTS team_members;
