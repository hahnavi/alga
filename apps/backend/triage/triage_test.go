package triage

import (
	"testing"

	"alga/matching"
)

func TestEvaluateConditionExact(t *testing.T) {
	if !matching.MatchCondition("critical", "exact", "critical") {
		t.Error("expected exact match to succeed")
	}
	if matching.MatchCondition("warning", "exact", "critical") {
		t.Error("expected exact mismatch to fail")
	}
}

func TestEvaluateConditionContains(t *testing.T) {
	if !matching.MatchCondition("prod-us-east", "contains", "east") {
		t.Error("expected contains to succeed")
	}
	if matching.MatchCondition("prod-us-east", "contains", "west") {
		t.Error("expected contains mismatch to fail")
	}
}

func TestEvaluateConditionExists(t *testing.T) {
	if !matching.MatchCondition("anything", "exists", "") {
		t.Error("expected exists to succeed on non-empty")
	}
	if matching.MatchCondition("", "exists", "") {
		t.Error("expected exists to fail on empty")
	}
}

func TestEvaluateConditionNotExists(t *testing.T) {
	if !matching.MatchCondition("", "not_exists", "") {
		t.Error("expected not_exists to succeed on empty")
	}
	if matching.MatchCondition("anything", "not_exists", "") {
		t.Error("expected not_exists to fail on non-empty")
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		s, pattern string
		want       bool
	}{
		{"prod-us-east-1", "prod-*", true},
		{"prod-us-east-1", "*-east-*", true},
		{"prod-us-east-1", "*west*", false},
		{"disk-pressure", "disk*", true},
	}
	for _, tt := range tests {
		got := matching.WildcardMatch(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("WildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestParseTriageResponse(t *testing.T) {
	raw := `{"decision":"auto_resolve","confidence":0.95,"severity":"info","category":"infrastructure","reasoning":"Known disk pressure pattern","suggested_actions":["check disk usage"],"enrichment":{"service_owner":"platform","runbook_url":"https://example.com/disk"}}`
	resp, err := ParseTriageResponse(raw)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if resp.Decision != "auto_resolve" {
		t.Errorf("expected auto_resolve, got %s", resp.Decision)
	}
	if resp.Confidence != 0.95 {
		t.Errorf("expected 0.95, got %f", resp.Confidence)
	}
}

func TestParseTriageResponseMarkdownBlock(t *testing.T) {
	raw := "```json\n{\"decision\":\"investigate\",\"confidence\":0.8,\"severity\":\"high\",\"category\":\"application\",\"reasoning\":\"test\",\"suggested_actions\":[],\"enrichment\":{}}\n```"
	resp, err := ParseTriageResponse(raw)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if resp.Decision != "investigate" {
		t.Errorf("expected investigate, got %s", resp.Decision)
	}
}

func TestParseTriageResponseMissingDecision(t *testing.T) {
	_, err := ParseTriageResponse(`{"confidence": 0.5}`)
	if err == nil {
		t.Error("expected error for missing decision")
	}
}
