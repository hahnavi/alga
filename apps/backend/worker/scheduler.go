// scheduler.go is the canonical entry point for the InvestigationScheduler.
// It defines the scheduler struct, its configuration setters, the leader
// election gate, and the main run/tick loop. The cohesive concerns that grew
// out of this type — agent disconnect handling, dispatch/candidate scoring,
// ICS role assignment, and the background sweeps — live in their sibling
// scheduler_*.go files within this package.
package worker

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"alga/knowledge"
	"alga/logger"
	"alga/metrics"
	"alga/oncall"
	"alga/prompt"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

// ssePublisher publishes SSE events to the frontend; used in two places:
// ICS role assignment and investigation status changes.
type ssePublisher interface {
	Publish(event sse.Event)
}

// AgentResolver is the abstraction the scheduler uses to talk to the SSE
// hubs. Both the production DefaultInvestigationForwarder and tests
// satisfy this interface. In production, only DefaultInvestigationForwarder
// and the test mocks satisfy this interface.
type AgentResolver interface {
	ForwardToAgent(agentIDHex, investigationID, senderID, senderName, message string) error
	ForwardDispatchToAgent(agentIDHex, investigationID, senderID, senderName, message, systemContext string) error
	ForwardEventToAgent(agentIDHex string, event sse.Event) error
	AgentOnline(agentIDHex string) bool
}

// schedulerTickInterval is how often the scheduler retries even with no
// pending work. Idle ticks do call tickAgentRoleAssignment (every 3rd
// tick) which hits the DB; the leader-lease renewal is the primary purpose.
const schedulerTickInterval = 5 * time.Second

// failureBackoff is how long the scheduler refuses to re-attempt an
// investigation that has just failed to forward (e.g. transient SSE write
// error). Without this the same investigation can spin between pending →
// assigned → reset → pending many times per second.
const failureBackoff = 30 * time.Second

// maxDispatchAttempts is the maximum number of claim-dispatch-sweep cycles
// before the investigation is marked as failed. On the failure path, 6
// attempts × 10-minute timeout ≈ 60 minutes. On the success path, total
// wall-time is well under a minute.
const maxDispatchAttempts = 6

// dispatchAttempt tracks the cumulative dispatch count and last activity
// for an investigation. lastSeen drives the runMapPurge retention window
// so in-flight retries aren't reset by the periodic sweeper.
type dispatchAttempt struct {
	count    int
	lastSeen time.Time
}

type alertInvestigationLifecycle interface {
	CompleteIfAllAlertsResolved(ctx context.Context, req store.AlertInvestigationLifecycleCompletionRequest) error
}

type InvestigationScheduler struct {
	alertInvestigationStore    store.AlertInvestigationStore
	incidentInvestigationStore store.IncidentInvestigationStore
	incidentCoordinationStore  store.IncidentCoordinationStore
	agentTokenStore            store.AgentTokenStore
	resolver                   AgentResolver
	knowledge                  knowledge.Service
	valkeyClient               *valkey.Client
	presence                   *valkey.Presence
	leader                     *valkey.LeaderLease
	ssePublisher               ssePublisher
	activeTTL                  time.Duration
	disconnectGrace            time.Duration
	investigationTimeout       time.Duration
	maxConcurrent              int
	notify                     chan struct{}
	stopCh                     chan struct{}
	stopOnce                   sync.Once
	stopped                    atomic.Bool
	wg                         sync.WaitGroup

	alertStore     store.Store
	stalePublisher staleInvestigatePublisher
	staleThreshold time.Duration
	staleInterval  time.Duration

	slaSweepPublisher slaSweepPublisher
	slaSweepInterval  time.Duration

	dataRetentionDays int
	pruneInterval     time.Duration

	teamStore store.TeamStore

	backoffMu sync.Mutex
	backoff   map[string]time.Time

	nudged sync.Map

	dispatchMu       sync.Mutex
	dispatchAttempts map[string]dispatchAttempt

	incidentStore      store.IncidentStore
	incidentChannelMgr summaryChannelMgr
	auditStore         store.AuditStore

	summaryEnabled           bool
	summaryDefaultInterval   time.Duration
	summarySeverityIntervals map[string]time.Duration

	handoffDetector *oncall.HandoffDetector
	notifyPublisher NotificationPublisher
	handoffSkipOnce bool

	playbookEnricher *prompt.PlaybookEnricher

	healthTracker *AgentHealthTracker

	icsRoleStore                store.ICSRoleStore
	agentRoleTickCounter        int
	alertInvestigationLifecycle alertInvestigationLifecycle

	coordinationTaskStore         store.CoordinationTaskStore
	coordinationTaskSweepInterval time.Duration
}

