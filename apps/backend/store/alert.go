package store

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/alert"
	"alga/ent/alertevent"
	"alga/ent/alertinvestigationalert"
	"alga/ent/counter"
	"alga/ent/deliverytarget"
	enticsrole "alga/ent/icsroleassignment"
	entincident "alga/ent/incident"
	"alga/ent/predicate"
	"alga/logger"
)

type pgAlertStore struct {
	pgStoreBase
	triageResultStore       TriageResultStore
	triageRuleStore         TriageRuleStore
	alertInvestigationStore AlertInvestigationStore
}

func newPGAlertStore(client *ent.Client) Store {
	return &pgAlertStore{
		pgStoreBase:             pgStoreBase{client: client},
		triageResultStore:       &pgTriageResultStore{pgStoreBase{client: client}},
		triageRuleStore:         &pgTriageRuleStore{pgStoreBase{client: client}},
		alertInvestigationStore: newPGAlertInvestigationStore(client),
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

func (s *pgAlertStore) nextAlertNumber(ctx context.Context) (int64, error) {
	return nextPgCounter(ctx, s.client, "alerts")
}

func (s *pgAlertStore) insertAlertEvent(ctx context.Context, alertID uuid.UUID, ev AlertEvent) error {
	_, err := s.client.AlertEvent.Create().
		SetAlertID(alertID).
		SetType(ev.Type).
		SetTimestamp(ev.Timestamp).
		SetActorUsername(ev.ActorUsername).
		SetActorDisplayName(ev.ActorDisplayName).
		SetActorUserID(ev.ActorUserID).
		SetSource(ev.Source).
		Save(ctx)
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

	n, err := s.nextAlertNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate alert number: %w", err)
	}
	record.AlertNumber = n

	b := s.client.Alert.Create().
		SetFingerprint(record.Fingerprint).
		SetStatus(record.Status).
		SetAcknowledged(record.Acknowledged).
		SetSilenced(record.Silenced).
		SetLabels(record.Labels).
		SetAnnotations(record.Annotations).
		SetValues(record.Values).
		SetStartsAt(record.StartsAt).
		SetGeneratorURL(record.GeneratorURL).
		SetAlertNumber(n).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if record.EndsAt != nil {
		b.SetEndsAt(*record.EndsAt)
	}

	a, err := b.Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to insert alert: %w", err)
	}
	record.ID = a.ID

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

	if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
		return 0, fmt.Errorf("failed to insert fired event: %w", err)
	}

	return n, nil
}

func (s *pgAlertStore) GetByFingerprint(fingerprint string) (*AlertRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	a, err := s.client.Alert.Query().
		Where(alert.Fingerprint(fingerprint)).
		Order(ent.Desc(alert.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		return handleQueryErr[*AlertRecord](err, "alert")
	}
	rec, err := s.toAlertRecord(ctx, a)
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

	a, err := s.client.Alert.Query().
		Where(alert.Fingerprint(fingerprint), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		return handleQueryErr[*AlertRecord](err, "open alert")
	}
	rec, err := s.toAlertRecord(ctx, a)
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

	a, err := s.client.Alert.Query().
		Where(alert.AlertNumber(alertNumber)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*AlertRecord](err, "alert")
	}
	rec, err := s.toAlertRecord(ctx, a)
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

	a, err := s.client.Alert.Query().
		Where(alert.AlertNumber(alertNumber), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.client.Alert.UpdateOneID(a.ID).
		SetAcknowledged(true).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}

	ev := AlertEventWithActor("acknowledged", now, actor)
	if err := s.insertAlertEvent(ctx, a.ID, ev); err != nil {
		logger.ErrorCtx(ctx, "Failed to insert acknowledged event for alert", "component", "store", "alert_id", a.ID, "error", err)
	}
	return nil
}

