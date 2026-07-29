package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"alga/db"
	"alga/store"
)

// alertExpunger / incidentExpunger are satisfied by the concrete pg stores. Kept
// local (and out of the Store/IncidentStore interfaces) so this backfill does
// not ripple through every test fake.
type alertExpunger interface {
	ExpungeSoftDeletedAlertsChildren(ctx context.Context) (int, error)
}
type incidentExpunger interface {
	ExpungeSoftDeletedIncidentsChildren(ctx context.Context) (int, error)
}

var cleanupDeletedCmd = &cobra.Command{
	Use:   "cleanup-deleted",
	Short: "Hard-delete stale children of soft-deleted alerts and incidents",
	Long: `One-time backfill that removes investigation artifacts (investigations,
events, updates, threads, memories) tied to alerts/incidents that were soft-deleted
before the cascade-on-delete behavior existed. Idempotent; safe to re-run.`,
	RunE: runCleanupDeleted,
}

func runCleanupDeleted(cmd *cobra.Command, args []string) error {
	cfg := loadConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cli, err := db.New(cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer cli.Close()

	stores, err := store.NewStores(cli, time.Hour, 0)
	if err != nil {
		return fmt.Errorf("init stores: %w", err)
	}

	an, err := expungeAlerts(ctx, stores.Alert)
	if err != nil {
		return err
	}
	in, err := expungeIncidents(ctx, stores.Incident)
	if err != nil {
		return err
	}
	fmt.Printf("Expunged children of %d soft-deleted alert(s) and %d soft-deleted incident(s).\n", an, in)
	return nil
}

func expungeAlerts(ctx context.Context, s store.Store) (int, error) {
	e, ok := s.(alertExpunger)
	if !ok {
		return 0, fmt.Errorf("alert store does not support expunge (concrete type %T)", s)
	}
	n, err := e.ExpungeSoftDeletedAlertsChildren(ctx)
	if err != nil {
		return 0, fmt.Errorf("expunge alerts: %w", err)
	}
	return n, nil
}

func expungeIncidents(ctx context.Context, s store.IncidentStore) (int, error) {
	e, ok := s.(incidentExpunger)
	if !ok {
		return 0, fmt.Errorf("incident store does not support expunge (concrete type %T)", s)
	}
	n, err := e.ExpungeSoftDeletedIncidentsChildren(ctx)
	if err != nil {
		return 0, fmt.Errorf("expunge incidents: %w", err)
	}
	return n, nil
}