// staleInvestigatePublisher is the subset of rabbitmq.Publisher used by the
// stale alert sweep to publish InvestigateMessage jobs.
type staleInvestigatePublisher interface {
	PublishInvestigation(ctx context.Context, msg rabbitmq.InvestigateMessage) error
}

// slaSweepPublisher is the subset of rabbitmq.Publisher used by the SLA sweep
// loop to publish SLA sweep request ticks (decided DT-E1: scheduler-owned).
type slaSweepPublisher interface {
	PublishSLASweep(ctx context.Context, msg rabbitmq.SLASweepMessage) error
}

// NotificationPublisher is the subset of rabbitmq.Publisher used by the
// handoff detector to dispatch on-call shift change notifications.
type NotificationPublisher interface {
	PublishNotificationDispatch(ctx context.Context, msg rabbitmq.NotificationDispatchMessage) error
}

type summaryChannelMgr interface {
	PostAgentSummary(ctx context.Context, incident *store.IncidentRecord, agentName string, text string) error
	IsSupported() bool
}

// SetKnowledge wires an optional shared-knowledge aggregator. When set, each
// outbound scheduler prompt is appended with past-incident / KB / concurrent
// context produced by the aggregator. A nil value disables enrichment.
func (s *InvestigationScheduler) SetKnowledge(k knowledge.Service) {
	s.knowledge = k
}

func (s *InvestigationScheduler) SetPlaybookEnricher(e *prompt.PlaybookEnricher) {
	s.playbookEnricher = e
}

func (s *InvestigationScheduler) SetICSRoleStore(rs store.ICSRoleStore) {
	s.icsRoleStore = rs
}

func (s *InvestigationScheduler) SetAlertInvestigationLifecycleService(svc alertInvestigationLifecycle) {
	s.alertInvestigationLifecycle = svc
}

// SetValkeyClient wires the Valkey client used to publish active-investigation
// entries for real-time cross-agent awareness. ttl controls how long a
// registration survives before auto-expiring; zero means 30 minutes.
func (s *InvestigationScheduler) SetValkeyClient(c *valkey.Client, ttl time.Duration) {
	s.valkeyClient = c
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	s.activeTTL = ttl
}

// SetPresence injects the Valkey-backed presence registry. When set, the
// scheduler asks the registry instead of the local resolver, so any replica
// can discover agents connected to peer replicas.
func (s *InvestigationScheduler) SetPresence(p *valkey.Presence) {
	s.presence = p
}

// SetLeaderLease enables HA leader election. When wired, only the replica
// holding the lease performs scheduling work; other replicas stay warm and
// take over on TTL expiry.
func (s *InvestigationScheduler) SetLeaderLease(l *valkey.LeaderLease) {
	s.leader = l
}

func (s *InvestigationScheduler) SetInvestigationTimeout(d time.Duration) {
	if d <= 0 {
		d = 10 * time.Minute
	}
	s.investigationTimeout = d
}

func (s *InvestigationScheduler) SetSSEPublisher(p ssePublisher) {
	s.ssePublisher = p
}

func (s *InvestigationScheduler) SetAuditStore(a store.AuditStore) {
	s.auditStore = a
}

// SetAlertStore wires the alert store needed by the stale alert sweep. When
// set alongside SetStalePublisher and SetStaleConfig, the scheduler launches a
// background goroutine that periodically publishes investigation jobs for
// firing alerts that have no associated investigation.
func (s *InvestigationScheduler) SetAlertStore(store store.Store) {
	s.alertStore = store
}

func (s *InvestigationScheduler) SetTeamStore(ts store.TeamStore) {
	s.teamStore = ts
}

