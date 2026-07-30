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

type pgAlertStore struct {
	pgStoreBase
	triageResultStore       TriageResultStore
	triageRuleStore         TriageRuleStore
	alertInvestigationStore AlertInvestigationStore
}

func newPGAlertStore(db *bun.DB) Store {
	return &pgAlertStore{
		pgStoreBase:             pgStoreBase{db: db},
		triageResultStore:       &pgTriageResultStore{pgStoreBase{db: db}},
		triageRuleStore:         &pgTriageRuleStore{pgStoreBase{db: db}},
		alertInvestigationStore: newPGAlertInvestigationStore(db),
	}
}

func (s *pgAlertStore) TriageResultStore() TriageResultStore {
	return s.triageResultStore
}

func (s *pgAlertStore) TriageRuleStore() TriageRuleStore {
	return s.triageRuleStore
}

// attachInvestigationSummaries batch-loads the current alert investigation
// summary (assigned agent + status) for the given alert records and attaches
// it to each record's Investigation field. A nil alertInvestigationStore or
// a failed lookup is treated as a soft failure: the records are returned
// without summaries and a warning is logged so list/detail requests still
// succeed even if the investigation store is temporarily unavailable.
func (s *pgAlertStore) attachInvestigationSummaries(ctx context.Context, records []AlertRecord) {
	if len(records) == 0 || s.alertInvestigationStore == nil {
		return
	}
	alertNumbers := make([]int64, 0, len(records))
	seen := make(map[int64]struct{}, len(records))
	for _, r := range records {
		if r.AlertNumber <= 0 {
			continue
		}
		if _, ok := seen[r.AlertNumber]; ok {
			continue
		}
		seen[r.AlertNumber] = struct{}{}
		alertNumbers = append(alertNumbers, r.AlertNumber)
	}
	if len(alertNumbers) == 0 {
		return
	}
	summaries, err := s.alertInvestigationStore.GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx, alertNumbers)
	if err != nil {
		logger.WarnCtx(ctx, "Failed to load alert investigation summaries; alerts will be returned without assigned-actor info",
			"component", "store", "alert_count", len(alertNumbers), "error", err)
		return
	}
	for i := range records {
		if records[i].AlertNumber <= 0 {
			continue
		}
		if summary, ok := summaries[records[i].AlertNumber]; ok {
			rec := records[i]
			s := summary
			rec.Investigation = &s
			records[i] = rec
		}
	}
}

func newAlertEventModel(alertID uuid.UUID, ev AlertEvent) *models.AlertEvent {
	return &models.AlertEvent{
		IDModel:          models.IDModel{ID: models.NewUUID()},
		AlertID:          alertID,
		Type:             ev.Type,
		Timestamp:        ev.Timestamp,
		ActorUsername:    ev.ActorUsername,
		ActorDisplayName: ev.ActorDisplayName,
		ActorUserID:      ev.ActorUserID,
		Source:           ev.Source,
	}
}

func (s *pgAlertStore) insertAlertEvent(ctx context.Context, alertID uuid.UUID, ev AlertEvent) error {
	_, err := s.db.NewInsert().Model(newAlertEventModel(alertID, ev)).Exec(ctx)
	return err
}

