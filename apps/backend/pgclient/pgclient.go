package pgclient

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/nyaruka/phonenumbers"

	"alga/ent"
	"alga/logger"
	"alga/trace"
)

type Client struct {
	Ent    *ent.Client
	Driver *entsql.Driver
	DB     *sql.DB
	DSN    string
}

func New(dsn string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	// Attach the pgx query tracer (one db.query span per SQL statement) only
	// when tracing is enabled; otherwise it stays nil so the driver adds zero
	// overhead (W1).
	if t := trace.PGXTracer(); t != nil {
		cfg.Tracer = t
	}

	db := sql.OpenDB(pgxstdlib.GetConnector(*cfg))
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(30 * time.Second)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Info("connected to PostgreSQL", "component", "pgclient")

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))

	return &Client{
		Ent:    client,
		Driver: drv,
		DB:     db,
		DSN:    dsn,
	}, nil
}

func (c *Client) Close() {
	_ = c.Ent.Close()
	if c.DB != nil {
		_ = c.DB.Close()
	}
}

func ApplyMigrations(ctx context.Context, dsn string) error {
	logger.Info("applying database migrations", "component", "pgclient")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := migrateEscalationCollapseToJSON(ctx, db); err != nil {
		return err
	}
	if err := dropLegacySchema(ctx, db); err != nil {
		return err
	}
	if err := applySoftDeleteSchema(ctx, db); err != nil {
		return err
	}
	if err := migrateAlertsIDToUUID(ctx, db); err != nil {
		return err
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))
	defer func() { _ = client.Close() }()

	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("apply ent schema: %w", err)
	}
	if err := migrateInvestigationThreadMessageReplyToColumn(ctx, db); err != nil {
		return err
	}
	if err := migrateScheduleLayerRestrictions(ctx, db); err != nil {
		return err
	}
	if err := applyOnCallScheduleUniqueTeam(ctx, db); err != nil {
		return err
	}
	if err := migrateGroupsToTeams(ctx, db); err != nil {
		return err
	}
	if err := applyUserVoiceOptOut(ctx, db); err != nil {
		return err
	}
	if err := applyUserPhoneCountry(ctx, db); err != nil {
		return err
	}
	if err := backfillUserPhoneCountry(ctx, db); err != nil {
		return err
	}
	if err := backfillTeamSchedules(ctx, db); err != nil {
		return err
	}
	if err := ApplyPgVector(ctx, db); err != nil {
		return fmt.Errorf("apply pgvector: %w", err)
	}
	if err := ApplyKnowledgeFTS(ctx, db); err != nil {
		return fmt.Errorf("apply knowledge fts: %w", err)
	}
	logger.Info("database migrations applied successfully", "component", "pgclient")
	return nil
}