func (s *InvestigationScheduler) SetIncidentStore(is store.IncidentStore) {
	s.incidentStore = is
}

func (s *InvestigationScheduler) SetIncidentInvestigationStore(iis store.IncidentInvestigationStore) {
	s.incidentInvestigationStore = iis
}

func (s *InvestigationScheduler) SetIncidentCoordinationStore(ics store.IncidentCoordinationStore) {
	s.incidentCoordinationStore = ics
}

func (s *InvestigationScheduler) SetCoordinationTaskStore(cts store.CoordinationTaskStore) {
	s.coordinationTaskStore = cts
}

func (s *InvestigationScheduler) SetCoordinationTaskSweepInterval(d time.Duration) {
	s.coordinationTaskSweepInterval = d
}

func (s *InvestigationScheduler) SetIncidentChannelMgr(mgr summaryChannelMgr) {
	s.incidentChannelMgr = mgr
}

func (s *InvestigationScheduler) SetSummaryConfig(enabled bool, defaultInterval time.Duration, severityIntervals map[string]time.Duration) {
	s.summaryEnabled = enabled
	if defaultInterval <= 0 {
		defaultInterval = 15 * time.Minute
	}
	s.summaryDefaultInterval = defaultInterval
	s.summarySeverityIntervals = severityIntervals
}

func (s *InvestigationScheduler) SetHandoffDetector(d *oncall.HandoffDetector, notifyPublisher NotificationPublisher) {
	s.handoffDetector = d
	s.notifyPublisher = notifyPublisher
	s.handoffSkipOnce = true
}

func (s *InvestigationScheduler) effectiveSummaryInterval(severity string) time.Duration {
	if s.summarySeverityIntervals != nil {
		if d, ok := s.summarySeverityIntervals[severity]; ok && d > 0 {
			return d
		}
	}
	return s.summaryDefaultInterval
}

// SetStalePublisher wires the RabbitMQ publisher used by the stale alert sweep.
func (s *InvestigationScheduler) SetStalePublisher(p staleInvestigatePublisher) {
	s.stalePublisher = p
}

// SetStaleConfig configures the stale alert sweep cadence and threshold. A
// zero or negative threshold disables the sweep.
func (s *InvestigationScheduler) SetStaleConfig(threshold, interval time.Duration) {
	if threshold <= 0 {
		return
	}
	s.staleThreshold = threshold
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	s.staleInterval = interval
}

func (s *InvestigationScheduler) SetDataRetention(days int, interval time.Duration) {
	if days <= 0 {
		return
	}
	s.dataRetentionDays = days
	if interval <= 0 {
		interval = time.Hour
	}
	s.pruneInterval = interval
}

// SetSLAPublisher wires the RabbitMQ publisher used by the SLA sweep loop.
// Without a publisher the sweep never starts (no external producer is
// assumed — see the DT-E1 decision in spec 05_incidents/04).
func (s *InvestigationScheduler) SetSLAPublisher(p slaSweepPublisher) {
	s.slaSweepPublisher = p
}

// SetSLASweepInterval configures how often the leader publishes an SLA sweep
// request tick. Values <= 0 disable publication; positive values below 5s are
// clamped to 5s so a misconfigured interval cannot churn the queue.
func (s *InvestigationScheduler) SetSLASweepInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	s.slaSweepInterval = max(d, 5*time.Second)
}

func NewInvestigationScheduler(
	alertInvestigationStore store.AlertInvestigationStore,
	agentTokenStore store.AgentTokenStore,
	resolver AgentResolver,
	maxConcurrent int,
) *InvestigationScheduler {
	mc := maxConcurrent
	mc = max(mc, 1)
	return &InvestigationScheduler{
		alertInvestigationStore: alertInvestigationStore,
		agentTokenStore:         agentTokenStore,
		resolver:                resolver,
		maxConcurrent:           mc,
		disconnectGrace:         defaultDisconnectGrace,
		notify:                  make(chan struct{}, 1),
		stopCh:                  make(chan struct{}),
		backoff:                 make(map[string]time.Time),
		dispatchAttempts:        make(map[string]dispatchAttempt),
		healthTracker:           NewAgentHealthTracker(50),
	}
}

