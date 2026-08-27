package valkey

import "time"

// Key prefixes for cached data. All keys are prefixed with "alga:cache:".
const (
	PrefixDashboardStats     = "alga:cache:dashboard:stats"
	PrefixOnCallWho          = "alga:cache:oncall:who:"
	PrefixServicesList       = "alga:cache:services:list"
	PrefixRoutes             = "alga:cache:routes"
	PrefixIntegrations       = "alga:cache:integrations"
	PrefixEscalationPolicies = "alga:cache:escalation:policies"
)

// TTLs for cached data. These values balance freshness against database load.
var (
	TTLDashboardStats     = 10 * time.Second
	TTLOnCallWho          = 5 * time.Minute
	TTLServicesList       = 30 * time.Second
	TTLRoutes             = 30 * time.Second
	TTLIntegrations       = 30 * time.Second
	TTLEscalationPolicies = 30 * time.Second
)