// migrateEscalationCollapseToJSON folds the legacy escalation_levels and
// escalation_targets child tables into the new escalation_policies.levels
// JSONB column. The migration runs before dropLegacySchema (so the legacy
// tables are still readable for backfill) and before ent.Schema.Create (so
// ent does not try to ADD COLUMN with no DEFAULT across populated rows).
//
// Order of operations:
//  1. Confirm escalation_policies exists; no-op otherwise (fresh DB).
//  2. ADD COLUMN IF NOT EXISTS levels JSONB DEFAULT '[]'::jsonb.
//  3. Backfill existing rows. If the legacy escalation_levels table is still
//     present, group its rows by policy_id and collect per-level targets
//     from escalation_targets into the JSON shape expected by the runtime
//     store (EscalationLevelRecord). Rows whose policy has no legacy levels
//     receive '[]'. Rows already populated are not overwritten.
//  4. ALTER COLUMN levels SET NOT NULL so the DB matches the new schema's
//     NOT NULL declaration. The backfill in step 3 guarantees this is safe.
//
// This function is idempotent: re-running the safe ALTER + UPDATE statements
// converges to the same end state.
func migrateEscalationCollapseToJSON(ctx context.Context, db *sql.DB) error {
	var policyTableOID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('escalation_policies')::regclass::text`).Scan(&policyTableOID); err != nil {
		return fmt.Errorf("escalation collapse: check escalation_policies exists: %w", err)
	}
	if !policyTableOID.Valid {
		return nil
	}

	if _, err := db.ExecContext(ctx,
		`ALTER TABLE escalation_policies ADD COLUMN IF NOT EXISTS levels JSONB DEFAULT '[]'::jsonb`,
	); err != nil {
		return fmt.Errorf("escalation collapse: add levels column: %w", err)
	}

	var legacyLevelsOID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('escalation_levels')::regclass::text`).Scan(&legacyLevelsOID); err != nil {
		return fmt.Errorf("escalation collapse: check escalation_levels exists: %w", err)
	}

	if legacyLevelsOID.Valid {
		if _, err := db.ExecContext(ctx, `
			UPDATE escalation_policies ep
			SET levels = COALESCE((
				SELECT jsonb_agg(
					jsonb_build_object(
						'level_number', el.level_number,
						'delay_minutes', el.delay_minutes,
						'notify_channels', COALESCE(el.notify_channels, '[]'::jsonb),
							'targets', COALESCE((
								SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
									'target_type', et.target_type,
									'target_user_id', to_jsonb(et.target_user_id),
									'target_team_id', to_jsonb(et.target_team_id)
								)) ORDER BY et.target_type, et.id)
								FROM escalation_targets et
								WHERE et.level_id = el.id
							), '[]'::jsonb)
					)
					ORDER BY el.level_number
				)
				FROM escalation_levels el
				WHERE el.policy_id = ep.id
			), '[]'::jsonb)
			WHERE ep.levels IS NULL OR ep.levels = '[]'::jsonb
		`); err != nil {
			return fmt.Errorf("escalation collapse: backfill from legacy tables: %w", err)
		}
	}

	if _, err := db.ExecContext(ctx,
		`UPDATE escalation_policies SET levels = '[]'::jsonb WHERE levels IS NULL`,
	); err != nil {
		return fmt.Errorf("escalation collapse: default empty levels: %w", err)
	}

	if _, err := db.ExecContext(ctx,
		`ALTER TABLE escalation_policies ALTER COLUMN levels SET NOT NULL`,
	); err != nil {
		return fmt.Errorf("escalation collapse: set levels not null: %w", err)
	}
	return nil
}

