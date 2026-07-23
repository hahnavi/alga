package rabbitmq

func DetermineAlertSeverity(alerts []CorrelatedAlert) string {
	for _, a := range alerts {
		if sev, ok := a.Labels["severity"]; ok {
			if sev == "critical" || sev == "page" {
				return "critical"
			}
			if sev == "high" {
				return "high"
			}
		}
	}
	for _, a := range alerts {
		if sev, ok := a.Labels["severity"]; ok {
			return sev
		}
	}
	return "warning"
}
