package agent

import (
	"context"

	"github.com/google/uuid"

	"alga/oncall"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
	"alga/webhook"
)

func (e *AgentToolExecutor) SetThreadStore(ts store.InvestigationThreadStore) {
	e.threadStore = ts
}

func (e *AgentToolExecutor) SetAlertSideEffects(se *AgentAlertSideEffects) {
	e.alertSideEffects = se
}

func (e *AgentToolExecutor) SetSSEBroker(broker *sse.Broker, vkClient *valkey.Client) {
	e.sseBroker = broker
	e.vkClient = vkClient
	if broker != nil {
		e.ssePublisher = &sse.DualPublisher{Broker: broker, VKClient: vkClient}
	}
}

func (e *AgentToolExecutor) SetAuditStore(as store.AuditStore) {
	e.auditStore = as
}

func (e *AgentToolExecutor) SetInvestigationForwarder(f webhook.InvestigationAgentForwarder) {
	e.investigationForwarder = f
}

func (e *AgentToolExecutor) SetPendingNotifier(n pendingNotifier) {
	e.pendingNotifier = n
}

func (e *AgentToolExecutor) SetMemoryExtractor(m memoryExtractor) {
	e.memoryExtractor = m
}

func (e *AgentToolExecutor) SetNotificationStore(ns store.NotificationStore) {
	e.notificationStore = ns
}

func (e *AgentToolExecutor) SetUserStore(us store.UserStore) {
	e.userStore = us
}

func (e *AgentToolExecutor) SetTriageResultStore(st store.TriageResultStore) {
	e.triageResultStore = st
}

func (e *AgentToolExecutor) SetIncidentStore(st store.IncidentStore) {
	e.incidentStore = st
}

func (e *AgentToolExecutor) SetIncidentInvestigationStore(st store.IncidentInvestigationStore) {
	e.incidentInvestigationStore = st
}

func (e *AgentToolExecutor) SetIncidentCoordinationStore(st store.IncidentCoordinationStore) {
	e.incidentCoordinationStore = st
}

func (e *AgentToolExecutor) SetPostMortemStore(st store.PostMortemStore) {
	e.postmortemStore = st
}

func (e *AgentToolExecutor) SetServiceStore(st store.ServiceStore) {
	e.serviceStore = st
}

func (e *AgentToolExecutor) SetEscalationStore(st store.EscalationStore) {
	e.escalationStore = st
}

func (e *AgentToolExecutor) SetOnCallResolver(r *oncall.Resolver) {
	e.onCallResolver = r
}

func (e *AgentToolExecutor) SetICSRoleStore(st store.ICSRoleStore) {
	e.icsRoleStore = st
}

func (e *AgentToolExecutor) SetIncidentDocumentStore(st store.IncidentDocumentStore) {
	e.incidentDocumentStore = st
}

func (e *AgentToolExecutor) SetEscalationPublisher(p *rabbitmq.Publisher) {
	e.escalationPublisher = p
}

func (e *AgentToolExecutor) SetAlertInvestigationLifecycleService(svc AlertInvestigationLifecycle) {
	e.lifecycle = svc
}

// SetAlertCascade wires the cross-domain alert-cascade runner (api.runAlertCascade).
// Injected so the agent package does not import package api.
func (e *AgentToolExecutor) SetAlertCascade(fn func(ctx context.Context, alertStore store.Store, auditStore store.AuditStore, publisher *sse.DualPublisher, incidentNumber int64, agentID uuid.UUID, agentName string) store.AlertCascadeResult) {
	e.runAlertCascadeFn = fn
}

// SetPostMortemBuilder wires the cross-domain post-mortem draft builder
// (api.buildPostMortemDraft). Injected so the agent package does not import
// package api.
func (e *AgentToolExecutor) SetPostMortemBuilder(fn func(ctx context.Context, documentStore store.IncidentDocumentStore, coordinationStore store.IncidentCoordinationStore, incidentStore store.IncidentStore, alertStore store.Store, inc *store.IncidentRecord, summary string) *store.PostMortemRecord) {
	e.buildPostMortemDraftFn = fn
}

// SetCoordinationForwarder wires the cross-domain coordination-update forwarder
// (api coordination helpers). Injected so the agent package does not import
// package api.
func (e *AgentToolExecutor) SetCoordinationForwarder(fn func(ctx context.Context, incidentNumber int64, messageText string, mentions []string, agentRec *store.AgentTokenRecord)) {
	e.forwardCoordinationUpdateFn = fn
}
