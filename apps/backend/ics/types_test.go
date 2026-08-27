package ics

import "testing"

func TestValidEndReason(t *testing.T) {
	t.Parallel()

	for _, reason := range []EndReason{EndReasonReplaced, EndReasonIncidentResolved, EndReasonAssigned, EndReasonAgentOffline, EndReasonIncidentClosed} {
		if !ValidEndReason(reason) {
			t.Errorf("ValidEndReason(%q) = false, want true", reason)
		}
	}
	for _, reason := range []EndReason{"", "bogus", "closed", "INCIDENT_CLOSED"} {
		if ValidEndReason(reason) {
			t.Errorf("ValidEndReason(%q) = true, want false", reason)
		}
	}
}
