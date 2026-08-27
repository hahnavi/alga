package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/google/uuid"

	"alga/api"
	"alga/api/agent"
	"alga/api/platform"
	"alga/config"
	"alga/correlator"
	algacrypto "alga/crypto"
	"alga/db"
	"alga/email"
	"alga/escalation"
	"alga/ics"
	"alga/incidentchannel"
	"alga/knowledge"
	"alga/logger"
	"alga/mattermost"
	"alga/memory"
	"alga/notification"
	"alga/oncall"
	"alga/prompt"
	"alga/rabbitmq"
	"alga/routing"
	"alga/secretprovider"
	"alga/servicetracker"
	"alga/slack"
	"alga/sse"
	"alga/store"
	"alga/telnyx"
	"alga/trace"
	"alga/triage"
	"alga/twilio"
	"alga/valkey"
	"alga/webhook"
	"alga/worker"
)

func redactURI(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "xxxxx"
	}
	return u.Redacted()
}

type alertEventPublisher struct {
	broker   *sse.Broker
	vkClient *valkey.Client
}

func (p *alertEventPublisher) PublishAlertEvent(action string, record store.AlertRecord) {
	event := sse.Event{
		Type: action,
		Data: record,
	}
	p.broker.Publish(event)
	if p.vkClient != nil {
		if err := sse.PublishToValkey(context.Background(), p.vkClient.Client(), event); err != nil {
			logger.Error("Failed to publish alert event to Valkey", "error", err)
		}
	}
}

