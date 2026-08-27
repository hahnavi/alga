package triage

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/config"
	"alga/rabbitmq"
	"alga/store"
)

// autoPromoteStore serves a scripted AutoPromoteCandidate answer and records
// Create calls; every other method panics via the nil embedded interface.
type autoPromoteStore struct {
	store.TriageResultStore
	decision string
	count    int64
	created  []*store.TriageResultRecord
}

func (s *autoPromoteStore) AutoPromoteCandidate(_ context.Context, _ string) (string, int64, error) {
	return s.decision, s.count, nil
}

func (s *autoPromoteStore) Create(_ context.Context, record *store.TriageResultRecord) (*store.TriageResultRecord, error) {
	record.ID = uuid.New()
	s.created = append(s.created, record)
	return record, nil
}

// stubEnabledRulesStore returns no enabled rules so the engine reaches the
// auto-promote / LLM stage.
type stubEnabledRulesStore struct {
	store.TriageRuleStore
}

func (s *stubEnabledRulesStore) ListEnabled(_ context.Context) ([]store.TriageRuleRecord, error) {
	return nil, nil
}

type countingLLM struct{ calls int }

func (l *countingLLM) Generate(_ context.Context, _, _ string) (string, error) {
	l.calls++
	return `{"decision":"investigate","confidence":0.7,"severity":"high","category":"application","reasoning":"llm said so","suggested_actions":[],"enrichment":{}}`, nil
}

func autoPromoteEngine(t *testing.T, trs *autoPromoteStore, llm *countingLLM, promoteCount int) *Engine {
	t.Helper()
	cfg := &config.Config{
		TriageAutoPromoteConfirmedCount: promoteCount,
		TriageLLMURL:                    "http://llm.test",
		TriageLLMModel:                  "test-model",
	}
	return NewEngine(cfg, NewRuleEvaluator(&stubEnabledRulesStore{}), llm, nil, trs)
}

func autoPromoteMsg() rabbitmq.TriageMessage {
	return rabbitmq.TriageMessage{
		CorrelationKey: "autopromote-key",
		Alerts: []rabbitmq.CorrelatedAlert{{
			Fingerprint: "fp-1",
			Labels:      map[string]string{"alertname": "HighCPU"},
		}},
	}
}

// TestAutoPromoteSkipsLLM pins the WP-A15 behavior: once the confirmed
// key+decision count reaches TRIAGE_AUTO_PROMOTE_CONFIRMED_COUNT, triage
// completes without an LLM call and records the promotion in reasoning.
func TestAutoPromoteSkipsLLM(t *testing.T) {
	trs := &autoPromoteStore{decision: store.TriageDecisionInvestigate, count: 3}
	llm := &countingLLM{}
	e := autoPromoteEngine(t, trs, llm, 3)

	res, err := e.Process(context.Background(), autoPromoteMsg())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if llm.calls != 0 {
		t.Errorf("LLM calls = %d, want 0 (auto-promote must skip the LLM)", llm.calls)
	}
	if res.Response.Decision != store.TriageDecisionInvestigate {
		t.Errorf("decision = %q, want the promoted decision", res.Response.Decision)
	}
	if res.Record.Confidence != 1.0 {
		t.Errorf("confidence = %f, want 1.0", res.Record.Confidence)
	}
	if !strings.Contains(res.Record.Reasoning, "auto-promoted") {
		t.Errorf("reasoning = %q, want the auto-promotion record", res.Record.Reasoning)
	}
	if len(trs.created) != 1 {
		t.Fatalf("created records = %d, want 1", len(trs.created))
	}
	if trs.created[0].Outcome != store.TriageResultOutcomePending {
		t.Errorf("outcome = %q, want %q (persisted like a normal result)", trs.created[0].Outcome, store.TriageResultOutcomePending)
	}
}

// TestAutoPromoteBelowThresholdCallsLLM verifies the LLM still runs while the
// confirmation count is under the threshold.
func TestAutoPromoteBelowThresholdCallsLLM(t *testing.T) {
	trs := &autoPromoteStore{decision: store.TriageDecisionInvestigate, count: 2}
	llm := &countingLLM{}
	e := autoPromoteEngine(t, trs, llm, 3)

	res, err := e.Process(context.Background(), autoPromoteMsg())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if llm.calls != 1 {
		t.Errorf("LLM calls = %d, want 1 below the threshold", llm.calls)
	}
	if res.Record.Reasoning != "llm said so" {
		t.Errorf("reasoning = %q, want the LLM reasoning", res.Record.Reasoning)
	}
}

// TestAutoPromoteDisabledWhenCountZero verifies TRIAGE_AUTO_PROMOTE_CONFIRMED_COUNT=0
// disables the skip even with many confirmations.
func TestAutoPromoteDisabledWhenCountZero(t *testing.T) {
	trs := &autoPromoteStore{decision: store.TriageDecisionInvestigate, count: 99}
	llm := &countingLLM{}
	e := autoPromoteEngine(t, trs, llm, 0)

	if _, err := e.Process(context.Background(), autoPromoteMsg()); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if llm.calls != 1 {
		t.Errorf("LLM calls = %d, want 1 when auto-promote is disabled", llm.calls)
	}
}
