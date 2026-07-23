package metrics

import "expvar"

var (
	EscalationsFired             = expvar.NewInt("alga_escalations_fired_total")
	SLABreachesResponse          = expvar.NewInt("alga_sla_breach_response_total")
	SLABreachesResolve           = expvar.NewInt("alga_sla_breach_resolve_total")
	StuckInvestigationsEscalated = expvar.NewInt("alga_stuck_investigations_escalated_total")
	VoiceCallsPlaced             = expvar.NewInt("alga_voice_calls_placed_total")
	VoiceCallsSuppressed         = expvar.NewInt("alga_voice_calls_suppressed_total")
)
