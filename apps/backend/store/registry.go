package store

import (
	"time"

	"alga/pgclient"
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
	Counter               CounterStore
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
}

func NewStores(cli *pgclient.Client, sessionExpiry, sessionMaxLifetime time.Duration) (*Stores, error) {
	client := cli.Ent

	actionItemStore := newPGActionItemStore(client)
	postmortemStore := newPGPostMortemStore(client, actionItemStore)
	deliveryStore := newPGNotificationDeliveryStore(client)
	personalAccessTokenStore := newPGPersonalAccessTokenStore(client)

	return &Stores{
		Alert:                 newPGAlertStore(client),
		WebhookToken:          newPGWebhookTokenStore(client),
		User:                  newPGUserStore(client),
		Session:               newPGSessionStore(client, sessionExpiry, sessionMaxLifetime),
		Audit:                 newPGAuditStore(client),
		Integration:           newPGIntegrationStore(client),
		RouteRules:            newPGRouteRulesStore(client),
		InvestigationThread:   newPGInvestigationThreadStore(client),
		AlertInvestigation:    newPGAlertInvestigationStore(client),
		IncidentInvestigation: newPGIncidentInvestigationStore(client),
		AgentToken:            newPGAgentTokenStore(client),
		AgentDM:               newPGAgentDMStore(client),
		Notification:          newPGNotificationStore(client),
		Incident:              newPGIncidentStore(client),
		IncidentCoordination:  newPGIncidentCoordinationStore(client),
		CoordinationTask:      newPGCoordinationTaskStore(client),
		Service:               newPGServiceStore(client),
		Team:                  newPGTeamStore(client),
		Escalation:            newPGEscalationStore(client),
		OnCall:                newPGOnCallStore(client),
		PostMortem:            postmortemStore,
		ActionItem:            actionItemStore,
		Delivery:              deliveryStore,
		PersonalToken:         personalAccessTokenStore,
		Dashboard:             newPGDashboardStore(client),
		Knowledge:             newPGKnowledgeStore(client, cli.DB),
		AgentAsk:              newPGAgentAskStore(client),
		SystemConfig:          newPGSystemConfigStore(client),
		AgentMemory:           newPGAgentMemoryStore(client, cli.DB),
		MaintenanceWindow:     newPGMaintenanceWindowStore(client),
		TriageResult:          newPGTriageResultStore(client),
		TriageRule:            newPGTriageRuleStore(client),
		Counter:               newPGCounterStore(client),
		ICSRole:               newPGICSRoleStore(client),
		IncidentDocument:      newPGIncidentDocumentStore(client),
		Handoff:               newPGHandoffStore(client),
		Playbook:              newPGPlaybookStore(client),
		Heartbeat:             newPGHeartbeatStore(client),
		StatusPage:            newPGStatusPageStore(client),
		OIDCProvider:          newPGOIDCProviderStore(client),
		OIDCIdentity:          newPGOIDCIdentityStore(client),
		CredentialProvider:    newPGCredentialProviderStore(client),
		SharedSecret:          newPGSharedSecretStore(client),
		Outbox:                newPGOutboxStore(client),
	}, nil
}
