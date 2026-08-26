package webhook

import (
	"net/http"
	"strings"

	"alga/api/platform"
)

// webhookError writes the standard JSON error envelope for public webhook
// surfaces (WP-B4). Plain-text http.Error responses previously fed
// log-forgement and noise channels into operator dashboards; vendors and
// ingestors do not parse error bodies, so the format change is safe.
func webhookError(w http.ResponseWriter, code platform.ErrorCode, message string) {
	platform.WriteError(w, code, message)
}

func parseInternalNote(message string) (isInternal bool, cleaned string) {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "\U0001F512") {
		return true, strings.TrimSpace(strings.TrimPrefix(trimmed, "\U0001F512"))
	}
	return false, message
}
