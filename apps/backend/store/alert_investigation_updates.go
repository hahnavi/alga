// alert_investigation_updates.go contains alert investigation message,
// event, and update-entry operations: appending alerts, marking alerts
// current, adding update entries/events, and editing/deleting messages.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
	"alga/rabbitmq"
)

func (s *pgAlertInvestigationStore) AppendAlertsToAlertInvestigation(ctx context.Context, id string, alerts []rabbitmq.CorrelatedAlert) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var inv models.AlertInvestigation
		if err := tx.NewSelect().Model(&inv).Where("alert_investigation_id = ?", id).Scan(ctx); err != nil {
			return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
		}

		var existingAlerts []models.AlertInvestigationAlert
		if err := tx.NewSelect().Model(&existingAlerts).
			Where("alert_investigation_id = ?", inv.ID).
			Scan(ctx); err != nil {
			return fmt.Errorf("failed to query existing alert investigation alerts: %w", err)
		}

		seen := make(map[string]struct{}, len(existingAlerts))
		for _, alert := range existingAlerts {
			seen[alert.Fingerprint] = struct{}{}
		}
		for _, alert := range alerts {
			if _, ok := seen[alert.Fingerprint]; ok {
				continue
			}
			if err := retireCurrentAlertInvestigationLinks(ctx, tx, []rabbitmq.CorrelatedAlert{alert}); err != nil {
				return err
			}
			if err := createAlertInvestigationAlert(ctx, tx, inv.ID, alert); err != nil {
				return err
			}
			seen[alert.Fingerprint] = struct{}{}
		}

		if _, err := tx.NewUpdate().Model((*models.AlertInvestigation)(nil)).
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", inv.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to update alert investigation timestamp: %w", err)
		}
		return nil
	})
}

func (s *pgAlertInvestigationStore) MarkAlertInvestigationAlertsCurrent(ctx context.Context, investigationID string, current bool) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.AlertInvestigationAlert)(nil)).
		Set("current = ?", current).
		Where("alert_investigation_id IN (SELECT id FROM alert_investigations WHERE alert_investigation_id = ?)", investigationID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark alert investigation alerts current: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to mark alert investigation alerts current: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
	}
	return nil
}

func retireCurrentAlertInvestigationLinks(ctx context.Context, db bun.IDB, alerts []rabbitmq.CorrelatedAlert) error {
	alertNumberSet := make(map[int64]struct{})
	for _, alert := range alerts {
		if alert.AlertNumber > 0 {
			alertNumberSet[alert.AlertNumber] = struct{}{}
		}
	}
	if len(alertNumberSet) == 0 {
		return nil
	}

	alertNumbers := make([]int64, 0, len(alertNumberSet))
	for alertNumber := range alertNumberSet {
		alertNumbers = append(alertNumbers, alertNumber)
	}

	// Resolve alert IDs for the given alert numbers.
	var alertIDs []uuid.UUID
	if err := db.NewSelect().Model((*models.Alert)(nil)).
		Column("id").
		Where("alert_number IN (?)", bun.List(alertNumbers)).
		Scan(ctx, &alertIDs); err != nil {
		return fmt.Errorf("failed to resolve alert ids for current alert investigation links: %w", err)
	}

	// Retire current links: match by alert_number OR alert_id.
	q := db.NewUpdate().Model((*models.AlertInvestigationAlert)(nil)).
		Set("current = false").
		Where("current = true").
		Where("alert_number IN (?)", bun.List(alertNumbers))
	if len(alertIDs) > 0 {
		q = q.WhereOr("alert_id IN (?)", bun.List(alertIDs))
	}

	if _, err := q.Exec(ctx); err != nil {
		return fmt.Errorf("failed to retire current alert investigation links: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) AddAlertInvestigationUpdate(ctx context.Context, id string, update InvestigationUpdate) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.AlertInvestigation
	if err := s.db.NewSelect().Model(&inv).Where("alert_investigation_id = ?", id).Scan(ctx); err != nil {
		return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
	}

	if err := createAlertInvestigationUpdate(ctx, s.db, inv.ID, update); err != nil {
		return err
	}
	if _, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", inv.ID).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to update alert investigation timestamp: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) AppendAlertInvestigationEvent(ctx context.Context, investigationUUID uuid.UUID, event AlertInvestigationEvent) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return createAlertInvestigationEvent(ctx, s.db, investigationUUID, event)
}

func (s *pgAlertInvestigationStore) UpdateAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string, message string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	uid, err := uuid.Parse(updateID)
	if err != nil {
		return fmt.Errorf("invalid update ID %q: %w", updateID, err)
	}

	res, err := s.db.NewUpdate().Model((*models.AlertInvestigationUpdate)(nil)).
		Set("message = ?", message).
		Set("edited = true").
		Where("id = ?", uid).
		Where("alert_investigation_id IN (SELECT id FROM alert_investigations WHERE alert_investigation_id = ?)", investigationID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update alert investigation message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update alert investigation message: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation update %s not found in %s", updateID, investigationID)
	}
	return nil
}

func (s *pgAlertInvestigationStore) DeleteAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	uid, err := uuid.Parse(updateID)
	if err != nil {
		return fmt.Errorf("invalid update ID %q: %w", updateID, err)
	}

	res, err := s.db.NewDelete().Model((*models.AlertInvestigationUpdate)(nil)).
		Where("id = ?", uid).
		Where("alert_investigation_id IN (SELECT id FROM alert_investigations WHERE alert_investigation_id = ?)", investigationID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete alert investigation message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete alert investigation message: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation update %s not found in %s", updateID, investigationID)
	}
	return nil
}

func (s *pgAlertInvestigationStore) SetAlertInvestigationUpdateMMPostID(ctx context.Context, investigationID string, updateID string, mmPostID string) error {
	return s.setAlertInvestigationUpdateField(ctx, investigationID, updateID, "mm_post_id", "mm_post_id = ?", mmPostID)
}

func (s *pgAlertInvestigationStore) SetAlertInvestigationUpdateSlackMessageTS(ctx context.Context, investigationID string, updateID string, slackMessageTS string) error {
	return s.setAlertInvestigationUpdateField(ctx, investigationID, updateID, "slack_message_ts", "slack_message_ts = ?", slackMessageTS)
}

func (s *pgAlertInvestigationStore) setAlertInvestigationUpdateField(ctx context.Context, investigationID string, updateID string, fieldName string, setExpr string, setVal any) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	uid, err := uuid.Parse(updateID)
	if err != nil {
		return fmt.Errorf("invalid update ID %q: %w", updateID, err)
	}

	res, err := s.db.NewUpdate().Model((*models.AlertInvestigationUpdate)(nil)).
		Set(setExpr, setVal).
		Where("id = ?", uid).
		Where("alert_investigation_id IN (SELECT id FROM alert_investigations WHERE alert_investigation_id = ?)", investigationID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set update %s: %w", fieldName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to set update %s: %w", fieldName, err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation update %s not found in %s", updateID, investigationID)
	}
	return nil
}
