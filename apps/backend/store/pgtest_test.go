//go:build integration

package store

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"

	"alga/db"
)

func init() {
	goose.SetLogger(goose.NopLogger())

	if os.Getenv("SECRET_PEPPER") == "" {
		_ = os.Setenv("SECRET_PEPPER", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	}
	if os.Getenv("ENCRYPTION_KEYS") == "" {
		_ = os.Setenv("ENCRYPTION_KEYS", "1:"+base64.StdEncoding.EncodeToString(make([]byte, 32)))
	}
}

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx,
		"pgvector/pgvector:0.8.5-pg18",
		postgres.WithDatabase("alga_test"),
		postgres.WithUsername("alga"),
		postgres.WithPassword("alga_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	sqlDB, err := db.OpenSQLDB(dsn)
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	goose.SetBaseFS(db.MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	_ = sqlDB.Close()

	cli, err := db.New(dsn)
	if err != nil {
		t.Fatalf("create db client: %v", err)
	}
	t.Cleanup(func() { cli.Close() })

	return cli.DB
}

func newTestStores(t *testing.T) *Stores {
	t.Helper()
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}
	return stores
}

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}