// dropLegacySchema removes orphaned tables and redundant indexes left behind by
// prior schema changes. Every statement is idempotent (IF EXISTS) so existing
// deployments converge without a separate migration step.
//
// Four categories of legacy debris are cleaned up:
//
//  1. Renamed-then-abandoned tables. The original unified investigation model
//     was split into alert_investigations* and incident_investigations*; the
//     old tables were renamed to legacy_* (never dropped) and are now empty
//     and unreferenced by any Ent schema or Go code.
//
//  2. Redundant explicit unique indexes. Several schemas declared uniqueness
//     at both the field level (.Unique()) and the index level
//     (index.Fields(...).Unique()). The field-level declaration creates a
//     "<table>_<column>_key" index via PostgreSQL's default naming; the
//     explicit declaration created a second "<entity>_<field>" index on the
//     same column. The explicit declarations have been removed from the
//     schemas, so these "<entity>_<field>" indexes are now orphaned.
//
//  3. Legacy _key indexes on number columns that no longer have any unique
//     declaration in the schema (left over from earlier schema versions).
//
//  4. The counters counter_id index, which duplicated the primary key.
func dropLegacySchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`DROP TABLE IF EXISTS legacy_investigation_alerts, legacy_investigation_updates, legacy_investigations CASCADE`,
		`DROP TABLE IF EXISTS recurring_issues CASCADE`,
		// Drop the legacy escalation tables that were collapsed into a single
		// escalation_policies row with a JSONB levels column. The new schema
		// owns the entire policy shape; the old child tables are unreachable
		// from any Ent code path.
		`DROP TABLE IF EXISTS escalation_targets CASCADE`,
		`DROP TABLE IF EXISTS escalation_levels CASCADE`,
		`ALTER TABLE IF EXISTS alert_investigations DROP COLUMN IF EXISTS alert_investigation_number`,
		`ALTER TABLE IF EXISTS incident_investigations DROP COLUMN IF EXISTS incident_investigation_number`,
		`DROP INDEX IF EXISTS alert_alert_number`,
		`DROP INDEX IF EXISTS agenttoken_token_hash`,
		`DROP INDEX IF EXISTS alertinvestigation_alert_investigation_id`,
		`DROP INDEX IF EXISTS alertinvestigation_alert_investigation_number`,
		`DROP INDEX IF EXISTS counter_id`,
		`DROP INDEX IF EXISTS escalationpolicy_name`,
		// Drop the legacy per-table indexes left behind by the old
		// escalation_levels / escalation_targets schema. The collapsed
		// escalation_policies row no longer has child tables, so the
		// policy_id / level_id indexes (auto-created from the .Unique() and
		// index.Fields() declarations) are orphaned.
		`DROP INDEX IF EXISTS escalation_level_policy_id_level_number`,
		`DROP INDEX IF EXISTS escalation_target_level_id`,
		`DROP INDEX IF EXISTS group_name`,
		`DROP INDEX IF EXISTS incident_incident_id`,
		`DROP INDEX IF EXISTS incident_incident_number`,
		`ALTER TABLE IF EXISTS incidents ALTER COLUMN incident_number SET NOT NULL`,
		`ALTER TABLE IF EXISTS incidents DROP COLUMN IF EXISTS incident_id`,
		`DROP INDEX IF EXISTS incidents_incident_id_key`,
		`DROP INDEX IF EXISTS incidentinvestigation_incident_investigation_id`,
		`DROP INDEX IF EXISTS incidentinvestigation_incident_investigation_number`,
		`DROP INDEX IF EXISTS investigationthread_thread_id`,
		`DROP INDEX IF EXISTS oncallschedule_name`,
		// On-call schedules no longer store name/description/timezone: the name
		// is derived dynamically from the team and timezone lives per-layer.
		// Drop the legacy columns (with their NOT NULL/unique constraints) so
		// inserts converge after ent stops managing them.
		`ALTER TABLE IF EXISTS on_call_schedules DROP COLUMN IF EXISTS name`,
		`ALTER TABLE IF EXISTS on_call_schedules DROP COLUMN IF EXISTS description`,
		`ALTER TABLE IF EXISTS on_call_schedules DROP COLUMN IF EXISTS timezone`,
		`DROP INDEX IF EXISTS passwordresettoken_token_hash`,
		`DROP INDEX IF EXISTS personalaccesstoken_token_hash`,
		`DROP INDEX IF EXISTS postmortem_incident_id`,
		`DROP INDEX IF EXISTS service_name`,
		`DROP INDEX IF EXISTS session_id_hash`,
		`DROP INDEX IF EXISTS team_name`,
		`DROP INDEX IF EXISTS triageresult_triage_number`,
		`DROP INDEX IF EXISTS user_email`,
		`DROP INDEX IF EXISTS webhooktoken_token_hash`,
		`DROP INDEX IF EXISTS alert_investigations_alert_investigation_number_key`,
		`DROP INDEX IF EXISTS incident_investigations_incident_investigation_number_key`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("legacy schema cleanup %q: %w", stmt, err)
		}
	}
	return nil
}

// applySoftDeleteSchema converges the schema toward alert/incident soft-delete.
// It is idempotent: ADD COLUMN IF NOT EXISTS for the new nullable deleted_at
// columns, and a one-time rebuild of the alert_fingerprint partial unique
// index to add "deleted_at IS NULL" to its predicate. The index rebuild is
// gated on the current predicate so it runs once and then no-ops, preserving
// the "at most one open alert per fingerprint" invariant on live/multi-replica
// systems.
func applySoftDeleteSchema(ctx context.Context, db *sql.DB) error {
	addStmts := []string{
		`ALTER TABLE IF EXISTS alerts ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ`,
		`ALTER TABLE IF EXISTS incidents ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ`,
	}
	for _, stmt := range addStmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("soft-delete schema %q: %w", stmt, err)
		}
	}

	// Rebuild alert_fingerprint only if it still has the old predicate
	// (missing "deleted_at IS NULL"). On a fresh DB the alerts table does not
	// exist yet (ent.Schema.Create runs after this function), so the rebuild
	// is skipped and ent creates the index from the schema definition. On a
	// live DB that has already converged, the predicate already contains
	// "deleted_at IS NULL" so the rebuild no-ops.
	var alertsTableExists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='alerts'
		)`).Scan(&alertsTableExists); err != nil {
		return fmt.Errorf("soft-delete schema: check alerts table exists: %w", err)
	}
	if !alertsTableExists {
		return nil
	}

	var def sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT pg_get_indexdef(to_regclass('public.alert_fingerprint'))`,
	).Scan(&def); err != nil {
		return fmt.Errorf("soft-delete schema: read alert_fingerprint def: %w", err)
	}
	if def.Valid && strings.Contains(strings.ToLower(def.String), "deleted_at is null") {
		return nil
	}
	rebuild := []string{
		`DROP INDEX IF EXISTS alert_fingerprint`,
		`CREATE UNIQUE INDEX IF NOT EXISTS alert_fingerprint ON alerts (fingerprint) WHERE status != 'resolved' AND deleted_at IS NULL`,
	}
	for _, stmt := range rebuild {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("soft-delete schema %q: %w", stmt, err)
		}
	}
	return nil
}

