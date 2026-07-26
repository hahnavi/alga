package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/triageresult"
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

func newPGTriageResultStore(client *ent.Client) TriageResultStore {
	return &pgTriageResultStore{pgStoreBase{client: client}}
}

func pgTriageResultToRecord(e *ent.TriageResult) *TriageResultRecord {
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
	var overriddenBy *uuid.UUID
	if e.OverriddenBy != uuid.Nil {
		overriddenBy = &e.OverriddenBy
	}
	severityInput := ""
	if e.SeverityInput != nil {
		severityInput = string(*e.SeverityInput)
	}
	severityClassified := ""
	if e.SeverityClassified != nil {
		severityClassified = string(*e.SeverityClassified)
	}
	category := ""
	if e.Category != nil {
		category = string(*e.Category)
	}
	overriddenTo := ""
	if e.OverriddenTo != nil {
		overriddenTo = string(*e.OverriddenTo)
	}
	return &TriageResultRecord{
		ID:                 e.ID,
		TriageNumber:       e.TriageNumber,
		CorrelationKey:     e.CorrelationKey,
		AlertCount:         e.AlertCount,
		AlertFingerprints:  fps,
		AlertLabels:        labels,
		SeverityInput:      severityInput,
		Decision:           string(e.Decision),
		Confidence:         e.Confidence,
		SeverityClassified: severityClassified,
		Category:           category,
		Reasoning:          e.Reasoning,
		SuggestedActions:   actions,
		Enrichment:         enrichment,
		ContextUsed:        contextUsed,
		Outcome:            string(e.Outcome),
		OverriddenTo:       overriddenTo,
		OverriddenBy:       overriddenBy,
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

	b := s.client.TriageResult.Create().
		SetTriageNumber(record.TriageNumber).
		SetCorrelationKey(record.CorrelationKey).
		SetAlertCount(record.AlertCount).
		SetAlertFingerprints(record.AlertFingerprints).
		SetAlertLabels(record.AlertLabels).
		SetSeverityInput(triageresult.SeverityInput(record.SeverityInput)).
		SetDecision(triageresult.Decision(record.Decision)).
		SetConfidence(record.Confidence).
		SetSeverityClassified(triageresult.SeverityClassified(record.SeverityClassified)).
		SetCategory(triageresult.Category(record.Category)).
		SetReasoning(record.Reasoning).
		SetSuggestedActions(record.SuggestedActions).
		SetEnrichment(record.Enrichment).
		SetContextUsed(record.ContextUsed).
		SetOutcome(triageresult.Outcome(record.Outcome)).
		SetOverriddenTo(triageresult.OverriddenTo(record.OverriddenTo)).
		SetModelUsed(record.ModelUsed).
		SetTriageDurationMs(record.TriageDurationMs).
		SetTraceID(record.TraceID).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if record.OverriddenBy != nil {
		b.SetOverriddenBy(*record.OverriddenBy)
	}
	if record.OverriddenAt != nil {
		b.SetNillableOverriddenAt(record.OverriddenAt)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert triage result: %w", err)
	}
	record.ID = saved.ID
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

	b := s.client.TriageResult.UpdateOneID(uid).SetUpdatedAt(time.Now().UTC())

	if patch.Decision != "" {
		b.SetDecision(triageresult.Decision(patch.Decision))
	}
	if patch.Confidence != 0 {
		b.SetConfidence(patch.Confidence)
	}
	if patch.SeverityClassified != "" {
		b.SetSeverityClassified(triageresult.SeverityClassified(patch.SeverityClassified))
	}
	if patch.Category != "" {
		b.SetCategory(triageresult.Category(patch.Category))
	}
	if patch.Reasoning != "" {
		b.SetReasoning(patch.Reasoning)
	}
	if patch.Outcome != "" {
		b.SetOutcome(triageresult.Outcome(patch.Outcome))
	}
	if patch.OverriddenTo != "" {
		b.SetOverriddenTo(triageresult.OverriddenTo(patch.OverriddenTo))
	}
	if patch.OverriddenBy != nil {
		b.SetNillableOverriddenBy(patch.OverriddenBy)
	}
	if patch.OverriddenAt != nil {
		b.SetNillableOverriddenAt(patch.OverriddenAt)
	}
	if patch.ModelUsed != "" {
		b.SetModelUsed(patch.ModelUsed)
	}
	if patch.TriageDurationMs != 0 {
		b.SetTriageDurationMs(patch.TriageDurationMs)
	}
	if patch.TraceID != "" {
		b.SetTraceID(patch.TraceID)
	}
	if patch.SuggestedActions != nil {
		b.SetSuggestedActions(patch.SuggestedActions)
	}
	if patch.Enrichment != nil {
		b.SetEnrichment(patch.Enrichment)
	}
	if patch.ContextUsed != nil {
		b.SetContextUsed(patch.ContextUsed)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.New("triage result not found")
		}
		return nil, fmt.Errorf("failed to update triage result: %w", err)
	}
	return pgTriageResultToRecord(saved), nil
}

func (s *pgTriageResultStore) Get(ctx context.Context, id string) (*TriageResultRecord, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}
	tr, err := s.client.TriageResult.Get(ctx, uid)
	if err != nil {
		return handleQueryErr[*TriageResultRecord](err, "triage result")
	}
	return pgTriageResultToRecord(tr), nil
}

