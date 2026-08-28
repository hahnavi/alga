package correlator

import (
	"fmt"
	"strings"
)

type CorrelationRule struct {
	Strategy      string
	GroupByLabels []string
	// Window overrides the global correlation window for this rule's
	// alertname, in seconds. > 0 buffers alerts for that name for this long
	// before flushing; <= 0 falls back to the global window.
	Window int64
}

type SuppressionRule struct {
	AlertName string
	Namespace string
	Labels    map[string]string
}

func IsSuppressed(labels map[string]string, rules []SuppressionRule) bool {
	if labels == nil || len(rules) == 0 {
		return false
	}
	alertName := labels["alertname"]
	ns := labels["namespace"]
	for _, r := range rules {
		if r.AlertName != "" && r.AlertName != alertName {
			continue
		}
		if r.Namespace != "" && r.Namespace != ns {
			continue
		}
		match := true
		for k, v := range r.Labels {
			if labels[k] != v {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func CorrelationKeyWithRules(labels map[string]string, rule *CorrelationRule) (string, map[string]string) {
	if rule == nil || rule.Strategy == "" || rule.Strategy == "key" {
		return CorrelationKey(labels)
	}

	if labels == nil {
		labels = map[string]string{}
	}

	switch rule.Strategy {
	case "solo":
		alertName := labels["alertname"]
		if alertName == "" {
			alertName = "unknown"
		}
		return fmt.Sprintf("solo:%s:%s", alertName, uniqueSuffix(labels)), discriminatorsFromLabels(labels)
	case "namespace":
		ns := labels["namespace"]
		if ns == "" {
			ns = "_default"
		}
		return "ns:" + ns, map[string]string{"namespace": ns}
	case "label_match":
		parts := make([]string, 0, len(rule.GroupByLabels))
		for _, k := range rule.GroupByLabels {
			v := labels[k]
			if v == "" {
				v = "_"
			}
			parts = append(parts, v)
		}
		return "lbl:" + strings.Join(parts, ":"), discriminatorsFromLabels(labels)
	default:
		return CorrelationKey(labels)
	}
}

func discriminatorsFromLabels(labels map[string]string) map[string]string {
	disc := make(map[string]string, 2)
	if v := labels["namespace"]; v != "" {
		disc["namespace"] = v
	}
	if v := labels["alertname"]; v != "" {
		disc["alertname"] = v
	}
	return disc
}

func uniqueSuffix(labels map[string]string) string {
	k, _ := CorrelationKey(labels)
	if len(k) >= 16 {
		return k[0:16]
	}
	return k
}
