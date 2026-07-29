-- +goose Up
CREATE TABLE playbook_steps (
    id UUID PRIMARY KEY,
    playbook_id UUID NOT NULL,
    step_number INT NOT NULL CHECK (step_number > 0),
    title TEXT NOT NULL,
    description TEXT NULL,
    expected_duration TEXT NULL,
    command TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT playbook_steps_playbook_id_fk FOREIGN KEY (playbook_id) REFERENCES playbooks (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX playbook_steps_playbook_id_step_number ON playbook_steps (playbook_id, step_number);

-- +goose Down
DROP TABLE IF EXISTS playbook_steps;
