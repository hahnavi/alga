-- +goose Up
ALTER TABLE agent_tokens ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE agent_asks ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE alert_investigation_events ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE alert_investigation_updates ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- +goose Down
ALTER TABLE agent_tokens DROP COLUMN IF EXISTS updated_at;
ALTER TABLE agent_asks DROP COLUMN IF EXISTS updated_at;
ALTER TABLE alert_investigation_events DROP COLUMN IF EXISTS updated_at;
ALTER TABLE alert_investigation_updates DROP COLUMN IF EXISTS updated_at;
