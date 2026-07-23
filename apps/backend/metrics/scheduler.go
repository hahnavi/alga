package metrics

import "expvar"

// Scheduler metrics — incremented from apps/backend/worker scheduler.
var (
	SchedulerPending                 = expvar.NewInt("alga_scheduler_pending")
	SchedulerScheduledTotal          = expvar.NewInt("alga_scheduler_scheduled_total")
	SchedulerBindFailedTotal         = expvar.NewInt("alga_scheduler_bind_failed_total")
	SchedulerNoCandidateTotal        = expvar.NewInt("alga_scheduler_no_candidate_total")
	SchedulerSkipActiveBackoffTotal  = expvar.NewInt("alga_scheduler_skip_active_backoff_total")
	SchedulerTickDurationMs          = expvar.NewInt("alga_scheduler_tick_duration_ms")
	SchedulerTickTotal               = expvar.NewInt("alga_scheduler_tick_total")
	SchedulerAgentCapacityUse        = expvar.NewInt("alga_scheduler_agent_capacity_used")
	SchedulerAgentCapacityMax        = expvar.NewInt("alga_scheduler_agent_capacity_total")
	SchedulerIsLeader                = expvar.NewInt("alga_scheduler_is_leader")
	SchedulerOnlineAgents            = expvar.NewInt("alga_scheduler_online_agents")
	SchedulerNudgeTotal              = expvar.NewInt("alga_scheduler_nudge_total")
	SchedulerStaleAlertsSwept        = expvar.NewInt("alga_scheduler_stale_alerts_swept")
	SchedulerStaleInvestigationsMade = expvar.NewInt("alga_scheduler_stale_investigations_created")
	SchedulerStaleSweepTickTotal     = expvar.NewInt("alga_scheduler_stale_sweep_tick_total")
	SchedulerIncidentSweepTickTotal  = expvar.NewInt("alga_scheduler_incident_sweep_tick_total")
	SummarySweepTotal                = expvar.NewInt("alga_scheduler_summary_sweep_total")
	SummaryDispatchedTotal           = expvar.NewInt("alga_scheduler_summary_dispatched_total")
	SummarySkippedTotal              = expvar.NewInt("alga_scheduler_summary_skipped_total")
	SchedulerDispatchLatencyMs       = expvar.NewInt("alga_scheduler_dispatch_latency_ms")
	SchedulerDLQTotal                = expvar.NewInt("alga_scheduler_dlq_total")
)
