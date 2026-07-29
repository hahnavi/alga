-- +goose Up

-- alert_investigations ------------------------------------------------------
-- public_id (renamed from alert_investigation_id) is the human/public TEXT id;
-- it is distinct from the UUID primary key. agent_id stays TEXT (empty-string
-- sentinel via Set("agent_id = '' ")). The FK for
-- promoted_incident_investigation_id is added after incident_investigations.
CREATE TABLE alert_investigations (
    id UUID PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    correlation_key TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'assigned', 'investigating', 'promoted', 'complete', 'failed', 'cancelled', 'timed_out', 'paused')),
    agent_id TEXT DEFAULT '',
    agent_name TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    primary_thread_id TEXT DEFAULT '',
    slack_channel_id TEXT DEFAULT '',
    slack_thread_ts TEXT DEFAULT '',
    mm_post_id TEXT DEFAULT '',
    mm_thread_id TEXT DEFAULT '',
    promoted_incident_id UUID REFERENCES incidents (id) ON DELETE SET NULL,
    promoted_incident_investigation_id UUID,
    summary JSONB,
    findings JSONB,
    evidence JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    completed_reason TEXT DEFAULT '',
    completed_by_type TEXT DEFAULT '',
    completed_by_id TEXT DEFAULT '',
    completed_by_name TEXT DEFAULT '',
    started_at TIMESTAMPTZ,
    investigating_duration_ms BIGINT DEFAULT 0,
    primary_alert_fingerprint TEXT NOT NULL DEFAULT '',
    primary_alert_number BIGINT CHECK (primary_alert_number >= 0),
    escalation_level TEXT DEFAULT '',
    triage_result_id UUID REFERENCES triage_results (id) ON DELETE SET NULL,
    triage_decision TEXT DEFAULT '',
    triage_enrichment JSONB,
    assignee_type TEXT NOT NULL DEFAULT 'agent' CHECK (assignee_type IN ('agent', 'user', 'system', 'grafana')),
    assignee_id UUID
);
CREATE INDEX idx_alert_investigations_status_created_at ON alert_investigations (status, created_at);
CREATE INDEX idx_alert_investigations_correlation_key_status ON alert_investigations (correlation_key, status);
CREATE INDEX idx_alert_investigations_promoted_incident_id ON alert_investigations (promoted_incident_id);
CREATE INDEX idx_alert_investigations_promoted_incident_inv_id ON alert_investigations (promoted_incident_investigation_id);
CREATE INDEX idx_alert_investigations_primary_alert_fingerprint ON alert_investigations (primary_alert_fingerprint);
CREATE INDEX idx_alert_investigations_primary_alert_number ON alert_investigations (primary_alert_number);
CREATE INDEX idx_alert_investigations_triage_result_id ON alert_investigations (triage_result_id);
CREATE INDEX idx_alert_investigations_assignee ON alert_investigations (assignee_type, assignee_id, status);
CREATE TRIGGER trg_alert_investigations_set_updated_at BEFORE UPDATE ON alert_investigations FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- alert_investigation_alerts ------------------------------------------------
-- investigation_id (renamed from alert_investigation_id) is the UUID FK to the
-- parent investigation. Renamed to disambiguate from the TEXT public_id.
CREATE TABLE alert_investigation_alerts (
    id UUID PRIMARY KEY,
    investigation_id UUID NOT NULL REFERENCES alert_investigations (id) ON DELETE CASCADE,
    alert_id UUID REFERENCES alerts (id) ON DELETE SET NULL,
    fingerprint TEXT NOT NULL,
    alert_number BIGINT NOT NULL DEFAULT 0 CHECK (alert_number >= 0),
    status TEXT DEFAULT '',
    alertname TEXT DEFAULT '',
    namespace TEXT DEFAULT '',
    labels JSONB,
    annotations JSONB,
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    generator_url TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_alert_investigation_alerts_fingerprint ON alert_investigation_alerts (fingerprint);
CREATE INDEX idx_alert_investigation_alerts_investigation_id ON alert_investigation_alerts (investigation_id);
CREATE UNIQUE INDEX idx_alert_investigation_alerts_alert_number_current ON alert_investigation_alerts (alert_number, current) WHERE current = true AND alert_number > 0;
CREATE UNIQUE INDEX idx_alert_investigation_alerts_alert_id_current ON alert_investigation_alerts (alert_id, current) WHERE current = true AND alert_id IS NOT NULL;
CREATE TRIGGER trg_alert_investigation_alerts_set_updated_at BEFORE UPDATE ON alert_investigation_alerts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- alert_investigation_events ------------------------------------------------
CREATE TABLE alert_investigation_events (
    id UUID PRIMARY KEY,
    alert_investigation_id UUID NOT NULL REFERENCES alert_investigations (id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('assigned', 'started', 'requeued', 'agent_offline_before_start', 'agent_offline_during_work', 'dispatch_failed', 'auto_completed', 'completed')),
    reason TEXT DEFAULT '',
    actor_type TEXT DEFAULT 'system',
    actor_id TEXT DEFAULT '',
    actor_name TEXT DEFAULT '',
    agent_id TEXT DEFAULT '',
    agent_name TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_alert_investigation_events_inv_created ON alert_investigation_events (alert_investigation_id, created_at);
CREATE INDEX idx_alert_investigation_events_event_type ON alert_investigation_events (event_type);
CREATE TRIGGER trg_alert_investigation_events_set_updated_at BEFORE UPDATE ON alert_investigation_events FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- alert_investigation_updates -----------------------------------------------
CREATE TABLE alert_investigation_updates (
    id UUID PRIMARY KEY,
    alert_investigation_id UUID NOT NULL REFERENCES alert_investigations (id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    message TEXT NOT NULL,
    source TEXT NOT NULL,
    internal BOOLEAN NOT NULL DEFAULT false,
    edited BOOLEAN NOT NULL DEFAULT false,
    user_id TEXT,
    username TEXT,
    mm_post_id TEXT DEFAULT '',
    slack_message_ts TEXT DEFAULT '',
    quoted_update_id TEXT,
    mentions JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_alert_investigation_updates_created_at ON alert_investigation_updates (created_at);
CREATE INDEX idx_alert_investigation_updates_investigation_id ON alert_investigation_updates (alert_investigation_id);
CREATE TRIGGER trg_alert_investigation_updates_set_updated_at BEFORE UPDATE ON alert_investigation_updates FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- incident_investigations ---------------------------------------------------
CREATE TABLE incident_investigations (
    id UUID PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    incident_id UUID REFERENCES incidents (id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'assigned', 'investigating', 'paused', 'complete', 'cancelled', 'coordinating')),
    agent_id TEXT DEFAULT '',
    agent_name TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    primary_thread_id TEXT DEFAULT '',
    slack_channel_id TEXT DEFAULT '',
    slack_thread_ts TEXT DEFAULT '',
    mm_post_id TEXT DEFAULT '',
    mm_thread_id TEXT DEFAULT '',
    source_alert_investigation_id UUID REFERENCES alert_investigations (id) ON DELETE SET NULL,
    summary JSONB,
    findings JSONB,
    evidence JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    investigating_duration_ms BIGINT DEFAULT 0,
    parent_investigation_id UUID,
    assignee_type TEXT NOT NULL DEFAULT 'agent' CHECK (assignee_type IN ('agent', 'user', 'system', 'grafana')),
    assignee_id UUID,
    CONSTRAINT fk_incident_investigations_parent FOREIGN KEY (parent_investigation_id) REFERENCES incident_investigations (id) ON DELETE SET NULL
);
CREATE INDEX idx_incident_investigations_status_created_at ON incident_investigations (status, created_at);
CREATE INDEX idx_incident_investigations_source_alert_inv_id ON incident_investigations (source_alert_investigation_id);
CREATE INDEX idx_incident_investigations_incident_id_status ON incident_investigations (incident_id, status);
CREATE INDEX idx_incident_investigations_parent_id ON incident_investigations (parent_investigation_id);
CREATE INDEX idx_incident_investigations_assignee ON incident_investigations (assignee_type, assignee_id, status);
CREATE TRIGGER trg_incident_investigations_set_updated_at BEFORE UPDATE ON incident_investigations FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Deferred FK: alert_investigations.promoted_incident_investigation_id.
ALTER TABLE alert_investigations
    ADD CONSTRAINT fk_alert_investigations_promoted_incident_inv
    FOREIGN KEY (promoted_incident_investigation_id) REFERENCES incident_investigations (id) ON DELETE SET NULL;

-- incident_investigation_updates (append-only; created_at only) -------------
CREATE TABLE incident_investigation_updates (
    id UUID PRIMARY KEY,
    incident_investigation_id UUID NOT NULL REFERENCES incident_investigations (id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    message TEXT NOT NULL,
    source TEXT NOT NULL,
    internal BOOLEAN NOT NULL DEFAULT false,
    edited BOOLEAN NOT NULL DEFAULT false,
    user_id TEXT,
    username TEXT,
    mm_post_id TEXT DEFAULT '',
    slack_message_ts TEXT DEFAULT '',
    quoted_update_id TEXT,
    mentions JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_incident_investigation_updates_created_at ON incident_investigation_updates (created_at);
CREATE INDEX idx_incident_investigation_updates_inv_id ON incident_investigation_updates (incident_investigation_id);

-- investigation_threads -----------------------------------------------------
CREATE TABLE investigation_threads (
    id UUID PRIMARY KEY,
    thread_id TEXT NOT NULL UNIQUE,
    owner_type TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_investigation_threads_owner ON investigation_threads (owner_type, owner_id);
CREATE TRIGGER trg_investigation_threads_set_updated_at BEFORE UPDATE ON investigation_threads FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- investigation_thread_messages ---------------------------------------------
CREATE TABLE investigation_thread_messages (
    id UUID PRIMARY KEY,
    thread_id UUID NOT NULL REFERENCES investigation_threads (id) ON DELETE CASCADE,
    type TEXT NOT NULL DEFAULT 'comment',
    source TEXT NOT NULL DEFAULT 'user',
    message TEXT NOT NULL,
    internal BOOLEAN NOT NULL DEFAULT false,
    edited BOOLEAN NOT NULL DEFAULT false,
    user_id TEXT DEFAULT '',
    username TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    mm_post_id TEXT DEFAULT '',
    slack_message_ts TEXT DEFAULT '',
    reply_to_message_id TEXT DEFAULT '',
    mentions JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_investigation_thread_messages_thread_created ON investigation_thread_messages (thread_id, created_at);
CREATE TRIGGER trg_investigation_thread_messages_set_updated_at BEFORE UPDATE ON investigation_thread_messages FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Deferred FK: coordination_tasks.linked_investigation_id (from 00006).
ALTER TABLE coordination_tasks
    ADD CONSTRAINT coordination_tasks_linked_investigation_id_fk
    FOREIGN KEY (linked_investigation_id) REFERENCES alert_investigations (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE coordination_tasks DROP CONSTRAINT IF EXISTS coordination_tasks_linked_investigation_id_fk;
DROP TABLE IF EXISTS investigation_thread_messages CASCADE;
DROP TABLE IF EXISTS investigation_threads CASCADE;
DROP TABLE IF EXISTS incident_investigation_updates CASCADE;
ALTER TABLE alert_investigations DROP CONSTRAINT IF EXISTS fk_alert_investigations_promoted_incident_inv;
DROP TABLE IF EXISTS incident_investigations CASCADE;
DROP TABLE IF EXISTS alert_investigation_updates CASCADE;
DROP TABLE IF EXISTS alert_investigation_events CASCADE;
DROP TABLE IF EXISTS alert_investigation_alerts CASCADE;
DROP TABLE IF EXISTS alert_investigations CASCADE;
