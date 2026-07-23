package metrics

import "expvar"

// Correlator metrics — incremented from apps/backend/correlator.
var (
	CorrelatorAlertsTotal     = expvar.NewInt("alga_correlator_alerts_total")
	CorrelatorMergedTotal     = expvar.NewInt("alga_correlator_merged_total")
	CorrelatorPublishedTotal  = expvar.NewInt("alga_correlator_published_total")
	CorrelatorDroppedTotal    = expvar.NewInt("alga_correlator_dropped_total")
	CorrelatorWindowOpenTotal = expvar.NewInt("alga_correlator_window_open_total")
	CorrelatorWindowDepth     = expvar.NewInt("alga_correlator_window_depth")
	CorrelatorFlushTotal      = expvar.NewInt("alga_correlator_flush_total")
	CorrelatorFailClosedTotal = expvar.NewInt("alga_correlator_fail_closed_total")
)