func (a *App) wire() error {
	var err error

	if err := a.cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Install the global OpenTelemetry TracerProvider + W3C propagator. When
	// tracing is disabled (the default: no ALGA_OTEL_ENABLED / OTLP endpoint),
	// trace.Init installs a noop provider and returns a no-op shutdown, so this
	// is zero-cost and silent in local dev and tests (W1).
	otelShutdown, err := trace.Init(a.cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize tracing: %w", err)
	}
	a.otelShutdown = otelShutdown

	if a.cfg.PostgresAutoMigrate {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := db.ApplyMigrations(ctx, a.cfg.PostgresDSN); err != nil {
			cancel()
			return fmt.Errorf("failed to run Postgres startup migrations: %w", err)
		}
		cancel()
	}

	logger.InitWithFormat(a.cfg.LogLevel, a.cfg.LogFormat, a.cfg.LogFile)
	logger.Info("Alga backend starting", "env", a.cfg.Environment, "log_level", a.cfg.LogLevel)

	a.valkeyClient, err = valkey.NewClient(a.cfg.ValkeyAddr, a.cfg.ValkeyPassword, a.cfg.ValkeyDB)
	if err != nil {
		return fmt.Errorf("failed to connect to Valkey: %w", err)
	}
	if a.valkeyClient != nil {
		if err := a.valkeyClient.Ping(context.Background()); err != nil {
			logger.Warn("Valkey ping failed", "error", err)
		} else {
			logger.Info("Connected to Valkey", "addr", a.cfg.ValkeyAddr)
		}
	}

	a.cache = valkey.NewCache(a.valkeyClient)
	a.cancelSet = valkey.NewCancelSet(a.valkeyClient, a.cfg.CancelSetTTL)
	if a.cfg.IdempotencyEnabled && a.valkeyClient != nil {
		a.idempotency = valkey.NewIdempotencyCache(a.valkeyClient)
		logger.Info("Idempotency-Key replay enabled", "ttl", a.cfg.IdempotencyTTL)
	}

	replicaID, herr := os.Hostname()
	if herr != nil || replicaID == "" {
		replicaID = fmt.Sprintf("alga-%d", time.Now().UnixNano())
	}

	presence := valkey.NewPresence(a.valkeyClient, a.cfg.AgentPresenceTTL, replicaID)
	leaderLease := valkey.NewLeaderLease(a.valkeyClient, "alga:scheduler:leader", a.cfg.SchedulerLeaderTTL)

	a.rabbitClient, err = rabbitmq.NewClient(a.cfg.RabbitMQURI)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	if a.rabbitClient != nil {
		logger.Info("Connected to RabbitMQ", "addr", redactURI(a.cfg.RabbitMQURI))
	}

	sessionExpiry := time.Duration(a.cfg.SessionExpiryHrs) * time.Hour
	// Absolute (max) session lifetime (ASVS V3.2/V3.3), independent of the
	// sliding idle expiry. Defaults to 12h; see config.SessionMaxLifetime.
	sessionMaxLifetime := a.cfg.SessionMaxLifetime

	var memorySvc memory.Service

	a.dbCli, err = db.NewWithPool(a.cfg.PostgresDSN, db.PoolConfig{
		MaxOpenConns:    a.cfg.PostgresMaxOpenConns,
		MaxIdleConns:    a.cfg.PostgresMaxIdleConns,
		ConnMaxIdleTime: a.cfg.PostgresConnMaxIdleTime,
		ConnMaxLifetime: a.cfg.PostgresConnMaxLifetime,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Postgres: %w", err)
	}
	logger.Info("Connected to Postgres", "addr", redactURI(a.cfg.PostgresDSN))

	a.stores, err = store.NewStores(a.dbCli, sessionExpiry, sessionMaxLifetime)
	if err != nil {
		return fmt.Errorf("failed to initialize Postgres stores: %w", err)
	}

	onCallResolver := oncall.NewResolver(a.stores.OnCall)
	handoffDetector := oncall.NewHandoffDetector(a.stores.OnCall, a.stores.Handoff, a.valkeyClient, onCallResolver)

	policyEngine := escalation.NewPolicyEngineWithOpsTeam(a.stores.Escalation, a.stores.OnCall, onCallResolver, a.stores.Team, a.cfg.OpsTeamName)

	a.loadStoredIntegrations()
	loadedSysCfg := a.loadStoredSystemConfig()

	var sessionStore store.SessionStore
	if a.valkeyClient != nil {
		sessionStore = valkey.NewSessionStore(a.valkeyClient, sessionExpiry, sessionMaxLifetime)
		logger.Info("Using Valkey for session storage")
	} else {
		sessionStore = a.stores.Session
	}

	seedCtx, seedCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := a.stores.CredentialProvider.SeedDefaultInternalProvider(seedCtx); err != nil {
		logger.Warn("Failed to seed default credential provider", "error", err)
	}
	seedCancel()

	// Seed ops-team with any existing admin; setup wizard will create the first admin.
	usersList, uErr := a.stores.User.ListUsers()
	if uErr != nil {
		logger.Warn("Failed to list users for ops-team seed", "error", uErr)
	} else {
		for _, u := range usersList {
			if u.Role == "admin" {
				if err := a.stores.Team.SeedOpsTeam(context.Background(), u.ID); err != nil {
					logger.Warn("Failed to seed ops-team team", "error", err)
				}
				seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := seedOpsTeamEscalation(seedCtx, a.stores, a.cfg.OpsTeamName); err != nil {
					logger.Warn("Failed to seed ops-team escalation policy", "error", err)
				}
				seedCancel()
				break
			}
		}
	}

	mmClient := mattermost.NewClient(a.cfg.MattermostURL, a.cfg.MattermostWebhookSecret, a.cfg.MattermostTeam)
	mmClient.SetDisabled(a.cfg.MattermostDisabled)
	slackClient := slack.NewClient(a.cfg.SlackBotToken)
	// Only one voice provider is active at a time. The inactive provider's
	// client is left nil so its callback route is not registered and it cannot
	// be used by the dispatcher. Switching providers requires a restart.
	var twilioClient *twilio.Client
	var telnyxClient *telnyx.Client
	if strings.EqualFold(a.cfg.VoiceProvider, "telnyx") {
		telnyxClient = telnyx.NewClient(a.cfg.TelnyxAPIKey, a.cfg.TelnyxConnectionID, a.cfg.TelnyxFromNumber, a.cfg.TelnyxTTSVoice, a.cfg.TelnyxTTSLanguage, a.cfg.TelnyxTTSAPIKeyRef)
		telnyxClient.SetDisabled(a.cfg.TelnyxDisabled)
		telnyxClient.SetCallbackBaseURL(a.cfg.AlgaBaseURL)
		if a.cfg.TelnyxPublicKey != "" {
			if err := telnyxClient.SetPublicKey(a.cfg.TelnyxPublicKey); err != nil {
				logger.Warn("Failed to configure Telnyx webhook public key", "component", "telnyx", "error", err)
			}
		}
	} else {
		twilioClient = twilio.NewClient(a.cfg.TwilioAccountSID, a.cfg.TwilioAuthToken, a.cfg.TwilioFromNumber)
		twilioClient.SetDisabled(a.cfg.TwilioDisabled)
		twilioClient.SetCallbackBaseURL(a.cfg.AlgaBaseURL)
	}
	var activeVoice notification.VoiceProvider = twilioClient
	if telnyxClient != nil {
		activeVoice = telnyxClient
	}

	routes, err := a.stores.RouteRules.Get()
	if err != nil {
		return fmt.Errorf("failed to load route rules: %w", err)
	}
	routingEngine := routing.NewEngine(routes)

	var routingDefaults []routing.Destination
	if a.cfg.MattermostDefaultChannel != "" && mmClient.Enabled() {
		routingDefaults = append(routingDefaults, routing.Destination{Provider: "mattermost", Channel: a.cfg.MattermostDefaultChannel})
	}
	if a.cfg.SlackDefaultChannel != "" {
		routingDefaults = append(routingDefaults, routing.Destination{Provider: "slack", Channel: a.cfg.SlackDefaultChannel})
	}
	routingEngine.SetDefaults(routingDefaults)

	var dedupCache webhook.DedupCache
	if a.valkeyClient != nil {
		dedupCache = valkey.NewDedupCache(a.valkeyClient)
	}

	whReceiver := webhook.NewReceiver(routingEngine, mmClient, slackClient, a.stores.Alert, a.stores.WebhookToken, dedupCache, a.cfg.WebhookAllowQueryToken)
	if a.idempotency != nil {
		whReceiver.SetIdempotency(a.idempotency, a.cfg.IdempotencyTTL)
	}
	mux := whReceiver.Router()

	mmWebhookHandler := webhook.NewMattermostWebhookHandler(a.stores.AlertInvestigation, a.stores.Audit, a.cfg.MattermostWebhookSecret)
	mux.HandleFunc("/webhooks/mattermost", mmWebhookHandler.ServeHTTP)

	slackWebhookHandler := webhook.NewSlackWebhookHandler(a.stores.AlertInvestigation, a.stores.Audit, a.cfg.SlackSigningSecret)
	slackWebhookHandler.SetSlackClient(slackClient)
	mux.HandleFunc("/webhooks/slack", slackWebhookHandler.ServeHTTP)

	if mmClient.Enabled() {
		if botName, err := mmClient.GetUsername(context.Background()); err == nil && botName != "" { // startup: no caller ctx
			mmWebhookHandler.AddBotUsername(botName)
			logger.Info("Registered Mattermost bot username for webhook dedup", "bot_name", botName)
		} else if err != nil {
			logger.Warn("Failed to fetch Mattermost bot username", "error", err)
		}
	}

	if slackClient.Enabled() {
		if botID, err := slackClient.GetBotUserID(context.Background()); err == nil && botID != "" { // startup: no caller ctx
			slackWebhookHandler.AddBotUserID(botID)
			logger.Info("Registered Slack bot user ID for webhook dedup", "bot_id", botID)
		} else if err != nil {
			logger.Warn("Failed to fetch Slack bot user ID", "error", err)
		}
	}

	agentChatRouter := whReceiver.ChatRouter()
	agentExecutor := agent.NewAgentToolExecutor(a.stores.AlertInvestigation, mmClient, slackClient, a.stores.AgentDM, agentChatRouter)

	a.sseBroker = sse.NewBroker()
	if a.valkeyClient != nil {
		a.sseBroker.StartValkeySubscription(a.shutdownCtx, a.valkeyClient.Client())
		a.sseBroker.StartValkeyUserSubscription(a.shutdownCtx, a.valkeyClient.Client())
		a.sseBroker.StartValkeyAgentSubscription(a.shutdownCtx, a.valkeyClient.Client())
	}
	whReceiver.SetEventPublisher(&alertEventPublisher{broker: a.sseBroker, vkClient: a.valkeyClient})

	mmWebhookHandler.SetSSEBroker(a.sseBroker, a.valkeyClient)
	slackWebhookHandler.SetSSEBroker(a.sseBroker, a.valkeyClient)
	slackWebhookHandler.SetIncidentLookupStore(a.stores.Incident)
	slackWebhookHandler.SetIncidentCoordinationStore(a.stores.IncidentCoordination)
	slackWebhookHandler.SetEscalationTimeline(a.stores.Incident)
	agentExecutor.SetSSEBroker(a.sseBroker, a.valkeyClient)
	whReceiver.SetSSEBroker(a.sseBroker, a.valkeyClient)

	alertPub := &alertEventPublisher{broker: a.sseBroker, vkClient: a.valkeyClient}
	slackWebhookHandler.SetAlertStore(a.stores.Alert, alertPub, dedupCache)
	agentExecutor.SetAlertSideEffects(&agent.AgentAlertSideEffects{
		Store: a.stores.Alert,
		Publish: func(rec *store.AlertRecord) {
			alertPub.PublishAlertEvent("alert_updated", *rec)
		},
		Dedup: dedupCache,
		SyncChat: func(ctx context.Context, rec *store.AlertRecord) {
			alert := webhook.AlertFromStoreRecord(rec)
			if err := webhook.UpdateChatPostsForAlert(ctx, whReceiver.ChatRouter(), rec, alert); err != nil {
				logger.Error("Failed to sync chat posts for alert", "component", "wire", "fingerprint", rec.Fingerprint, "error", err)
			}
		},
	})

	agentExecutor.SetAuditStore(a.stores.Audit)
	agentExecutor.SetNotificationStore(a.stores.Notification)
	agentExecutor.SetUserStore(a.stores.User)
	agentExecutor.SetIncidentStore(a.stores.Incident)
	agentExecutor.SetIncidentInvestigationStore(a.stores.IncidentInvestigation)
	agentExecutor.SetIncidentCoordinationStore(a.stores.IncidentCoordination)
	agentExecutor.SetCoordinationTaskStore(a.stores.CoordinationTask)
	agentExecutor.SetPostMortemStore(a.stores.PostMortem)
	agentExecutor.SetServiceStore(a.stores.Service)
	agentExecutor.SetEscalationStore(a.stores.Escalation)
	agentExecutor.SetOnCallResolver(onCallResolver)
	agentExecutor.SetICSRoleStore(a.stores.ICSRole)
	agentExecutor.SetIncidentDocumentStore(a.stores.IncidentDocument)
	if memorySvc != nil {
		agentExecutor.SetMemoryExtractor(memorySvc)
	}

	agentSSE := agent.NewAgentSSEHandler(a.sseBroker, a.valkeyClient, presence, a.stores.AgentToken, agentExecutor)
	agentSSE.SetAllowQueryToken(a.cfg.AgentSSEAllowQueryToken)
	if o := strings.TrimSpace(a.cfg.AgentSSEAllowedOrigins); o != "" {
		var origins []string
		for _, part := range strings.Split(o, ",") {
			if p := strings.TrimSpace(part); p != "" {
				origins = append(origins, p)
			}
		}
		if len(origins) > 0 {
			agentSSE.SetAllowedOrigins(origins)
			logger.Info("Agent SSE Origin allowlist enabled", "origins", len(origins))
		}
	}

	invForwarder := &api.DefaultInvestigationForwarder{
		AgentTokens:                a.stores.AgentToken,
		AgentSSE:                   agentSSE,
		Presence:                   presence,
		AlertInvestigationStore:    a.stores.AlertInvestigation,
		IncidentInvestigationStore: a.stores.IncidentInvestigation,
		IncidentStore:              a.stores.Incident,
	}
	mmWebhookHandler.SetInvestigationForwarder(invForwarder)
	slackWebhookHandler.SetInvestigationForwarder(invForwarder)
	agentExecutor.SetInvestigationForwarder(invForwarder)
	whReceiver.SetInvestigationForwarder(invForwarder)

	mux.HandleFunc("/api/v1/events", api.AuthMiddlewareSSE(a.sseBroker.Handler(), sessionStore, a.stores.User))
	logger.Info("SSE real-time updates enabled")

	var publisher *rabbitmq.Publisher
	var channelManager *incidentchannel.Manager
	if a.rabbitClient != nil {
		publisher, err = rabbitmq.NewPublisher(a.rabbitClient)
		if err != nil {
			logger.Warn("Failed to create RabbitMQ publisher", "error", err)
		} else {
			whReceiver.SetPublisher(publisher)
			// W6: route alert ingestion through the transactional outbox so
			// events survive a broker outage. Only enabled when the publisher
			// exists (otherwise the legacy direct-publish fallback is used).
			if a.stores.Outbox != nil {
				whReceiver.SetOutboxStore(a.stores.Outbox)
			}
			logger.Info("Async alert processing via RabbitMQ enabled")
		}

		a.workerSet, err = worker.NewWorkerSet(a.rabbitClient)
		if err != nil {
			logger.Warn("Failed to create worker set", "error", err)
			a.workerSet = nil
		}
	}

	if a.workerSet != nil {
		a.workerSet.SetAlertWorker(worker.NewAlertWorker(whReceiver, a.stores.Alert, publisher))
	}

	whReceiver.SetAlertInvestigationStore(a.stores.AlertInvestigation)
	whReceiver.SetIncidentInvestigationStore(a.stores.IncidentInvestigation)
	if a.rabbitClient != nil && a.workerSet != nil && a.valkeyClient != nil && publisher != nil {
		a.correlator = correlator.NewCorrelator(a.valkeyClient, publisher, correlator.Config{
			Window:      a.cfg.CorrelationWindow,
			CooldownTTL: a.cfg.CorrelationCooldownTTL,
		})
		a.correlator.SetAlertInvestigationStore(a.stores.AlertInvestigation)
		a.correlator.SetAgentNotifier(invForwarder)
		a.correlator.SetTriageEnabled(a.cfg.TriageEnabled)
		whReceiver.SetCorrelator(a.correlator)
		logger.Info("SRE Agent correlation enabled", "cooldown", a.cfg.CorrelationCooldownTTL.String())
	}

	var alertLifecycle *api.AlertInvestigationLifecycleService

	if a.rabbitClient != nil && a.workerSet != nil {
		invWorker := worker.NewInvestigateWorker(
			mmClient,
			slackClient,
			a.stores.Alert,
			a.stores.AlertInvestigation,
			a.sseBroker,
			worker.InvestigateConfig{
				InvestigationTimeout:        a.cfg.InvestigationTimeout,
				InvestigationChannel:        a.cfg.InvestigationChannel,
				MaxConcurrentInvestigations: a.cfg.MaxConcurrentInvestigations,
				CriticalSeverityLabels:      a.cfg.CriticalSeverityLabels,
			},
		)
		invWorker.SetValkeyClient(a.valkeyClient)
		invWorker.SetCancelSet(a.cancelSet)

		knowledgeAggregator := knowledge.NewAggregator(
			knowledge.NewEpisodicFinder(a.stores.AlertInvestigation),
			knowledge.DefaultConfig(),
		).
			WithNotes(knowledge.NewNotesFinder(a.stores.Knowledge)).
			WithConcurrent(knowledge.NewConcurrentFinder(a.valkeyClient))

		if a.cfg.MemoryEnabled && a.stores.AgentMemory != nil {
			var embedder memory.Embedder
			var llm memory.LLM
			if a.cfg.MemoryEmbeddingURL != "" {
				e, eerr := memory.NewOpenAIEmbedder(a.cfg.MemoryEmbeddingURL, a.cfg.MemoryEmbeddingAPIKey, a.cfg.MemoryEmbeddingModel)
				if eerr != nil {
					return fmt.Errorf("invalid MEMORY_EMBEDDING_MODEL: %w", eerr)
				}
				embedder = e
			} else {
				embedder = memory.NewNoopEmbedder()
			}
			if a.cfg.MemoryLLMURL != "" {
				llm = memory.NewOpenAILLM(a.cfg.MemoryLLMURL, a.cfg.MemoryLLMAPIKey, a.cfg.MemoryLLMModel)
			}
			autoExtract := a.cfg.MemoryAutoExtract
			if !autoExtract && a.cfg.MemoryLLMURL != "" {
				autoExtract = true
			}
			var extractor *memory.Extractor
			if llm != nil {
				extractor = memory.NewExtractor(llm, embedder, a.stores.AgentMemory)
			}
			memorySvc = memory.NewService(a.stores.AgentMemory, extractor, embedder, a.cfg.MemoryMaxPerInvestigation, autoExtract)
			knowledgeAggregator = knowledgeAggregator.WithMemory(knowledge.NewMemoryFinder(memorySvc))
			logger.Info("Shared agent memory enabled", "auto_extract", autoExtract, "embedding_model", a.cfg.MemoryEmbeddingModel)
		}

		channelManager = incidentchannel.NewManager(slackClient, a.stores.Incident, a.stores.User, a.cfg.AlgaBaseURL)
		channelManager.SetOnCallResolver(&incidentOnCallResolver{
			serviceStore:    a.stores.Service,
			escalationStore: a.stores.Escalation,
			onCallStore:     a.stores.OnCall,
			resolver:        onCallResolver,
		})

		a.scheduler = worker.NewInvestigationScheduler(
			a.stores.AlertInvestigation,
			a.stores.AgentToken,
			invForwarder,
			a.cfg.MaxConcurrentInvestigations,
		)
		a.scheduler.SetKnowledge(knowledgeAggregator)
		a.scheduler.SetValkeyClient(a.valkeyClient, a.cfg.InvestigationTimeout+10*time.Minute)
		a.scheduler.SetPresence(presence)
		a.scheduler.SetLeaderLease(leaderLease)
		a.scheduler.SetDisconnectGrace(a.cfg.AgentDisconnectGrace)
		a.scheduler.SetInvestigationTimeout(a.cfg.InvestigationTimeout)
		a.scheduler.SetTeamStore(a.stores.Team)
		a.scheduler.SetSSEPublisher(&sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient})
		invWorker.SetNotifier(a.scheduler)

		if a.stores.Alert != nil && publisher != nil {
			a.scheduler.SetAlertStore(a.stores.Alert)
			a.scheduler.SetStalePublisher(publisher)
			a.scheduler.SetStaleConfig(a.cfg.StaleAlertThreshold, a.cfg.StaleAlertSweepInterval)
			a.scheduler.SetDataRetention(a.cfg.DataRetentionDays, time.Hour)
			// DT-E3 retention family rides the same hourly leader-gated loop
			// (audit logs reuse the SetAuditStore store below).
			a.scheduler.SetAuditRetention(a.cfg.AuditRetentionDays)
			a.scheduler.SetRetentionStores(a.stores.TriageResult, a.stores.Delivery, a.stores.PasswordReset)
		}

		a.scheduler.SetIncidentStore(a.stores.Incident)
		a.scheduler.SetIncidentInvestigationStore(a.stores.IncidentInvestigation)
		a.scheduler.SetIncidentCoordinationStore(a.stores.IncidentCoordination)
		a.scheduler.SetIncidentChannelMgr(channelManager)
		a.scheduler.SetAuditStore(a.stores.Audit)

		if a.cfg.IncidentSummaryEnabled {
			interval := a.cfg.IncidentSummaryInterval
			if interval <= 0 {
				interval = 15 * time.Minute
			}
			a.scheduler.SetSummaryConfig(true, interval, a.cfg.IncidentSummaryIntervals)
		}

		a.scheduler.SetHandoffDetector(handoffDetector, publisher)

		playbookEnricher := prompt.NewPlaybookEnricher(a.stores.Playbook)
		a.scheduler.SetPlaybookEnricher(playbookEnricher)

		a.scheduler.SetICSRoleStore(a.stores.ICSRole)
		a.scheduler.SetCoordinationTaskStore(a.stores.CoordinationTask)
		a.scheduler.SetCoordinationTaskSweepInterval(5 * time.Minute)

		// DT-E1 / WP-A3: the scheduler leader owns SLA sweep tick publication;
		// interval <= 0 disables publication entirely.
		if publisher != nil {
			a.scheduler.SetSLAPublisher(publisher)
			a.scheduler.SetSLASweepInterval(a.cfg.SLASweepInterval)
		}

		ssePublisher := &sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient}
		alertLifecycle = api.NewAlertInvestigationLifecycleService(a.stores.Alert, a.stores.AlertInvestigation, a.stores.Audit, ssePublisher, a.scheduler)
		a.scheduler.SetAlertInvestigationLifecycleService(alertLifecycle)
		whReceiver.SetAlertInvestigationLifecycleService(alertLifecycle)
		agentExecutor.SetAlertInvestigationLifecycleService(alertLifecycle)

		whReceiver.SetPendingNotifier(a.scheduler)
		agentExecutor.SetPendingNotifier(a.scheduler)
		whReceiver.SetMaintenanceStore(a.stores.MaintenanceWindow)
		whReceiver.SetAuditStore(a.stores.Audit)

		if a.valkeyClient != nil {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("goroutine panic recovered", "panic", r, "location", "agent-events-subscriber")
					}
				}()
				if err := presence.SubscribeEvents(a.shutdownCtx, func(ev valkey.AgentEvent) {
					switch ev.Type {
					case valkey.AgentEventOnline:
						a.scheduler.OnAgentOnline(ev.AgentID)
					case valkey.AgentEventOffline:
						if ev.Replica == replicaID {
							return
						}
						a.scheduler.OnAgentOffline(ev.AgentID)
					}
				}); err != nil {
					logger.Warn("agent-events subscription exited", "error", err)
				}
			}()
		}

		a.scheduler.Start()

		a.workerSet.SetInvestigateWorker(invWorker)
		logger.Info("SRE Agent investigate worker enabled", "max_concurrent", a.cfg.MaxConcurrentInvestigations, "replica", replicaID)
	}

	var googleMeetClient ics.MeetSpaceCreator
	if a.cfg.GoogleMeetEnabled && a.cfg.GoogleMeetCredentialsPath != "" {
		mc, err := ics.NewMeetClient(a.cfg.GoogleMeetCredentialsPath)
		if err != nil {
			logger.Warn("Google Meet client disabled", "component", "wire", "error", err)
		} else {
			googleMeetClient = mc
			logger.Info("Google Meet client enabled", "component", "wire", "auto_create", a.cfg.GoogleMeetAutoCreate)
		}
	}

	if a.workerSet != nil && publisher != nil {
		incidentWorker := worker.NewIncidentWorker(a.stores.Incident, a.stores.IncidentInvestigation, a.stores.Alert, publisher, &sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient}, a.valkeyClient, a.stores.ICSRole, publisher, a.stores.Service, onCallResolver, a.stores.OnCall, a.stores.Escalation, a.stores.User, publisher)
		if a.scheduler != nil {
			incidentWorker.SetNotifier(a.scheduler)
		}
		a.workerSet.SetIncidentWorker(incidentWorker)
		incidentWorker.SetCancelSet(a.cancelSet)

		escalationWorker := worker.NewEscalationWorker(a.stores.Escalation, a.stores.Incident, policyEngine, &sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient}, publisher, a.valkeyClient)
		escalationWorker.SetCancelSet(a.cancelSet)
		a.workerSet.SetEscalationWorker(escalationWorker)

		escalationSweepWorker := worker.NewEscalationSweepWorker(a.stores.Escalation, publisher, a.valkeyClient)
		a.workerSet.SetEscalationSweepWorker(escalationSweepWorker)

		actionItemSweepWorker := worker.NewActionItemSweepWorker(a.stores.ActionItem, a.stores.Incident)
		actionItemSweepWorker.SetSignals(a.stores.Notification, &sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient}, a.valkeyClient)
		a.workerSet.SetActionItemSweepWorker(actionItemSweepWorker)

		heartbeatSweepWorker := worker.NewHeartbeatSweepWorker(a.stores.Heartbeat, a.stores.Alert, a.stores.Audit, whReceiver)
		a.workerSet.SetHeartbeatSweepWorker(heartbeatSweepWorker)

		slaWorker := worker.NewSLAWorker(a.stores.Incident, &sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient}, a.valkeyClient)
		slaWorker.SetCoordinationStore(a.stores.IncidentCoordination)
		slaWorker.SetICSRoleStore(a.stores.ICSRole)
		slaWorker.SetForwarder(invForwarder)
		slaWorker.SetEscalationPublisher(publisher)
		slaWorker.SetServiceStore(a.stores.Service)
		slaWorker.SetStatusUpdateInterval(a.cfg.StatusUpdateInterval)
		a.workerSet.SetSLAWorker(slaWorker)

		stuckInvEscWorker := worker.NewStuckInvestigationEscalationWorker(
			a.stores.AlertInvestigation,
			a.stores.Incident,
			a.stores.Service,
			a.stores.Team,
			publisher,
			a.valkeyClient,
			a.cfg.StuckInvestigationEscalationMultiplier,
			a.cfg.StuckInvestigationEscalationTickInterval,
			a.cfg.InvestigationTimeout,
			a.cfg.OpsTeamName,
		)
		a.workerSet.SetStuckInvestigationEscalationWorker(stuckInvEscWorker)

		icsProvisioner := api.NewICSWarRoomProvisioner(a.stores.ICSRole, a.stores.IncidentDocument, a.stores.Incident, provisionMeetClient(a.cfg, googleMeetClient))
		icsWorker := worker.NewICSWorker(icsProvisioner, publisher)
		icsWorker.SetSSEPublisher(&sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient})
		icsWorker.SetValkeyClient(a.valkeyClient)
		a.workerSet.SetICSWorker(icsWorker)

		// W6 transactional outbox: a worker drains the outbox table and
		// republishes stored payloads to RabbitMQ. Registered whenever the
		// publisher is available so events are never lost if the broker is
		// briefly unavailable.
		if a.stores.Outbox != nil {
			outboxWorker := worker.NewOutboxWorker(a.stores.Outbox, publisher, 0, a.cfg.OutboxRetention)
			a.workerSet.SetOutboxWorker(outboxWorker)
			logger.Info("Transactional outbox publisher worker enabled", "component", "wire")
		}
	}

	if a.cfg.TriageEnabled && publisher != nil {
		ruleEvaluator := triage.NewRuleEvaluator(a.stores.TriageRule)
		triageLLMClient := triage.NewLLMClient(a.cfg.TriageLLMURL, a.cfg.TriageLLMAPIKey, a.cfg.TriageLLMModel)
		triageEngine := triage.NewEngine(a.cfg, ruleEvaluator, triageLLMClient, publisher,
			a.stores.TriageResult)

		triageWorker := triage.NewWorker(triageEngine, publisher, a.stores.Alert)
		triageWorker.SetCancelSet(a.cancelSet)
		triageWorker.SetAuditStore(a.stores.Audit)
		a.workerSet.SetTriageWorker(triageWorker)

		outcomeWorker := triage.NewOutcomeWorker(
			a.stores.TriageResult, a.stores.Alert,
			time.Minute*5, time.Hour, a.cfg.TriageAutoPromoteConfirmedCount,
		)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("goroutine panic recovered", "panic", r, "location", "triage-outcome-worker")
				}
			}()
			outcomeWorker.Start(a.shutdownCtx)
		}()
	}

	// WorkerSet.Start() is called after every worker has been registered
	// (including the email + notification-dispatch workers wired further
	// down after apiServer is built). Starting earlier silently drops any
	// worker registered post-Start from the consume loop.
	if a.valkeyClient != nil {
		a.loginLimiter = valkey.NewLoginRateLimiter(a.valkeyClient, 5, 15*time.Minute, 30*time.Minute)
		a.rateLimiter = valkey.NewRateLimiter(a.valkeyClient, a.cfg.RateLimitGeneralPerMinute)
		a.agentRateLimiter = valkey.NewRateLimiter(a.valkeyClient, a.cfg.RateLimitAgentPerMinute)
	} else {
		a.loginLimiter = api.NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute)
		a.rateLimiter = api.NewRateLimiter(a.cfg.RateLimitGeneralPerMinute, time.Minute)
		a.agentRateLimiter = api.NewRateLimiter(a.cfg.RateLimitAgentPerMinute, time.Minute)
	}

	a.apiServer = api.NewServer(a.cfg, a.stores.Alert, a.stores.WebhookToken, a.stores.AgentToken, a.stores.User, sessionStore, a.stores.Audit, a.stores.Integration, a.stores.RouteRules, sessionExpiry, mmClient, slackClient, twilioClient, telnyxClient, whReceiver.SetRoutingEngine, a.loginLimiter, a.rateLimiter, a.stores.AlertInvestigation, a.stores.IncidentInvestigation, a.stores.InvestigationThread, a.stores.Notification, a.stores.Dashboard, a.stores.PersonalToken)
	whReceiver.SetRateLimiter(a.rateLimiter)
	mmWebhookHandler.SetRateLimiter(a.rateLimiter)
	slackWebhookHandler.SetRateLimiter(a.rateLimiter)
	a.apiServer.SetAlertBroadcaster(&alertEventPublisher{broker: a.sseBroker, vkClient: a.valkeyClient})
	a.apiServer.SetSSEBroker(a.sseBroker, a.valkeyClient)
	a.apiServer.SetCancelSet(a.cancelSet)
	a.apiServer.SetIdempotency(a.idempotency, a.cfg.IdempotencyTTL)
	a.apiServer.InitSlackOAuthHandler(slackClient)
	a.apiServer.InitGoogleOAuthHandler()
	a.apiServer.InitSlackSignInHandler()
	a.apiServer.InitUserSlackHandler()
	a.apiServer.InitUserGoogleHandler()
	a.apiServer.SetAgentSSE(agentSSE)
	// Wire cross-domain executor callbacks (closures over the operator Server;
	// see api/agent_bridges.go). Set before agent.Service registers routes.
	agentExecutor.SetAlertCascade(a.apiServer.AgentAlertCascadeFn())
	agentExecutor.SetPostMortemBuilder(a.apiServer.AgentPostMortemBuilderFn())
	agentExecutor.SetCoordinationForwarder(a.apiServer.AgentCoordinationForwarderFn())

	agentService := agent.NewService(
		a.cfg,
		agentExecutor,
		agentSSE,
		a.stores.AgentToken,
		a.stores.AgentDM,
		a.stores.AgentAsk,
		memorySvc,
		a.stores.Audit,
		a.apiServer.PlatformAuthDeps(),
		platform.AgentRateLimitDeps{RateLimiter: a.agentRateLimiter},
		&sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient},
		agent.WithAlertStores(a.stores.Alert, a.stores.Incident, a.stores.IncidentCoordination, a.stores.IncidentInvestigation, a.stores.Playbook),
		agent.WithOnCall(a.stores.OnCall, onCallResolver, a.stores.User),
		agent.WithICSRoles(a.stores.ICSRole),
		agent.WithIncidentChannelManager(channelManager),
		agent.WithVKClient(a.valkeyClient),
		agent.WithIdempotencyCache(a.idempotency, a.cfg.IdempotencyTTL),
		agent.WithResolveAlert(a.apiServer.AgentResolveAlertFn()),
		agent.WithReopenAlert(a.apiServer.AgentReopenAlertFn()),
		agent.WithWriteAlertsQueryResponse(a.apiServer.AgentWriteAlertsQueryResponseFn()),
		agent.WithPostIncidentSummaryFromAgent(a.apiServer.AgentPostIncidentSummaryFn()),
		agent.WithScheduleDisplayName(a.apiServer.AgentScheduleDisplayNameFn()),
		agent.WithRevokeTokenByID(a.apiServer.AgentRevokeTokenByIDFn()),
	)
	a.apiServer.SetAgentService(agentService)
	a.apiServer.SetAgentRateLimiter(a.agentRateLimiter)
	a.apiServer.SetKnowledgeStore(a.stores.Knowledge)
	a.apiServer.SetSystemConfigStore(a.stores.SystemConfig)
	if loadedSysCfg != nil {
		a.apiServer.SetSystemConfigUpdatedAt(loadedSysCfg.UpdatedAt)
	}
	a.apiServer.SetSummaryConfigApplier(func(enabled bool, defaultInterval time.Duration, severityIntervals map[string]time.Duration) {
		if a.scheduler != nil {
			a.scheduler.SetSummaryConfig(enabled, defaultInterval, severityIntervals)
		}
	})
	a.apiServer.SetMemoryService(memorySvc)
	a.apiServer.SetMaintenanceWindowStore(a.stores.MaintenanceWindow)
	a.apiServer.SetHeartbeatStore(a.stores.Heartbeat)
	a.apiServer.SetStatusPageStore(a.stores.StatusPage)
	a.apiServer.SetOIDCStores(a.stores.OIDCProvider, a.stores.OIDCIdentity)
	a.apiServer.SetCredentialStores(a.stores.CredentialProvider, a.stores.SharedSecret, secretprovider.NewRegistry())
	a.apiServer.InitOIDCHandler()
	a.apiServer.SetTriageResultStore(a.stores.TriageResult)
	a.apiServer.SetTriageRuleStore(a.stores.TriageRule)
	a.apiServer.SetIncidentStore(a.stores.Incident)
	a.apiServer.SetIncidentCoordinationStore(a.stores.IncidentCoordination)
	a.apiServer.SetCoordinationTaskStore(a.stores.CoordinationTask)
	a.apiServer.SetIncidentChannelManager(channelManager)
	a.apiServer.SetGoogleMeetClient(googleMeetClient)
	a.apiServer.SetServiceStore(a.stores.Service)
	a.apiServer.SetTeamStore(a.stores.Team)
	a.apiServer.SetEscalationStore(a.stores.Escalation)
	a.apiServer.SetOnCallStore(a.stores.OnCall)
	a.apiServer.SetOnCallResolver(onCallResolver)
	a.apiServer.SetPostMortemStore(a.stores.PostMortem)
	a.apiServer.SetActionItemStore(a.stores.ActionItem)
	a.apiServer.SetHandoffStore(a.stores.Handoff)
	a.apiServer.SetPlaybookStore(a.stores.Playbook)
	statusTracker := servicetracker.NewStatusTracker(a.stores.Service, a.stores.Incident, &sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient})
	a.apiServer.SetStatusTracker(statusTracker)
	a.apiServer.SetICSRoleStore(a.stores.ICSRole)
	a.apiServer.SetIncidentDocumentStore(a.stores.IncidentDocument)
	if a.cfg.MemoryLLMURL != "" {
		a.apiServer.SetSummaryLLM(memory.NewOpenAILLM(a.cfg.MemoryLLMURL, a.cfg.MemoryLLMAPIKey, a.cfg.MemoryLLMModel))
		a.apiServer.StartSummaryScheduler()
	}
	a.apiServer.SetInvestigationForwarder(invForwarder)
	a.apiServer.SetDedupCache(dedupCache)
	a.apiServer.SetSlackWebhookHandler(slackWebhookHandler)
	if a.correlator != nil {
		a.apiServer.SetCooldownRemover(a.correlator)
		a.apiServer.SetInvestigator(a.correlator)
	}
	a.apiServer.SetAlertIngestor(whReceiver)
	if alertLifecycle != nil {
		a.apiServer.SetAlertInvestigationLifecycleService(alertLifecycle)
	}

	emailSender := email.NewSender(a.cfg.SMTPHost, a.cfg.SMTPPort, a.cfg.SMTPUser, a.cfg.SMTPPassword, a.cfg.SMTPFrom, a.cfg.SMTPSkipTLSVerify)
	a.apiServer.SetPasswordResetDeps(nil, emailSender)
	if a.workerSet != nil {
		a.workerSet.SetEmailWorker(worker.NewEmailWorker(emailSender, publisher))

		notifDispatcher := notification.NewDispatcher(a.stores.User, a.stores.Delivery, a.stores.Incident, emailSender, slackClient, activeVoice, publisher, a.valkeyClient)
		notifDispatchWorker := worker.NewNotificationDispatchWorker(a.stores.Notification, notifDispatcher, &sse.DualPublisher{Broker: a.sseBroker, VKClient: a.valkeyClient}, publisher)
		notifDispatchWorker.SetCancelSet(a.cancelSet)
		notifDispatchWorker.SetIncidentStore(a.stores.Incident)
		a.workerSet.SetNotificationDispatchWorker(notifDispatchWorker)
	}
	if a.workerSet != nil {
		a.workerSet.Start()
	}
	if a.scheduler != nil {
		a.apiServer.SetPendingNotifier(a.scheduler)
	}

	if publisher != nil {
		a.apiServer.SetRabbitMQPublisher(publisher)
		agentExecutor.SetEscalationPublisher(publisher)
	}
	agentExecutor.SetThreadStore(a.stores.InvestigationThread)
	a.apiServer.SetCache(a.cache)
	a.apiServer.Register(mux)

	if a.valkeyClient != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("goroutine panic recovered", "panic", r, "location", "peer-findings-subscriber")
				}
			}()
			if err := a.valkeyClient.SubscribePeerFindings(a.shutdownCtx, func(f valkey.PeerFinding) {
				agent.BroadcastPeerFinding(agentSSE, f)
			}); err != nil {
				logger.Warn("peer-finding subscriber exited", "error", err)
			}
		}()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine panic recovered", "panic", r, "location", "peer-ask-expirer")
			}
		}()
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-a.shutdownCtx.Done():
				return
			case <-tick.C:
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			n, err := a.stores.AgentAsk.ExpirePending(ctx)
			cancel()
			if err != nil {
				logger.Warn("peer-ask expirer failed", "error", err)
			} else if n > 0 {
				logger.Debug("peer-ask expirer expired stale asks", "count", n)
			}
		}
	}()

	if memorySvc != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("goroutine panic recovered", "panic", r, "location", "memory-expirer")
				}
			}()
			tick := time.NewTicker(5 * time.Minute)
			defer tick.Stop()
			for {
				select {
				case <-a.shutdownCtx.Done():
					return
				case <-tick.C:
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				n, err := memorySvc.DeleteExpired(ctx)
				cancel()
				if err != nil {
					logger.Warn("memory-expirer failed", "error", err)
				} else if n > 0 {
					logger.Debug("memory-expirer expired memories", "count", n)
				}
			}
		}()
	}

	// session-reaper: hard-deletes PostgreSQL sessions past their idle expiry
	// or absolute max lifetime (ASVS V3.2/V3.3). Valkey sessions self-expire
	// via TTL, so only the PG path needs sweeping; the absolute cap is also
	// enforced on read, but this keeps the table from growing unbounded.
	// Runs every 15 minutes.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine panic recovered", "panic", r, "location", "session-reaper")
			}
		}()
		tick := time.NewTicker(15 * time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-a.shutdownCtx.Done():
				return
			case <-tick.C:
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			n, err := a.stores.Session.DeleteExpired(ctx)
			cancel()
			if err != nil {
				logger.Warn("session-reaper failed", "error", err)
			} else if n > 0 {
				logger.Debug("session-reaper deleted expired sessions", "count", n)
			}
		}
	}()

	handler := api.RequestIDMiddleware(mux)
	handler = api.SecurityHeaders(handler)
	// HSTS is emitted on every HTTPS request, independent of SecureCookies
	// (ASVS V12.5, SPEC gap M3). The middleware checks r.TLS /
	// X-Forwarded-Proto itself; SecureCookies remains the cookie-attribute
	// toggle only.
	handler = api.StrictTransportSecurity(handler)
	// Wrap the whole mux with otelhttp so every inbound request gets a root
	// span (when tracing is enabled). The span context is carried in the
	// request context, so downstream logs automatically pick up trace_id /
	// span_id via logger's contextHandler, and SQL queries get db.query spans.
	// When tracing is off, otelhttp is a no-op pass-through (W1).
	handler = otelhttp.NewHandler(handler, "alga.http")

	a.server = &http.Server{
		Addr:              ":" + a.cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: a.cfg.ServerReadHeaderTimeout,
		ReadTimeout:       a.cfg.ServerReadTimeout,
		WriteTimeout:      a.cfg.ServerWriteTimeout,
		IdleTimeout:       a.cfg.ServerIdleTimeout,
		MaxHeaderBytes:    a.cfg.ServerMaxHeaderBytes,
	}

	return nil
}

