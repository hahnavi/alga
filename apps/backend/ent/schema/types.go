package schema

type RouteCondition struct {
	Source   string `json:"source,omitempty"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
}

type RouteTarget struct {
	Provider string `json:"provider,omitempty"`
	Channel  string `json:"channel"`
}

type RouteConfig struct {
	MatchMode  string           `json:"match_mode,omitempty"`
	Conditions []RouteCondition `json:"conditions,omitempty"`
	Targets    []RouteTarget    `json:"targets,omitempty"`
	Silenced   bool             `json:"silenced,omitempty"`
}
