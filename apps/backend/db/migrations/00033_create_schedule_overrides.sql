-- +goose Up
CREATE TABLE schedule_overrides (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL,
    user_id UUID NOT NULL,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    created_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT schedule_overrides_schedule_id_fk FOREIGN KEY (schedule_id) REFERENCES on_call_schedules (id) ON DELETE CASCADE,
    CONSTRAINT schedule_overrides_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE INDEX schedule_overrides_schedule_id ON schedule_overrides (schedule_id);
CREATE INDEX schedule_overrides_user_id ON schedule_overrides (user_id);
CREATE INDEX schedule_overrides_start_at_end_at ON schedule_overrides (start_at, end_at);

-- +goose Down
DROP TABLE IF EXISTS schedule_overrides;
