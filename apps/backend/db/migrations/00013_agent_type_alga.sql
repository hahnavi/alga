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