func (a *App) loadStoredIntegrations() {
	storedIntegrations, err := a.stores.Integration.Get()
	if err != nil {
		logger.Warn("Failed to load stored integrations", "error", err)
	} else if storedIntegrations != nil {
		if a.cfg.MattermostURL == "" {
			a.cfg.MattermostURL = storedIntegrations.MattermostURL
		}
		if a.cfg.MattermostWebhookSecret == "" {
			a.cfg.MattermostWebhookSecret = storedIntegrations.MattermostWebhookSecret
		}
		if a.cfg.MattermostTeam == "" {
			a.cfg.MattermostTeam = storedIntegrations.MattermostTeam
		}
		if a.cfg.MattermostDefaultChannel == "" {
			a.cfg.MattermostDefaultChannel = storedIntegrations.MattermostDefaultChannel
		}
		if a.cfg.SlackBotToken == "" {
			a.cfg.SlackBotToken = storedIntegrations.SlackBotToken
		}
		if a.cfg.SlackDefaultChannel == "" {
			a.cfg.SlackDefaultChannel = storedIntegrations.SlackDefaultChannel
		}
		if a.cfg.SlackSigningSecret == "" {
			a.cfg.SlackSigningSecret = storedIntegrations.SlackSigningSecret
		}
		if a.cfg.TwilioAccountSID == "" {
			a.cfg.TwilioAccountSID = storedIntegrations.TwilioAccountSID
		}
		if a.cfg.TwilioAuthToken == "" {
			a.cfg.TwilioAuthToken = storedIntegrations.TwilioAuthToken
		}
		if a.cfg.TwilioFromNumber == "" {
			a.cfg.TwilioFromNumber = storedIntegrations.TwilioFromNumber
		}
		if a.cfg.TelnyxAPIKey == "" {
			a.cfg.TelnyxAPIKey = storedIntegrations.TelnyxAPIKey
		}
		if a.cfg.TelnyxConnectionID == "" {
			a.cfg.TelnyxConnectionID = storedIntegrations.TelnyxConnectionID
		}
		if a.cfg.TelnyxFromNumber == "" {
			a.cfg.TelnyxFromNumber = storedIntegrations.TelnyxFromNumber
		}
		if a.cfg.TelnyxPublicKey == "" {
			a.cfg.TelnyxPublicKey = storedIntegrations.TelnyxPublicKey
		}
		if a.cfg.TelnyxTTSVoice == "" {
			a.cfg.TelnyxTTSVoice = storedIntegrations.TelnyxTTSVoice
		}
		if a.cfg.TelnyxTTSLanguage == "" {
			a.cfg.TelnyxTTSLanguage = storedIntegrations.TelnyxTTSLanguage
		}
		if a.cfg.TelnyxTTSAPIKeyRef == "" {
			a.cfg.TelnyxTTSAPIKeyRef = storedIntegrations.TelnyxTTSAPIKeyRef
		}
		// Env VOICE_PROVIDER wins; otherwise fall back to the DB value.
		if os.Getenv("VOICE_PROVIDER") == "" && storedIntegrations.VoiceProvider != "" {
			a.cfg.VoiceProvider = config.NormalizeVoiceProvider(storedIntegrations.VoiceProvider)
		}
		a.cfg.MattermostDisabled = storedIntegrations.MattermostDisabled
		a.cfg.TwilioDisabled = storedIntegrations.TwilioDisabled
		a.cfg.TelnyxDisabled = storedIntegrations.TelnyxDisabled
	}
}

