package memory

import (
	"fmt"
	"strings"
	"time"

	"alga/store"
	"alga/strutil"
)

const extractionSystemPrompt = `You are an SRE memory extraction system. Your job is to extract discrete, factual memory statements from completed investigation data.

Given an investigation's context (alerts, timeline, root cause, resolution), extract memories that would be useful for future investigations of similar issues.

Rules:
1. Each memory must be a self-contained factual statement (15-80 words)
2. No pronouns — use full names/identifiers
3. Be specific: include service names, namespaces, versions, thresholds
4. Each memory should capture ONE fact
5. Resolve relative dates to absolute context
6. Do NOT fabricate information
7. Do NOT echo the input
8. Include infrastructure details (deployment names, cluster names, namespaces)

Memory types:
- "fact": A confirmed fact about the system (e.g., "Service payment-api in namespace production has a known memory leak in the connection pool since v2.4.0")
- "pattern": A recurring pattern observed across incidents (e.g., "When alert HighCPU fires for checkout-service, it is typically caused by unoptimized database queries during peak hours")
- "procedure": A recommended investigation or remediation step (e.g., "For DiskPressure alerts on node-pool-a, check log rotation on /var/log and increase PVC size to 100Gi")

Respond with JSON:
{
  "memories": [
    {
      "text": "the factual statement",
      "type": "fact|pattern|procedure",
      "confidence": 0.9,
      "entities": ["service-name", "namespace-name"]
    }
  ]
}

If the investigation has no useful extractable knowledge, return {"memories": []}.`

func buildExtractionUserPrompt(inv *store.AlertInvestigationRecord, updates []store.InvestigationUpdate) string {
	var b strings.Builder

	now := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(&b, "Observation date: %s\n\n", now)

	fmt.Fprintf(&b, "=== INVESTIGATION %s ===\n", inv.AlertInvestigationID)
	fmt.Fprintf(&b, "Status: %s\n", inv.Status)
	if inv.AgentType != "" {
		fmt.Fprintf(&b, "Agent: %s (%s)\n", inv.AgentName, inv.AgentType)
	}
	if inv.CorrelationKey != "" {
		fmt.Fprintf(&b, "Correlation key: %s\n", inv.CorrelationKey)
	}

	if len(inv.Alerts) > 0 {
		fmt.Fprintf(&b, "\n--- ALERTS (%d) ---\n", len(inv.Alerts))
		for i, a := range inv.Alerts {
			if i >= 5 {
				fmt.Fprintf(&b, "... and %d more\n", len(inv.Alerts)-5)
				break
			}
			fmt.Fprintf(&b, "Alert %d: %s\n", i+1, a.Labels["alertname"])
			if a.Labels["namespace"] != "" {
				fmt.Fprintf(&b, "  namespace: %s\n", a.Labels["namespace"])
			}
			if a.Labels["service"] != "" {
				fmt.Fprintf(&b, "  service: %s\n", a.Labels["service"])
			}
			if a.Labels["deployment"] != "" {
				fmt.Fprintf(&b, "  deployment: %s\n", a.Labels["deployment"])
			}
			if a.Annotations["summary"] != "" {
				fmt.Fprintf(&b, "  summary: %s\n", a.Annotations["summary"])
			}
			if a.Annotations["description"] != "" {
				desc := a.Annotations["description"]
				if len(desc) > 300 {
					desc = desc[:300] + "..."
				}
				fmt.Fprintf(&b, "  description: %s\n", desc)
			}
		}
	}

	if inv.Summary != nil {
		fmt.Fprintf(&b, "\n--- OUTCOME ---\n")
		if inv.Summary.RootCause != "" {
			fmt.Fprintf(&b, "Root cause: %s\n", inv.Summary.RootCause)
		}
		if inv.Summary.Resolution != "" {
			fmt.Fprintf(&b, "Resolution: %s\n", inv.Summary.Resolution)
		}
		if inv.Summary.Summary != "" {
			fmt.Fprintf(&b, "Summary: %s\n", inv.Summary.Summary)
		}
		for _, e := range inv.Summary.Evidence {
			fmt.Fprintf(&b, "Evidence: %s\n", e)
		}
		for _, a := range inv.Summary.RecommendedActions {
			fmt.Fprintf(&b, "Recommended action: %s\n", a)
		}
	}

	if len(updates) > 0 {
		fmt.Fprintf(&b, "\n--- TIMELINE (last %d entries) ---\n", min(len(updates), 20))
		start := max(0, len(updates)-20)
		for _, u := range updates[start:] {
			if u.Internal {
				continue
			}
			src := string(u.Source)
			fmt.Fprintf(&b, "[%s/%s] %s\n", src, string(u.Type), strutil.TruncateOneLine(u.Message, 200))
		}
	}

	return b.String()
}

type extractionResult struct {
	Memories []extractedMemory `json:"memories"`
}

type extractedMemory struct {
	Text       string   `json:"text"`
	Type       string   `json:"type"`
	Confidence float64  `json:"confidence"`
	Entities   []string `json:"entities"`
}
