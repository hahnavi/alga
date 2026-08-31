package incident

import "slices"

var validTransitions = map[string][]string{
	"detected":  {"triaging", "active", "cancelled"},
	"triaging":  {"active", "cancelled"},
	"active":    {"mitigated", "resolved", "cancelled"},
	"mitigated": {"resolved", "active"},
	"resolved":  {"closed", "active"},
	"closed":    {"active"},
	"cancelled": {},
}

func IsValidTransition(from, to string) bool {
	available, ok := validTransitions[from]
	if !ok {
		return false
	}
	return slices.Contains(available, to)
}

// actionTransitions is the single authority for the lifecycle actions every
// mutation path (operator API, agent tools, workers, scheduler auto-ack) must
// enforce. Each entry's from→to pair is validated against validTransitions by
// TestActionTransitionsMatchLifecycleMap, so the two maps cannot drift.
var actionTransitions = map[string]struct {
	from []string
	to   string
}{
	"acknowledge":  {from: []string{"detected"}, to: "active"},
	"begin-triage": {from: []string{"detected"}, to: "triaging"},
	"promote":      {from: []string{"triaging"}, to: "active"},
	"mitigate":     {from: []string{"active"}, to: "mitigated"},
	"resolve":      {from: []string{"active", "mitigated"}, to: "resolved"},
	"close":        {from: []string{"resolved"}, to: "closed"},
	"reopen":       {from: []string{"mitigated", "resolved", "closed"}, to: "active"},
	"cancel":       {from: []string{"detected", "triaging", "active"}, to: "cancelled"},
}

// ActionSources returns the legal source statuses for the named lifecycle
// action, for use as the from-status list of the store's optimistic
// concurrency guard. Unknown actions return nil, which the store treats as
// "no guard" — callers must only pass the fixed action names above.
func ActionSources(action string) []string {
	entry, ok := actionTransitions[action]
	if !ok {
		return nil
	}
	out := make([]string, len(entry.from))
	copy(out, entry.from)
	return out
}

// ActionTarget returns the target status of the named lifecycle action.
func ActionTarget(action string) string {
	entry, ok := actionTransitions[action]
	if !ok {
		return ""
	}
	return entry.to
}

func AvailableTransitions(status string) []string {
	available, ok := validTransitions[status]
	if !ok {
		return nil
	}
	out := make([]string, len(available))
	copy(out, available)
	return out
}

// ExpectedIncidentPhaseActions returns a short checklist of actions expected in
// the current incident lifecycle phase. It's injected into the commander system
// prompt so the orchestrator knows what to do next without a separate playbook
// entity. Returns nil for terminal/unknown phases.
func ExpectedIncidentPhaseActions(status string) []string {
	switch status {
	case "detected":
		return []string{"Acknowledge/triage the incident", "Assign commander/communicator/responder roles", "Ensure responder agents are investigating likely-affected services (mention them or assign child investigations)"}
	case "triaging":
		return []string{"Complete triage", "Promote to active", "Ensure responder agents are assigned to investigate"}
	case "active":
		return []string{"Ensure investigations cover all affected services", "Track progress via coordination messages and status updates", "Collect responder findings as handoffs complete", "Publish status updates (ask the communicator via mention or publish directly)"}
	case "mitigated":
		return []string{"Verify recovery via responder evidence", "Collect final responder findings", "Publish resolved status update", "Set resolution docs (root_cause, impact_assessment, actions_taken, resolution)", "Resolve the incident"}
	case "resolved":
		return []string{"Confirm resolution docs are complete", "Close the incident"}
	case "closed", "cancelled":
		return nil
	}
	return nil
}