func (s *pgAlertStore) ResolveAlertByNumber(alertNumber int64, actor *EventActor) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	a, err := s.client.Alert.Query().
		Where(alert.AlertNumber(alertNumber), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found or not firing: %w", ErrAlertNotFiring)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.client.Alert.UpdateOneID(a.ID).
		SetStatus("resolved").
		SetUpdatedAt(now).
		Save(ctx)
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

	a, err := s.client.Alert.Query().
		Where(alert.AlertNumber(alertNumber), alert.StatusEQ("resolved"), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to query alert: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.client.Alert.UpdateOneID(a.ID).
		SetStatus("firing").
		SetAcknowledged(false).
		SetUpdatedAt(now).
		Save(ctx)
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

	a, err := s.client.Alert.Query().
		Where(alert.AlertNumber(alertNumber), alert.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to query alert: %w", err)
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := hardDeleteAlertCascade(ctx, tx, a); err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := tx.Alert.UpdateOneID(a.ID).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to soft-delete alert: %w", err)
	}

	return tx.Commit()
}

func (s *pgAlertStore) UpdateStatus(fingerprint, status string, resolvedEvent *AlertEvent) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	a, err := s.client.Alert.Query().Where(alert.Fingerprint(fingerprint), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	upd := s.client.Alert.UpdateOneID(a.ID).SetStatus(status).SetUpdatedAt(now)

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

	_, err = upd.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update alert status: %w", err)
	}
	return nil
}

func (s *pgAlertStore) UpdateStatusSilenced(fingerprint string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	a, err := s.client.Alert.Query().Where(alert.Fingerprint(fingerprint), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.client.Alert.UpdateOneID(a.ID).
		SetStatus("resolved").
		SetSilenced(true).
		SetUpdatedAt(now).
		Save(ctx)
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

	a, err := s.client.Alert.Query().Where(alert.Fingerprint(fingerprint), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find alert: %w", err)
	}

	_, err = s.client.DeliveryTarget.Delete().Where(deliverytarget.HasAlertWith(alert.ID(a.ID))).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear delivery targets: %w", err)
	}

	if targets == nil {
		targets = []DeliveryTarget{}
	}
	for _, t := range targets {
		_, err = s.client.DeliveryTarget.Create().
			SetAlertID(a.ID).
			SetProvider(t.Provider).
			SetChannel(t.Channel).
			SetChannelName(t.ChannelName).
			SetPostID(t.PostID).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to insert delivery target: %w", err)
		}
	}

	_, err = s.client.Alert.UpdateOneID(a.ID).SetUpdatedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update alert: %w", err)
	}
	return nil
}

func (s *pgAlertStore) AcknowledgeAlert(fingerprint string, actor *EventActor) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	a, err := s.client.Alert.Query().Where(alert.Fingerprint(fingerprint), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	if a.Acknowledged {
		return nil
	}

	now := time.Now().UTC()
	ackEv := AlertEventWithActor("acked", now, actor)

	_, err = s.client.Alert.UpdateOneID(a.ID).
		SetAcknowledged(true).
		SetUpdatedAt(now).
		Save(ctx)
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

	a, err := s.client.Alert.Query().
		Where(alert.Fingerprint(fingerprint), alert.Status("resolved"), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found or not resolved: %w", ErrAlertNotResolved)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	if ev.Timestamp.IsZero() {
		ev.Timestamp = now
	}

	_, err = s.client.Alert.UpdateOneID(a.ID).
		SetStatus("firing").
		SetAcknowledged(false).
		SetSilenced(false).
		SetUpdatedAt(now).
		Save(ctx)
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

	a, err := s.client.Alert.Query().
		Where(alert.Fingerprint(fingerprint), alert.StatusNEQ("resolved"), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found or not firing: %w", ErrAlertNotFiring)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	now := time.Now().UTC()
	ev := AlertEventWithActor("resolved", now, actor)

	_, err = s.client.Alert.UpdateOneID(a.ID).
		SetStatus("resolved").
		SetUpdatedAt(now).
		Save(ctx)
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

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer rollbackTx(tx)

	a, err := tx.Alert.Query().
		Where(alert.Fingerprint(fingerprint), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to query alert: %w", err)
	}

	if err := hardDeleteAlertCascade(ctx, tx, a); err != nil {
		return err
	}

	now := time.Now().UTC()
	n, err := tx.Alert.Update().
		Where(alert.ID(a.ID), alert.DeletedAtIsNil()).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to soft-delete alert: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
	}

	return tx.Commit()
}

