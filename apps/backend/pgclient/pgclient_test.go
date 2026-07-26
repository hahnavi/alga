package pgclient

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