// migrateAlertsIDToUUID converts the alerts.id primary key (and the four
// child-table FK columns that reference it) from the legacy auto-increment
// bigint to UUID. The function is idempotent: it no-ops on a fresh database
// (alerts table absent) and on a database that has already been migrated
// (alerts.id is already UUID).
//
// Child tables touched:
//   - alert_events.alert_events       (auto edge column)
//   - alert_investigation_alerts.alert_id (explicit field)
//   - delivery_targets.alert_delivery_targets (auto edge column)
//   - incident_alerts.alert_id       (junction, composite PK with incident_id)
//
// The migration runs before ent's Schema.Create so the generated schema
// (which declares UUID for alerts.id and the FK columns) converges with the
// database without a diff.
func migrateAlertsIDToUUID(ctx context.Context, db *sql.DB) error {
	var tableExists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='alerts'
		)`).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("check alerts table exists: %w", err)
	}
	if !tableExists {
		return nil
	}

	var colType string
	err = db.QueryRowContext(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_schema='public' AND table_name='alerts' AND column_name='id'
	`).Scan(&colType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// alerts table exists but the id column is gone — nothing to migrate.
			return nil
		}
		return fmt.Errorf("read alerts.id data_type: %w", err)
	}
	if colType == "uuid" {
		return nil
	}

	logger.Info("migrating alerts.id from integer to UUID", "component", "pgclient")

	stmts := []string{
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS id_new UUID DEFAULT gen_random_uuid() NOT NULL`,
		`ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS alert_events_new UUID`,
		`ALTER TABLE alert_investigation_alerts ADD COLUMN IF NOT EXISTS alert_id_new UUID`,
		`ALTER TABLE delivery_targets ADD COLUMN IF NOT EXISTS alert_delivery_targets_new UUID`,
		`ALTER TABLE incident_alerts ADD COLUMN IF NOT EXISTS alert_id_new UUID`,
		`UPDATE alert_events ae SET alert_events_new = a.id_new FROM alerts a WHERE ae.alert_events = a.id`,
		`UPDATE alert_investigation_alerts aia SET alert_id_new = a.id_new FROM alerts a WHERE aia.alert_id = a.id`,
		`UPDATE delivery_targets dt SET alert_delivery_targets_new = a.id_new FROM alerts a WHERE dt.alert_delivery_targets = a.id`,
		`UPDATE incident_alerts ia SET alert_id_new = a.id_new FROM alerts a WHERE ia.alert_id = a.id`,
		`ALTER TABLE alert_events DROP CONSTRAINT IF EXISTS alert_events_alerts_events`,
		`ALTER TABLE alert_investigation_alerts DROP CONSTRAINT IF EXISTS alert_investigation_alerts_alerts_alert_investigation_alerts`,
		`ALTER TABLE delivery_targets DROP CONSTRAINT IF EXISTS delivery_targets_alerts_delivery_targets`,
		`ALTER TABLE incident_alerts DROP CONSTRAINT IF EXISTS incident_alerts_alert_id`,
		`ALTER TABLE incident_alerts DROP CONSTRAINT IF EXISTS incident_alerts_pkey`,
		`ALTER TABLE alert_events DROP COLUMN IF EXISTS alert_events`,
		`ALTER TABLE alert_investigation_alerts DROP COLUMN IF EXISTS alert_id`,
		`ALTER TABLE delivery_targets DROP COLUMN IF EXISTS alert_delivery_targets`,
		`ALTER TABLE incident_alerts DROP COLUMN IF EXISTS alert_id`,
		`ALTER TABLE alert_events RENAME COLUMN alert_events_new TO alert_events`,
		`ALTER TABLE alert_investigation_alerts RENAME COLUMN alert_id_new TO alert_id`,
		`ALTER TABLE delivery_targets RENAME COLUMN alert_delivery_targets_new TO alert_delivery_targets`,
		`ALTER TABLE incident_alerts RENAME COLUMN alert_id_new TO alert_id`,
		`ALTER TABLE alerts DROP CONSTRAINT IF EXISTS alerts_pkey`,
		`ALTER TABLE alerts DROP COLUMN IF EXISTS id`,
		`ALTER TABLE alerts RENAME COLUMN id_new TO id`,
		`DROP SEQUENCE IF EXISTS alerts_id_seq`,
		`ALTER TABLE alerts ADD PRIMARY KEY (id)`,
		`ALTER TABLE incident_alerts ADD PRIMARY KEY (incident_id, alert_id)`,
		`ALTER TABLE alerts ALTER COLUMN id DROP DEFAULT`,
	}

	// Run the destructive DDL inside a single transaction so a partial failure
	// rolls back every prior ALTER/UPDATE and the database is not left in a
	// half-migrated, inconsistent state.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alerts UUID migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("alerts UUID migration %q: %w", stmt, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alerts UUID migration: %w", err)
	}

	logger.Info("alerts.id migrated to UUID successfully", "component", "pgclient")
	return nil
}

// migrateScheduleLayerRestrictions flattens the legacy free-form "restrictions"
// JSON column on schedule_layers into the explicit start_time/end_time/
// days_of_week columns introduced by the Opsgenie-style schedule revamp, then
// drops the legacy column.
//
// It is idempotent: it no-ops once the restrictions column has been removed.
// ent's Schema.Create runs before this function, so the new columns already
// exist when the backfill executes. Only rows with a non-empty restrictions
// array are migrated; a single layer uses a single daily-active window, so the
// first restriction entry wins (this matches the new one-window-per-layer
// model; OR-over-multiple-restrictions is expressed via multiple layers).
func migrateInvestigationThreadMessageReplyToColumn(ctx context.Context, db *sql.DB) error {
	var hasOldCol int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='investigation_thread_messages' AND column_name='quoted_message_id'
	`).Scan(&hasOldCol); err != nil {
		return fmt.Errorf("check quoted_message_id column: %w", err)
	}
	if hasOldCol == 0 {
		return nil
	}

	var hasNewCol int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='investigation_thread_messages' AND column_name='reply_to_message_id'
	`).Scan(&hasNewCol); err != nil {
		return fmt.Errorf("check reply_to_message_id column: %w", err)
	}

	if hasNewCol == 0 {
		logger.Info("renaming investigation_thread_messages.quoted_message_id to reply_to_message_id", "component", "pgclient")
		if _, err := db.ExecContext(ctx,
			`ALTER TABLE investigation_thread_messages RENAME COLUMN quoted_message_id TO reply_to_message_id`,
		); err != nil {
			return fmt.Errorf("rename quoted_message_id column: %w", err)
		}
		return nil
	}

	logger.Info("merging legacy investigation_thread_messages.quoted_message_id into reply_to_message_id", "component", "pgclient")
	if _, err := db.ExecContext(ctx, `
		UPDATE investigation_thread_messages
		SET reply_to_message_id = quoted_message_id
		WHERE COALESCE(NULLIF(reply_to_message_id, ''), '') = '' AND COALESCE(NULLIF(quoted_message_id, ''), '') <> ''
	`); err != nil {
		return fmt.Errorf("copy quoted_message_id into reply_to_message_id: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE investigation_thread_messages DROP COLUMN quoted_message_id`,
	); err != nil {
		return fmt.Errorf("drop legacy quoted_message_id column: %w", err)
	}
	return nil
}

