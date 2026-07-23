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
		return []string{"Acknowledge/triage the incident", "Assign commander/communicator/responder roles", "Dispatch initial investigate tasks for likely-affected services"}
	case "triaging":
		return []string{"Complete triage", "Promote to active", "Dispatch investigate tasks"}
	case "active":
		return []string{"Ensure investigate tasks are dispatched for all affected services", "Track child task progress via alga_list_tasks", "Synthesize findings as they complete", "Publish status updates (dispatch communicate tasks or publish directly)"}
	case "mitigated":
		return []string{"Verify recovery via responder evidence", "Synthesize final findings", "Publish resolved status update", "Set resolution docs (root_cause, impact_assessment, actions_taken, resolution)", "Resolve the incident"}
	case "resolved":
		return []string{"Confirm resolution docs are complete", "Close the incident"}
	case "closed", "cancelled":
		return nil
	}
	return nil
}
