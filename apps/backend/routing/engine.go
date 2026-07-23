package routing

import (
	"strings"
	"sync"

	"alga/config"
	"alga/logger"
	"alga/matching"
	"alga/types"
)

type normalizedRule struct {
	conditions []normalizedCondition
	index      int
}

// Engine routes alerts to Mattermost based on configuration
type Engine struct {
	mu                  sync.RWMutex
	rules               []Rule
	normalizedRules     []normalizedRule
	DefaultDestinations []Destination
}

// Destination identifies where an alert should be delivered.
type Destination struct {
	Provider   string `json:"provider" yaml:"provider"`
	Channel    string `json:"channel" yaml:"channel"`
	IsSilenced bool   `json:"is_silenced" yaml:"is_silenced"`
}

// RouteResult is the outcome of routing a single alert.
type RouteResult struct {
	Silenced     bool
	Destinations []Destination
}

// Rule represents a single routing rule
type Rule struct {
	MatchMode    string // pre-normalized to lowercase
	Conditions   []config.RouteCondition
	Silenced     bool
	Destinations []Destination
}

type normalizedCondition struct {
	Source   string // pre-normalized to lowercase
	Field    string // pre-trimmed
	Operator string // pre-normalized to lowercase
	Value    string
}

// NewEngine creates a new routing engine with the given rules
func NewEngine(rulesConfig []config.RouteConfig) *Engine {
	var rules []Rule
	var allNorm []normalizedRule
	for i, rc := range rulesConfig {
		var dests []Destination
		for _, t := range rc.Targets {
			ch := strings.TrimSpace(t.Channel)
			if ch == "" {
				continue
			}
			dests = append(dests, Destination{
				Provider: normalizeProvider(t.Provider),
				Channel:  ch,
			})
		}
		var normConds []normalizedCondition
		for _, c := range rc.Conditions {
			normConds = append(normConds, normalizedCondition{
				Source:   strings.ToLower(strings.TrimSpace(c.Source)),
				Field:    strings.TrimSpace(c.Field),
				Operator: strings.ToLower(strings.TrimSpace(c.Operator)),
				Value:    c.Value,
			})
		}
		rules = append(rules, Rule{
			MatchMode:    strings.ToLower(strings.TrimSpace(rc.MatchMode)),
			Conditions:   rc.Conditions,
			Silenced:     rc.Silenced,
			Destinations: dests,
		})
		allNorm = append(allNorm, normalizedRule{
			conditions: normConds,
			index:      i,
		})
	}
	return &Engine{rules: rules, normalizedRules: allNorm}
}

// Route processes an alert and returns destinations or silenced.
// DefaultDestinations are always included. Route destinations are added on top.
// A silenced rule match overrides everything and suppresses the alert entirely.
func (e *Engine) Route(alert types.Alert) RouteResult {
	e.mu.RLock()
	defaults := e.DefaultDestinations
	e.mu.RUnlock()

	for i, rule := range e.rules {
		if !e.match(i, rule, alert) {
			continue
		}
		if rule.Silenced {
			logger.Debug("route matched", "component", "routing", "rule_index", i, "alert_name", alert.Labels["alertname"], "silenced", rule.Silenced)
			return RouteResult{Silenced: true}
		}
		if len(rule.Destinations) > 0 {
			logger.Debug("route matched", "component", "routing", "rule_index", i, "alert_name", alert.Labels["alertname"], "silenced", rule.Silenced)
			dests := make([]Destination, 0, len(defaults)+len(rule.Destinations))
			dests = append(dests, defaults...)
			dests = append(dests, rule.Destinations...)
			return RouteResult{Destinations: deduplicateDestinations(dests)}
		}
	}
	logger.Debug("no route matched, using defaults", "component", "routing", "alert_name", alert.Labels["alertname"])
	if len(defaults) > 0 {
		return RouteResult{Destinations: defaults}
	}
	return RouteResult{Silenced: true}
}