func migrateScheduleLayerRestrictions(ctx context.Context, db *sql.DB) error {
	var hasCol int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='schedule_layers' AND column_name='restrictions'
	`).Scan(&hasCol); err != nil {
		return fmt.Errorf("schedule layer restrictions migration: check column: %w", err)
	}
	if hasCol == 0 {
		return nil
	}

	logger.Info("migrating schedule_layers.restrictions into explicit columns", "component", "pgclient")

	rows, err := db.QueryContext(ctx, `
		SELECT id::text, restrictions::text FROM schedule_layers
		WHERE restrictions IS NOT NULL AND restrictions::text NOT IN ('', '[]', 'null')
	`)
	if err != nil {
		return fmt.Errorf("schedule layer restrictions migration: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, restrictionsText string
		if err := rows.Scan(&id, &restrictionsText); err != nil {
			return fmt.Errorf("schedule layer restrictions migration: scan: %w", err)
		}
		var restrictions []map[string]any
		if err := json.Unmarshal([]byte(restrictionsText), &restrictions); err != nil || len(restrictions) == 0 {
			continue
		}
		r := restrictions[0]
		startTime, _ := r["start_time"].(string)
		endTime, _ := r["end_time"].(string)
		startDay, _ := r["start_day_of_week"].(string)
		endDay, _ := r["end_day_of_week"].(string)

		if startTime == "" {
			startTime = "00:00"
		}
		daysOfWeek := expandDayRange(startDay, endDay)

		daysJSON, _ := json.Marshal(daysOfWeek)
		if _, err := db.ExecContext(ctx,
			`UPDATE schedule_layers SET start_time = $1, end_time = $2, days_of_week = $3::jsonb WHERE id = $4::uuid`,
			startTime, endTime, string(daysJSON), id,
		); err != nil {
			return fmt.Errorf("schedule layer restrictions migration: update %s: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("schedule layer restrictions migration: rows: %w", err)
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE schedule_layers DROP COLUMN IF EXISTS restrictions`); err != nil {
		return fmt.Errorf("schedule layer restrictions migration: drop column: %w", err)
	}

	logger.Info("schedule_layers.restrictions migrated successfully", "component", "pgclient")
	return nil
}

