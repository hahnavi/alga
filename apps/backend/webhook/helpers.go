package webhook

import "strings"

func parseInternalNote(message string) (isInternal bool, cleaned string) {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "\U0001F512") {
		return true, strings.TrimSpace(strings.TrimPrefix(trimmed, "\U0001F512"))
	}
	return false, message
}