// ExpungeSoftDeletedAlertsChildren hard-deletes the investigation artifacts of
// every already-tombstoned alert. It is a one-time backfill for stale children
// that predate the cascade-on-delete behavior, and is idempotent (safe to
// re-run). Not part of the Store interface; callers reach it via type assertion.
func (s *pgAlertStore) ExpungeSoftDeletedAlertsChildren(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := s.client.Alert.Query().Where(alert.DeletedAtNotNil()).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query soft-deleted alerts: %w", err)
	}
	processed := 0
	for _, a := range rows {
		err := func() error {
			tx, err := s.client.Tx(ctx)
			if err != nil {
				return err
			}
			defer rollbackTx(tx)
			if err := hardDeleteAlertCascade(ctx, tx, a); err != nil {
				return err
			}
			return tx.Commit()
		}()
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

	query := s.client.Alert.Query().Where(alert.DeletedAtIsNil())
	sortField, sortDesc := parseSortFromFilter(filter, alert.FieldUpdatedAt, true)
	if sortDesc {
		query = query.Order(ent.Desc(sortField))
	} else {
		query = query.Order(ent.Asc(sortField))
	}

	if status, ok := filter["status"].(string); ok {
		query = query.Where(alert.Status(status))
	}
	if ack, ok := filter["acknowledged"].(bool); ok {
		query = query.Where(alert.Acknowledged(ack))
	}
	if alertNum, ok := filter["alert_number"]; ok {
		switch v := alertNum.(type) {
		case int64:
			query = query.Where(alert.AlertNumber(v))
		case float64:
			query = query.Where(alert.AlertNumber(int64(v)))
		}
	}
	if sev, ok := filter["severity"].(string); ok {
		pred := predicate.Alert(func(sel *sql.Selector) {
			lbl := sel.C(alert.FieldLabels)
			sel.Where(sqljson.ValueContains(lbl, sev, sqljson.Path("severity")))
		})
		query = query.Where(pred)
	}
	if search, ok := filter["search"].(string); ok {
		pattern := "%" + search + "%"
		pred := predicate.Alert(func(sel *sql.Selector) {
			lblCol := sel.C(alert.FieldLabels)
			annCol := sel.C(alert.FieldAnnotations)
			fpPred := sql.EQ(sel.C(alert.FieldFingerprint), search)
			jsonILike := func(col, key string) *sql.Predicate {
				return sql.P(func(b *sql.Builder) {
					b.WriteString("jsonb_extract_path_text(").Ident(col).WriteString(", ")
					b.Arg(key)
					b.WriteString(") ILIKE ")
					b.Arg(pattern)
				})
			}
			sel.Where(sql.Or(
				jsonILike(lblCol, "alertname"),
				jsonILike(annCol, "description"),
				jsonILike(annCol, "summary"),
				jsonILike(lblCol, "namespace"),
				fpPred,
			))
		})
		query = query.Where(pred)
	}
	if channel, ok := filter["channel"].(string); ok {
		pred := predicate.Alert(func(sel *sql.Selector) {
			lbl := sel.C(alert.FieldLabels)
			sel.Where(sqljson.ValueContains(lbl, channel, sqljson.Path("channel")))
		})
		query = query.Where(pred)
	}
	if provider, ok := filter["provider"].(string); ok {
		pred := predicate.Alert(func(sel *sql.Selector) {
			lbl := sel.C(alert.FieldLabels)
			sel.Where(sqljson.ValueContains(lbl, provider, sqljson.Path("provider")))
		})
		query = query.Where(pred)
	}
	if startDate, ok := filter["start_date"].(time.Time); ok {
		query = query.Where(alert.CreatedAtGTE(startDate))
	}
	if endDate, ok := filter["end_date"].(time.Time); ok {
		query = query.Where(alert.CreatedAtLTE(endDate))
	}

	limit, skip := extractLimitSkip(filter, 20)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if skip > 0 {
		query = query.Offset(skip)
	}

	_, summaryOnly := filter["summary"]
	if !summaryOnly {
		query = query.
			WithEvents(func(q *ent.AlertEventQuery) { q.Order(ent.Asc(alertevent.FieldTimestamp)) }).
			WithDeliveryTargets()
	}

	alerts, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}

	records := make([]AlertRecord, 0, len(alerts))
	for _, a := range alerts {
		if summaryOnly {
			records = append(records, s.toAlertRecordSummary(a))
		} else {
			rec, err := s.toAlertRecord(ctx, a)
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
	n, err := s.client.Alert.Delete().
		Where(alert.CreatedAtLT(olderThan), alert.StatusEQ("resolved"), alert.DeletedAtIsNil()).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old alerts: %w", err)
	}
	return int64(n), nil
}

func (s *pgAlertStore) CountOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	n, err := s.client.Alert.Query().
		Where(alert.CreatedAtLT(olderThan), alert.StatusEQ("resolved"), alert.DeletedAtIsNil()).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count old alerts: %w", err)
	}
	return int64(n), nil
}

