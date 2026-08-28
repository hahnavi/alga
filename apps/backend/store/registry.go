package store

import (
	"time"

	"alga/db"
)

type Stores struct {
	Alert                 Store
	WebhookToken          WebhookTokenStore
	User                  UserStore
	Session               SessionStore
	Audit                 AuditStore
	Integration           IntegrationStore
	RouteRules            RouteRulesStore
	InvestigationThread   InvestigationThreadStore
	AlertInvestigation    AlertInvestigationStore
	IncidentInvestigation IncidentInvestigationStore
	AgentToken            AgentTokenStore
	AgentDM               AgentDMStore
	Notification          NotificationStore
	Incident              IncidentStore
	IncidentCoordination  IncidentCoordinationStore
	CoordinationTask      CoordinationTaskStore
	Service               ServiceStore
	Team                  TeamStore
	Escalation            EscalationStore
	OnCall                OnCallStore
	PostMortem            PostMortemStore
	ActionItem            ActionItemStore
	Delivery              NotificationDeliveryStore
	PersonalToken         PersonalAccessTokenStore
	Dashboard             DashboardStore
	Knowledge             KnowledgeStore
	AgentAsk              AgentAskStore
	SystemConfig          SystemConfigStore
	AgentMemory           AgentMemoryStore
	MaintenanceWindow     MaintenanceWindowStore
	TriageResult          TriageResultStore
	TriageRule            TriageRuleStore
	ICSRole               ICSRoleStore
	IncidentDocument      IncidentDocumentStore
	Handoff               HandoffStore
	Playbook              PlaybookStore
	Heartbeat             HeartbeatStore
	StatusPage            StatusPageStore
	OIDCProvider          OIDCProviderStore
	OIDCIdentity          OIDCIdentityStore
	CredentialProvider    CredentialProviderStore
	SharedSecret          SharedSecretStore
	Outbox                OutboxStore
	PasswordReset         PasswordResetStore
}

func NewStores(cli *db.Client, sessionExpiry, sessionMaxLifetime time.Duration) (*Stores, error) {
	bunDB := cli.DB

	actionItemStore := newPGActionItemStore(bunDB)
	postmortemStore := newPGPostMortemStore(bunDB, actionItemStore)
	deliveryStore := newPGNotificationDeliveryStore(bunDB)
	personalAccessTokenStore := newPGPersonalAccessTokenStore(bunDB)

	return &Stores{
		Alert:                 newPGAlertStore(bunDB),
		WebhookToken:          newPGWebhookTokenStore(bunDB),
		User:                  newPGUserStore(bunDB),
		Session:               newPGSessionStore(bunDB, sessionExpiry, sessionMaxLifetime),
		Audit:                 newPGAuditStore(bunDB),
		Integration:           newPGIntegrationStore(bunDB),
		RouteRules:            newPGRouteRulesStore(bunDB),
		InvestigationThread:   newPGInvestigationThreadStore(bunDB),
		AlertInvestigation:    newPGAlertInvestigationStore(bunDB),
		IncidentInvestigation: newPGIncidentInvestigationStore(bunDB),
		AgentToken:            newPGAgentTokenStore(bunDB),
		AgentDM:               newPGAgentDMStore(bunDB),
		Notification:          newPGNotificationStore(bunDB),
		Incident:              newPGIncidentStore(bunDB),
		IncidentCoordination:  newPGIncidentCoordinationStore(bunDB),
		CoordinationTask:      newPGCoordinationTaskStore(bunDB),
		Service:               newPGServiceStore(bunDB),
		Team:                  newPGTeamStore(bunDB),
		Escalation:            newPGEscalationStore(bunDB),
		OnCall:                newPGOnCallStore(bunDB),
		PostMortem:            postmortemStore,
		ActionItem:            actionItemStore,
		Delivery:              deliveryStore,
		PersonalToken:         personalAccessTokenStore,
		Dashboard:             newPGDashboardStore(bunDB),
		Knowledge:             newPGKnowledgeStore(bunDB),
		AgentAsk:              newPGAgentAskStore(bunDB),
		SystemConfig:          newPGSystemConfigStore(bunDB),
		AgentMemory:           newPGAgentMemoryStore(bunDB),
		MaintenanceWindow:     newPGMaintenanceWindowStore(bunDB),
		TriageResult:          newPGTriageResultStore(bunDB),
		TriageRule:            newPGTriageRuleStore(bunDB),
		ICSRole:               newPGICSRoleStore(bunDB),
		IncidentDocument:      newPGIncidentDocumentStore(bunDB),
		Handoff:               newPGHandoffStore(bunDB),
		Playbook:              newPGPlaybookStore(bunDB),
		Heartbeat:             newPGHeartbeatStore(bunDB),
		StatusPage:            newPGStatusPageStore(bunDB),
		OIDCProvider:          newPGOIDCProviderStore(bunDB),
		OIDCIdentity:          newPGOIDCIdentityStore(bunDB),
		CredentialProvider:    newPGCredentialProviderStore(bunDB),
		SharedSecret:          newPGSharedSecretStore(bunDB),
		Outbox:                newPGOutboxStore(bunDB),
		PasswordReset:         newPGPasswordResetStore(bunDB),
	}, nil
}
