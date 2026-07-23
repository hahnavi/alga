package knowledge

import (
	"fmt"
	"time"
)

// humanizeDuration returns a short human-readable duration like "3m", "2h",
// or "5d". Used for rendering "started N ago" on concurrent investigations.
func humanizeDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
