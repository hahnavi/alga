package pgclient

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go/ast"
	"go/parser"
	"go/token"
)

func TestApplyMigrationsDoesNotPassNilSchemaOption(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "pgclient.go", nil, 0)
	if err != nil {
		t.Fatalf("parse pgclient.go: %v", err)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Create" {
			return true
		}
		parent, ok := selector.X.(*ast.SelectorExpr)
		if !ok || parent.Sel.Name != "Schema" {
			return true
		}
		for _, arg := range call.Args {
			ident, ok := arg.(*ast.Ident)
			if ok && ident.Name == "nil" {
				t.Fatalf("Schema.Create must not receive nil migrate options at %s", fset.Position(arg.Pos()))
			}
		}
		return true
	})
}

// TestMigrateEscalationCollapseToJSON_Backfill verifies that
// migrateEscalationCollapseToJSON folds the legacy escalation_levels +
// escalation_targets rows into the new escalation_policies.levels JSONB
// column with the runtime store's expected JSON shape, and that the column
// ends up NOT NULL.
func TestMigrateEscalationCollapseToJSON_Backfill(t *testing.T) {
	db := setupIsolatedPG(t)
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustExec(t, ctx, db, `
		CREATE TABLE escalation_policies (
			id UUID PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			repeat_count INTEGER NOT NULL DEFAULT 3,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	mustExec(t, ctx, db, `
		CREATE TABLE escalation_levels (
			id UUID PRIMARY KEY,
			policy_id UUID NOT NULL REFERENCES escalation_policies(id) ON DELETE CASCADE,
			level_number INTEGER NOT NULL,
			delay_minutes INTEGER NOT NULL DEFAULT 5,
			notify_channels JSONB NOT NULL DEFAULT '[]'::jsonb
		)`)
	mustExec(t, ctx, db, `
		CREATE TABLE escalation_targets (
			id UUID PRIMARY KEY,
			level_id UUID NOT NULL REFERENCES escalation_levels(id) ON DELETE CASCADE,
			target_type TEXT NOT NULL DEFAULT 'user',
			target_user_id UUID,
			target_team_id UUID,
			target_schedule_id UUID
		)`)

	policyA := uuid.New()
	policyB := uuid.New()
	mustExec(t, ctx, db, `INSERT INTO escalation_policies (id, name) VALUES ($1, 'a'), ($2, 'b')`, policyA, policyB)

	level1 := uuid.New()
	level2 := uuid.New()
	uid := uuid.New()
	sid := uuid.New()
	mustExec(t, ctx, db, `
		INSERT INTO escalation_levels (id, policy_id, level_number, delay_minutes, notify_channels)
		VALUES
			($1, $2, 1, 0, '["voice"]'::jsonb),
			($3, $2, 2, 10, '[]'::jsonb)`,
		level1, policyA, level2)
	mustExec(t, ctx, db, `
		INSERT INTO escalation_targets (id, level_id, target_type, target_user_id, target_team_id)
		VALUES
			($1, $2, 'user', $3, NULL),
			($4, $2, 'team', NULL, $5),
			($6, $7, 'user', $3, NULL)`,
		uuid.New(), level1, uid,
		uuid.New(), sid,
		uuid.New(), level2)

	if err := migrateEscalationCollapseToJSON(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var got string
	if err := db.QueryRowContext(ctx,
		`SELECT levels::text FROM escalation_policies WHERE id = $1`, policyA,
	).Scan(&got); err != nil {
		t.Fatalf("query policy A: %v", err)
	}

	wantA := `[{"level_number":1,"delay_minutes":0,"notify_channels":["voice"],"targets":` +
		`[{"target_type":"team","target_team_id":"` + sid.String() + `"},` +
		`{"target_type":"user","target_user_id":"` + uid.String() + `"}]},` +
		`{"level_number":2,"delay_minutes":10,"notify_channels":[],"targets":` +
		`[{"target_type":"user","target_user_id":"` + uid.String() + `"}]}]`
	if !jsonEqual(t, got, wantA) {
		t.Fatalf("policy A levels mismatch:\n got: %s\nwant: %s", got, wantA)
	}

	var gotB string
	if err := db.QueryRowContext(ctx,
		`SELECT levels::text FROM escalation_policies WHERE id = $1`, policyB,
	).Scan(&gotB); err != nil {
		t.Fatalf("query policy B: %v", err)
	}
	if gotB != `[]` {
		t.Fatalf("policy B levels should be empty array, got %q", gotB)
	}

	var nullable string
	if err := db.QueryRowContext(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_name='escalation_policies' AND column_name='levels'
	`).Scan(&nullable); err != nil {
		t.Fatalf("query column metadata: %v", err)
	}
	if nullable != "NO" {
		t.Fatalf("levels column should be NOT NULL, got is_nullable=%q", nullable)
	}
}

