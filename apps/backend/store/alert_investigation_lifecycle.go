// alert_investigation_lifecycle.go contains alert investigation status,
// completion, requeue, promotion, claim, and deletion operations.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
	"alga/logger"
)

func (s *pgAlertInvestigationStore) CompleteAlertInvestigation(ctx context.Context, id string, completion AlertInvestigationCompletion) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now().UTC()

		res, err := tx.NewUpdate().Model((*models.AlertInvestigation)(nil)).
			Set("status = ?", AlertInvestigationStatusComplete).
			Set("completed_at = ?", now).
			Set("completed_reason = ?", completion.Reason).
			Set("completed_by_type = ?", completion.ActorType).
			Set("completed_by_id = ?", completion.ActorID).
			Set("completed_by_name = ?", completion.ActorName).
			Set("updated_at = ?", now).
			Where("id = ?", invUUID).
			Where("status IN (?)", bun.In([]string{
				AlertInvestigationStatusPending,
				AlertInvestigationStatusAssigned,
				AlertInvestigationStatusInvestigating,
				AlertInvestigationStatusPaused,
			})).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to complete alert investigation: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to complete alert investigation: %w", err)
		}
		if n == 0 {
			// Check if already terminal for idempotency.
			var existing models.AlertInvestigation
			if lookupErr := tx.NewSelect().Model(&existing).Where("id = ?", invUUID).Scan(ctx); lookupErr == nil && IsTerminalInvestigationStatus(existing.Status) {
				logger.InfoCtx(ctx, "alert investigation already terminal, treating completion as idempotent", "alert_investigation_id", id, "status", existing.Status)
				return nil
			}
			return fmt.Errorf("failed to complete alert investigation: %w", ErrInvestigationNotFound)
		}

		// Load the investigation to get agent fields for the event.
		var inv models.AlertInvestigation
		if err := tx.NewSelect().Model(&inv).Where("id = ?", invUUID).Scan(ctx); err != nil {
			return fmt.Errorf("failed to reload alert investigation for event: %w", err)
		}

		if err := createAlertInvestigationEvent(ctx, tx, inv.ID, AlertInvestigationEvent{
			EventType: AlertInvestigationEventCompleted,
			Reason:    completion.EventReason,
			ActorType: completion.ActorType,
			ActorID:   completion.ActorID,
			ActorName: completion.ActorName,
			AgentID:   inv.AgentID,
			AgentName: inv.AgentName,
			AgentType: inv.AgentType,
			CreatedAt: now,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *pgAlertInvestigationStore) RequeueAlertInvestigation(ctx context.Context, id string, requeue AlertInvestigationRequeue) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now().UTC()

		res, err := tx.NewUpdate().Model((*models.AlertInvestigation)(nil)).
			Set("status = ?", AlertInvestigationStatusPending).
			Set("started_at = NULL").
			Set("updated_at = ?", now).
			Where("id = ?", invUUID).
			Where("status IN (?)", bun.In([]string{
				AlertInvestigationStatusAssigned,
				AlertInvestigationStatusInvestigating,
				AlertInvestigationStatusPaused,
			})).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to requeue alert investigation: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to requeue alert investigation: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("failed to requeue alert investigation: %w", ErrInvestigationNotFound)
		}

		// Load the investigation to get agent fields for the event.
		var inv models.AlertInvestigation
		if err := tx.NewSelect().Model(&inv).Where("id = ?", invUUID).Scan(ctx); err != nil {
			return fmt.Errorf("failed to reload alert investigation for event: %w", err)
		}

		eventType := requeue.EventType
		if eventType == "" {
			eventType = AlertInvestigationEventRequeued
		}
		if err := createAlertInvestigationEvent(ctx, tx, inv.ID, AlertInvestigationEvent{
			EventType: eventType,
			Reason:    requeue.Reason,
			ActorType: requeue.ActorType,
			ActorName: requeue.ActorName,
			AgentID:   inv.AgentID,
			AgentName: inv.AgentName,
			AgentType: inv.AgentType,
			Metadata:  requeue.Metadata,
			CreatedAt: now,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *pgAlertInvestigationStore) MarkAlertInvestigationPromoted(ctx context.Context, id string, incidentID string, incidentNumber int64, incidentInvestigationID string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	incidentUUID, err := uuid.Parse(incidentID)
	if err != nil {
		return nil, fmt.Errorf("invalid incident id: %w", err)
	}
	incidentInvestigationUUID, err := uuid.Parse(incidentInvestigationID)
	if err != nil {
		return nil, fmt.Errorf("invalid incident investigation id: %w", err)
	}

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var inv models.AlertInvestigation
		if err := tx.NewSelect().Model(&inv).Where("alert_investigation_id = ?", id).Scan(ctx); err != nil {
			return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
		}

		now := time.Now().UTC()
		_, err := tx.NewUpdate().Model((*models.AlertInvestigation)(nil)).
			Set("status = ?", AlertInvestigationStatusPromoted).
			Set("promoted_incident_id = ?", incidentUUID).
			Set("promoted_incident_investigation_id = ?", incidentInvestigationUUID).
			Set("updated_at = ?", now).
			Where("id = ?", inv.ID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to mark alert investigation promoted: %w", err)
		}

		message := fmt.Sprintf("Promoted alert investigation to incident **#%d**.", incidentNumber)
		if err := createAlertInvestigationUpdate(ctx, tx, inv.ID, InvestigationUpdate{
			Type:      UpdateTypeProgress,
			Message:   message,
			Source:    UpdateSourceSystem,
			Internal:  true,
			CreatedAt: now,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetAlertInvestigation(ctx, id)
}

func (s *pgAlertInvestigationStore) UpdateAlertInvestigationStatus(ctx context.Context, id string, status string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now().UTC()).
		Where("alert_investigation_id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update alert investigation status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update alert investigation status: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
	}
	return nil
}

func (s *pgAlertInvestigationStore) TransitionAlertInvestigationStatus(ctx context.Context, id string, fromStatuses []string, toStatus string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("status = ?", toStatus).
		Set("updated_at = ?", now).
		Where("id = ?", invUUID)

	if len(fromStatuses) > 0 {
		q = q.Where("status IN (?)", bun.In(fromStatuses))
	}

	switch toStatus {
	case AlertInvestigationStatusInvestigating:
		q = q.Set("started_at = ?", now)
	case AlertInvestigationStatusComplete, AlertInvestigationStatusFailed, AlertInvestigationStatusCancelled, AlertInvestigationStatusTimedOut:
		q = q.Set("completed_at = ?", now)
	}

	_, err = q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to transition alert investigation status: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) PatchAlertInvestigationOutcome(ctx context.Context, id string, rootCause *string, resolution *string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	var inv models.AlertInvestigation
	if err := s.db.NewSelect().Model(&inv).Where("id = ?", invUUID).Scan(ctx); err != nil {
		if isNotFound(err) {
			return ErrAlertInvestigationNotFound
		}
		return fmt.Errorf("load alert investigation: %w", err)
	}

	summary := inv.Summary
	if summary == nil {
		summary = &models.AlertInvestigationSummary{}
	}
	if rootCause != nil {
		summary.RootCause = *rootCause
	}
	if resolution != nil {
		summary.Resolution = *resolution
	}

	_, err = s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("summary = ?", summary).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", invUUID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to patch alert investigation outcome: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) UpdateAlertInvestigationAgent(ctx context.Context, id string, agentID string, agentName string, agentType string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	_, err = s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("agent_id = ?", agentID).
		Set("agent_name = ?", agentName).
		Set("agent_type = ?", agentType).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", invUUID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update alert investigation agent: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) ClaimPendingAlertInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid alert investigation id: %w", err)
	}

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("status = ?", AlertInvestigationStatusAssigned).
		Set("agent_id = ?", agentID).
		Set("agent_name = ?", agentName).
		Set("agent_type = ?", agentType).
		Set("started_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", invUUID).
		Where("status = ?", AlertInvestigationStatusPending).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending alert investigation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending alert investigation: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("failed to claim pending alert investigation: %w", ErrInvestigationNotFound)
	}

	var inv models.AlertInvestigation
	if err := s.db.NewSelect().Model(&inv).Where("id = ?", invUUID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload claimed alert investigation: %w", err)
	}
	return s.toAlertInvestigationRecord(ctx, &inv)
}