// expandDayRange turns an inclusive weekday range (full English names, Sunday
// first) into an explicit list. An empty bound on either side yields an empty
// list (meaning "all days" downstream). Wrap-around ranges (e.g.
// Friday->Tuesday) expand across the Sunday boundary.
func expandDayRange(startDay, endDay string) []string {
	days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	startIdx, endIdx := -1, -1
	for i, d := range days {
		if d == startDay {
			startIdx = i
		}
		if d == endDay {
			endIdx = i
		}
	}
	if startIdx < 0 || endIdx < 0 {
		return nil
	}
	var out []string
	if startIdx <= endIdx {
		for i := startIdx; i <= endIdx; i++ {
			out = append(out, days[i])
		}
	} else {
		for i := startIdx; i < len(days); i++ {
			out = append(out, days[i])
		}
		for i := 0; i <= endIdx; i++ {
			out = append(out, days[i])
		}
	}
	return out
}

// applyOnCallScheduleUniqueTeam enforces the "at most one schedule per team"
// invariant via a partial unique index on team_id. NULL team_ids are allowed
// (legacy orphan schedules) so the predicate excludes them. The index is
// created outside of ent's schema management, mirroring the ApplyKnowledgeFTS
// expression-index pattern, and is idempotent via IF NOT EXISTS.
func applyOnCallScheduleUniqueTeam(ctx context.Context, db *sql.DB) error {
	var tableExists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='on_call_schedules'
		)`).Scan(&tableExists); err != nil {
		return fmt.Errorf("on-call unique team: check table exists: %w", err)
	}
	if !tableExists {
		return nil
	}
	if _, err := db.ExecContext(ctx,
		`CREATE UNIQUE INDEX IF NOT EXISTS oncallschedule_team_id_unique
		 ON on_call_schedules (team_id) WHERE team_id IS NOT NULL`); err != nil {
		return fmt.Errorf("on-call unique team: create index: %w", err)
	}
	return nil
}

// backfillTeamSchedules creates an empty schedule for every team that does not
// yet have one, enforcing the Opsgenie-style "one schedule per team" invariant
// for teams created before the revamp. Idempotent: a LEFT JOIN skips teams that
// already have a schedule, and re-runs converge to a no-op.
func backfillTeamSchedules(ctx context.Context, db *sql.DB) error {
	var bothExist bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='teams')
		   AND EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='on_call_schedules')
	`).Scan(&bothExist); err != nil {
		return fmt.Errorf("backfill team schedules: check tables: %w", err)
	}
	if !bothExist {
		return nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT t.id::text FROM teams t
		LEFT JOIN on_call_schedules s ON s.team_id = t.id
		WHERE s.id IS NULL
	`)
	if err != nil {
		return fmt.Errorf("backfill team schedules: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var created int
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("backfill team schedules: scan: %w", err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO on_call_schedules (id, team_id, created_at, updated_at)
			VALUES (gen_random_uuid(), $1::uuid, now(), now())
		`, id); err != nil {
			logger.WarnCtx(ctx, "backfill team schedules: could not create schedule (team conflict?)", "component", "pgclient", "team_id", id, "error", err)
			continue
		}
		created++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill team schedules: rows: %w", err)
	}
	if created > 0 {
		logger.Info("backfilled empty schedules for teams", "component", "pgclient", "count", created)
	}
	return nil
}