func (s *pgAlertStore) Close() {}

func (s *pgAlertStore) ListUninvestigatedAlerts(ctx context.Context, threshold time.Duration) ([]AlertRecord, error) {
	cutoff := time.Now().UTC().Add(-threshold)

	notInvestigated := predicate.Alert(func(sel *sql.Selector) {
		aia := sql.Table(alertinvestigationalert.Table)
		inv := sql.Table(alertinvestigationalert.AlertInvestigationInverseTable)
		terminalAny := make([]any, len(InvestigationTerminalStatuses))
		for i, ts := range InvestigationTerminalStatuses {
			terminalAny[i] = ts
		}
		sub := sql.Select(aia.C(alertinvestigationalert.FieldFingerprint)).
			From(aia).
			Join(inv).
			On(aia.C(alertinvestigationalert.AlertInvestigationColumn), inv.C("id")).
			Where(
				sql.And(
					sql.ColumnsEQ(sel.C(alert.FieldFingerprint), aia.C(alertinvestigationalert.FieldFingerprint)),
					sql.NotIn(inv.C("status"), terminalAny...),
				),
			)
		sel.Where(sql.NotExists(sub))
	})

	notHandledByActiveIncident := predicate.Alert(func(sel *sql.Selector) {
		ia := sql.Table(alert.IncidentsTable)
		inc := sql.Table(alert.IncidentsInverseTable)
		terminalAny := make([]any, len(IncidentTerminalStatuses))
		for i, ts := range IncidentTerminalStatuses {
			terminalAny[i] = ts
		}
		sub := sql.Select(ia.C("alert_id")).
			From(ia).
			Join(inc).
			On(ia.C("incident_id"), inc.C("id")).
			Where(
				sql.And(
					sql.ColumnsEQ(sel.C(alert.FieldID), ia.C("alert_id")),
					sql.NotIn(inc.C("status"), terminalAny...),
				),
			)
		sel.Where(sql.NotExists(sub))
	})

	alerts, err := s.client.Alert.Query().
		Where(
			alert.Status("firing"),
			alert.CreatedAtLTE(cutoff),
			alert.DeletedAtIsNil(),
			notInvestigated,
			notHandledByActiveIncident,
		).
		Order(ent.Desc(alert.FieldCreatedAt)).
		Limit(500).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query uninvestigated alerts: %w", err)
	}

	records := make([]AlertRecord, 0, len(alerts))
	for _, a := range alerts {
		rec, err := s.toAlertRecord(ctx, a)
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *pgAlertStore) toAlertRecordSummary(a *ent.Alert) AlertRecord {
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

func (s *pgAlertStore) toAlertRecord(ctx context.Context, a *ent.Alert) (*AlertRecord, error) {
	events := a.Edges.Events
	if events == nil {
		var err error
		events, err = a.QueryEvents().Order(ent.Asc(alertevent.FieldTimestamp)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query events: %w", err)
		}
	}
	targets := a.Edges.DeliveryTargets
	if targets == nil {
		var err error
		targets, err = a.QueryDeliveryTargets().All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query targets: %w", err)
		}
	}

	var alertEvents []AlertEvent
	for _, e := range events {
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
	for _, t := range targets {
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

	a, err := s.client.Alert.Query().
		Where(alert.Fingerprint(fingerprint), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldAlertNumber)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to find incident: %w", err)
	}

	_, err = s.client.Alert.UpdateOneID(a.ID).
		AddIncidentIDs(inc.ID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to link alert to incident: %w", err)
	}
	return nil
}

func (s *pgAlertStore) UnlinkAlertFromIncident(ctx context.Context, fingerprint string, incidentNumber int64) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	a, err := s.client.Alert.Query().
		Where(alert.Fingerprint(fingerprint), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldAlertNumber)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("alert not found: %w", ErrAlertNotFound)
		}
		return fmt.Errorf("failed to find alert: %w", err)
	}

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to find incident: %w", err)
	}

	_, err = s.client.Alert.UpdateOneID(a.ID).
		RemoveIncidentIDs(inc.ID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to unlink alert from incident: %w", err)
	}
	return nil
}