func (s *pgAlertStore) Create(record AlertRecord) (int64, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.StartsAt.IsZero() {
		record.StartsAt = now
	}

	m := &models.Alert{
		BaseModel: models.BaseModel{
			ID:        models.NewUUID(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Fingerprint:  record.Fingerprint,
		Status:       record.Status,
		Acknowledged: record.Acknowledged,
		Silenced:     record.Silenced,
		Labels:       record.Labels,
		Annotations:  record.Annotations,
		Values:       record.Values,
		StartsAt:     record.StartsAt,
		EndsAt:       record.EndsAt,
		GeneratorURL: record.GeneratorURL,
	}

	ev := AlertEvent{Type: "fired", Timestamp: record.StartsAt, Source: "grafana"}
	if record.InitialEvent != nil {
		ev = *record.InitialEvent
		if ev.Timestamp.IsZero() {
			ev.Timestamp = record.StartsAt
		}
		if ev.Type == "" {
			ev.Type = "fired"
		}
		if ev.Source == "" {
			ev.Source = "grafana"
		}
	}

	// Insert the alert and its initial event atomically so a partial failure
	// can't leave an alert without its "fired" event.
	if err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(m).
			ExcludeColumn("alert_number").
			Returning("alert_number").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to insert alert: %w", err)
		}
		if _, err := tx.NewInsert().Model(newAlertEventModel(m.ID, ev)).Exec(ctx); err != nil {
			return fmt.Errorf("failed to insert fired event: %w", err)
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return m.AlertNumber, nil
}

func (s *pgAlertStore) GetByFingerprint(fingerprint string) (*AlertRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	// Tombstone-inclusive lookup: WhereAllWithDeleted disables Bun's automatic
	// deleted_at IS NULL filter so callers can observe soft-deleted alerts.
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		WhereAllWithDeleted().
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*AlertRecord](err, "alert")
	}
	rec, err := s.toAlertRecord(ctx, &a)
	if err != nil {
		return nil, err
	}
	if rec != nil {
		s.attachInvestigationSummaries(ctx, []AlertRecord{*rec})
	}
	return rec, nil
}

func (s *pgAlertStore) GetOpenByFingerprint(fingerprint string) (*AlertRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*AlertRecord](err, "open alert")
	}
	rec, err := s.toAlertRecord(ctx, &a)
	if err != nil {
		return nil, err
	}
	if rec != nil {
		s.attachInvestigationSummaries(ctx, []AlertRecord{*rec})
	}
	return rec, nil
}

func (s *pgAlertStore) GetByAlertNumber(alertNumber int64) (*AlertRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("alert_number = ?", alertNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*AlertRecord](err, "alert")
	}
	rec, err := s.toAlertRecord(ctx, &a)
	if err != nil {
		return nil, err
	}
	if rec != nil {
		s.attachInvestigationSummaries(ctx, []AlertRecord{*rec})
	}
	return rec, nil
}

func (s *pgAlertStore) AcknowledgeAlertByNumber(alertNumber int64, actor *EventActor) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("alert_number = ?", alertNumber).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("acknowledged = ?", true).
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Where("acknowledged = ?", false).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}
	if rows == 0 {
		// Already acknowledged (retry or concurrent request); keep idempotent
		// and skip the duplicate event.
		return nil
	}

	ev := AlertEventWithActor("acked", now, actor)
	if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert acknowledged event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) ResolveAlertByNumber(alertNumber int64, actor *EventActor) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("alert_number = ?", alertNumber).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found or not firing: %w", ErrAlertNotFiring)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("status = ?", "resolved").
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve alert: %w", err)
	}

	ev := AlertEventWithActor("resolved", now, actor)
	if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert resolved event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) ReopenAlertByNumber(alertNumber int64, ev AlertEvent) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("alert_number = ?", alertNumber).
		Where("status = ?", "resolved").
		Where("deleted_at IS NULL").
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to query alert: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("status = ?", "firing").
		Set("acknowledged = ?", false).
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to reopen alert: %w", err)
	}

	if ev.Type == "" {
		ev.Type = "reopened"
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = now
	}
	if ev.Source == "" {
		ev.Source = "user"
	}
	if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert reopened event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) DeleteAlertByNumber(alertNumber int64) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("alert_number = ?", alertNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to query alert: %w", err)
	}

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := hardDeleteAlertCascade(ctx, tx, a.ID, a.AlertNumber); err != nil {
			return err
		}

		now := time.Now().UTC()
		_, err := tx.NewUpdate().Model((*models.Alert)(nil)).
			Set("deleted_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", a.ID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to soft-delete alert: %w", err)
		}
		return nil
	})
}

