package schema

type InvestigationSummary struct {
	Status             string   `json:"status"`
	RootCause          string   `json:"root_cause,omitempty"`
	Resolution         string   `json:"resolution,omitempty"`
	Summary            string   `json:"summary"`
	Findings           []string `json:"findings,omitempty"`
	Evidence           []string `json:"evidence,omitempty"`
	RecommendedActions []string `json:"recommended_actions,omitempty"`
	SeverityAssessment string   `json:"severity_assessment,omitempty"`
	EscalationLevel    string   `json:"escalation_level,omitempty"`
	RawResponse        string   `json:"raw_response,omitempty"`
}

type InvestigationFinding struct {
	Title    string   `json:"title"`
	Severity string   `json:"severity,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type EvidenceItem struct {
	Source    string `json:"source"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}