func (a *App) loadStoredSystemConfig() *store.SystemConfigValues {
	storedSysCfg, err := a.stores.SystemConfig.Get()
	if err != nil {
		logger.Warn("Failed to load stored system config", "error", err)
	} else if storedSysCfg != nil {
		if v := storedSysCfg.CorrelationWindow; v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				a.cfg.CorrelationWindow = d
			}
		}
		if v := storedSysCfg.CorrelationCooldownTTL; v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				a.cfg.CorrelationCooldownTTL = d
			}
		}
		if v := storedSysCfg.InvestigationTimeout; v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				a.cfg.InvestigationTimeout = d
			}
		}
		if v := storedSysCfg.MaxConcurrentInvestigations; v > 0 {
			a.cfg.MaxConcurrentInvestigations = v
		}
		if v := storedSysCfg.AgentPresenceTTL; v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				a.cfg.AgentPresenceTTL = d
			}
		}
		if v := storedSysCfg.AgentDisconnectGrace; v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				a.cfg.AgentDisconnectGrace = d
			}
		}
		if v := storedSysCfg.SchedulerLeaderTTL; v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				a.cfg.SchedulerLeaderTTL = d
			}
		}
		if v := storedSysCfg.SessionExpiryHours; v > 0 {
			a.cfg.SessionExpiryHrs = v
		}
		if v := storedSysCfg.LogLevel; v != "" {
			a.cfg.LogLevel = v
		}
		a.cfg.SlackIncidentChannelsEnabled = storedSysCfg.SlackIncidentChannelsEnabled
		if v := storedSysCfg.SlackIncidentChannelVisibility; v != "" {
			a.cfg.SlackIncidentChannelVisibility = v
		}
		if v := storedSysCfg.SlackIncidentChannelTriggerStatus; v != "" {
			a.cfg.SlackIncidentChannelTriggerStatus = v
		}
		a.cfg.SlackIncidentChannelArchiveOnClose = storedSysCfg.SlackIncidentChannelArchiveOnClose

		a.cfg.IncidentSummaryEnabled = storedSysCfg.IncidentSummaryEnabled
		if v := storedSysCfg.IncidentSummaryInterval; v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				a.cfg.IncidentSummaryInterval = d
			}
		}
		if storedSysCfg.IncidentSummaryIntervals != nil {
			severityMap := make(map[string]time.Duration, len(storedSysCfg.IncidentSummaryIntervals))
			for sev, val := range storedSysCfg.IncidentSummaryIntervals {
				if s, ok := val.(string); ok {
					if d, parseErr := time.ParseDuration(s); parseErr == nil {
						severityMap[sev] = d
					}
				}
			}
			a.cfg.IncidentSummaryIntervals = severityMap
		}

		// Authentication — Google OAuth.
		a.cfg.GoogleOAuthEnabled = storedSysCfg.GoogleOAuthEnabled
		if v := storedSysCfg.GoogleClientID; v != "" {
			a.cfg.GoogleClientID = v
		}
		if v := storedSysCfg.GoogleOAuthRedirectURL; v != "" {
			a.cfg.GoogleOAuthRedirectURL = v
		}
		if storedSysCfg.GoogleClientSecretEnc != "" {
			if pt, err := algacrypto.Default().DecryptString(storedSysCfg.GoogleClientSecretEnc); err == nil {
				a.cfg.GoogleClientSecret = pt
			} else {
				logger.Warn("failed to decrypt stored Google client secret; falling back to env config", "error", err)
			}
		}
	}
	return storedSysCfg
}

