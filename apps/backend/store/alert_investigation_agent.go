// alert_investigation_agent.go contains agent-scoped alert investigation
// operations: per-agent resets, active counts, and stalled investigation
// detection/reset.
package store

import (
	"context"
	"fmt"
	"time"

	"alga/ent"
	"alga/ent/alertinvestigation"
	"alga/ent/alertinvestigationevent"
	"alga/ent/alertinvestigationupdateentry"
	"alga/ent/predicate"
)

func (s *pgAlertInvestigationStore) ResetInvestigatingByAgent(ctx context.Context, agentID string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.client.AlertInvestigation.Update().
		Where(
			alertinvestigation.AgentIDEQ(agentID),
			alertinvestigation.StatusIn(alertinvestigation.Status(AlertInvestigationStatusInvestigating), alertinvestigation.Status(AlertInvestigationStatusPaused)),
		).
		SetStatus(alertinvestigation.Status(AlertInvestigationStatusPending)).
		ClearAgentID().
		ClearAgentName().
		ClearAgentType().
		ClearStartedAt().
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset investigating alert investigations by agent: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) ResetAssignedByAgent(ctx context.Context, agentID string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.client.AlertInvestigation.Update().
		Where(
			alertinvestigation.AgentIDEQ(agentID),
			alertinvestigation.StatusEQ(alertinvestigation.Status(AlertInvestigationStatusAssigned)),
		).
		SetStatus(alertinvestigation.Status(AlertInvestigationStatusPending)).
		ClearAgentID().
		ClearAgentName().
		ClearAgentType().
		ClearStartedAt().
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset assigned alert investigations by agent: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) CountActiveByAgent(ctx context.Context, agentID string) (int, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	count, err := s.client.AlertInvestigation.Query().
		Where(
			alertinvestigation.AgentIDEQ(agentID),
			alertinvestigation.StatusIn(alertinvestigation.Status(AlertInvestigationStatusAssigned), alertinvestigation.Status(AlertInvestigationStatusInvestigating), alertinvestigation.Status(AlertInvestigationStatusPaused)),
		).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count active alert investigations by agent: %w", err)
	}
	return count, nil
}

func (s *pgAlertInvestigationStore) CountActiveByAgents(ctx context.Context, agentIDs []string) (map[string]int, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	result := make(map[string]int, len(agentIDs))
	if len(agentIDs) == 0 {
		return result, nil
	}

	var groups []struct {
		AgentID string `json:"agent_id"`
		Count   int    `json:"count"`
	}
	err := s.client.AlertInvestigation.Query().
		Where(
			alertinvestigation.AgentIDIn(agentIDs...),
			alertinvestigation.StatusIn(alertinvestigation.Status(AlertInvestigationStatusAssigned), alertinvestigation.Status(AlertInvestigationStatusInvestigating), alertinvestigation.Status(AlertInvestigationStatusPaused)),
		).
		GroupBy(alertinvestigation.FieldAgentID).
		Aggregate(ent.Count()).
		Scan(ctx, &groups)
	if err != nil {
		return nil, fmt.Errorf("failed to batch count active investigations: %w", err)
	}

	for _, g := range groups {
		result[g.AgentID] = g.Count
	}

	for _, id := range agentIDs {
		if _, ok := result[id]; !ok {
			result[id] = 0
		}
	}
	return result, nil
}

func (s *pgAlertInvestigationStore) ListStalledAssignedAlertInvestigations(ctx context.Context, threshold time.Duration) ([]AlertInvestigationRecord, error) {
	return s.listStalledAlertInvestigationsByStatus(ctx, AlertInvestigationStatusAssigned, threshold, nil)
}

func (s *pgAlertInvestigationStore) ListStalledInvestigatingAlertInvestigations(ctx context.Context, threshold time.Duration) ([]AlertInvestigationRecord, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	extra := []predicate.AlertInvestigation{
		alertinvestigation.Not(
			alertinvestigation.HasUpdatesWith(
				alertinvestigationupdateentry.CreatedAtGTE(cutoff),
			),
		),
	}
	return s.listStalledAlertInvestigationsByStatus(ctx, AlertInvestigationStatusInvestigating, threshold, extra)
}

func (s *pgAlertInvestigationStore) listStalledAlertInvestigationsByStatus(ctx context.Context, status string, threshold time.Duration, extraPreds []predicate.AlertInvestigation) ([]AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	cutoff := time.Now().UTC().Add(-threshold)
	preds := []predicate.AlertInvestigation{
		alertinvestigation.StatusEQ(alertinvestigation.Status(status)),
		alertinvestigation.StartedAtLTE(cutoff),
	}
	preds = append(preds, extraPreds...)

	invs, err := s.client.AlertInvestigation.Query().Where(preds...).
		WithAlerts().
		WithUpdates(func(q *ent.AlertInvestigationUpdateEntryQuery) {
			q.Order(ent.Asc(alertinvestigationupdateentry.FieldCreatedAt))
		}).
		WithEvents(func(q *ent.AlertInvestigationEventQuery) { q.Order(ent.Asc(alertinvestigationevent.FieldCreatedAt)) }).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stalled alert investigations %s: %w", status, err)
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

func (s *pgAlertInvestigationStore) ResetStalledAssignedAlertInvestigations(timeout time.Duration) ([]string, error) {
	return s.resetStalledAlertInvestigationsByStatus(AlertInvestigationStatusAssigned, timeout, nil)
}

func (s *pgAlertInvestigationStore) ResetStalledInvestigatingAlertInvestigations(timeout time.Duration) ([]string, error) {
	cutoff := time.Now().UTC().Add(-timeout)
	extra := []predicate.AlertInvestigation{
		alertinvestigation.Not(
			alertinvestigation.HasUpdatesWith(
				alertinvestigationupdateentry.CreatedAtGTE(cutoff),
			),
		),
	}
	return s.resetStalledAlertInvestigationsByStatus(AlertInvestigationStatusInvestigating, timeout, extra)
}

func (s *pgAlertInvestigationStore) resetStalledAlertInvestigationsByStatus(status string, timeout time.Duration, extraPreds []predicate.AlertInvestigation) ([]string, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	cutoff := time.Now().UTC().Add(-timeout)
	preds := []predicate.AlertInvestigation{
		alertinvestigation.StatusEQ(alertinvestigation.Status(status)),
		alertinvestigation.StartedAtLTE(cutoff),
	}
	preds = append(preds, extraPreds...)

	invs, err := s.client.AlertInvestigation.Query().Where(preds...).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("reset stalled alert investigations %s: %w", status, err)
	}

	ids := make([]string, 0, len(invs))
	for _, inv := range invs {
		_, err := s.client.AlertInvestigation.UpdateOneID(inv.ID).
			SetStatus(alertinvestigation.Status(AlertInvestigationStatusPending)).
			ClearAgentID().
			ClearAgentName().
			ClearAgentType().
			ClearStartedAt().
			SetUpdatedAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			continue
		}
		ids = append(ids, inv.AlertInvestigationID)
	}
	return ids, nil
}
