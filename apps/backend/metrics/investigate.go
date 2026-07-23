package metrics

import "expvar"

// Investigate-worker-owned metrics. Declared here because the
// investigate worker (worker/investigate.go, worker/retry.go)
// is the primary writer.

var InvestigateWorkerCreateLatencyMs = expvar.NewInt("alga_investigate_worker_create_latency_ms")
var WorkerDLQTotal = expvar.NewInt("alga_worker_dlq_total")