// migrateGroupsToTeams moves the legacy notification-routing Group entity into
// the Team entity: for each group it creates a Team (idempotent on name) and
// migrates its flat member list into role-bearing TeamMember rows, then drops
// the groups table. Idempotent: it no-ops once the groups table is gone.
//
// This is the data side of merging Group into Team; the ops-team team it
// produces is the same team the scheduler's human-escalation hook resolves.
func migrateGroupsToTeams(ctx context.Context, db *sql.DB) error {
	var groupsExist bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='groups')
	`).Scan(&groupsExist); err != nil {
		return fmt.Errorf("migrate groups to teams: check groups table: %w", err)
	}
	if !groupsExist {
		return nil
	}

	var teamsExist bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='teams')
	`).Scan(&teamsExist); err != nil {
		return fmt.Errorf("migrate groups to teams: check teams table: %w", err)
	}
	if !teamsExist {
		return nil
	}

	logger.Info("migrating groups into teams", "component", "pgclient")

	rows, err := db.QueryContext(ctx, `
		SELECT id::text, name, COALESCE(description, ''), members::text FROM groups
	`)
	if err != nil {
		return fmt.Errorf("migrate groups to teams: query groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var migrated int
	for rows.Next() {
		var id, name, description, membersJSON string
		if err := rows.Scan(&id, &name, &description, &membersJSON); err != nil {
			return fmt.Errorf("migrate groups to teams: scan: %w", err)
		}
		if err := migrateOneGroupToTeam(ctx, db, name, description, membersJSON); err != nil {
			logger.WarnCtx(ctx, "migrate groups to teams: could not migrate group", "component", "pgclient", "group_id", id, "error", err)
			continue
		}
		migrated++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate groups to teams: rows: %w", err)
	}

	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS groups`); err != nil {
		return fmt.Errorf("migrate groups to teams: drop groups table: %w", err)
	}
	if migrated > 0 {
		logger.Info("groups migrated into teams", "component", "pgclient", "count", migrated)
	}
	return nil
}

// migrateOneGroupToTeam creates a team for a group (or reuses an existing team
// of the same name) and copies the flat member list into team_members rows.
func migrateOneGroupToTeam(ctx context.Context, db *sql.DB, name, description, membersJSON string) error {
	var teamID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO teams (id, name, description, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, now(), now())
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name, description).Scan(&teamID)
	if err != nil {
		return fmt.Errorf("upsert team: %w", err)
	}

	var members []string
	if membersJSON != "" && membersJSON != "null" {
		if err := json.Unmarshal([]byte(membersJSON), &members); err != nil {
			return fmt.Errorf("parse members: %w", err)
		}
	}
	for _, m := range members {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO team_members (id, team_id, user_id, role, created_at)
			VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'member', now())
			ON CONFLICT (team_id, user_id) DO NOTHING
		`, teamID, m); err != nil {
			logger.WarnCtx(ctx, "migrate groups to teams: skip member", "component", "pgclient", "team_id", teamID, "user_id", m, "error", err)
		}
	}
	return nil
}

func ApplyPgVector(ctx context.Context, db *sql.DB) error {
	logger.Debug("setting up pgvector extension", "component", "pgclient")

	_, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`)
	if err != nil {
		return fmt.Errorf("create vector extension: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		ALTER TABLE agent_memories ADD COLUMN IF NOT EXISTS vec vector(1536)
	`)
	if err != nil {
		return fmt.Errorf("add vec column: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_agent_memories_vec
		ON agent_memories USING hnsw (vec vector_cosine_ops)
	`)
	if err != nil {
		return fmt.Errorf("create hnsw index: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_agent_memories_fts
		ON agent_memories USING gin(to_tsvector('english', content))
	`)
	if err != nil {
		return fmt.Errorf("create fts index: %w", err)
	}
	return nil
}

