-- +goose Up
CREATE TABLE automation_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('alert', 'incident')),
    entity_id UUID NOT NULL,
    entity_number BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at TIMESTAMPTZ NULL
);
CREATE INDEX automation_events_pending ON automation_events (created_at, id) WHERE dispatched_at IS NULL;

CREATE TABLE automation_rules (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    description TEXT NOT NULL DEFAULT '',
    trigger_type TEXT NOT NULL,
    conditions JSONB NOT NULL DEFAULT '[]'::jsonb,
    actions JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    version INT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX automation_rules_trigger_enabled ON automation_rules (trigger_type) WHERE enabled AND archived_at IS NULL;
CREATE TRIGGER trg_automation_rules_set_updated_at BEFORE UPDATE ON automation_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE automation_runs (
    id UUID PRIMARY KEY,
    rule_id UUID NOT NULL REFERENCES automation_rules (id),
    event_id UUID NOT NULL REFERENCES automation_events (id),
    rule_name TEXT NOT NULL,
    rule_version INT NOT NULL,
    actions JSONB NOT NULL,
    event_payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    next_action INT NOT NULL DEFAULT 0 CHECK (next_action >= 0),
    attempt INT NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until TIMESTAMPTZ NULL,
    last_error TEXT NOT NULL DEFAULT '',
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (rule_id, event_id)
);
CREATE INDEX automation_runs_due ON automation_runs (next_attempt_at, created_at) WHERE status IN ('pending', 'running');
CREATE INDEX automation_runs_rule_created ON automation_runs (rule_id, created_at DESC);
CREATE TRIGGER trg_automation_runs_set_updated_at BEFORE UPDATE ON automation_runs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE automation_run_steps (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES automation_runs (id) ON DELETE CASCADE,
    action_index INT NOT NULL CHECK (action_index >= 0),
    attempt INT NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN ('succeeded', 'retrying', 'failed')),
    http_status INT NULL,
    error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX automation_run_steps_run ON automation_run_steps (run_id, action_index, attempt);

-- The event trigger is a temporary producer for all current write paths. The
-- planned incident domain-event outbox can replace this producer while keeping
-- the automation_events consumer contract and uniqueness at the source.
-- +goose StatementBegin
CREATE FUNCTION capture_alert_automation_event() RETURNS trigger AS $$
DECLARE
    kind TEXT;
    prior TEXT;
BEGIN
    IF NEW.deleted_at IS NOT NULL THEN RETURN NEW; END IF;
    IF TG_OP = 'INSERT' THEN
        kind := 'alert.created';
        prior := NULL;
    ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
        IF NEW.status = 'resolved' THEN kind := 'alert.resolved';
        ELSIF OLD.status = 'resolved' AND NEW.status = 'firing' THEN kind := 'alert.reopened';
        ELSE RETURN NEW;
        END IF;
        prior := OLD.status;
    ELSIF NEW.acknowledged AND NOT OLD.acknowledged THEN
        kind := 'alert.acknowledged';
        prior := OLD.status;
    ELSE
        RETURN NEW;
    END IF;
    INSERT INTO automation_events (event_type, entity_type, entity_id, entity_number, payload)
    VALUES (kind, 'alert', NEW.id, NEW.alert_number,
        jsonb_build_object('schema_version', 1, 'event_type', kind, 'entity_type', 'alert', 'entity_id', NEW.id,
            'entity_number', NEW.alert_number, 'status', NEW.status, 'previous_status', prior,
            'severity', COALESCE(NULLIF(NEW.severity_classified, ''), NEW.labels->>'severity', ''),
            'labels', NEW.labels, 'silenced', NEW.silenced));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER trg_alerts_automation_insert AFTER INSERT ON alerts FOR EACH ROW EXECUTE FUNCTION capture_alert_automation_event();
CREATE TRIGGER trg_alerts_automation_update AFTER UPDATE OF status, acknowledged ON alerts FOR EACH ROW EXECUTE FUNCTION capture_alert_automation_event();

-- +goose StatementBegin
CREATE FUNCTION capture_incident_automation_event() RETURNS trigger AS $$
DECLARE
    kind TEXT;
    prior TEXT;
BEGIN
    IF NEW.deleted_at IS NOT NULL THEN RETURN NEW; END IF;
    IF TG_OP = 'INSERT' THEN
        kind := 'incident.created';
        prior := NULL;
    ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
        kind := 'incident.status_changed';
        prior := OLD.status;
    ELSE
        RETURN NEW;
    END IF;
    INSERT INTO automation_events (event_type, entity_type, entity_id, entity_number, payload)
    VALUES (kind, 'incident', NEW.id, NEW.incident_number,
        jsonb_build_object('schema_version', 1, 'event_type', kind, 'entity_type', 'incident', 'entity_id', NEW.id,
            'entity_number', NEW.incident_number, 'status', NEW.status, 'previous_status', prior,
            'severity', NEW.severity, 'priority', NEW.priority, 'service_id', NEW.service_id,
            'labels', '{}'::jsonb));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER trg_incidents_automation_insert AFTER INSERT ON incidents FOR EACH ROW EXECUTE FUNCTION capture_incident_automation_event();
CREATE TRIGGER trg_incidents_automation_update AFTER UPDATE OF status ON incidents FOR EACH ROW EXECUTE FUNCTION capture_incident_automation_event();

-- +goose Down
DROP TRIGGER IF EXISTS trg_incidents_automation_update ON incidents;
DROP TRIGGER IF EXISTS trg_incidents_automation_insert ON incidents;
DROP FUNCTION IF EXISTS capture_incident_automation_event();
DROP TRIGGER IF EXISTS trg_alerts_automation_update ON alerts;
DROP TRIGGER IF EXISTS trg_alerts_automation_insert ON alerts;
DROP FUNCTION IF EXISTS capture_alert_automation_event();
DROP TABLE IF EXISTS automation_run_steps;
DROP TABLE IF EXISTS automation_runs;
DROP TABLE IF EXISTS automation_rules;
DROP TABLE IF EXISTS automation_events;
