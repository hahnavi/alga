-- the triage override endpoint accepts a reason but had nowhere to
-- persist it. Track why a human overrode the LLM/rule decision alongside the
-- existing overridden_to/by/at columns.

-- +goose Up
ALTER TABLE triage_results ADD COLUMN IF NOT EXISTS override_reason TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE triage_results DROP COLUMN IF EXISTS override_reason;
