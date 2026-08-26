-- +goose Up
-- WP-B6: gate agent secret fetches behind a `secrets` capability.
-- Grandfather clause: every non-revoked agent token that lacks the new
-- capability gets it appended once, preserving today's de-facto access.
-- Without this, every production agent would lose secret access at deploy
-- (secure but disruptive); granting it to future tokens by default would
-- make the gate theater. Re-running is a no-op (the WHERE filters rows that
-- already carry "secrets").

UPDATE agent_tokens
SET capabilities = capabilities || '"secrets"'::jsonb
WHERE revoked = false
  AND NOT capabilities ? 'secrets';

-- +goose Down
-- Authoring documentation / manual recovery script only: the binary executes
-- up-blocks exclusively (no down CLI path exists).
UPDATE agent_tokens
SET capabilities = capabilities - 'secrets'
WHERE capabilities ? 'secrets';