func (s *pgAlertStore) UpdateStatus(fingerprint, status string, resolvedEvent *AlertEvent) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()

	if status == "resolved" {
		ev := resolvedEvent
		if ev == nil {
			ev = &AlertEvent{Type: "resolved", Timestamp: now, Source: "grafana"}
		} else {
			cp := *ev
			if cp.Timestamp.IsZero() {
				cp.Timestamp = now
			}
			ev = &cp
		}
		if err := s.insertAlertEvent(ctx, a.ID, *ev); err != nil {
			logger.ErrorCtx(ctx, "Failed to insert resolved event for alert", "component", "store", "alert_id", a.ID, "error", err)
		}
	}

	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update alert status: %w", err)
	}
	return nil
}

func (s *pgAlertStore) UpdateStatusSilenced(fingerprint string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("status = ?", "resolved").
		Set("silenced = ?", true).
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to silence alert: %w", err)
	}

	if err := s.insertAlertEvent(ctx, a.ID, AlertEvent{Type: "resolved", Timestamp: now, Source: "system"}); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert silenced-resolved event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) UpdateDeliveryTargets(fingerprint string, targets []DeliveryTarget) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("failed to find alert: %w", err)
	}

	_, err = s.db.NewDelete().Model((*models.DeliveryTarget)(nil)).Where("alert_id = ?", a.ID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear delivery targets: %w", err)
	}

	if targets == nil {
		targets = []DeliveryTarget{}
	}
	for _, t := range targets {
		m := &models.DeliveryTarget{
			BaseModel: models.BaseModel{
				ID:        models.NewUUID(),
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			AlertID:     a.ID,
			Provider:    t.Provider,
			Channel:     t.Channel,
			ChannelName: t.ChannelName,
			PostID:      t.PostID,
		}
		_, err = s.db.NewInsert().Model(m).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to insert delivery target: %w", err)
		}
	}

	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).Set("updated_at = ?", time.Now().UTC()).Where("id = ?", a.ID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update alert: %w", err)
	}
	return nil
}

func (s *pgAlertStore) AcknowledgeAlert(fingerprint string, actor *EventActor) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	if a.Acknowledged {
		return nil
	}

	now := time.Now().UTC()
	ackEv := AlertEventWithActor("acked", now, actor)

	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("acknowledged = ?", true).
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}

	if err := s.insertAlertEvent(ctx, a.ID, ackEv); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert acked event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) ReopenAlert(fingerprint string, ev AlertEvent) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("status = ?", "resolved").
		Where("deleted_at IS NULL").
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found or not resolved: %w", ErrAlertNotResolved)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	if ev.Timestamp.IsZero() {
		ev.Timestamp = now
	}

	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("status = ?", "firing").
		Set("acknowledged = ?", false).
		Set("silenced = ?", false).
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to reopen alert: %w", err)
	}

	if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert reopen event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) ResolveAlertByUser(fingerprint string, actor *EventActor) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("status != ?", "resolved").
		Where("deleted_at IS NULL").
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found or not firing: %w", ErrAlertNotFiring)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	ev := AlertEventWithActor("resolved", now, actor)

	_, err = s.db.NewUpdate().Model((*models.Alert)(nil)).
		Set("status = ?", "resolved").
		Set("updated_at = ?", now).
		Where("id = ?", a.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve alert: %w", err)
	}

	if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert resolved event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) DeleteAlert(fingerprint string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var a models.Alert
		err := tx.NewSelect().Model(&a).
			Where("fingerprint = ?", fingerprint).
			Where("deleted_at IS NULL").
			Order("updated_at DESC").
			Limit(1).
			Scan(ctx)
		if err != nil {
			if isNotFound(err) {
				return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
			}
			return fmt.Errorf("failed to query alert: %w", err)
		}

		if err := hardDeleteAlertCascade(ctx, tx, a.ID, a.AlertNumber); err != nil {
			return err
		}

		now := time.Now().UTC()
		res, err := tx.NewUpdate().Model((*models.Alert)(nil)).
			Set("deleted_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", a.ID).
			Where("deleted_at IS NULL").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to soft-delete alert: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return nil
	})
}

