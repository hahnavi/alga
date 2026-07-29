-- +goose Up

-- Extensions ----------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS vector;

-- Shared trigger function ---------------------------------------------------
-- Authoritative updated_at maintenance. Bun's NewUpdate().Set(...) does not
-- bypass DB triggers, so this fires for every UPDATE regardless of the caller.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Human-readable number sequences -------------------------------------------
-- These replace the old hand-rolled `counters` CAS table. Explicit sequences
-- (not GENERATED AS IDENTITY) are required because ReserveIncidentNumber
-- allocates a number via nextval() without inserting a row. Gaps on rollback
-- are acceptable; the domain requires unique + monotonic, not gapless.
CREATE SEQUENCE alert_number_seq AS BIGINT START WITH 1 MINVALUE 1;
CREATE SEQUENCE incident_number_seq AS BIGINT START WITH 1 MINVALUE 1;
CREATE SEQUENCE triage_number_seq AS BIGINT START WITH 1 MINVALUE 1;

-- +goose Down
DROP SEQUENCE IF EXISTS triage_number_seq;
DROP SEQUENCE IF EXISTS incident_number_seq;
DROP SEQUENCE IF EXISTS alert_number_seq;
DROP FUNCTION IF EXISTS set_updated_at();
DROP EXTENSION IF EXISTS vector;
