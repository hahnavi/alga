package models

import (
	"database/sql"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/schema"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// allModels lists every table-backed model. Keep in sync with the migrations.
func allModels() []any {
	return []any{
		(*ActionItem)(nil), (*AgentAsk)(nil), (*AgentDMMessage)(nil), (*AgentMemory)(nil),
		(*AgentToken)(nil), (*Alert)(nil), (*AlertEvent)(nil), (*AlertInvestigation)(nil),
		(*AlertInvestigationAlert)(nil), (*AlertInvestigationEvent)(nil), (*AlertInvestigationUpdate)(nil),
		(*AuditLog)(nil), (*CredentialProvider)(nil), (*DeliveryTarget)(nil),
		(*EscalationPolicy)(nil), (*HandoffRecord)(nil), (*Heartbeat)(nil), (*ICSRoleAssignment)(nil),
		(*Incident)(nil), (*IncidentCoordinationMessage)(nil), (*IncidentDocument)(nil),
		(*IncidentInvestigation)(nil), (*IncidentInvestigationUpdate)(nil), (*IncidentTimelineEntry)(nil),
		(*Integration)(nil), (*InvestigationThread)(nil), (*InvestigationThreadMessage)(nil),
		(*KnowledgeNote)(nil), (*MaintenanceWindow)(nil), (*Notification)(nil), (*NotificationDeliveryLog)(nil),
		(*OIDCIdentity)(nil), (*OIDCProvider)(nil), (*OnCallSchedule)(nil), (*Outbox)(nil),
		(*PasswordResetToken)(nil), (*PersonalAccessToken)(nil), (*Playbook)(nil), (*PlaybookStep)(nil),
		(*PostMortem)(nil), (*RouteRules)(nil), (*ScheduleLayer)(nil), (*ScheduleOverride)(nil),
		(*Service)(nil), (*ServiceDependency)(nil), (*Session)(nil), (*SharedSecret)(nil),
		(*StatusPage)(nil), (*StatusPageComponent)(nil), (*SystemConfig)(nil), (*Team)(nil),
		(*TeamMember)(nil), (*TriageResult)(nil), (*TriageRule)(nil), (*User)(nil), (*WebhookToken)(nil),
		(*IncidentAlert)(nil),
	}
}

// TestModelColumnsMatchDB verifies every model's bun columns exist in the live
// schema. Set ALGA_SCHEMA_DSN to run (e.g. against a freshly-migrated scratch DB).
func TestModelColumnsMatchDB(t *testing.T) {
	dsn := os.Getenv("ALGA_SCHEMA_DSN")
	if dsn == "" {
		t.Skip("ALGA_SCHEMA_DSN not set")
	}
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if cerr := sqldb.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	}()
	tables := schema.NewTables(pgdialect.New())

	for _, m := range allModels() {
		tbl := tables.Get(reflect.TypeOf(m).Elem())

		dbCols := map[string]bool{}
		rows, err := sqldb.Query(`SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, tbl.Name)
		if err != nil {
			t.Fatalf("%s: query cols: %v", tbl.Name, err)
		}
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatalf("%s: scan col: %v", tbl.Name, err)
			}
			dbCols[c] = true
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("%s: close rows: %v", tbl.Name, err)
		}
		if len(dbCols) == 0 {
			t.Errorf("%-30s TABLE MISSING (model %s)", tbl.Name, tbl.TypeName)
			continue
		}

		var missing []string
		modelCols := map[string]bool{}
		for _, f := range tbl.Fields {
			modelCols[f.Name] = true
			if !dbCols[f.Name] {
				missing = append(missing, f.Name)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%-30s model cols absent from DB: %v", tbl.Name, missing)
		}

		var extra []string
		for c := range dbCols {
			if !modelCols[c] {
				extra = append(extra, c)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			t.Errorf("%-30s DB cols absent from model: %v", tbl.Name, extra)
		}
	}
}
