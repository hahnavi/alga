package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

const (
	TriageDecisionInvestigate = "investigate"
	TriageDecisionAutoResolve = "auto_resolve"
	TriageDecisionSuppress    = "suppress"
	TriageDecisionEscalate    = "escalate"
	TriageDecisionEnrichOnly  = "enrich_only"

	TriageResultOutcomePending    = "pending"
	TriageResultOutcomeConfirmed  = "confirmed"
	TriageResultOutcomeOverridden = "overridden"
)

type TriageResultRecord struct {
	ID                 uuid.UUID         `json:"id"`
	TriageNumber       int64             `json:"triage_number"`
	CorrelationKey     string            `json:"correlation_key"`
	AlertCount         int               `json:"alert_count"`
	AlertFingerprints  []string          `json:"alert_fingerprints"`
	AlertLabels        map[string]string `json:"alert_labels"`
	SeverityInput      string            `json:"severity_input"`
	Decision           string            `json:"decision"`
	Confidence         float64           `json:"confidence"`
	SeverityClassified string            `json:"severity_classified"`
	Category           string            `json:"category"`
	Reasoning          string            `json:"reasoning"`
	SuggestedActions   []string          `json:"suggested_actions"`
	Enrichment         map[string]any    `json:"enrichment"`
	ContextUsed        map[string]any    `json:"context_used"`
	Outcome            string            `json:"outcome"`
	OverriddenTo       string            `json:"overridden_to"`
	OverriddenBy       *uuid.UUID        `json:"overridden_by,omitempty"`
	OverriddenAt       *time.Time        `json:"overridden_at,omitempty"`
	ModelUsed          string            `json:"model_used"`
	TriageDurationMs   int64             `json:"triage_duration_ms"`
	TraceID            string            `json:"trace_id"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type TriageResultQuery struct {
	Decision  string
	Outcome   string
	Category  string
	Severity  string
	Search    string
	StartDate time.Time
	EndDate   time.Time
	Limit     int
	Skip      int
}

type TriageVolumeDay struct {
	Date   time.Time        `json:"date"`
	Counts map[string]int64 `json:"counts"`
}

type TriageResultStore interface {
	Create(ctx context.Context, record *TriageResultRecord) (*TriageResultRecord, error)
	Update(ctx context.Context, id string, patch *TriageResultRecord) (*TriageResultRecord, error)
	Get(ctx context.Context, id string) (*TriageResultRecord, error)
	List(ctx context.Context, q TriageResultQuery) ([]TriageResultRecord, int64, error)
	GetByCorrelationKey(ctx context.Context, key string, limit int) ([]TriageResultRecord, error)
	CountByOutcome(ctx context.Context) (confirmed, overridden, pending int64, err error)
	CountByDecision(ctx context.Context) (map[string]int64, error)
	CountByCategory(ctx context.Context) (map[string]int64, error)
	AvgConfidence(ctx context.Context) (float64, error)
	AvgDurationMs(ctx context.Context) (float64, error)
	VolumeTrend(ctx context.Context, days int) ([]TriageVolumeDay, error)
}

type pgTriageResultStore struct {
	pgStoreBase
}

func newPGTriageResultStore(db *bun.DB) TriageResultStore {
	return &pgTriageResultStore{pgStoreBase{db: db}}
}

func pgTriageResultToRecord(e *models.TriageResult) *TriageResultRecord {
	var fps []string
	if e.AlertFingerprints != nil {
		fps = e.AlertFingerprints
	} else {
		fps = []string{}
	}
	var labels map[string]string
	if e.AlertLabels != nil {
		labels = e.AlertLabels
	} else {
		labels = map[string]string{}
	}
	var actions []string
	if e.SuggestedActions != nil {
		actions = e.SuggestedActions
	} else {
		actions = []string{}
	}
	var enrichment map[string]any
	if e.Enrichment != nil {
		enrichment = e.Enrichment
	} else {
		enrichment = map[string]any{}
	}
	var contextUsed map[string]any
	if e.ContextUsed != nil {
		contextUsed = e.ContextUsed
	} else {
		contextUsed = map[string]any{}
	}
	severityInput := ""
	if e.SeverityInput != nil {
		severityInput = *e.SeverityInput
	}
	severityClassified := ""
	if e.SeverityClassified != nil {
		severityClassified = *e.SeverityClassified
	}
	category := ""
	if e.Category != nil {
		category = *e.Category
	}
	overriddenTo := ""
	if e.OverriddenTo != nil {
		overriddenTo = *e.OverriddenTo
	}
	return &TriageResultRecord{
		ID:                 e.ID,
		TriageNumber:       e.TriageNumber,
		CorrelationKey:     e.CorrelationKey,
		AlertCount:         e.AlertCount,
		AlertFingerprints:  fps,
		AlertLabels:        labels,
		SeverityInput:      severityInput,
		Decision:           e.Decision,
		Confidence:         e.Confidence,
		SeverityClassified: severityClassified,
		Category:           category,
		Reasoning:          e.Reasoning,
		SuggestedActions:   actions,
		Enrichment:         enrichment,
		ContextUsed:        contextUsed,
		Outcome:            e.Outcome,
		OverriddenTo:       overriddenTo,
		OverriddenBy:       e.OverriddenBy,
		OverriddenAt:       e.OverriddenAt,
		ModelUsed:          e.ModelUsed,
		TriageDurationMs:   e.TriageDurationMs,
		TraceID:            e.TraceID,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
	}
}

func (s *pgTriageResultStore) Create(ctx context.Context, record *TriageResultRecord) (*TriageResultRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.CorrelationKey == "" {
		return nil, errors.New("correlation_key is required")
	}
	if record.Decision == "" {
		return nil, errors.New("decision is required")
	}
	if record.AlertFingerprints == nil {
		record.AlertFingerprints = []string{}
	}
	if record.AlertLabels == nil {
		record.AlertLabels = map[string]string{}
	}
	if record.SuggestedActions == nil {
		record.SuggestedActions = []string{}
	}
	if record.Enrichment == nil {
		record.Enrichment = map[string]any{}
	}
	if record.ContextUsed == nil {
		record.ContextUsed = map[string]any{}
	}
	if record.Outcome == "" {
		record.Outcome = TriageResultOutcomePending
	}

	n, err := s.nextTriageNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("allocate triage number: %w", err)
	}
	record.TriageNumber = n

	var severityInput *string
	if record.SeverityInput != "" {
		severityInput = &record.SeverityInput
	}
	var severityClassified *string
	if record.SeverityClassified != "" {
		severityClassified = &record.SeverityClassified
	}
	var category *string
	if record.Category != "" {
		category = &record.Category
	}
	var overriddenTo *string
	if record.OverriddenTo != "" {
		overriddenTo = &record.OverriddenTo
	}

	m := &models.TriageResult{
		ID:                 models.NewUUID(),
		TriageNumber:       record.TriageNumber,
		CorrelationKey:     record.CorrelationKey,
		AlertCount:         record.AlertCount,
		AlertFingerprints:  record.AlertFingerprints,
		AlertLabels:        record.AlertLabels,
		SeverityInput:      severityInput,
		Decision:           record.Decision,
		Confidence:         record.Confidence,
		SeverityClassified: severityClassified,
		Category:           category,
		Reasoning:          record.Reasoning,
		SuggestedActions:   record.SuggestedActions,
		Enrichment:         record.Enrichment,
		ContextUsed:        record.ContextUsed,
		Outcome:            record.Outcome,
		OverriddenTo:       overriddenTo,
		OverriddenBy:       record.OverriddenBy,
		OverriddenAt:       record.OverriddenAt,
		ModelUsed:          record.ModelUsed,
		TriageDurationMs:   record.TriageDurationMs,
		TraceID:            record.TraceID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert triage result: %w", err)
	}
	record.ID = m.ID
	return record, nil
}

func (s *pgTriageResultStore) Update(ctx context.Context, id string, patch *TriageResultRecord) (*TriageResultRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}

	q := s.db.NewUpdate().Model((*models.TriageResult)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", uid)

	if patch.Decision != "" {
		q = q.Set("decision = ?", patch.Decision)
	}
	if patch.Confidence != 0 {
		q = q.Set("confidence = ?", patch.Confidence)
	}
	if patch.SeverityClassified != "" {
		q = q.Set("severity_classified = ?", patch.SeverityClassified)
	}
	if patch.Category != "" {
		q = q.Set("category = ?", patch.Category)
	}
	if patch.Reasoning != "" {
		q = q.Set("reasoning = ?", patch.Reasoning)
	}
	if patch.Outcome != "" {
		q = q.Set("outcome = ?", patch.Outcome)
	}
	if patch.OverriddenTo != "" {
		q = q.Set("overridden_to = ?", patch.OverriddenTo)
	}
	if patch.OverriddenBy != nil {
		q = q.Set("overridden_by = ?", *patch.OverriddenBy)
	}
	if patch.OverriddenAt != nil {
		q = q.Set("overridden_at = ?", *patch.OverriddenAt)
	}
	if patch.ModelUsed != "" {
		q = q.Set("model_used = ?", patch.ModelUsed)
	}
	if patch.TriageDurationMs != 0 {
		q = q.Set("triage_duration_ms = ?", patch.TriageDurationMs)
	}
	if patch.TraceID != "" {
		q = q.Set("trace_id = ?", patch.TraceID)
	}
	if patch.SuggestedActions != nil {
		q = q.Set("suggested_actions = ?", patch.SuggestedActions)
	}
	if patch.Enrichment != nil {
		q = q.Set("enrichment = ?", patch.Enrichment)
	}
	if patch.ContextUsed != nil {
		q = q.Set("context_used = ?", patch.ContextUsed)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update triage result: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, errors.New("triage result not found")
	}

	// Re-fetch to return the updated record
	var updated models.TriageResult
	if err := s.db.NewSelect().Model(&updated).Where("id = ?", uid).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to re-fetch triage result: %w", err)
	}
	return pgTriageResultToRecord(&updated), nil
}

func (s *pgTriageResultStore) Get(ctx context.Context, id string) (*TriageResultRecord, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}
	var tr models.TriageResult
	err = s.db.NewSelect().Model(&tr).Where("id = ?", uid).Scan(ctx)
	if err != nil {
		return handleQueryErr[*TriageResultRecord](err, "triage result")
	}
	return pgTriageResultToRecord(&tr), nil
}

func (s *pgTriageResultStore) List(ctx context.Context, q TriageResultQuery) ([]TriageResultRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.TriageResult)(nil))

	if decision := strings.TrimSpace(q.Decision); decision != "" {
		countQ = countQ.Where("decision = ?", decision)
	}
	if outcome := strings.TrimSpace(q.Outcome); outcome != "" {
		countQ = countQ.Where("outcome = ?", outcome)
	}
	if category := strings.TrimSpace(q.Category); category != "" {
		countQ = countQ.Where("category = ?", category)
	}
	if severity := strings.TrimSpace(q.Severity); severity != "" {
		countQ = countQ.Where("severity_input = ?", severity)
	}
	if !q.StartDate.IsZero() {
		countQ = countQ.Where("created_at >= ?", q.StartDate)
	}
	if !q.EndDate.IsZero() {
		countQ = countQ.Where("created_at <= ?", q.EndDate)
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count triage results: %w", err)
	}

	var items []models.TriageResult
	listQ := s.db.NewSelect().Model(&items).Order("created_at DESC")
	if decision := strings.TrimSpace(q.Decision); decision != "" {
		listQ = listQ.Where("decision = ?", decision)
	}
	if outcome := strings.TrimSpace(q.Outcome); outcome != "" {
		listQ = listQ.Where("outcome = ?", outcome)
	}
	if category := strings.TrimSpace(q.Category); category != "" {
		listQ = listQ.Where("category = ?", category)
	}
	if severity := strings.TrimSpace(q.Severity); severity != "" {
		listQ = listQ.Where("severity_input = ?", severity)
	}
	if !q.StartDate.IsZero() {
		listQ = listQ.Where("created_at >= ?", q.StartDate)
	}
	if !q.EndDate.IsZero() {
		listQ = listQ.Where("created_at <= ?", q.EndDate)
	}
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	err = listQ.Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list triage results: %w", err)
	}

	out := make([]TriageResultRecord, 0, len(items))
	for i := range items {
		tr := &items[i]
		if q.Search != "" {
			text := strings.TrimSpace(strings.ToLower(q.Search))
			if !strings.Contains(strings.ToLower(tr.CorrelationKey), text) &&
				!strings.Contains(strings.ToLower(tr.Decision), text) &&
				!strings.Contains(strings.ToLower(tr.Reasoning), text) &&
				!strings.Contains(strings.ToLower(tr.TraceID), text) {
				continue
			}
		}
		out = append(out, *pgTriageResultToRecord(tr))
	}
	if out == nil {
		out = []TriageResultRecord{}
	}
	return out, int64(total), nil
}

func (s *pgTriageResultStore) GetByCorrelationKey(ctx context.Context, key string, limit int) ([]TriageResultRecord, error) {
	var items []models.TriageResult
	q := s.db.NewSelect().Model(&items).
		Where("correlation_key = ?", key).
		Order("created_at DESC")

	if limit > 0 {
		q = q.Limit(limit)
	}

	err := q.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get triage results by correlation key: %w", err)
	}

	out := make([]TriageResultRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgTriageResultToRecord(&items[i]))
	}
	if out == nil {
		out = []TriageResultRecord{}
	}
	return out, nil
}

func (s *pgTriageResultStore) CountByOutcome(ctx context.Context) (confirmed, overridden, pending int64, err error) {
	c, err := s.db.NewSelect().Model((*models.TriageResult)(nil)).Where("outcome = ?", "confirmed").Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count confirmed: %w", err)
	}
	confirmed = int64(c)
	c, err = s.db.NewSelect().Model((*models.TriageResult)(nil)).Where("outcome = ?", "overridden").Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count overridden: %w", err)
	}
	overridden = int64(c)
	c, err = s.db.NewSelect().Model((*models.TriageResult)(nil)).Where("outcome = ?", "pending").Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count pending: %w", err)
	}
	pending = int64(c)
	return confirmed, overridden, pending, nil
}

func (s *pgTriageResultStore) CountByDecision(ctx context.Context) (map[string]int64, error) {
	var items []models.TriageResult
	err := s.db.NewSelect().Model(&items).Column("decision").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("query triage results for decision counts: %w", err)
	}
	m := make(map[string]int64)
	for i := range items {
		m[items[i].Decision]++
	}
	return m, nil
}

func (s *pgTriageResultStore) CountByCategory(ctx context.Context) (map[string]int64, error) {
	var items []models.TriageResult
	err := s.db.NewSelect().Model(&items).Column("category").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("query triage results for category counts: %w", err)
	}
	m := make(map[string]int64)
	for i := range items {
		if items[i].Category != nil && *items[i].Category != "" {
			m[*items[i].Category]++
		}
	}
	return m, nil
}

func (s *pgTriageResultStore) AvgConfidence(ctx context.Context) (float64, error) {
	var items []models.TriageResult
	err := s.db.NewSelect().Model(&items).Column("confidence").Scan(ctx)
	if err != nil {
		return 0, fmt.Errorf("query triage results for avg confidence: %w", err)
	}
	if len(items) == 0 {
		return 0, nil
	}
	var sum float64
	for i := range items {
		sum += items[i].Confidence
	}
	return sum / float64(len(items)), nil
}

func (s *pgTriageResultStore) AvgDurationMs(ctx context.Context) (float64, error) {
	var items []models.TriageResult
	err := s.db.NewSelect().Model(&items).Column("triage_duration_ms").Scan(ctx)
	if err != nil {
		return 0, fmt.Errorf("query triage results for avg duration: %w", err)
	}
	if len(items) == 0 {
		return 0, nil
	}
	var sum int64
	for i := range items {
		sum += items[i].TriageDurationMs
	}
	return float64(sum) / float64(len(items)), nil
}

func (s *pgTriageResultStore) VolumeTrend(ctx context.Context, days int) ([]TriageVolumeDay, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	var items []models.TriageResult
	err := s.db.NewSelect().Model(&items).
		Column("created_at", "decision").
		Where("created_at >= ?", cutoff).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("query triage results for volume trend: %w", err)
	}

	dayMap := make(map[string]map[string]int64)
	for i := range items {
		day := items[i].CreatedAt.UTC().Truncate(24 * time.Hour)
		key := day.Format(time.RFC3339)
		if _, ok := dayMap[key]; !ok {
			dayMap[key] = make(map[string]int64)
		}
		dayMap[key][items[i].Decision]++
	}

	var out []TriageVolumeDay
	for i := range days {
		day := cutoff.UTC().Truncate(24*time.Hour).AddDate(0, 0, i)
		key := day.Format(time.RFC3339)
		counts := dayMap[key]
		if counts == nil {
			counts = map[string]int64{}
		}
		out = append(out, TriageVolumeDay{Date: day, Counts: counts})
	}
	return out, nil
}

func (s *pgTriageResultStore) nextTriageNumber(ctx context.Context) (int64, error) {
	return nextPgCounter(ctx, s.db, "triage_results")
}