// ExpungeSoftDeletedAlertsChildren hard-deletes the investigation artifacts of
// every already-tombstoned alert. It is a one-time backfill for stale children
// that predate the cascade-on-delete behavior, and is idempotent (safe to
// re-run). Not part of the Store interface; callers reach it via type assertion.
func (s *pgAlertStore) ExpungeSoftDeletedAlertsChildren(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var rows []models.Alert
	err := s.db.NewSelect().Model(&rows).WhereDeleted().Scan(ctx)
	if err != nil {
		return 0, fmt.Errorf("query soft-deleted alerts: %w", err)
	}
	processed := 0
	for i := range rows {
		a := &rows[i]
		err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			return hardDeleteAlertCascade(ctx, tx, a.ID, a.AlertNumber)
		})
		if err != nil {
			return processed, fmt.Errorf("expunge alert %d: %w", a.AlertNumber, err)
		}
		processed++
	}
	return processed, nil
}

func (s *pgAlertStore) QueryAlerts(filter map[string]any) ([]AlertRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var alerts []models.Alert
	q := s.db.NewSelect().Model(&alerts).Where("deleted_at IS NULL")

	sortField, sortDesc := parseSortFromFilter(filter, "updated_at", true)
	if sortDesc {
		q = q.Order(sortField + " DESC")
	} else {
		q = q.Order(sortField + " ASC")
	}

	if status, ok := filter["status"].(string); ok {
		q = q.Where("status = ?", status)
	}
	if ack, ok := filter["acknowledged"].(bool); ok {
		q = q.Where("acknowledged = ?", ack)
	}
	if alertNum, ok := filter["alert_number"]; ok {
		switch v := alertNum.(type) {
		case int64:
			q = q.Where("alert_number = ?", v)
		case float64:
			q = q.Where("alert_number = ?", int64(v))
		}
	}
	if sev, ok := filter["severity"].(string); ok {
		q = q.Where("labels->>'severity' = ?", sev)
	}
	if search, ok := filter["search"].(string); ok {
		pattern := "%" + search + "%"
		q = q.Where("(labels->>'alertname' ILIKE ? OR annotations->>'description' ILIKE ? OR annotations->>'summary' ILIKE ? OR labels->>'namespace' ILIKE ? OR fingerprint = ?)",
			pattern, pattern, pattern, pattern, search)
	}
	if channel, ok := filter["channel"].(string); ok {
		q = q.Where("labels->>'channel' = ?", channel)
	}
	if provider, ok := filter["provider"].(string); ok {
		q = q.Where("labels->>'provider' = ?", provider)
	}
	if startDate, ok := filter["start_date"].(time.Time); ok {
		q = q.Where("created_at >= ?", startDate)
	}
	if endDate, ok := filter["end_date"].(time.Time); ok {
		q = q.Where("created_at <= ?", endDate)
	}

	limit, skip := extractLimitSkip(filter, 20)
	if limit > 0 {
		q = q.Limit(limit)
	}
	if skip > 0 {
		q = q.Offset(skip)
	}

	err := q.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}

	_, summaryOnly := filter["summary"]
	records := make([]AlertRecord, 0, len(alerts))
	for i := range alerts {
		if summaryOnly {
			records = append(records, s.toAlertRecordSummary(&alerts[i]))
		} else {
			rec, err := s.toAlertRecord(ctx, &alerts[i])
			if err != nil {
				return nil, err
			}
			records = append(records, *rec)
		}
	}
	s.attachInvestigationSummaries(ctx, records)
	return records, nil
}

func (s *pgAlertStore) DeleteOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	// Physically purge old resolved alerts. Bun's soft_delete filter restricts
	// this SELECT to live rows (deleted_at IS NULL), so existing tombstones are
	// left for a separate purge job. Each row is removed inside a tx:
	// hardDeleteAlertCascade clears investigation artifacts, then ForceDelete
	// removes the alert (FK cascades events, delivery_targets, incident_alerts).
	var rows []models.Alert
	err := s.db.NewSelect().Model(&rows).
		Column("id", "alert_number").
		Where("created_at < ?", olderThan).
		Where("status = ?", "resolved").
		Scan(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to query old alerts: %w", err)
	}

	var purged int64
	for i := range rows {
		a := &rows[i]
		if err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if err := hardDeleteAlertCascade(ctx, tx, a.ID, a.AlertNumber); err != nil {
				return err
			}
			if _, err := tx.NewDelete().Model((*models.Alert)(nil)).
				Where("id = ?", a.ID).
				ForceDelete().
				Exec(ctx); err != nil {
				return fmt.Errorf("purge alert row: %w", err)
			}
			return nil
		}); err != nil {
			return purged, fmt.Errorf("purge old alert %d: %w", a.AlertNumber, err)
		}
		purged++
	}
	return purged, nil
}

