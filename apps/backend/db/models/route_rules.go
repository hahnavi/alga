package models

// RouteRules is a singleton configuration row holding all notification routing rules.
type RouteRules struct {
	BaseModel `bun:"table:route_rules"`

	Routes []RouteConfig `bun:"routes,type:jsonb"`
}
