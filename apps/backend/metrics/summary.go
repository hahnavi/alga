package metrics

import "expvar"

// Summary-owned metrics. Declared here because the producer
// (api/incident_summary.go) is the only writer.

var SummaryPostedTotal = expvar.NewInt("alga_summary_posted_total")
