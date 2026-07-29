package models

import "github.com/google/uuid"

// RouteCondition defines a single routing condition for alert dispatch.
type RouteCondition struct {
	Source   string `json:"source,omitempty"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
}

// RouteTarget defines a delivery target for routed alerts.
type RouteTarget struct {
	Provider string `json:"provider,omitempty"`
	Channel  string `json:"channel"`
}

// RouteConfig defines the full routing configuration.
type RouteConfig struct {
	MatchMode  string           `json:"match_mode,omitempty"`
	Conditions []RouteCondition `json:"conditions,omitempty"`
	Targets    []RouteTarget    `json:"targets,omitempty"`
	Silenced   bool             `json:"silenced,omitempty"`
}

// InvestigationSummary captures the structured result of an investigation.
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

// AlertInvestigationSummary is an alias used by alert investigations.
type AlertInvestigationSummary = InvestigationSummary

// InvestigationFinding captures a single finding within an investigation.
type InvestigationFinding struct {
	Title    string   `json:"title"`
	Severity string   `json:"severity,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

// EvidenceItem captures a single piece of evidence.
type EvidenceItem struct {
	Source    string `json:"source"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

// EscalationLevelRecord defines one level in an escalation policy.
type EscalationLevelRecord struct {
	LevelNumber    int                      `json:"level_number"`
	DelayMinutes   int                      `json:"delay_minutes"`
	NotifyChannels []string                 `json:"notify_channels,omitempty"`
	Targets        []EscalationTargetRecord `json:"targets,omitempty"`
}

// EscalationTargetRecord captures one target inside an escalation level.
type EscalationTargetRecord struct {
	TargetType   string     `json:"target_type"`
	TargetUserID *uuid.UUID `json:"target_user_id,omitempty"`
	TargetTeamID *uuid.UUID `json:"target_team_id,omitempty"`
}