func (s *pgTriageResultStore) List(ctx context.Context, q TriageResultQuery) ([]TriageResultRecord, int64, error) {
	query := s.client.TriageResult.Query()

	if decision := strings.TrimSpace(q.Decision); decision != "" {
		query = query.Where(triageresult.DecisionEQ(triageresult.Decision(decision)))
	}
	if outcome := strings.TrimSpace(q.Outcome); outcome != "" {
		query = query.Where(triageresult.OutcomeEQ(triageresult.Outcome(outcome)))
	}
	if category := strings.TrimSpace(q.Category); category != "" {
		query = query.Where(triageresult.CategoryEQ(triageresult.Category(category)))
	}
	if severity := strings.TrimSpace(q.Severity); severity != "" {
		query = query.Where(triageresult.SeverityInputEQ(triageresult.SeverityInput(severity)))
	}
	if !q.StartDate.IsZero() {
		query = query.Where(triageresult.CreatedAtGTE(q.StartDate))
	}
	if !q.EndDate.IsZero() {
		query = query.Where(triageresult.CreatedAtLTE(q.EndDate))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count triage results: %w", err)
	}

	query = query.Order(ent.Desc(triageresult.FieldCreatedAt))

	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}

	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list triage results: %w", err)
	}

	out := make([]TriageResultRecord, 0, len(items))
	for _, tr := range items {
		if q.Search != "" {
			text := strings.TrimSpace(strings.ToLower(q.Search))
			if !strings.Contains(strings.ToLower(tr.CorrelationKey), text) &&
				!strings.Contains(strings.ToLower(string(tr.Decision)), text) &&
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
	query := s.client.TriageResult.Query().
		Where(triageresult.CorrelationKey(key)).
		Order(ent.Desc(triageresult.FieldCreatedAt))

	if limit > 0 {
		query = query.Limit(limit)
	}

	items, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get triage results by correlation key: %w", err)
	}

	out := make([]TriageResultRecord, 0, len(items))
	for _, tr := range items {
		out = append(out, *pgTriageResultToRecord(tr))
	}
	if out == nil {
		out = []TriageResultRecord{}
	}
	return out, nil
}

func (s *pgTriageResultStore) CountByOutcome(ctx context.Context) (confirmed, overridden, pending int64, err error) {
	c, err := s.client.TriageResult.Query().Where(triageresult.OutcomeEQ(triageresult.OutcomeConfirmed)).Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count confirmed: %w", err)
	}
	confirmed = int64(c)
	c, err = s.client.TriageResult.Query().Where(triageresult.OutcomeEQ(triageresult.OutcomeOverridden)).Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count overridden: %w", err)
	}
	overridden = int64(c)
	c, err = s.client.TriageResult.Query().Where(triageresult.OutcomeEQ(triageresult.OutcomePending)).Count(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count pending: %w", err)
	}
	pending = int64(c)
	return confirmed, overridden, pending, nil
}

func (s *pgTriageResultStore) CountByDecision(ctx context.Context) (map[string]int64, error) {
	items, err := s.client.TriageResult.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query triage results for decision counts: %w", err)
	}
	m := make(map[string]int64)
	for _, tr := range items {
		m[string(tr.Decision)]++
	}
	return m, nil
}

func (s *pgTriageResultStore) CountByCategory(ctx context.Context) (map[string]int64, error) {
	items, err := s.client.TriageResult.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query triage results for category counts: %w", err)
	}
	m := make(map[string]int64)
	for _, tr := range items {
		if tr.Category != nil && *tr.Category != "" {
			m[string(*tr.Category)]++
		}
	}
	return m, nil
}

func (s *pgTriageResultStore) AvgConfidence(ctx context.Context) (float64, error) {
	items, err := s.client.TriageResult.Query().All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query triage results for avg confidence: %w", err)
	}
	if len(items) == 0 {
		return 0, nil
	}
	var sum float64
	for _, tr := range items {
		sum += tr.Confidence
	}
	return sum / float64(len(items)), nil
}

func (s *pgTriageResultStore) AvgDurationMs(ctx context.Context) (float64, error) {
	items, err := s.client.TriageResult.Query().All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query triage results for avg duration: %w", err)
	}
	if len(items) == 0 {
		return 0, nil
	}
	var sum int64
	for _, tr := range items {
		sum += tr.TriageDurationMs
	}
	return float64(sum) / float64(len(items)), nil
}

func (s *pgTriageResultStore) VolumeTrend(ctx context.Context, days int) ([]TriageVolumeDay, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	items, err := s.client.TriageResult.Query().
		Where(triageresult.CreatedAtGTE(cutoff)).
		Order(ent.Asc(triageresult.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query triage results for volume trend: %w", err)
	}

	dayMap := make(map[string]map[string]int64)
	for _, tr := range items {
		day := tr.CreatedAt.UTC().Truncate(24 * time.Hour)
		key := day.Format(time.RFC3339)
		if _, ok := dayMap[key]; !ok {
			dayMap[key] = make(map[string]int64)
		}
		dayMap[key][string(tr.Decision)]++
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
	return nextPgCounter(ctx, s.client, "triage_results")
}
