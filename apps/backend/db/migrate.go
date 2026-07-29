package db

import (
	"context"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationsFS exposes the embedded migration files for test harnesses.
var MigrationsFS = migrationsFS

func ApplyMigrations(ctx context.Context, dsn string) error {
	sqlDB, err := OpenSQLDB(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
