-- +goose Up
CREATE TABLE on_call_schedules (
    id UUID PRIMARY KEY,
    team_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT on_call_schedules_team_id_fk FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX on_call_schedules_team_id ON on_call_schedules (team_id) WHERE team_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS on_call_schedules;
