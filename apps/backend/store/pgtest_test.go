package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"alga/ent"
)

// newTestEntClient provisions an isolated PostgreSQL schema for a single test,
// applies the ent schema into it, and returns an ent client backed by it. The
// schema is dropped on cleanup. This replaces the previous in-memory SQLite
// harness (which forced a slow cgo build of go-sqlite3) with the real
// PostgreSQL engine the app runs on, matching production types/constraints.
//
// Schema-based isolation is used (rather than separate databases) because it
// needs only CREATE privilege, not CREATEDB. Each test gets its own schema for
// full isolation, selected via the connection's search_path.
//
// DSN resolution order: ALGA_TEST_POSTGRES_DSN, POSTGRES_DSN, then the
// POSTGRES_DSN line found in a .env file by walking up from the test's working
// directory. Tests skip when no DSN is available (e.g. CI without Postgres).
func newTestEntClient(t *testing.T) *ent.Client {
	t.Helper()

	baseDSN := resolveTestDSN(t)
	schema := "alga_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")

	if err := createTestSchema(baseDSN, schema); err != nil {
		t.Fatalf("create test schema %s: %v", schema, err)
	}

	testDSN, err := dsnWithSearchPath(baseDSN, schema)
	if err != nil {
		t.Fatalf("build test dsn: %v", err)
	}

	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("open test connection: %v", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Schema.Create(ctx); err != nil {
		_ = client.Close()
		_ = db.Close()
		_ = dropTestSchema(baseDSN, schema)
		t.Fatalf("apply ent schema: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
		_ = dropTestSchema(baseDSN, schema)
	})

	return client
}

func resolveTestDSN(t *testing.T) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("ALGA_TEST_POSTGRES_DSN")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); v != "" {
		return v
	}
	if v := loadDSNFromEnvFile(t, ".env"); v != "" {
		return v
	}
	t.Skip("no POSTGRES_DSN configured for store tests; set POSTGRES_DSN or add it to .env")
	return ""
}

func loadDSNFromEnvFile(t *testing.T, name string) string {
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

func dsnWithSearchPath(dsn, schema string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func createTestSchema(baseDSN, schema string) error {
	admin, err := sql.Open("pgx", baseDSN)
	if err != nil {
		return fmt.Errorf("open admin connection: %w", err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

func dropTestSchema(baseDSN, schema string) error {
	admin, err := sql.Open("pgx", baseDSN)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema)); err != nil {
		return err
	}
	return nil
}

// mustCreateUser inserts a minimal user row and returns its id, for tests that
// need a valid FK target now that sessions, oidc identities, password-reset
// tokens, and personal access tokens carry real foreign keys to users.
func mustCreateUser(t *testing.T, client *ent.Client) uuid.UUID {
	t.Helper()
	u, err := client.User.Create().
		SetEmail("test-" + uuid.NewString() + "@example.com").
		SetPassword("x").
		SetRole("viewer").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return u.ID
}
