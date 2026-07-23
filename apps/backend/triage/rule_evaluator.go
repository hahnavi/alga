package triage

import (
	"context"
	"fmt"
	"strings"

	"alga/matching"
	"alga/store"
)

type RuleEvaluator struct {
	store store.TriageRuleStore
}

func NewRuleEvaluator(s store.TriageRuleStore) *RuleEvaluator {
	return &RuleEvaluator{store: s}
}

type RuleMatch struct {
	Rule       *store.TriageRuleRecord
	Decision   string
	Severity   string
	Category   string
	Enrichment map[string]any
}

func (e *RuleEvaluator) Evaluate(ctx context.Context, labels map[string]string, annotations map[string]string) (*RuleMatch, error) {
	rules, err := e.store.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list triage rules: %w", err)
	}
	for _, rule := range rules {
		if matchesRule(rule, labels, annotations) {
			enrichment := rule.Enrichment
			if enrichment == nil {
				enrichment = map[string]any{}
			}
			return &RuleMatch{
				Rule:       &rule,
				Decision:   rule.Decision,
				Severity:   rule.Severity,
				Category:   rule.Category,
				Enrichment: enrichment,
			}, nil
		}
	}
	return nil, nil
}

func matchesRule(rule store.TriageRuleRecord, labels, annotations map[string]string) bool {
	if len(rule.Conditions) == 0 {
		return false
	}
	allMatch := rule.MatchMode == "all"
	for _, cond := range rule.Conditions {
		field, _ := cond["field"].(string)
		operator, _ := cond["operator"].(string)
		value, _ := cond["value"].(string)

		var fieldValue string
		if strings.HasPrefix(field, "label:") {
			fieldValue = labels[strings.TrimPrefix(field, "label:")]
		} else if strings.HasPrefix(field, "annotation:") {
			fieldValue = annotations[strings.TrimPrefix(field, "annotation:")]
		} else if field == "alertname" {
			fieldValue = labels["alertname"]
		} else if field == "namespace" {
			fieldValue = labels["namespace"]
		} else if field == "severity" {
			fieldValue = labels["severity"]
		} else {
			fieldValue = labels[field]
		}

		matched := matching.MatchCondition(fieldValue, operator, value)
		if allMatch && !matched {
			return false
		}
		if !allMatch && matched {
			return true
		}
	}
	return allMatch
}
