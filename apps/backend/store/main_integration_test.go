//go:build integration

package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"alga/db"
)

// Shared integration harness. A single container is started once in TestMain
// and its stores are exposed as package-level globals for tests that use
// unique per-test keys and clean up after themselves. Tests needing an
// isolated database use newTestStores(t) instead (see pgtest_test.go).
var (
	sharedStores  *Stores
	alertsStore   Store
	alertInvStore AlertInvestigationStore
	incidentStore IncidentStore
)

func TestMain(m *testing.M) {
	code, err := runIntegrationSuite(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration harness setup failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runIntegrationSuite(m *testing.M) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
		return 1, fmt.Errorf("start postgres container: %w", err)
	}
	defer func() { _ = container.Terminate(context.Background()) }()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 1, fmt.Errorf("connection string: %w", err)
	}

	sqlDB, err := db.OpenSQLDB(dsn)
	if err != nil {
		return 1, fmt.Errorf("open sql db: %w", err)
	}
	goose.SetBaseFS(db.MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		_ = sqlDB.Close()
		return 1, fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		_ = sqlDB.Close()
		return 1, fmt.Errorf("goose up: %w", err)
	}
	_ = sqlDB.Close()

	cli, err := db.New(dsn)
	if err != nil {
		return 1, fmt.Errorf("create db client: %w", err)
	}
	defer cli.Close()

	sharedStores, err = NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		return 1, fmt.Errorf("create stores: %w", err)
	}
	alertsStore = sharedStores.Alert
	alertInvStore = sharedStores.AlertInvestigation
	incidentStore = sharedStores.Incident

	return m.Run(), nil
}