// provisionMeetClient returns the meet client only when auto-create is enabled;
// otherwise returns nil so the provisioner skips the Meet step. The manual
// create endpoint still uses the real client via SetGoogleMeetClient regardless.
func provisionMeetClient(cfg *config.Config, mc ics.MeetSpaceCreator) ics.MeetSpaceCreator {
	if mc == nil || !cfg.GoogleMeetAutoCreate {
		return nil
	}
	return mc
}

// incidentOnCallResolver adapts the on-call resolution helper to the
// incidentchannel.OnCallResolver interface so incident Slack channels can
// auto-invite the current on-call human.
type incidentOnCallResolver struct {
	serviceStore    store.ServiceStore
	escalationStore store.EscalationStore
	onCallStore     store.OnCallStore
	resolver        *oncall.Resolver
}

func (r *incidentOnCallResolver) ResolveOnCallUser(ctx context.Context, incident *store.IncidentRecord) (*uuid.UUID, error) {
	return oncall.ResolveOnCallUserForIncident(ctx, incident, r.serviceStore, r.escalationStore, r.onCallStore, r.resolver)
}

// seedOpsTeamEscalation makes the ops-team schedule + default escalation
// policy visible to the escalation engine. It is idempotent: the team and
// admin lead are already created by SeedOpsTeam; this function fills in the
// schedule, the default policy, and the team's escalation_policy_id pointer.
//
// On error we log a warning at the call site and continue boot so a missing
// schedule or conflict in the policy seed never blocks startup. Operators
// can rerun the seed by deleting the policy and restarting, or by manually
// creating the schedule and policy.
func seedOpsTeamEscalation(ctx context.Context, stores *store.Stores, opsTeamName string) error {
	if stores == nil {
		return errors.New("nil stores")
	}
	if opsTeamName == "" {
		opsTeamName = "ops-team"
	}

	opsTeam, err := stores.Team.GetTeamByName(ctx, opsTeamName)
	if err != nil {
		return fmt.Errorf("seed: failed to look up %s: %w", opsTeamName, err)
	}
	if opsTeam == nil {
		return fmt.Errorf("seed: team %s not found (SeedOpsTeam must run first)", opsTeamName)
	}

	// 1. Ensure a schedule exists for the team. Mirrors the auto-provision
	//    the team-create API does at api/team.go.
	existingSchedule, lerr := stores.OnCall.GetScheduleByTeam(ctx, opsTeam.ID)
	if lerr != nil && !errors.Is(lerr, store.ErrNotFound) {
		logger.Warn("seed: failed to look up schedule for ops-team, continuing without auto-policy", "error", lerr)
	} else if lerr == nil && existingSchedule != nil {
		// Schedule already exists; nothing to provision.
	} else {
		if _, cerr := stores.OnCall.CreateSchedule(ctx, &store.OnCallScheduleRecord{
			TeamID:   &opsTeam.ID,
			TeamName: opsTeam.Name,
		}); cerr != nil {
			return fmt.Errorf("seed: failed to create schedule for %s: %w", opsTeamName, cerr)
		}
	}

	// 2. Ensure the default policy exists and is well-formed. Idempotent by
	//    name. The policy targets the ops-team directly; the engine forces
	//    voice channels when the target team ID equals the configured
	//    ops-team ID. A previous seed version (or manual creation) may have
	//    left a same-named policy with empty levels — that pages nobody, so
	//    we repair it instead of silently skipping.
	const defaultPolicyName = "ops-team-default"
	desired := defaultOpsTeamPolicy(defaultPolicyName, opsTeam.ID)
	existing, perr := findPolicyByName(ctx, stores.Escalation, defaultPolicyName)
	if perr != nil {
		return fmt.Errorf("seed: failed to look up %s policy: %w", defaultPolicyName, perr)
	}
	if existing == nil {
		created, cerr := stores.Escalation.CreatePolicy(ctx, desired)
		if cerr != nil {
			return fmt.Errorf("seed: failed to create %s policy: %w", defaultPolicyName, cerr)
		}
		logger.Info("seed: created default ops-team escalation policy", "policy_id", created.ID, "team_id", opsTeam.ID)
		return nil
	}
	if policyNeedsRepair(existing, opsTeam.ID) {
		desired.ID = existing.ID
		desired.CreatedAt = existing.CreatedAt
		if _, uerr := stores.Escalation.UpdatePolicy(ctx, existing.ID, desired); uerr != nil {
			return fmt.Errorf("seed: failed to repair %s policy: %w", defaultPolicyName, uerr)
		}
		logger.Info("seed: repaired default ops-team escalation policy", "policy_id", existing.ID, "team_id", opsTeam.ID)
	}
	return nil
}

