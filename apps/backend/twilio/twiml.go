package twilio

import (
	"fmt"
	"strings"
)

func escapeXML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func buildTwiML(incidentNumber int64, level int, brief, callbackURL string) string {
	announcement := announcementText(incidentNumber, level, brief)
	prompt := "Press 1 to acknowledge. Press 2 to silence for one hour."
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Say voice="alice">%s</Say>
  <Gather numDigits="1" action="%s" method="POST">
    <Say>%s</Say>
  </Gather>
  <Say>No input received. Goodbye.</Say>
</Response>`, escapeXML(announcement), escapeXML(callbackURL), escapeXML(prompt))
}

// announcementText composes the spoken lead-in for an outbound IVR call. brief
// is a short incident title inserted between the level and the menu when
// non-empty, so the recipient hears what's burning before the DTMF prompt.
func announcementText(incidentNumber int64, level int, brief string) string {
	base := fmt.Sprintf("Alga escalation. Incident %d, escalation level %d.", incidentNumber, level)
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return base + " Press 1 to acknowledge. Press 2 to silence for one hour."
	}
	return base + " " + brief + ". Press 1 to acknowledge. Press 2 to silence for one hour."
}

const MaxIVRAttempts = 2

// RePromptTwiML is returned when the caller provided no digit on a prior gather.
// It re-runs the <Gather> with the same action URL (bumped to the next attempt
// by the caller) and, once attempt > MaxIVRAttempts, plays a goodbye instead.
func RePromptTwiML(callbackURL string, attempt int) string {
	prompt := "Press 1 to acknowledge. Press 2 to silence for one hour."
	if attempt > MaxIVRAttempts {
		return `<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Say>No input received. Goodbye.</Say>
</Response>`
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Gather numDigits="1" action="%s" method="POST">
    <Say>%s</Say>
  </Gather>
  <Say>No input received. Goodbye.</Say>
</Response>`, escapeXML(callbackURL), escapeXML(prompt))
}
