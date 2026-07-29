package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

// hardDeleteAlertCascade removes every investigation artifact linked to the
// alert (regardless of investigation status): the alert_investigations rows
// and their events/updates/join rows, agent memories scoped to those
// investigations, and the alert-owned investigation thread + its messages.
// It matches investigations by the alert's unique identity (id / alert_number)
// only — never by fingerprint, which can be shared across sibling alerts.
// It must run inside the alert delete tx so the tombstone set and the child
// cleanup commit atomically.
func hardDeleteAlertCascade(ctx context.Context, tx bun.Tx, alertID uuid.UUID, alertNumber int64) error {
	// Find linked investigations via the alert_investigation_alerts join table
	type invRef struct {
		ID                   uuid.UUID `bun:"id"`
		AlertInvestigationID string    `bun:"public_id"`
	}
	var invs []invRef
	err := tx.NewSelect().
		TableExpr("alert_investigations AS ai").
		ColumnExpr("ai.id, ai.public_id").
		Join("JOIN alert_investigation_alerts AS aia ON aia.investigation_id = ai.id").
		Where("(aia.alert_id = ? OR aia.alert_number = ?)", alertID, alertNumber).
		Scan(ctx, &invs)
	if err != nil {
		return fmt.Errorf("query linked alert investigations: %w", err)
	}
	if len(invs) == 0 {
		return deleteOwnerThreadInTx(ctx, tx, ThreadOwnerAlert, strconv.FormatInt(alertNumber, 10))
	}

	invUUIDs := make([]uuid.UUID, 0, len(invs))
	invStrIDs := make([]string, 0, len(invs))
	for _, inv := range invs {
		invUUIDs = append(invUUIDs, inv.ID)
		invStrIDs = append(invStrIDs, inv.AlertInvestigationID)
	}

	if _, err := tx.NewDelete().Model((*models.AlertInvestigationAlert)(nil)).
		Where("investigation_id IN (?)", bun.List(invUUIDs)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigation alerts: %w", err)
	}
	if _, err := tx.NewDelete().Model((*models.AlertInvestigationUpdate)(nil)).
		Where("alert_investigation_id IN (?)", bun.List(invUUIDs)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigation updates: %w", err)
	}
	if _, err := tx.NewDelete().Model((*models.AlertInvestigationEvent)(nil)).
		Where("alert_investigation_id IN (?)", bun.List(invUUIDs)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigation events: %w", err)
	}
	if _, err := tx.NewDelete().Model((*models.AlertInvestigation)(nil)).
		Where("id IN (?)", bun.List(invUUIDs)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigations: %w", err)
	}

	// Agent memories are scoped by the investigation's string business id.
	if _, err := tx.NewDelete().Model((*models.AgentMemory)(nil)).
		Where("investigation_id IN (?)", bun.List(invStrIDs)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete agent memories: %w", err)
	}

	return deleteOwnerThreadInTx(ctx, tx, ThreadOwnerAlert, strconv.FormatInt(alertNumber, 10))
}

// hardDeleteIncidentCascade removes every investigation artifact for the
// incident (regardless of status): the incident_investigations rows + their
// updates, agent memories scoped to those investigations, and the
// incident-owned investigation thread + its messages. source_alert_investigation_id
// back-refs on alert investigations are SET NULL via the existing FK, so they
// need no handling here. It must run inside the incident delete tx.
func hardDeleteIncidentCascade(ctx context.Context, tx bun.Tx, incidentID uuid.UUID, incidentNumber int64) error {
	type invRef struct {
		ID                      uuid.UUID `bun:"id"`
		IncidentInvestigationID string    `bun:"public_id"`
	}
	var invs []invRef
	err := tx.NewSelect().
		TableExpr("incident_investigations").
		ColumnExpr("id, public_id").
		Where("incident_id = ?", incidentID).
		Scan(ctx, &invs)
	if err != nil {
		return fmt.Errorf("query incident investigations: %w", err)
	}
	if len(invs) > 0 {
		invUUIDs := make([]uuid.UUID, 0, len(invs))
		invStrIDs := make([]string, 0, len(invs))
		for _, inv := range invs {
			invUUIDs = append(invUUIDs, inv.ID)
			invStrIDs = append(invStrIDs, inv.IncidentInvestigationID)
		}
		if _, err := tx.NewDelete().Model((*models.IncidentInvestigationUpdate)(nil)).
			Where("incident_investigation_id IN (?)", bun.List(invUUIDs)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete incident investigation updates: %w", err)
		}
		if _, err := tx.NewDelete().Model((*models.IncidentInvestigation)(nil)).
			Where("id IN (?)", bun.List(invUUIDs)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete incident investigations: %w", err)
		}
		if _, err := tx.NewDelete().Model((*models.AgentMemory)(nil)).
			Where("investigation_id IN (?)", bun.List(invStrIDs)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete agent memories: %w", err)
		}
	}
	return deleteOwnerThreadInTx(ctx, tx, ThreadOwnerIncidentInvestigation, strconv.FormatInt(incidentNumber, 10))
}

// deleteOwnerThreadInTx deletes the polymorphic owner thread (and its messages)
// for (ownerType, ownerID). ownerID is the entity NUMBER as a string for both
// alert-owned and incident-owned threads.
func deleteOwnerThreadInTx(ctx context.Context, tx bun.Tx, ownerType, ownerID string) error {
	var threadIDs []uuid.UUID
	err := tx.NewSelect().Model((*models.InvestigationThread)(nil)).
		Column("id").
		Where("owner_type = ?", ownerType).
		Where("owner_id = ?", ownerID).
		Scan(ctx, &threadIDs)
	if err != nil {
		return fmt.Errorf("query owner thread: %w", err)
	}
	if len(threadIDs) == 0 {
		return nil
	}
	if _, err := tx.NewDelete().Model((*models.InvestigationThreadMessage)(nil)).
		Where("thread_id IN (?)", bun.List(threadIDs)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete thread messages: %w", err)
	}
	if _, err := tx.NewDelete().Model((*models.InvestigationThread)(nil)).
		Where("id IN (?)", bun.List(threadIDs)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete owner thread: %w", err)
	}
	return nil
}
