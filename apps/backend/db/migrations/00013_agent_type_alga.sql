-- +goose Up
-- Add the "alga" agent type to the allowed agent_type CHECK constraints.

ALTER TABLE agent_tokens
    DROP CONSTRAINT IF EXISTS agent_tokens_agent_type_check,
    ADD CONSTRAINT agent_tokens_agent_type_check
        CHECK (agent_type IN ('alga', 'hermes', 'openclaw', 'other'));

ALTER TABLE agent_asks
    DROP CONSTRAINT IF EXISTS agent_asks_from_agent_type_check,
    ADD CONSTRAINT agent_asks_from_agent_type_check
        CHECK (from_agent_type IN ('alga', 'hermes', 'openclaw', 'other')),
    DROP CONSTRAINT IF EXISTS agent_asks_to_agent_type_check,
    ADD CONSTRAINT agent_asks_to_agent_type_check
        CHECK (to_agent_type IN ('alga', 'hermes', 'openclaw', 'other'));

-- +goose Down
-- Fail closed: refuse to downgrade while any row still uses the "alga" agent
-- type. Restoring the legacy CHECK constraints would otherwise error mid-way
-- (or force silent data loss), so demand an explicit data migration first.
-- +goose StatementBegin
DO $$
DECLARE
    alga_count integer;
BEGIN
    SELECT COUNT(*) INTO alga_count
    FROM (
        SELECT 1 FROM agent_tokens WHERE agent_type = 'alga'
        UNION ALL
        SELECT 1 FROM agent_asks WHERE from_agent_type = 'alga'
        UNION ALL
        SELECT 1 FROM agent_asks WHERE to_agent_type = 'alga'
    ) AS alga_rows;

    IF alga_count > 0 THEN
        RAISE EXCEPTION
            'cannot downgrade migration 00013: % row(s) still use agent_type "alga"; migrate this data to a legacy agent type before downgrading',
            alga_count;
    END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

ALTER TABLE agent_tokens
    DROP CONSTRAINT IF EXISTS agent_tokens_agent_type_check,
    ADD CONSTRAINT agent_tokens_agent_type_check
        CHECK (agent_type IN ('hermes', 'openclaw', 'other'));

ALTER TABLE agent_asks
    DROP CONSTRAINT IF EXISTS agent_asks_from_agent_type_check,
    ADD CONSTRAINT agent_asks_from_agent_type_check
        CHECK (from_agent_type IN ('hermes', 'openclaw', 'other')),
    DROP CONSTRAINT IF EXISTS agent_asks_to_agent_type_check,
    ADD CONSTRAINT agent_asks_to_agent_type_check
        CHECK (to_agent_type IN ('hermes', 'openclaw', 'other'));
