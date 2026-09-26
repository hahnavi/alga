// alert_investigation_agent.go contains agent-scoped alert investigation
// operations: dispatch-lease expiry/renewal, active counts, and stalled
// investigation detection (for stuck-investigation escalation).
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"alga/db/models"
)

// defaultInvestigationLease is the fallback lease applied when callers pass a
// non-positive duration.
const defaultInvestigationLease = 10 * time.Minute

// ExpireAlertInvestigationLeases requeues assigned/investigating rows whose
// dispatch lease has lapsed and returns their public ids. The conditional
// UPDATE is atomic: rows that an agent completes or transitions between
// listing and expiry are never clobbered, because the status guard is
// re-evaluated at write time.
func (s *pgAlertInvestigationStore) ExpireAlertInvestigationLeases(ctx context.Context) ([]string, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	var ids []string
	err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("status = ?", AlertInvestigationStatusPending).
		Set("agent_id = ''").
		Set("agent_name = ''").
		Set("agent_type = ''").
		Set("started_at = NULL").
		Set("lease_until = NULL").
		Set("updated_at = ?", now).
		Where("status IN (?)", bun.List([]string{AlertInvestigationStatusAssigned, AlertInvestigationStatusInvestigating})).
		Where("lease_until IS NOT NULL AND lease_until < ?", now).
		Returning("public_id").
		Scan(ctx, &ids)
	if err != nil {
		return nil, fmt.Errorf("failed to expire alert investigation leases: %w", err)
	}
	return ids, nil
}

// RenewAlertInvestigationLeases extends the lease of every investigating row
// owned by the agent. Called from the agent heartbeat so actively-connected
// agents keep their in-flight work; assigned rows are deliberately NOT renewed
// so a connected-but-unresponsive agent's dispatch is re-prompted by expiry.
func (s *pgAlertInvestigationStore) RenewAlertInvestigationLeases(ctx context.Context, agentID string, lease time.Duration) error {
	if lease <= 0 {
		lease = defaultInvestigationLease
	}
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("lease_until = ?", now.Add(lease)).
		Set("updated_at = ?", now).
		Where("agent_id = ?", agentID).
		Where("status = ?", AlertInvestigationStatusInvestigating).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to renew alert investigation leases: %w", err)
	}
	return nil
}

// ExpireAlertInvestigationLeasesByAgent shortens the leases of an agent's
// active rows after a disconnect: assigned rows lapse immediately (the agent
// never started), investigating rows get one reconnect grace window so a
// transient blip doesn't kill in-flight work. Paused rows are untouched — the
// agent paused them deliberately.
func (s *pgAlertInvestigationStore) ExpireAlertInvestigationLeasesByAgent(ctx context.Context, agentID string, investigatingGrace time.Duration) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	if _, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("lease_until = ?", now).
		Set("updated_at = ?", now).
		Where("agent_id = ?", agentID).
		Where("status = ?", AlertInvestigationStatusAssigned).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to expire assigned alert investigation leases by agent: %w", err)
	}
	if _, err := s.db.NewUpdate().Model((*models.AlertInvestigation)(nil)).
		Set("lease_until = ?", now.Add(investigatingGrace)).
		Set("updated_at = ?", now).
		Where("agent_id = ?", agentID).
		Where("status = ?", AlertInvestigationStatusInvestigating).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to expire investigating alert investigation leases by agent: %w", err)
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
	if err := q.Scan(ctx, &invs); err != nil {
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
