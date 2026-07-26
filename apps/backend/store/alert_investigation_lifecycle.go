// alert_investigation_lifecycle.go contains alert investigation status,
// completion, requeue, promotion, claim, and deletion operations.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/alertinvestigation"
	"alga/ent/alertinvestigationalert"
	"alga/ent/alertinvestigationevent"
	"alga/ent/alertinvestigationupdateentry"
	entschema "alga/ent/schema"
	"alga/logger"
)

func (s *pgAlertInvestigationStore) CompleteAlertInvestigation(ctx context.Context, id string, completion AlertInvestigationCompletion) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin alert investigation completion transaction: %w", err)
	}
	defer rollbackTx(tx)

	now := time.Now().UTC()
	inv, err := tx.Client().AlertInvestigation.UpdateOneID(invUUID).
		Where(alertinvestigation.StatusIn(
			alertinvestigation.Status(AlertInvestigationStatusPending),
			alertinvestigation.Status(AlertInvestigationStatusAssigned),
			alertinvestigation.Status(AlertInvestigationStatusInvestigating),
			alertinvestigation.Status(AlertInvestigationStatusPaused),
		)).
		SetStatus(alertinvestigation.Status(AlertInvestigationStatusComplete)).
		SetCompletedAt(now).
		SetCompletedReason(completion.Reason).
		SetCompletedByType(completion.ActorType).
		SetCompletedByID(completion.ActorID).
		SetCompletedByName(completion.ActorName).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			if existing, lookupErr := tx.Client().AlertInvestigation.Get(ctx, invUUID); lookupErr == nil && existing != nil && IsTerminalInvestigationStatus(string(existing.Status)) {
				logger.InfoCtx(ctx, "alert investigation already terminal, treating completion as idempotent", "alert_investigation_id", id, "status", string(existing.Status))
				return nil
			}
		}
		return fmt.Errorf("failed to complete alert investigation: %w", err)
	}

	if err := createAlertInvestigationEvent(ctx, tx.Client(), inv.ID, AlertInvestigationEvent{
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit alert investigation completion: %w", err)
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

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin alert investigation requeue transaction: %w", err)
	}
	defer rollbackTx(tx)

	now := time.Now().UTC()
	inv, err := tx.Client().AlertInvestigation.UpdateOneID(invUUID).
		Where(alertinvestigation.StatusIn(
			alertinvestigation.Status(AlertInvestigationStatusAssigned),
			alertinvestigation.Status(AlertInvestigationStatusInvestigating),
			alertinvestigation.Status(AlertInvestigationStatusPaused),
		)).
		SetStatus(alertinvestigation.Status(AlertInvestigationStatusPending)).
		ClearStartedAt().
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to requeue alert investigation: %w", err)
	}

	eventType := requeue.EventType
	if eventType == "" {
		eventType = AlertInvestigationEventRequeued
	}
	if err := createAlertInvestigationEvent(ctx, tx.Client(), inv.ID, AlertInvestigationEvent{
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit alert investigation requeue: %w", err)
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

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin alert investigation promotion transaction: %w", err)
	}
	defer rollbackTx(tx)

	inv, err := tx.Client().AlertInvestigation.Query().
		Where(alertinvestigation.AlertInvestigationID(id)).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("alert investigation not found: %w", ErrInvestigationNotFound)
	}

	now := time.Now().UTC()
	updated, err := tx.Client().AlertInvestigation.UpdateOneID(inv.ID).
		SetStatus(alertinvestigation.Status(AlertInvestigationStatusPromoted)).
		SetPromotedIncidentID(incidentUUID).
		SetPromotedIncidentInvestigationID(incidentInvestigationUUID).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to mark alert investigation promoted: %w", err)
	}

	message := fmt.Sprintf("Promoted alert investigation to incident **#%d**.", incidentNumber)
	if err := createAlertInvestigationUpdate(ctx, tx.Client(), updated.ID, InvestigationUpdate{
		Type:      UpdateTypeProgress,
		Message:   message,
		Source:    UpdateSourceSystem,
		Internal:  true,
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit alert investigation promotion: %w", err)
	}

	return s.GetAlertInvestigation(ctx, id)
}

