package metrics

import "expvar"

var (
	IncidentsCreatedTotal   = expvar.NewInt("alga_incidents_created_total")
	IncidentsActive         = expvar.NewInt("alga_incidents_active")
	IncidentsResolvedTotal  = expvar.NewInt("alga_incidents_resolved_total")
	IncidentsMitigatedTotal = expvar.NewInt("alga_incidents_mitigated_total")
	IncidentsClosedTotal    = expvar.NewInt("alga_incidents_closed_total")
	IncidentsCancelledTotal = expvar.NewInt("alga_incidents_cancelled_total")
	IncidentsReopenedTotal  = expvar.NewInt("alga_incidents_reopened_total")
)