// SetDefaults updates the default destinations used for every alert.
func (e *Engine) SetDefaults(defaults []Destination) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.DefaultDestinations = defaults
}

func deduplicateDestinations(dests []Destination) []Destination {
	seen := make(map[string]bool, len(dests))
	result := make([]Destination, 0, len(dests))
	for _, d := range dests {
		key := d.Provider + ":" + d.Channel
		if !seen[key] {
			seen[key] = true
			result = append(result, d)
		}
	}
	return result
}

// match checks whether a rule's conditions are satisfied by the alert.
func (e *Engine) match(ruleIdx int, rule Rule, alert types.Alert) bool {
	if len(rule.Conditions) == 0 {
		return false
	}

	var normConds []normalizedCondition
	if ruleIdx >= 0 && ruleIdx < len(e.normalizedRules) && e.normalizedRules[ruleIdx].index == ruleIdx {
		normConds = e.normalizedRules[ruleIdx].conditions
	}

	matchAny := rule.MatchMode == "any"
	matchedCount := 0

	conds := rule.Conditions
	if normConds != nil {
		for i, condition := range conds {
			actual, _ := readNormalizedConditionField(normConds[i], alert)
			matched := matching.MatchCondition(actual, normConds[i].Operator, condition.Value)
			if matchAny && matched {
				return true
			}
			if !matchAny && !matched {
				return false
			}
			if matched {
				matchedCount++
			}
		}
	} else {
		for _, condition := range conds {
			actual, _ := readConditionField(condition, alert)
			op := strings.ToLower(strings.TrimSpace(condition.Operator))
			matched := matching.MatchCondition(actual, op, condition.Value)
			if matchAny && matched {
				return true
			}
			if !matchAny && !matched {
				return false
			}
			if matched {
				matchedCount++
			}
		}
	}

	if matchAny {
		return matchedCount > 0
	}
	return true
}

func readNormalizedConditionField(nc normalizedCondition, alert types.Alert) (string, bool) {
	switch nc.Source {
	case "annotations":
		v, ok := alert.Annotations[nc.Field]
		return v, ok
	case "alert":
		switch nc.Field {
		case "status":
			return alert.Status, true
		case "fingerprint":
			return alert.Fingerprint, true
		case "generator_url":
			return alert.GeneratorURL, true
		case "silence_url":
			return alert.SilenceURL, true
		case "dashboard_url":
			return alert.DashboardURL, true
		case "panel_url":
			return alert.PanelURL, true
		case "alertname":
			return alert.Labels["alertname"], true
		default:
			return "", false
		}
	default:
		v, ok := alert.Labels[nc.Field]
		return v, ok
	}
}

func readConditionField(condition config.RouteCondition, alert types.Alert) (string, bool) {
	source := strings.ToLower(strings.TrimSpace(condition.Source))
	field := strings.TrimSpace(condition.Field)
	switch source {
	case "annotations":
		v, ok := alert.Annotations[field]
		return v, ok
	case "alert":
		switch field {
		case "status":
			return alert.Status, true
		case "fingerprint":
			return alert.Fingerprint, true
		case "generator_url":
			return alert.GeneratorURL, true
		case "silence_url":
			return alert.SilenceURL, true
		case "dashboard_url":
			return alert.DashboardURL, true
		case "panel_url":
			return alert.PanelURL, true
		case "alertname":
			return alert.Labels["alertname"], true
		default:
			return "", false
		}
	default:
		v, ok := alert.Labels[field]
		return v, ok
	}
}

func normalizeProvider(provider string) string {
	if provider == "slack" {
		return "slack"
	}
	return "mattermost"
}

func FindCommonKeyValues(maps []map[string]string) map[string]string {
	if len(maps) == 0 {
		return map[string]string{}
	}
	result := map[string]string{}
	for k, v := range maps[0] {
		result[k] = v
	}
	for i := 1; i < len(maps); i++ {
		for k, v := range result {
			if maps[i][k] != v {
				delete(result, k)
			}
		}
	}
	return result
}