func (s *pgAlertStore) GetAlertsByIncident(ctx context.Context, incidentNumber int64) ([]string, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[[]string](err, "incident")
	}

	alerts, err := s.client.Alert.Query().
		Where(alert.HasIncidentsWith(entincident.IDEQ(inc.ID))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get alerts by incident: %w", err)
	}

	fingerprints := make([]string, 0, len(alerts))
	for _, a := range alerts {
		fingerprints = append(fingerprints, a.Fingerprint)
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

	linked, err := s.client.Alert.Query().
		Where(alert.HasIncidentsWith(entincident.IncidentNumber(incidentNumber)), alert.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return AlertCascadeResult{}, fmt.Errorf("failed to list alerts for incident cascade: %w", err)
	}

	// Look up the incident commander for the incident (preferring active, otherwise the latest assignee)
	var icActor *EventActor
	if ic, err := s.client.ICSRoleAssignment.Query().
		Where(
			enticsrole.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
			enticsrole.RoleTypeEQ("incident_commander"),
		).
		Order(ent.Desc(enticsrole.FieldStatus), ent.Desc(enticsrole.FieldStartedAt)).
		WithUser().
		WithAgentToken().
		First(ctx); err == nil && ic != nil {
		icActor = &EventActor{
			Source: "incident_cascade",
		}
		if ic.AssigneeType == "user" && ic.Edges.User != nil {
			icActor.UserID = ic.Edges.User.ID.String()
			icActor.Username = ic.Edges.User.Email
			icActor.DisplayName = ic.Edges.User.FullName
		} else if ic.AssigneeType == "agent" && ic.Edges.AgentToken != nil {
			icActor.UserID = ic.Edges.AgentToken.ID.String()
			icActor.Username = ic.Edges.AgentToken.Name
			icActor.DisplayName = ic.Edges.AgentToken.Name
		}
	}

	now := time.Now().UTC()
	result := AlertCascadeResult{
		Resolved: make([]AlertRecord, 0),
		Skipped:  make([]AlertRef, 0),
		Failed:   make([]AlertRef, 0),
	}
	for _, a := range linked {
		ref := AlertRef{AlertNumber: a.AlertNumber, Fingerprint: a.Fingerprint}
		if a.Status == "resolved" {
			result.Skipped = append(result.Skipped, ref)
			continue
		}
		if _, err := s.client.Alert.UpdateOneID(a.ID).
			SetStatus("resolved").
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			logger.ErrorCtx(ctx, "Failed to resolve alert during incident cascade",
				"component", "store", "incident_number", incidentNumber,
				"alert_number", ref.AlertNumber, "fingerprint", ref.Fingerprint, "error", err)
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
		// Reflect the successful flip in the in-memory entity so the broadcast
		// record mirrors the just-persisted state, then convert via the shared
		// helper. The full record lets callers emit alert_updated (the event
		// type every alert page subscribes to) instead of an unconsumed
		// alert_resolved event.
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

	a, err := s.client.Alert.Query().
		Where(alert.AlertNumber(alertNumber)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[[]IncidentRecord](err, "alert")
	}

	incs, err := a.QueryIncidents().
		Order(ent.Desc(entincident.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get incidents by alert: %w", err)
	}

	records := make([]IncidentRecord, 0, len(incs))
	incidentStore := &pgIncidentStore{pgStoreBase: s.pgStoreBase}
	for _, inc := range incs {
		records = append(records, *incidentStore.toIncidentRecord(inc))
	}
	return records, nil
}

func nextPgCounter(ctx context.Context, client *ent.Client, name string) (int64, error) {
	const maxCounterRetries = 10
	for range maxCounterRetries {
		c, err := client.Counter.Get(ctx, name)
		if err != nil && !ent.IsNotFound(err) {
			return 0, fmt.Errorf("counter get: %w", err)
		}
		if ent.IsNotFound(err) {
			created, err := client.Counter.Create().SetID(name).SetSeq(1).Save(ctx)
			if err != nil {
				if pgIsDuplicateKey(err) {
					continue
				}
				return 0, fmt.Errorf("counter create: %w", err)
			}
			return created.Seq, nil
		}
		updated, err := client.Counter.UpdateOneID(name).
			AddSeq(1).
			Where(counter.SeqEQ(c.Seq)).
			Save(ctx)
		if err == nil {
			return updated.Seq, nil
		}
		if ent.IsNotFound(err) {
			continue
		}
		return 0, fmt.Errorf("counter increment: %w", err)
	}
	return 0, fmt.Errorf("counter increment: CAS failed after %d retries", maxCounterRetries)
}