// KnowledgeFTSExpression is the immutable, normalized full-text expression
// indexed on knowledge_notes (title + body + tags). It MUST stay byte-identical
// between the GIN expression index created in ApplyKnowledgeFTS and the WHERE
// clause emitted by the knowledge store text search; otherwise PostgreSQL cannot
// reuse the index and the search degrades to a sequential scan.
//
// The two-argument to_tsvector with a literal regconfig ('english') is
// IMMUTABLE, which is what makes it eligible for an expression index.
const KnowledgeFTSExpression = `to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body_markdown, '') || ' ' || coalesce(tags::text, ''))`

// ApplyKnowledgeFTS creates the GIN full-text index over knowledge_notes. It
// mirrors the idx_agent_memories_fts pattern: an expression index maintained
// automatically by PostgreSQL on insert/update, entirely outside of Ent's schema
// awareness. Idempotent via IF NOT EXISTS, so existing deployments converge on
// the next migration run without a separate step.
func ApplyKnowledgeFTS(ctx context.Context, db *sql.DB) error {
	logger.Debug("setting up knowledge_notes full-text index", "component", "pgclient")

	_, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_notes_fts ON knowledge_notes USING gin(`+KnowledgeFTSExpression+`)`)
	if err != nil {
		return fmt.Errorf("create knowledge fts index: %w", err)
	}
	return nil
}

// applyUserVoiceOptOut adds the users.voice_opt_out column for the cost-control
// guard that lets a user explicitly opt out of voice-call escalation. Idempotent
// via IF NOT EXISTS; default false so existing users keep receiving voice calls
// until they opt out.
func applyUserVoiceOptOut(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE IF EXISTS users ADD COLUMN IF NOT EXISTS voice_opt_out BOOLEAN NOT NULL DEFAULT FALSE`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("user voice_opt_out schema %q: %w", stmt, err)
		}
	}
	return nil
}

// applyUserPhoneCountry adds the users.phone_country column so the frontend can
// pre-select the correct country on the phone input. Pairs with
// backfillUserPhoneCountry which populates the column for existing rows from
// the stored E.164 number via libphonenumber. Idempotent via IF NOT EXISTS.
func applyUserPhoneCountry(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE IF EXISTS users ADD COLUMN IF NOT EXISTS phone_country TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("user phone_country schema %q: %w", stmt, err)
		}
	}
	return nil
}

// backfillUserPhoneCountry populates users.phone_country from the existing
// users.phone E.164 number for rows that don't have one yet. No-op once every
// row has a non-empty country. The libphonenumber region code for the number
// is used; for numbers whose prefix is shared across multiple countries (e.g.
// +1 = NANP, +7 = RU/KZ) libphonenumber returns the most likely region. Old
// rows with malformed phones are left as empty country so the frontend falls
// back to longest-prefix detection.
func backfillUserPhoneCountry(ctx context.Context, db *sql.DB) error {
	var usersExist bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='users'
		)`).Scan(&usersExist); err != nil {
		return fmt.Errorf("backfill user phone_country: check users table: %w", err)
	}
	if !usersExist {
		return nil
	}

	var hasCol int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='users' AND column_name='phone_country'
	`).Scan(&hasCol); err != nil {
		return fmt.Errorf("backfill user phone_country: check column: %w", err)
	}
	if hasCol == 0 {
		return nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id::text, phone FROM users
		WHERE COALESCE(phone_country, '') = '' AND COALESCE(phone, '') <> ''
	`)
	if err != nil {
		return fmt.Errorf("backfill user phone_country: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type pending struct {
		id      string
		country string
	}
	var batch []pending
	for rows.Next() {
		var id, phone string
		if err := rows.Scan(&id, &phone); err != nil {
			return fmt.Errorf("backfill user phone_country: scan: %w", err)
		}
		num, err := phonenumbers.Parse(phone, "")
		if err != nil || !phonenumbers.IsValidNumber(num) {
			continue
		}
		region := phonenumbers.GetRegionCodeForNumber(num)
		if region == "" || region == "ZZ" {
			continue
		}
		batch = append(batch, pending{id: id, country: region})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill user phone_country: rows: %w", err)
	}
	if len(batch) == 0 {
		return nil
	}

	for _, p := range batch {
		if _, err := db.ExecContext(ctx,
			`UPDATE users SET phone_country = $1, updated_at = updated_at WHERE id = $2::uuid`,
			p.country, p.id,
		); err != nil {
			return fmt.Errorf("backfill user phone_country: update %s: %w", p.id, err)
		}
	}
	logger.Info("backfilled user phone_country", "component", "pgclient", "count", len(batch))
	return nil
}
