package tools

import (
	"fmt"
	"strings"
)

// Evidence gate for coordination task results.
//
// An SRE agent's value is bounded by the evidence behind its claims: a root
// cause without supporting observations is speculation. The gate enforces the
// contract mechanically (a feedback sensor in the agent harness) instead of
// relying on prompt guidance alone: any task result that carries a causal
// claim must also carry the evidence for it. Results without claims (e.g. a
// published status update id) pass untouched.

// claimResultKeys are top-level result keys whose presence means the result
// asserts a causal claim about the incident.
var claimResultKeys = []string{"root_cause", "root_cause_candidate", "finding", "findings"}

// evidenceResultKeys are top-level result keys that carry supporting
// evidence. "verification" is accepted alongside "evidence" because the
// responder handoff contract reports recovery checks under that name.
var evidenceResultKeys = []string{"evidence", "verification"}

// resultHasClaim reports whether the task result asserts a causal claim.
func resultHasClaim(result map[string]any) bool {
	for _, key := range claimResultKeys {
		if !isEmptyResultValue(result[key]) {
			return true
		}
	}
	return false
}

// resultHasEvidence reports whether the task result carries non-empty
// evidence for its claims.
func resultHasEvidence(result map[string]any) bool {
	for _, key := range evidenceResultKeys {
		if !isEmptyResultValue(result[key]) {
			return true
		}
	}
	return false
}

// missingEvidenceReason returns the in-band error message for a task result
// that asserts a claim without evidence, or "" when the result passes the
// gate. The message states the contract so the LLM can retry with evidence.
func missingEvidenceReason(result map[string]any) string {
	if !resultHasClaim(result) || resultHasEvidence(result) {
		return ""
	}
	return fmt.Sprintf(
		"result asserts a claim (%s) without evidence; include an %q key describing the specific observations that support it (command output, metric, log line, alert state) and mark anything unverified as unverified — do not fill evidence gaps with speculation",
		joinedClaimKeys(result), "evidence",
	)
}

// joinedClaimKeys lists the claim keys actually present in the result, for
// the error message.
func joinedClaimKeys(result map[string]any) string {
	var keys []string
	for _, key := range claimResultKeys {
		if !isEmptyResultValue(result[key]) {
			keys = append(keys, key)
		}
	}
	return strings.Join(keys, ", ")
}

// isEmptyResultValue reports whether a decoded JSON result value should be
// treated as absent: nil, empty string, empty slice, or empty map. Strings
// containing only whitespace count as empty.
func isEmptyResultValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(val) == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	default:
		return false
	}
}
