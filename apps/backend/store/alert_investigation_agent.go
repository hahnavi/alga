// alert_investigation_agent.go contains agent-scoped alert investigation
// operations: per-agent resets, active counts, and stalled investigation
// detection/reset.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"alga/db/models"
)

func (s *pgAlertInvestigationStore) ResetInvestigatingByAgent(ctx context.Context, agentID string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("status = ?", AlertInvestigationStatusPending).
		Set("agent_id = ''").
		Set("agent_name = ''").
		Set("agent_type = ''").
		Set("started_at = NULL").
		Set("updated_at = ?", time.Now().UTC()).
		Where("agent_id = ?", agentID).
		Where("status IN (?)", bun.List([]string{AlertInvestigationStatusInvestigating, AlertInvestigationStatusPaused})).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset investigating alert investigations by agent: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) ResetAssignedByAgent(ctx context.Context, agentID string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("status = ?", AlertInvestigationStatusPending).
		Set("agent_id = ''").
		Set("agent_name = ''").
		Set("agent_type = ''").
		Set("started_at = NULL").
		Set("updated_at = ?", time.Now().UTC()).
		Where("agent_id = ?", agentID).
		Where("status = ?", AlertInvestigationStatusAssigned).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset assigned alert investigations by agent: %w", err)
	}
	return nil
}

func (s *pgAlertInvestigationStore) CountActiveByAgent(ctx context.Context, agentID string) (int, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	count, err := s.db.NewSelect().Model((*models.AlertInvestigation)(nil)).
		Where("agent_id = ?", agentID).
		Where("status IN (?)", bun.List([]string{AlertInvestigationStatusAssigned, AlertInvestigationStatusInvestigating, AlertInvestigationStatusPaused})).
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
		AgentID string `bun:"agent_id"`
		Count   int    `bun:"count"`
	}
	err := s.db.NewSelect().
		ColumnExpr("agent_id, count(*) as count").
		Model((*models.AlertInvestigation)(nil)).
		Where("agent_id IN (?)", bun.List(agentIDs)).
		Where("status IN (?)", bun.List([]string{AlertInvestigationStatusAssigned, AlertInvestigationStatusInvestigating, AlertInvestigationStatusPaused})).
		Group("agent_id").
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
	return s.listStalledAlertInvestigationsByStatus(ctx, AlertInvestigationStatusAssigned, threshold, false)
}

func (s *pgAlertInvestigationStore) ListStalledInvestigatingAlertInvestigations(ctx context.Context, threshold time.Duration) ([]AlertInvestigationRecord, error) {
	return s.listStalledAlertInvestigationsByStatus(ctx, AlertInvestigationStatusInvestigating, threshold, true)
}

func (s *pgAlertInvestigationStore) listStalledAlertInvestigationsByStatus(ctx context.Context, status string, threshold time.Duration, requireNoRecentUpdates bool) ([]AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	cutoff := time.Now().UTC().Add(-threshold)

	q := s.db.NewSelect().Model((*models.AlertInvestigation)(nil)).
		Where("status = ?", status).
		Where("started_at <= ?", cutoff)

	if requireNoRecentUpdates {
		q = q.Where("NOT EXISTS (SELECT 1 FROM alert_investigation_updates u WHERE u.alert_investigation_id = alert_investigation.id AND u.created_at >= ?)", cutoff)
	}

	var invs []models.AlertInvestigation
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("list stalled alert investigations %s: %w", status, err)
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

func (s *pgAlertInvestigationStore) ResetStalledAssignedAlertInvestigations(timeout time.Duration) ([]string, error) {
	return s.resetStalledAlertInvestigationsByStatus(AlertInvestigationStatusAssigned, timeout, false)
}

func (s *pgAlertInvestigationStore) ResetStalledInvestigatingAlertInvestigations(timeout time.Duration) ([]string, error) {
	return s.resetStalledAlertInvestigationsByStatus(AlertInvestigationStatusInvestigating, timeout, true)
}

func (s *pgAlertInvestigationStore) resetStalledAlertInvestigationsByStatus(status string, timeout time.Duration, requireNoRecentUpdates bool) ([]string, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	cutoff := time.Now().UTC().Add(-timeout)

	q := s.db.NewSelect().Model((*models.AlertInvestigation)(nil)).
		Where("status = ?", status).
		Where("started_at <= ?", cutoff)

	if requireNoRecentUpdates {
		q = q.Where("NOT EXISTS (SELECT 1 FROM alert_investigation_updates u WHERE u.alert_investigation_id = alert_investigation.id AND u.created_at >= ?)", cutoff)
	}

	var invs []models.AlertInvestigation
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("reset stalled alert investigations %s: %w", status, err)
	}

	ids := make([]string, 0, len(invs))
	for _, inv := range invs {
		_, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
			Set("status = ?", AlertInvestigationStatusPending).
			Set("agent_id = ''").
			Set("agent_name = ''").
			Set("agent_type = ''").
			Set("started_at = NULL").
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", inv.ID).
			Exec(ctx)
		if err != nil {
			continue
		}
		ids = append(ids, inv.AlertInvestigationID)
	}
	return ids, nil
}