func (s *pgAlertInvestigationStore) UpdateAlertInvestigationStatus(ctx context.Context, id string, status string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	n, err := s.client.AlertInvestigation.Update().
		Where(alertinvestigation.AlertInvestigationID(id)).
		SetStatus(alertinvestigation.Status(status)).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
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

	entStatuses := make([]alertinvestigation.Status, len(fromStatuses))
	for i, s := range fromStatuses {
		entStatuses[i] = alertinvestigation.Status(s)
	}

	b := s.client.AlertInvestigation.UpdateOneID(invUUID).
		Where(alertinvestigation.StatusIn(entStatuses...)).
		SetStatus(alertinvestigation.Status(toStatus)).
		SetUpdatedAt(time.Now().UTC())

	switch toStatus {
	case AlertInvestigationStatusInvestigating:
		b.SetStartedAt(time.Now().UTC())
	case AlertInvestigationStatusComplete, AlertInvestigationStatusFailed, AlertInvestigationStatusCancelled, AlertInvestigationStatusTimedOut:
		b.SetCompletedAt(time.Now().UTC())
	}

	_, err = b.Save(ctx)
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

	inv, err := s.client.AlertInvestigation.Get(ctx, invUUID)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrAlertInvestigationNotFound
		}
		return fmt.Errorf("load alert investigation: %w", err)
	}

	summary := inv.Summary
	if summary == nil {
		summary = &entschema.AlertInvestigationSummary{}
	}
	if rootCause != nil {
		summary.RootCause = *rootCause
	}
	if resolution != nil {
		summary.Resolution = *resolution
	}

	_, err = s.client.AlertInvestigation.UpdateOneID(invUUID).
		SetSummary(summary).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
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

	_, err = s.client.AlertInvestigation.UpdateOneID(invUUID).
		SetAgentID(agentID).
		SetAgentName(agentName).
		SetAgentType(agentType).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
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
	n, err := s.client.AlertInvestigation.UpdateOneID(invUUID).
		Where(alertinvestigation.StatusEQ(alertinvestigation.Status(AlertInvestigationStatusPending))).
		SetStatus(alertinvestigation.Status(AlertInvestigationStatusAssigned)).
		SetAgentID(agentID).
		SetAgentName(agentName).
		SetAgentType(agentType).
		SetStartedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending alert investigation: %w", err)
	}

	return s.toAlertInvestigationRecord(ctx, n)
}

func (s *pgAlertInvestigationStore) ListPendingAlertInvestigations(ctx context.Context, limit int64) ([]AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invs, err := s.client.AlertInvestigation.Query().
		Where(alertinvestigation.StatusEQ(alertinvestigation.Status(AlertInvestigationStatusPending))).
		Order(ent.Asc(alertinvestigation.FieldCreatedAt)).
		Limit(int(limit)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending alert investigations: %w", err)
	}

	records := make([]AlertInvestigationRecord, 0, len(invs))
	for _, inv := range invs {
		rec, err := s.toAlertInvestigationRecord(ctx, inv)
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

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin delete alert investigation transaction: %w", err)
	}
	defer rollbackTx(tx)

	inv, err := tx.Client().AlertInvestigation.Query().
		Where(alertinvestigation.AlertInvestigationID(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrAlertInvestigationNotFound
		}
		return fmt.Errorf("load alert investigation for delete: %w", err)
	}

	if _, err := tx.Client().AlertInvestigationAlert.Delete().
		Where(alertinvestigationalert.AlertInvestigationID(inv.ID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete alert investigation alerts: %w", err)
	}

	if _, err := tx.Client().AlertInvestigationUpdateEntry.Delete().
		Where(alertinvestigationupdateentry.AlertInvestigationID(inv.ID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete alert investigation updates: %w", err)
	}

	if _, err := tx.Client().AlertInvestigationEvent.Delete().
		Where(alertinvestigationevent.AlertInvestigationID(inv.ID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete alert investigation events: %w", err)
	}

	if err := tx.Client().AlertInvestigation.DeleteOneID(inv.ID).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete alert investigation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete alert investigation: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) SetAlertInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	invUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid alert investigation id: %w", err)
	}

	u := s.client.AlertInvestigation.UpdateOneID(invUUID).
		SetAssigneeType(alertinvestigation.AssigneeType(assigneeType)).
		SetUpdatedAt(time.Now().UTC())

	if assigneeID != nil {
		u.SetAssigneeID(*assigneeID)
	} else {
		u.ClearAssigneeID()
	}

	if assigneeType != InvestigationActorAgent {
		u.SetAgentID("").SetAgentName("").SetAgentType("")
	}

	if _, err := u.Save(ctx); err != nil {
		return fmt.Errorf("failed to set alert investigation assignee: %w", err)
	}
	return nil
}
