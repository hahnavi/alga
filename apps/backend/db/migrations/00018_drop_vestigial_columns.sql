-- +goose Up
-- WP-D2: drop vestigial columns whose values are provably default-only
-- because no writer exists anywhere in the codebase.
--
-- audit_logs.request_id deliberately stays: WP-C9 wired request-ID
-- correlation into the audit LogRecord path, so that column carries live
-- data as of the 2026-08 remediation (the finding predates that work).

-- webhook_tokens.revoked: RevokeToken hard-deletes rows and nothing ever
-- sets true. The defensive DELETE keeps hand-inserted or legacy soft-revoked
-- rows from silently turning valid after the read filters disappear.
DELETE FROM webhook_tokens WHERE revoked = true;

ALTER TABLE webhook_tokens DROP COLUMN revoked;
CREATE INDEX webhook_tokens_lookup_prefix ON webhook_tokens (lookup_prefix);

ALTER TABLE incidents DROP COLUMN auto_confirmed;

ALTER TABLE agent_tokens DROP COLUMN default_for_investigation;
DROP INDEX IF EXISTS idx_agent_tokens_default_for_investigation;

-- +goose Down
-- Authoring documentation / manual recovery script only: the binary executes
-- up-blocks exclusively (no down CLI path exists). Dropped data is
-- unrecoverable by design — every dropped value was false by construction.
ALTER TABLE webhook_tokens ADD COLUMN revoked BOOLEAN NOT NULL DEFAULT false;
DROP INDEX IF EXISTS webhook_tokens_lookup_prefix;
CREATE INDEX webhook_tokens_lookup_prefix ON webhook_tokens (lookup_prefix) WHERE revoked = false;

ALTER TABLE incidents ADD COLUMN auto_confirmed BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE agent_tokens ADD COLUMN default_for_investigation BOOLEAN DEFAULT false;
CREATE INDEX idx_agent_tokens_default_for_investigation ON agent_tokens (default_for_investigation) WHERE default_for_investigation = true;
