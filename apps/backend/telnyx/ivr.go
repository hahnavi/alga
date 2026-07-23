package telnyx

import (
	"fmt"
	"strings"
)

// AnnouncementText builds the spoken lead-in for an IVR call. brief is a short
// incident title inserted between the level and the menu when non-empty, so the
// recipient hears what's burning before being asked to ack/silence.
func AnnouncementText(incidentNumber int64, level int, brief string) string {
	base := fmt.Sprintf("Alga escalation. Incident %d, escalation level %d.", incidentNumber, level)
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return base + " Press 1 to acknowledge. Press 2 to silence for one hour."
	}
	return base + " " + brief + ". Press 1 to acknowledge. Press 2 to silence for one hour."
}

func PromptText() string {
	return "Press 1 to acknowledge. Press 2 to silence for one hour."
}

func AcknowledgedText() string {
	return "Incident acknowledged. Escalation stopped. Goodbye."
}

func SilencedText() string {
	return "Escalation silenced for one hour. Goodbye."
}

// GatherText composes the announcement (with optional brief) and the menu
// prompt, spoken together while collecting a single DTMF digit.
func GatherText(incidentNumber int64, level int, brief string) string {
	return AnnouncementText(incidentNumber, level, brief) + " " + PromptText()
}