func (s *InvestigationScheduler) Start() {
	s.wg.Add(1)
	go s.run()
	if s.alertStore != nil && s.stalePublisher != nil && s.staleThreshold > 0 {
		s.wg.Add(1)
		go s.runStaleSweep()
	}
	if s.slaSweepPublisher != nil && s.slaSweepInterval > 0 {
		s.wg.Add(1)
		go s.runSLASweep()
	}
	if s.alertStore != nil && s.dataRetentionDays > 0 {
		s.wg.Add(1)
		go s.runPrune()
	}
	s.wg.Add(1)
	go s.runMapPurge()
	if s.handoffDetector != nil && s.notifyPublisher != nil {
		s.wg.Add(1)
		go s.runHandoffTick()
	}
	if s.summaryEnabled && s.incidentStore != nil && s.incidentChannelMgr != nil {
		s.wg.Add(1)
		go s.runSummarySweep()
	}
	if s.incidentStore != nil && s.incidentInvestigationStore != nil {
		s.wg.Add(1)
		go s.runIncidentSweep()
	}
	if s.coordinationTaskStore != nil {
		s.wg.Add(1)
		go s.runCoordinationTaskSweep()
	}
	logger.Info("Investigation scheduler started", "component", "scheduler", "max_concurrent", s.maxConcurrent, "leader", s.leader != nil, "stale_sweep", s.staleThreshold > 0, "sla_sweep", s.slaSweepPublisher != nil && s.slaSweepInterval > 0, "incident_sweep", s.incidentStore != nil, "summary_sweep", s.summaryEnabled)
}

func (s *InvestigationScheduler) Stop() {
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		close(s.stopCh)
	})
	s.wg.Wait()
	if s.leader != nil {
		s.leader.Release(context.Background())
	}
	logger.Info("Investigation scheduler stopped", "component", "scheduler")
}

func (s *InvestigationScheduler) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-s.notify:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "notify", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.tick(context.Background())
			}()
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "ticker", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.tick(context.Background())
			}()
		}
	}
}

// tick is one scheduling pass. When leader-election is configured, only
// the lease holder does scheduling work; other replicas just renew the
// lease and exit fast. Single-replica deployments skip the lease entirely.
func (s *InvestigationScheduler) tick(ctx context.Context) {
	start := time.Now()
	defer func() {
		metrics.SchedulerTickTotal.Add(1)
		metrics.SchedulerTickDurationMs.Set(time.Since(start).Milliseconds())
	}()

	// Refresh online-agent count at the top of every tick so the metric
	// doesn't go stale during quiet periods when schedule() short-circuits.
	// When Valkey presence is unavailable, the existing schedule()-time
	// path still updates the metric via filterOnlineAgents.
	if s.presence != nil && s.presence.Available() {
		if ids, err := s.presence.ListOnlineAgents(ctx); err == nil {
			metrics.SchedulerOnlineAgents.Set(int64(len(ids)))
		}
	}

	if !s.acquireLeadership(ctx) {
		metrics.SchedulerIsLeader.Set(0)
		return
	}
	metrics.SchedulerIsLeader.Set(1)

	s.schedule(ctx)
	s.scheduleIncidentInvestigations(ctx)
	s.scheduleCoordinationTasks(ctx)

	s.agentRoleTickCounter++
	if s.agentRoleTickCounter >= 3 {
		s.agentRoleTickCounter = 0
		s.tickAgentRoleAssignment(ctx)
	}
}

// acquireLeadership returns true when this replica owns the lease (or no
// lease was configured). It tries renewal first and only falls back to a
// fresh acquire if renewal failed (e.g. lease expired during a long pause).
func (s *InvestigationScheduler) acquireLeadership(ctx context.Context) bool {
	if s.leader == nil {
		return true
	}
	if s.leader.IsLeader() {
		ok, err := s.leader.Renew(ctx)
		if err != nil {
			logger.Warn("Scheduler leader renew failed; will retry acquire", "component", "scheduler", "error", err)
		}
		if ok {
			return true
		}
	}
	ok, err := s.leader.Acquire(ctx)
	if err != nil {
		logger.Warn("Scheduler leader acquire failed; skipping tick", "component", "scheduler", "error", err)
		return false
	}
	return ok
}
