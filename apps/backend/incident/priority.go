package incident

var validSeverities = map[string]struct{}{
	"critical": {},
	"high":     {},
	"warning":  {},
	"info":     {},
}

var validIncidentTypes = map[string]struct{}{
	"real":        {},
	"alert":       {},
	"degradation": {},
}

var validImpacts = map[string]struct{}{
	"high":   {},
	"medium": {},
	"low":    {},
}

var validPriorities = map[string]struct{}{
	"P1": {},
	"P2": {},
	"P3": {},
	"P4": {},
	"P5": {},
}

var priorityMatrix = map[string]map[string]string{
	"critical": {"high": "P1", "medium": "P2", "low": "P3"},
	"high":     {"high": "P2", "medium": "P3", "low": "P4"},
	"warning":  {"high": "P3", "medium": "P4", "low": "P5"},
	"info":     {"high": "P4", "medium": "P5", "low": "P5"},
}

func ComputePriority(severity, impact string) string {
	sevMap, ok := priorityMatrix[severity]
	if !ok {
		return "P5"
	}
	p, ok := sevMap[impact]
	if !ok {
		return "P5"
	}
	return p
}

func ValidSeverity(s string) bool {
	_, ok := validSeverities[s]
	return ok
}

func ValidIncidentType(t string) bool {
	_, ok := validIncidentTypes[t]
	return ok
}

func ValidImpact(s string) bool {
	_, ok := validImpacts[s]
	return ok
}

func ValidPriority(p string) bool {
	_, ok := validPriorities[p]
	return ok
}

func SeverityRank(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "high":
		return 1
	case "warning":
		return 2
	case "info":
		return 3
	default:
		return 4
	}
}

func PriorityRank(priority string) int {
	switch priority {
	case "P1":
		return 0
	case "P2":
		return 1
	case "P3":
		return 2
	case "P4":
		return 3
	case "P5":
		return 4
	default:
		return 5
	}
}