func (s *pgAlertStore) CountOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	n, err := s.db.NewSelect().Model((*models.Alert)(nil)).
		Where("created_at < ?", olderThan).
		Where("status = ?", "resolved").
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count old alerts: %w", err)
	}
	return int64(n), nil
}

func (s *pgAlertStore) Close() {}

func (s *pgAlertStore) ListUninvestigatedAlerts(ctx context.Context, threshold time.Duration) ([]AlertRecord, error) {
	cutoff := time.Now().UTC().Add(-threshold)

	// Build the terminal statuses for investigations and incidents
	invTerminal := InvestigationTerminalStatuses
	incTerminal := IncidentTerminalStatuses

	var alerts []models.Alert
	err := s.db.NewSelect().Model(&alerts).
		Where("status = ?", "firing").
		Where("created_at <= ?", cutoff).
		Where("deleted_at IS NULL").
		// Not investigated: no non-terminal investigation linked via fingerprint
		Where("NOT EXISTS (SELECT 1 FROM alert_investigation_alerts aia JOIN alert_investigations ai ON aia.investigation_id = ai.id WHERE aia.fingerprint = alert.fingerprint AND ai.status NOT IN (?))", bun.List(invTerminal)).
		// Not handled by active incident
		Where("NOT EXISTS (SELECT 1 FROM incident_alerts ia JOIN incidents inc ON ia.incident_id = inc.id WHERE ia.alert_id = alert.id AND inc.deleted_at IS NULL AND inc.status NOT IN (?))", bun.List(incTerminal)).
		Order("created_at DESC").
		Limit(500).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query uninvestigated alerts: %w", err)
	}

	records := make([]AlertRecord, 0, len(alerts))
	for i := range alerts {
		rec, err := s.toAlertRecord(ctx, &alerts[i])
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgAlertStore) toAlertRecordSummary(a *models.Alert) AlertRecord {
	rec := AlertRecord{
		Fingerprint:  a.Fingerprint,
		Status:       a.Status,
		Acknowledged: a.Acknowledged,
		Silenced:     a.Silenced,
		Labels:       a.Labels,
		Annotations:  a.Annotations,
		Values:       a.Values,
		StartsAt:     a.StartsAt,
		GeneratorURL: a.GeneratorURL,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
		AlertNumber:  a.AlertNumber,
		DeletedAt:    a.DeletedAt,
	}
	if a.EndsAt != nil {
		rec.EndsAt = a.EndsAt
	}
	return rec
}

