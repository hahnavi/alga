// alert_investigation_lookup.go contains alert investigation thread and
// similarity lookups: MatterMost/Slack thread resolution and similar
// investigation retrieval for episodic memory.
package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"alga/db/models"
	"alga/logger"
)

func (s *pgAlertInvestigationStore) GetAlertInvestigationByMMThread(ctx context.Context, mmThreadID string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.AlertInvestigation
	err := s.db.NewSelect().Model(&inv).Where("mm_thread_id = ?", mmThreadID).Scan(ctx)
	if err != nil {
		return handleQueryErr[*AlertInvestigationRecord](err, "alert investigation")
	}
	return s.toAlertInvestigationRecord(ctx, &inv)
}

func (s *pgAlertInvestigationStore) GetAlertInvestigationBySlackThread(ctx context.Context, channelID string, threadTS string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inv models.AlertInvestigation
	err := s.db.NewSelect().Model(&inv).
		Where("slack_channel_id = ?", channelID).
		Where("slack_thread_ts = ?", threadTS).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*AlertInvestigationRecord](err, "alert investigation")
	}
	return s.toAlertInvestigationRecord(ctx, &inv)
}

func (s *pgAlertInvestigationStore) FindSimilarAlertInvestigations(ctx context.Context, q SimilarAlertInvestigationsQuery) ([]AlertInvestigationRecord, error) {
	if q.CorrelationKey == "" && q.AlertName == "" {
		return []AlertInvestigationRecord{}, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}

	query := s.db.NewSelect().Model((*models.AlertInvestigation)(nil)).
		Where("status IN (?)", bun.List([]string{AlertInvestigationStatusComplete, AlertInvestigationStatusTimedOut})).
		Order("completed_at DESC").
		Limit(limit)

	if q.CorrelationKey != "" {
		query = query.Where("correlation_key = ?", q.CorrelationKey)
	}
	if q.ExcludeInvestigationID != "" {
		query = query.Where("alert_investigation_id != ?", q.ExcludeInvestigationID)
	}

	var invs []models.AlertInvestigation
	if err := query.Scan(ctx); err != nil {
		return nil, fmt.Errorf("find similar alert investigations: %w", err)
	}

	// Exclude investigations whose primary linked alert has been soft-deleted,
	// so deleted context never enters the agent's shared-knowledge / episodic
	// memory. Snapshot rows leave AlertID unset (see createAlertInvestigationAlert),
	// so we resolve the live alert by fingerprint. Hard-deleted alerts leave no
	// row at all; in that case no deleted_at IS NULL row exists and the filter is a
	// no-op (the investigation is kept — which is correct, since there's no
	// soft-delete signal to act on).
	filtered := invs[:0]
	for i := range invs {
		inv := &invs[i]
		var primaryAlert models.AlertInvestigationAlert
		err := s.db.NewSelect().Model(&primaryAlert).
			Where("alert_investigation_id = ?", inv.ID).
			Order("id ASC").
			Limit(1).
			Scan(ctx)
		if err != nil {
			logger.WarnCtx(ctx, "episodic filter: failed to load primary alert edge",
				"component", "store",
				"investigation_id", inv.AlertInvestigationID,
				"error", err)
			filtered = append(filtered, *inv)
			continue
		}
		hasLive, lerr := s.db.NewSelect().Model((*models.Alert)(nil)).
			Where("fingerprint = ?", primaryAlert.Fingerprint).
			Where("deleted_at IS NULL").
			Exists(ctx)
		if lerr != nil {
			logger.WarnCtx(ctx, "episodic filter: failed to check live alert existence",
				"component", "store",
				"investigation_id", inv.AlertInvestigationID,
				"fingerprint", primaryAlert.Fingerprint,
				"error", lerr)
		}
		if hasLive {
			filtered = append(filtered, *inv)
		}
	}
	invs = filtered

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