// TestMigrateEscalationCollapseToJSON_Idempotent verifies that re-running the
// migration on a database that has already converged is a no-op and that the
// function also tolerates a fresh DB with only escalation_policies (legacy
// child tables already gone).
func TestMigrateEscalationCollapseToJSON_Idempotent(t *testing.T) {
	db := setupIsolatedPG(t)
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustExec(t, ctx, db, `
		CREATE TABLE escalation_policies (
			id UUID PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			repeat_count INTEGER NOT NULL DEFAULT 3,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	pid := uuid.New()
	mustExec(t, ctx, db, `INSERT INTO escalation_policies (id, name) VALUES ($1, 'a')`, pid)

	for i := 0; i < 2; i++ {
		if err := migrateEscalationCollapseToJSON(ctx, db); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
	}

	var got string
	if err := db.QueryRowContext(ctx,
		`SELECT levels::text FROM escalation_policies WHERE id = $1`, pid,
	).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != `[]` {
		t.Fatalf("expected empty levels, got %q", got)
	}

	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS escalation_policies CASCADE`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := migrateEscalationCollapseToJSON(ctx, db); err != nil {
		t.Fatalf("no-op on missing table: %v", err)
	}
}

func setupIsolatedPG(t *testing.T) *sql.DB {
	t.Helper()
	baseDSN := resolvePGTestDSN(t)
	schema := "alga_pgclient_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if err := createPGTestSchema(baseDSN, schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	testDSN, err := dsnPGWithSearchPath(baseDSN, schema)
	if err != nil {
		t.Fatalf("build dsn: %v", err)
	}
	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = dropPGTestSchema(baseDSN, schema)
	})
	return db
}

func resolvePGTestDSN(t *testing.T) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("ALGA_TEST_POSTGRES_DSN")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); v != "" {
		return v
	}
	if v := loadDSNFromPGEnvFile(t, ".env"); v != "" {
		return v
	}
	t.Skip("no POSTGRES_DSN configured for pgclient tests; set POSTGRES_DSN or add it to .env")
	return ""
}

func loadDSNFromPGEnvFile(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, name)
		if data, err := os.ReadFile(candidate); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "POSTGRES_DSN=") {
					v := strings.Trim(strings.TrimPrefix(line, "POSTGRES_DSN="), `"'`)
					if v != "" {
						return v
					}
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func dsnPGWithSearchPath(dsn, schema string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func createPGTestSchema(baseDSN, schema string) error {
	admin, err := sql.Open("pgx", baseDSN)
	if err != nil {
		return fmt.Errorf("open admin: %w", err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

func dropPGTestSchema(baseDSN, schema string) error {
	admin, err := sql.Open("pgx", baseDSN)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	_, err = admin.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema))
	return err
}

func mustExec(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// jsonEqual reports whether two JSON strings represent the same JSON value,
// ignoring whitespace and object key order (jsonb's storage order differs
// from Go's struct field order).
func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		t.Fatalf("jsonEqual: parse a: %v", err)
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		t.Fatalf("jsonEqual: parse b: %v", err)
	}
	return reflect.DeepEqual(av, bv)
}
