// alert_investigation_lookup.go contains alert investigation thread and
// similarity lookups: MatterMost/Slack thread resolution and similar
// investigation retrieval for episodic memory.
package store

import (
	"context"
	"fmt"

	"alga/ent"
	alertent "alga/ent/alert"
	"alga/ent/alertinvestigation"
	"alga/ent/alertinvestigationalert"
	"alga/ent/alertinvestigationevent"
	"alga/ent/alertinvestigationupdateentry"
	"alga/logger"
)

func (s *pgAlertInvestigationStore) GetAlertInvestigationByMMThread(ctx context.Context, mmThreadID string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	return s.getAlertInvestigationBy(ctx, alertinvestigation.MmThreadID(mmThreadID))
}

func (s *pgAlertInvestigationStore) GetAlertInvestigationBySlackThread(ctx context.Context, channelID string, threadTS string) (*AlertInvestigationRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	return s.getAlertInvestigationBy(ctx,
		alertinvestigation.SlackChannelID(channelID),
		alertinvestigation.SlackThreadTs(threadTS),
	)
}

func (s *pgAlertInvestigationStore) FindSimilarAlertInvestigations(ctx context.Context, q SimilarAlertInvestigationsQuery) ([]AlertInvestigationRecord, error) {
	if q.CorrelationKey == "" && q.AlertName == "" {
		return []AlertInvestigationRecord{}, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}

	query := s.client.AlertInvestigation.Query().
		Where(alertinvestigation.StatusIn(AlertInvestigationStatusComplete, AlertInvestigationStatusTimedOut)).
		Limit(limit)

	if q.CorrelationKey != "" {
		query = query.Where(alertinvestigation.CorrelationKey(q.CorrelationKey))
	}
	if q.ExcludeInvestigationID != "" {
		query = query.Where(alertinvestigation.AlertInvestigationIDNEQ(q.ExcludeInvestigationID))
	}

	invs, err := query.
		WithAlerts().
		WithUpdates(func(q *ent.AlertInvestigationUpdateEntryQuery) {
			q.Order(ent.Asc(alertinvestigationupdateentry.FieldCreatedAt))
		}).
		WithEvents(func(q *ent.AlertInvestigationEventQuery) { q.Order(ent.Asc(alertinvestigationevent.FieldCreatedAt)) }).
		Order(ent.Desc(alertinvestigation.FieldCompletedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("find similar alert investigations: %w", err)
	}

	// Exclude investigations whose primary linked alert has been soft-deleted,
	// so deleted context never enters the agent's shared-knowledge / episodic
	// memory. Snapshot rows leave AlertID unset (see createAlertInvestigationAlert),
	// so we resolve the live alert by fingerprint. Hard-deleted alerts leave no
	// row at all; in that case no DeletedAtIsNil row exists and the filter is a
	// no-op (the investigation is kept — which is correct, since there's no
	// soft-delete signal to act on).
	filtered := invs[:0]
	for _, inv := range invs {
		primaryAlert, err := inv.QueryAlerts().Order(ent.Asc(alertinvestigationalert.FieldID)).First(ctx)
		if err != nil || primaryAlert == nil {
			logger.WarnCtx(ctx, "episodic filter: failed to load primary alert edge",
				"component", "store",
				"investigation_id", inv.AlertInvestigationID,
				"error", err)
			filtered = append(filtered, inv)
			continue
		}
		hasLive, lerr := s.client.Alert.Query().
			Where(alertent.Fingerprint(primaryAlert.Fingerprint), alertent.DeletedAtIsNil()).
			Exist(ctx)
		if lerr != nil {
			logger.WarnCtx(ctx, "episodic filter: failed to check live alert existence",
				"component", "store",
				"investigation_id", inv.AlertInvestigationID,
				"fingerprint", primaryAlert.Fingerprint,
				"error", lerr)
		}
		if hasLive {
			filtered = append(filtered, inv)
		}
	}
	invs = filtered

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