// defaultOpsTeamPolicy builds the canonical ops-team-default policy record.
// One level, one team target pointing at ops-team; the engine forces voice
// channels at dispatch time when the target team ID equals the configured
// ops-team ID.
func defaultOpsTeamPolicy(name string, opsTeamID uuid.UUID) *store.EscalationPolicyRecord {
	teamID := opsTeamID
	return &store.EscalationPolicyRecord{
		Name:        name,
		Description: "Default policy for the ops-team; channels are forced to voice at dispatch time when this policy fires.",
		RepeatCount: 0,
		Levels: []store.EscalationLevelRecord{
			{
				LevelNumber:    1,
				DelayMinutes:   0,
				NotifyChannels: nil,
				Targets: []store.EscalationTargetRecord{
					{TargetType: "team", TargetTeamID: &teamID},
				},
			},
		},
	}
}

// policyNeedsRepair reports whether a seeded policy is missing the level-1
// ops-team target the seed intends. A policy left over from an older seed (or
// created manually) may carry an empty Levels slice — that pages nobody, so
// the caller repairs it. Hand-tuned multi-level policies are left alone as
// long as level 1 still targets the configured ops-team.
func policyNeedsRepair(p *store.EscalationPolicyRecord, opsTeamID uuid.UUID) bool {
	if p == nil {
		return true
	}
	for _, lvl := range p.Levels {
		if lvl.LevelNumber != 1 {
			continue
		}
		for _, tgt := range lvl.Targets {
			if tgt.TargetType == "team" && tgt.TargetTeamID != nil && *tgt.TargetTeamID == opsTeamID {
				return false
			}
		}
	}
	return true
}

func findPolicyByName(ctx context.Context, escalationStore store.EscalationStore, name string) (*store.EscalationPolicyRecord, error) {
	const listScanLimit = 200
	policies, _, err := escalationStore.ListPolicies(ctx, listScanLimit, 0)
	if err != nil {
		return nil, err
	}
	for _, p := range policies {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, nil
}
