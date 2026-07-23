// alert_investigation_updates.go contains alert investigation message,
// event, and update-entry operations: appending alerts, marking alerts
// current, adding update entries/events, and editing/deleting messages.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	alertent "alga/ent/alert"
	"alga/ent/alertinvestigation"
	"alga/ent/alertinvestigationalert"
	"alga/ent/alertinvestigationupdateentry"
	"alga/ent/predicate"
	"alga/rabbitmq"
)

func (s *pgAlertInvestigationStore) AppendAlertsToAlertInvestigation(ctx context.Context, id string, alerts []rabbitmq.CorrelatedAlert) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin alert investigation transaction: %w", err)
	}
	defer rollbackTx(tx)
	txClient := tx.Client()

	inv, err := txClient.AlertInvestigation.Query().
		Where(alertinvestigation.AlertInvestigationID(id)).
		WithAlerts().
		Only(ctx)
	if err != nil {
		return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
	}

	seen := make(map[string]struct{}, len(inv.Edges.Alerts))
	for _, alert := range inv.Edges.Alerts {
		seen[alert.Fingerprint] = struct{}{}
	}
	for _, alert := range alerts {
		if _, ok := seen[alert.Fingerprint]; ok {
			continue
		}
		if err := retireCurrentAlertInvestigationLinks(ctx, txClient, []rabbitmq.CorrelatedAlert{alert}); err != nil {
			return err
		}
		if err := createAlertInvestigationAlert(ctx, txClient, inv.ID, alert); err != nil {
			return err
		}
		seen[alert.Fingerprint] = struct{}{}
	}

	if _, err := txClient.AlertInvestigation.UpdateOneID(inv.ID).SetUpdatedAt(time.Now().UTC()).Save(ctx); err != nil {
		return fmt.Errorf("failed to update alert investigation timestamp: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit alert investigation transaction: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) MarkAlertInvestigationAlertsCurrent(ctx context.Context, investigationID string, current bool) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	n, err := s.client.AlertInvestigationAlert.Update().
		Where(alertinvestigationalert.HasAlertInvestigationWith(alertinvestigation.AlertInvestigationID(investigationID))).
		SetCurrent(current).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark alert investigation alerts current: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
	}
	return nil
}

func retireCurrentAlertInvestigationLinks(ctx context.Context, client *ent.Client, alerts []rabbitmq.CorrelatedAlert) error {
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

	linkPreds := []predicate.AlertInvestigationAlert{
		alertinvestigationalert.AlertNumberIn(alertNumbers...),
	}
	alertIDs, err := client.Alert.Query().
		Where(alertent.AlertNumberIn(alertNumbers...)).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve alert ids for current alert investigation links: %w", err)
	}
	if len(alertIDs) > 0 {
		linkPreds = append(linkPreds, alertinvestigationalert.AlertIDIn(alertIDs...))
	}

	if _, err := client.AlertInvestigationAlert.Update().
		Where(
			alertinvestigationalert.Current(true),
			alertinvestigationalert.Or(linkPreds...),
		).
		SetCurrent(false).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to retire current alert investigation links: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) AddAlertInvestigationUpdate(ctx context.Context, id string, update InvestigationUpdate) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inv, err := s.client.AlertInvestigation.Query().
		Where(alertinvestigation.AlertInvestigationID(id)).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
	}

	if err := createAlertInvestigationUpdate(ctx, s.client, inv.ID, update); err != nil {
		return err
	}
	if _, err := s.client.AlertInvestigation.UpdateOneID(inv.ID).SetUpdatedAt(time.Now().UTC()).Save(ctx); err != nil {
		return fmt.Errorf("failed to update alert investigation timestamp: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) AppendAlertInvestigationEvent(ctx context.Context, investigationUUID uuid.UUID, event AlertInvestigationEvent) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return createAlertInvestigationEvent(ctx, s.client, investigationUUID, event)
}

func (s *pgAlertInvestigationStore) UpdateAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string, message string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	uid, err := uuid.Parse(updateID)
	if err != nil {
		return fmt.Errorf("invalid update ID %q: %w", updateID, err)
	}

	n, err := s.client.AlertInvestigationUpdateEntry.Update().
		Where(
			alertinvestigationupdateentry.HasAlertInvestigationWith(alertinvestigation.AlertInvestigationID(investigationID)),
			alertinvestigationupdateentry.ID(uid),
		).
		SetMessage(message).
		SetEdited(true).
		Save(ctx)
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

	n, err := s.client.AlertInvestigationUpdateEntry.Delete().
		Where(
			alertinvestigationupdateentry.HasAlertInvestigationWith(alertinvestigation.AlertInvestigationID(investigationID)),
			alertinvestigationupdateentry.ID(uid),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete alert investigation message: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation update %s not found in %s", updateID, investigationID)
	}
	return nil
}

func (s *pgAlertInvestigationStore) SetAlertInvestigationUpdateMMPostID(ctx context.Context, investigationID string, updateID string, mmPostID string) error {
	return s.setAlertInvestigationUpdateField(ctx, investigationID, updateID, "mm_post_id",
		func(u *ent.AlertInvestigationUpdateEntryUpdate) {
			u.SetMmPostID(mmPostID)
		},
	)
}

func (s *pgAlertInvestigationStore) SetAlertInvestigationUpdateSlackMessageTS(ctx context.Context, investigationID string, updateID string, slackMessageTS string) error {
	return s.setAlertInvestigationUpdateField(ctx, investigationID, updateID, "slack_message_ts",
		func(u *ent.AlertInvestigationUpdateEntryUpdate) {
			u.SetSlackMessageTs(slackMessageTS)
		},
	)
}

func (s *pgAlertInvestigationStore) setAlertInvestigationUpdateField(ctx context.Context, investigationID string, updateID string, fieldName string, apply func(*ent.AlertInvestigationUpdateEntryUpdate)) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	uid, err := uuid.Parse(updateID)
	if err != nil {
		return fmt.Errorf("invalid update ID %q: %w", updateID, err)
	}

	builder := s.client.AlertInvestigationUpdateEntry.Update().
		Where(
			alertinvestigationupdateentry.HasAlertInvestigationWith(alertinvestigation.AlertInvestigationID(investigationID)),
			alertinvestigationupdateentry.ID(uid),
		)
	apply(builder)

	n, err := builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set update %s: %w", fieldName, err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation update %s not found in %s", updateID, investigationID)
	}
	return nil
}
