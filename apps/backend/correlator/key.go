package correlator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// canonicalKeys is the ordered set of "workload identity" labels we use to
// build the correlation key. The first match wins; we always combine it with
// alertname so the same alertname firing across multiple workloads doesn't
// collide.
var canonicalKeys = []string{"deployment", "statefulset", "daemonset", "job"}

// CorrelationKey returns the deterministic key string used to group alerts,
// plus a small map of discriminator labels for the scheduler to use as
// spread / affinity hints. Both outputs are derived from the same labels so
// callers can rely on the key string alone for buffering, but the
// discriminators describe *why* alerts grouped (e.g. namespace=prod,
// deployment=api).
func CorrelationKey(labels map[string]string) (string, map[string]string) {
	if labels == nil {
		labels = map[string]string{}
	}
	alertname := strings.TrimSpace(labels["alertname"])
	disc := make(map[string]string, 4)
	if v := strings.TrimSpace(labels["namespace"]); v != "" {
		disc["namespace"] = v
	}
	if alertname != "" {
		disc["alertname"] = alertname
	}
	for _, k := range canonicalKeys {
		if v := strings.TrimSpace(labels[k]); v != "" {
			disc[k] = v
			parts := make([]string, 0, 3)
			if ns := strings.TrimSpace(labels["namespace"]); ns != "" {
				parts = append(parts, ns)
			}
			parts = append(parts, v, alertname)
			return strings.Join(parts, ":"), disc
		}
	}

	ns := strings.TrimSpace(labels["namespace"])
	if ns != "" || alertname != "" {
		parts := make([]string, 0, 2)
		if ns != "" {
			parts = append(parts, ns)
		}
		if alertname != "" {
			parts = append(parts, alertname)
		}
		return strings.Join(parts, ":"), disc
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s,", k, labels[k])
	}
	return "unkeyed:" + hex.EncodeToString(h.Sum(nil))[:16], disc
}