func (s *pgAlertStore) toAlertRecord(ctx context.Context, a *models.Alert) (*AlertRecord, error) {
	// Load events
	var events []models.AlertEvent
	err := s.db.NewSelect().Model(&events).
		Where("alert_id = ?", a.ID).
		Order("timestamp ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	// Load delivery targets
	var targets []models.DeliveryTarget
	err = s.db.NewSelect().Model(&targets).
		Where("alert_id = ?", a.ID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query targets: %w", err)
	}

	var alertEvents []AlertEvent
	for i := range events {
		e := &events[i]
		alertEvents = append(alertEvents, AlertEvent{
			Type:             e.Type,
			Timestamp:        e.Timestamp,
			ActorUsername:    e.ActorUsername,
			ActorDisplayName: e.ActorDisplayName,
			ActorUserID:      e.ActorUserID,
			Source:           e.Source,
		})
	}

	var deliveryTargets []DeliveryTarget
	for i := range targets {
		t := &targets[i]
		deliveryTargets = append(deliveryTargets, DeliveryTarget{
			Provider:    t.Provider,
			Channel:     t.Channel,
			ChannelName: t.ChannelName,
			PostID:      t.PostID,
		})
	}

	rec := &AlertRecord{
		Fingerprint:     a.Fingerprint,
		Status:          a.Status,
		Acknowledged:    a.Acknowledged,
		Silenced:        a.Silenced,
		Labels:          a.Labels,
		Annotations:     a.Annotations,
		Values:          a.Values,
		StartsAt:        a.StartsAt,
		GeneratorURL:    a.GeneratorURL,
		Events:          alertEvents,
		DeliveryTargets: deliveryTargets,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
		AlertNumber:     a.AlertNumber,
		DeletedAt:       a.DeletedAt,
	}
	if a.EndsAt != nil {
		rec.EndsAt = a.EndsAt
	}
	return rec, nil
}

func (s *pgAlertStore) LinkAlertToIncident(ctx context.Context, fingerprint string, incidentNumber int64) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("deleted_at IS NULL").
		Order("alert_number DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	var inc models.Incident
	err = s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", incidentNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to find incident: %w", err)
	}

	// Insert into the M2M join table incident_alerts
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO incident_alerts (incident_id, alert_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
		inc.ID, a.ID)
	if err != nil {
		return fmt.Errorf("failed to link alert to incident: %w", err)
	}
	return nil
}

func (s *pgAlertStore) UnlinkAlertFromIncident(ctx context.Context, fingerprint string, incidentNumber int64) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("fingerprint = ?", fingerprint).
		Where("deleted_at IS NULL").
		Order("alert_number DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	var inc models.Incident
	err = s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", incidentNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to find incident: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		"DELETE FROM incident_alerts WHERE incident_id = ? AND alert_id = ?",
		inc.ID, a.ID)
	if err != nil {
		return fmt.Errorf("failed to unlink alert from incident: %w", err)
	}
	return nil
}

func (s *pgAlertStore) GetAlertsByIncident(ctx context.Context, incidentNumber int64) ([]string, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", incidentNumber).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[[]string](err, "incident")
	}

	var alerts []models.Alert
	// Include soft-deleted alerts so linked tombstones stay visible to the
	// incident's Linked Alerts card; live-only consumers filter on DeletedAt.
	err = s.db.NewSelect().Model(&alerts).
		Column("fingerprint").
		Join("JOIN incident_alerts ia ON ia.alert_id = alert.id").
		Where("ia.incident_id = ?", inc.ID).
		WhereAllWithDeleted().
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get alerts by incident: %w", err)
	}

	fingerprints := make([]string, 0, len(alerts))
	for i := range alerts {
		fingerprints = append(fingerprints, alerts[i].Fingerprint)
	}
	return fingerprints, nil
}

