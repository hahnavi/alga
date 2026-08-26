package api

// Readiness describes optional pipeline components (for operators / probes).
type Readiness struct {
	CorrelatorEnabled        bool               `json:"correlator_enabled"`
	InvestigateWorkerEnabled bool               `json:"investigate_worker_enabled"`
	ValkeyConfigured         bool               `json:"valkey_configured"`
	RabbitMQConfigured       bool               `json:"rabbitmq_configured"`
	HAMode                   bool               `json:"ha_mode"`
	Replica                  string             `json:"replica,omitempty"`
	Scheduler                SchedulerSnapshot  `json:"scheduler"`
	Correlator               CorrelatorSnapshot `json:"correlator"`
	Triage                   TriageSnapshot     `json:"triage"`
	Notes                    []string           `json:"notes,omitempty"`
}

// SchedulerSnapshot is a point-in-time view of the scheduler's metrics.
// Useful as a single readiness call instead of forcing operators to scrape
// /metrics on every replica.
type SchedulerSnapshot struct {
	IsLeader                   bool  `json:"is_leader"`
	PendingDepth               int64 `json:"pending_depth"`
	OnlineAgents               int64 `json:"online_agents"`
	CapacityUsed               int64 `json:"capacity_used"`
	CapacityTotal              int64 `json:"capacity_total"`
	ScheduledTotal             int64 `json:"scheduled_total"`
	NoCandidate                int64 `json:"no_candidate_total"`
	BindFailedTotal            int64 `json:"bind_failed_total"`
	TickTotal                  int64 `json:"tick_total"`
	TickDurationMs             int64 `json:"tick_duration_ms"`
	NudgeTotal                 int64 `json:"nudge_total"`
	StaleAlertsSwept           int64 `json:"stale_alerts_swept"`
	StaleInvestigationsCreated int64 `json:"stale_investigations_created"`
	StaleSweepTickTotal        int64 `json:"stale_sweep_tick_total"`
}

// CorrelatorSnapshot mirrors SchedulerSnapshot for the alert correlator.
type CorrelatorSnapshot struct {
	AlertsTotal     int64 `json:"alerts_total"`
	MergedTotal     int64 `json:"merged_total"`
	PublishedTotal  int64 `json:"published_total"`
	DroppedTotal    int64 `json:"dropped_total"`
	WindowOpenTotal int64 `json:"window_open_total"`
	FlushTotal      int64 `json:"flush_total"`
	FailClosedTotal int64 `json:"fail_closed_total"`
}

type TriageSnapshot struct {
	Enabled       bool `json:"enabled"`
	LLMConfigured bool `json:"llm_configured"`
	MaxConcurrent int  `json:"max_concurrent"`
}

func (s *Server) SetReadiness(r Readiness) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readiness = r
}