func (s *pgAlertInvestigationStore) ListPendingAlertInvestigations(ctx context.Context, limit int64) ([]AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var invs []models.AlertInvestigation
	if err := s.db.NewSelect().Model(&invs).
		Where("status = ?", AlertInvestigationStatusPending).
		Order("created_at ASC").
		Limit(int(limit)).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list pending alert investigations: %w", err)
	}

	records := make([]AlertInvestigationRecord, 0, len(invs))
	for i := range invs {
		rec, err := s.toAlertInvestigationRecord(ctx, &invs[i])
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgAlertInvestigationStore) DeleteAlertInvestigation(ctx context.Context, id string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var inv models.AlertInvestigation
		if err := tx.NewSelect().Model(&inv).Where("alert_investigation_id = ?", id).Scan(ctx); err != nil {
			if isNotFound(err) {
				return ErrAlertInvestigationNotFound
			}
			return fmt.Errorf("load alert investigation for delete: %w", err)
		}

		if _, err := tx.NewDelete().Model((*models.AlertInvestigationAlert)(nil)).
			Where("alert_investigation_id = ?", inv.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete alert investigation alerts: %w", err)
		}

		if _, err := tx.NewDelete().Model((*models.AlertInvestigationUpdate)(nil)).
			Where("alert_investigation_id = ?", inv.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete alert investigation updates: %w", err)
		}

		if _, err := tx.NewDelete().Model((*models.AlertInvestigationEvent)(nil)).
			Where("alert_investigation_id = ?", inv.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete alert investigation events: %w", err)
		}

		if _, err := tx.NewDelete().Model((*models.AlertInvestigation)(nil)).
			Where("id = ?", inv.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete alert investigation: %w", err)
		}
		return nil
	})
}

func (s *pgAlertInvestigationStore) SetAlertInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	q := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("assignee_type = ?", assigneeType).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", invUUID)

	if assigneeID != nil {
		q = q.Set("assignee_id = ?", *assigneeID)
	} else {
		q = q.Set("assignee_id = NULL")
	}

	if assigneeType != InvestigationActorAgent {
		q = q.Set("agent_id = ''").Set("agent_name = ''").Set("agent_type = ''")
	}

	if _, err := q.Exec(ctx); err != nil {
		return fmt.Errorf("failed to set alert investigation assignee: %w", err)
	}
	return nil
}