// ResolveAlertsByIncident flips every firing alert linked to the incident to
// "resolved" and records a "resolved" AlertEvent per alert (source:
// "incident_cascade"). Already-resolved linked alerts are reported as Skipped
// and left untouched. Per-alert update failures are collected in Failed rather
// than aborting the batch. Mirrors ResolveAlertByNumber's per-alert side
// effects.
func (s *pgAlertStore) ResolveAlertsByIncident(ctx context.Context, incidentNumber int64, actor *EventActor) (AlertCascadeResult, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var linked []models.Alert
	err := s.db.NewSelect().Model(&linked).
		Join("JOIN incident_alerts ia ON ia.alert_id = alert.id").
		Join("JOIN incidents inc ON inc.id = ia.incident_id").
		Where("inc.incident_number = ?", incidentNumber).
		Where("inc.deleted_at IS NULL").
		Where("alert.deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return AlertCascadeResult{}, fmt.Errorf("failed to list alerts for incident cascade: %w", err)
	}

	// Look up the incident commander for the incident (preferring active, otherwise the latest assignee)
	var icActor *EventActor
	var ic models.ICSRoleAssignment
	icErr := s.db.NewSelect().Model(&ic).
		Join("JOIN incidents inc ON inc.id = ics_role_assignment.incident_id").
		Where("inc.incident_number = ?", incidentNumber).
		Where("inc.deleted_at IS NULL").
		Where("ics_role_assignment.role_type = ?", "incident_commander").
		Order("ics_role_assignment.status DESC, ics_role_assignment.started_at DESC").
		Limit(1).
		Scan(ctx)
	if icErr == nil {
		icActor = &EventActor{
			Source: "incident_cascade",
		}
		if ic.AssigneeType == "user" && ic.UserID != nil {
			var u models.User
			if uErr := s.db.NewSelect().Model(&u).Where("id = ?", *ic.UserID).Scan(ctx); uErr == nil {
				icActor.UserID = u.ID.String()
				icActor.Username = u.Email
				icActor.DisplayName = u.FullName
			}
		} else if ic.AssigneeType == "agent" && ic.AgentTokenID != nil {
			var at models.AgentToken
			if atErr := s.db.NewSelect().Model(&at).Where("id = ?", *ic.AgentTokenID).Scan(ctx); atErr == nil {
				icActor.UserID = at.ID.String()
				icActor.Username = at.Name
				icActor.DisplayName = at.Name
			}
		}
	}

	now := time.Now().UTC()
	result := AlertCascadeResult{
		Resolved: make([]AlertRecord, 0),
		Skipped:  make([]AlertRef, 0),
		Failed:   make([]AlertRef, 0),
	}
	for i := range linked {
		a := &linked[i]
		ref := AlertRef{AlertNumber: a.AlertNumber, Fingerprint: a.Fingerprint}
		if a.Status == "resolved" {
			result.Skipped = append(result.Skipped, ref)
			continue
		}
		_, updErr := s.db.NewUpdate().Model((*models.Alert)(nil)).
			Set("status = ?", "resolved").
			Set("updated_at = ?", now).
			Where("id = ?", a.ID).
			Exec(ctx)
		if updErr != nil {
			logger.ErrorCtx(ctx, "Failed to resolve alert during incident cascade",
				"component", "store", "incident_number", incidentNumber,
				"alert_number", ref.AlertNumber, "fingerprint", ref.Fingerprint, "error", updErr)
			result.Failed = append(result.Failed, ref)
			continue
		}

		ev := AlertEvent{Type: "resolved", Timestamp: now, Source: "incident_cascade"}
		effectiveActor := actor
		if icActor != nil {
			effectiveActor = icActor
		}
		if effectiveActor != nil {
			ev.ActorUserID = effectiveActor.UserID
			ev.ActorUsername = effectiveActor.Username
			ev.ActorDisplayName = effectiveActor.DisplayName
		}
		if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
			logger.ErrorCtx(ctx, "Failed to insert resolved event for cascaded alert",
				"component", "store", "alert_id", a.ID, "error", err)
		}
		// Reflect the successful flip in the in-memory model so the broadcast
		// record mirrors the just-persisted state.
		a.Status = "resolved"
		a.UpdatedAt = now
		rec, convErr := s.toAlertRecord(ctx, a)
		if convErr != nil {
			logger.ErrorCtx(ctx, "Failed to convert cascaded alert for broadcast",
				"component", "store", "alert_id", a.ID, "error", convErr)
			result.Failed = append(result.Failed, ref)
			continue
		}
		result.Resolved = append(result.Resolved, *rec)
	}
	return result, nil
}

func (s *pgAlertStore) GetIncidentsByAlertNumber(ctx context.Context, alertNumber int64) ([]IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var a models.Alert
	err := s.db.NewSelect().Model(&a).
		Where("alert_number = ?", alertNumber).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[[]IncidentRecord](err, "alert")
	}

	var incs []models.Incident
	err = s.db.NewSelect().Model(&incs).
		Join("JOIN incident_alerts ia ON ia.incident_id = incident.id").
		Where("ia.alert_id = ?", a.ID).
		Order("incident.created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get incidents by alert: %w", err)
	}

	incidentStore := &pgIncidentStore{pgStoreBase: s.pgStoreBase}
	records := make([]IncidentRecord, 0, len(incs))
	for i := range incs {
		records = append(records, *incidentStore.toIncidentRecord(&incs[i]))
	}
	return records, nil
}
