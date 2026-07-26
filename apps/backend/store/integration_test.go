//go:build integration

package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"alga/config"
	"alga/pgclient"
	"alga/rabbitmq"
)

var (
	alertsStore       Store
	userStore         UserStore
	sessionStore      SessionStore
	webhookTokenStore WebhookTokenStore
	agentTokenStore   AgentTokenStore
	auditStore        AuditStore
	integrationStore  IntegrationStore
	routeRulesStore   RouteRulesStore
	notificationStore NotificationStore
	alertInvStore     AlertInvestigationStore
	dashboardStore    DashboardStore
	knowledgeStore    KnowledgeStore
	agentAskStore     AgentAskStore
	agentDMStore      AgentDMStore
	incidentStore     IncidentStore
	pgClient          *pgclient.Client
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	dsn := os.Getenv("ALGA_TEST_PG_DSN")
	if dsn == "" {
		c, err := tcpostgres.Run(ctx,
			"postgres:18-alpine",
			tcpostgres.WithDatabase("alga_test"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(30*time.Second)),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = c.Terminate(ctx) }()

		connStr, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
			os.Exit(1)
		}
		dsn = connStr
	} else {
		reset, err := sql.Open("pgx", dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open schema reset connection: %v\n", err)
			os.Exit(1)
		}
		if _, err := reset.ExecContext(ctx, `DO $$ DECLARE r RECORD; BEGIN FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname='public') LOOP EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE'; END LOOP; END $$;`); err != nil {
			fmt.Fprintf(os.Stderr, "schema reset skipped: %v\n", err)
		}
		reset.Close()
	}

	if err := pgclient.ApplyMigrations(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "failed to apply migrations: %v\n", err)
		os.Exit(1)
	}

	cli, err := pgclient.New(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create pg client: %v\n", err)
		os.Exit(1)
	}
	pgClient = cli
	defer cli.Close()

	stores, err := NewStores(cli, 24*time.Hour, 24*time.Hour)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create stores: %v\n", err)
		os.Exit(1)
	}

	alertsStore = stores.Alert
	webhookTokenStore = stores.WebhookToken
	userStore = stores.User
	sessionStore = stores.Session
	auditStore = stores.Audit
	integrationStore = stores.Integration
	routeRulesStore = stores.RouteRules
	agentTokenStore = stores.AgentToken
	agentDMStore = stores.AgentDM
	notificationStore = stores.Notification
	dashboardStore = stores.Dashboard
	knowledgeStore = stores.Knowledge
	agentAskStore = stores.AgentAsk
	incidentStore = stores.Incident
	alertInvStore = stores.AlertInvestigation

	os.Exit(m.Run())
}

func TestAlertStore(t *testing.T) {
	ctx := context.Background()
	fp := "fp-" + uuid.New().String()[:8]

	t.Run("Create_and_GetByFingerprint", func(t *testing.T) {
		rec := AlertRecord{
			Fingerprint:  fp,
			Status:       "firing",
			Labels:       map[string]string{"alertname": "TestAlert", "severity": "critical"},
			Annotations:  map[string]string{"summary": "test alert"},
			GeneratorURL: "http://grafana/example",
		}
		if _, err := alertsStore.Create(rec); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := alertsStore.GetByFingerprint(fp)
		if err != nil {
			t.Fatalf("GetByFingerprint: %v", err)
		}
		if got == nil {
			t.Fatal("expected record, got nil")
		}
		if got.Fingerprint != fp {
			t.Errorf("Fingerprint = %q, want %q", got.Fingerprint, fp)
		}
		if got.Status != "firing" {
			t.Errorf("Status = %q, want firing", got.Status)
		}
		if got.Labels["alertname"] != "TestAlert" {
			t.Errorf("Labels[alertname] = %q, want TestAlert", got.Labels["alertname"])
		}
		if got.AlertNumber == 0 {
			t.Error("AlertNumber should be > 0")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		if err := alertsStore.UpdateStatus(fp, "resolved", nil); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := alertsStore.GetByFingerprint(fp)
		if got.Status != "resolved" {
			t.Errorf("Status = %q, want resolved", got.Status)
		}
	})

	t.Run("ReopenAlert", func(t *testing.T) {
		ev := AlertEvent{Type: "reopened", Source: "user"}
		if err := alertsStore.ReopenAlert(fp, ev); err != nil {
			t.Fatalf("ReopenAlert: %v", err)
		}
		got, _ := alertsStore.GetByFingerprint(fp)
		if got.Status != "firing" {
			t.Errorf("Status = %q, want firing", got.Status)
		}
	})

	t.Run("AcknowledgeAlert", func(t *testing.T) {
		actor := &EventActor{UserID: "u1", Username: "alice"}
		if err := alertsStore.AcknowledgeAlert(fp, actor); err != nil {
			t.Fatalf("AcknowledgeAlert: %v", err)
		}
		got, _ := alertsStore.GetByFingerprint(fp)
		if !got.Acknowledged {
			t.Error("expected Acknowledged = true")
		}
	})

	t.Run("ResolveAlertByUser", func(t *testing.T) {
		actor := &EventActor{UserID: "u1", Username: "alice"}
		if err := alertsStore.ResolveAlertByUser(fp, actor); err != nil {
			t.Fatalf("ResolveAlertByUser: %v", err)
		}
		got, _ := alertsStore.GetByFingerprint(fp)
		if got.Status != "resolved" {
			t.Errorf("Status = %q, want resolved", got.Status)
		}
	})

	t.Run("QueryAlerts", func(t *testing.T) {
		results, err := alertsStore.QueryAlerts(map[string]any{"status": "resolved"})
		if err != nil {
			t.Fatalf("QueryAlerts: %v", err)
		}
		if len(results) == 0 {
			t.Error("expected at least one resolved alert")
		}
	})

	t.Run("DeleteAlert", func(t *testing.T) {
		if err := alertsStore.DeleteAlert(fp); err != nil {
			t.Fatalf("DeleteAlert: %v", err)
		}
		got, _ := alertsStore.GetByFingerprint(fp)
		if got == nil {
			t.Fatal("expected soft-deleted tombstone to remain queryable")
		}
		if got.DeletedAt == nil {
			t.Error("expected DeletedAt to be set after soft-delete")
		}
	})

	_ = ctx
}

func TestUserStore(t *testing.T) {
	email := fmt.Sprintf("user-%s@test.com", uuid.New().String()[:8])
	var uid uuid.UUID

	t.Run("CreateUser_and_GetByEmail", func(t *testing.T) {
		rec, err := userStore.CreateUser(email, "P@ssw0rd!", "admin")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		uid = rec.ID
		got, err := userStore.GetByEmail(email)
		if err != nil {
			t.Fatalf("GetByEmail: %v", err)
		}
		if got == nil || got.Email != email {
			t.Errorf("Email = %q, want %q", got.Email, email)
		}
		if got.Role != "admin" {
			t.Errorf("Role = %q, want admin", got.Role)
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		got, err := userStore.GetByID(uid)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil || got.ID != uid {
			t.Error("ID mismatch")
		}
	})

	t.Run("Authenticate_success", func(t *testing.T) {
		rec, err := userStore.Authenticate(email, "P@ssw0rd!")
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if rec.Email != email {
			t.Errorf("Email = %q, want %q", rec.Email, email)
		}
	})

	t.Run("Authenticate_wrong_password", func(t *testing.T) {
		_, err := userStore.Authenticate(email, "wrongpassword")
		if err == nil {
			t.Error("expected error for wrong password")
		}
	})

	t.Run("CountAdmins", func(t *testing.T) {
		count, err := userStore.CountAdmins()
		if err != nil {
			t.Fatalf("CountAdmins: %v", err)
		}
		if count < 1 {
			t.Errorf("CountAdmins = %d, want >= 1", count)
		}
	})

	t.Run("UpdateUser", func(t *testing.T) {
		err := userStore.UpdateUser(uid, map[string]any{"full_name": "Test User"})
		if err != nil {
			t.Fatalf("UpdateUser: %v", err)
		}
		got, _ := userStore.GetByID(uid)
		if got.FullName != "Test User" {
			t.Errorf("FullName = %q, want Test User", got.FullName)
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		if err := userStore.DeleteUser(uid); err != nil {
			t.Fatalf("DeleteUser: %v", err)
		}
		got, _ := userStore.GetByID(uid)
		if got != nil {
			t.Error("expected nil after delete")
		}
	})
}

func TestSessionStore(t *testing.T) {
	user, err := userStore.CreateUser("session-test-"+uuid.NewString()+"@example.com", "P@ssw0rd!", "viewer")
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	userID := user.ID
	var sessionID string
	var refreshToken string

	t.Run("CreateSession_and_GetSession", func(t *testing.T) {
		rec, err := sessionStore.CreateSession(userID, "127.0.0.1", "test-agent")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		sessionID = rec.ID
		refreshToken = rec.RefreshToken
		got, err := sessionStore.GetSession(sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got == nil {
			t.Fatal("expected session, got nil")
		}
		if got.UserID != userID {
			t.Errorf("UserID mismatch")
		}
	})

	t.Run("RefreshSession", func(t *testing.T) {
		rec, err := sessionStore.RefreshSession(sessionID, "127.0.0.2", "new-agent")
		if err != nil {
			t.Fatalf("RefreshSession: %v", err)
		}
		if rec == nil {
			t.Fatal("expected refreshed session, got nil")
		}
		if rec.RefreshToken == refreshToken {
			t.Error("expected new refresh token after refresh")
		}
	})

	t.Run("GetSessionByRefreshToken", func(t *testing.T) {
		got, err := sessionStore.GetSessionByRefreshToken("")
		if err != nil {
			t.Fatalf("GetSessionByRefreshToken(empty): %v", err)
		}
		if got != nil {
			t.Error("expected nil for empty token")
		}
	})

	t.Run("DeleteAllUserSessions", func(t *testing.T) {
		if err := sessionStore.DeleteAllUserSessions(userID); err != nil {
			t.Fatalf("DeleteAllUserSessions: %v", err)
		}
		got, _ := sessionStore.GetSession(sessionID)
		if got != nil {
			t.Error("expected nil after deleting all user sessions")
		}
	})
}

func TestWebhookTokenStore(t *testing.T) {
	var tokenID uuid.UUID
	var rawToken string

	t.Run("CreateToken_and_ListTokens", func(t *testing.T) {
		rec, err := webhookTokenStore.CreateToken("test-token", nil)
		if err != nil {
			t.Fatalf("CreateToken: %v", err)
		}
		tokenID = rec.ID
		rawToken = rec.Token
		tokens, err := webhookTokenStore.ListTokens()
		if err != nil {
			t.Fatalf("ListTokens: %v", err)
		}
		if len(tokens) == 0 {
			t.Error("expected at least one token")
		}
	})

	t.Run("ValidateToken_valid", func(t *testing.T) {
		ok, err := webhookTokenStore.ValidateToken(rawToken)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if !ok {
			t.Error("expected token to be valid")
		}
	})

	t.Run("RevokeToken", func(t *testing.T) {
		if err := webhookTokenStore.RevokeToken(tokenID); err != nil {
			t.Fatalf("RevokeToken: %v", err)
		}
		ok, _ := webhookTokenStore.ValidateToken(rawToken)
		if ok {
			t.Error("expected token to be invalid after revocation")
		}
	})
}

func TestAgentTokenStore(t *testing.T) {
	var tokenID uuid.UUID
	var rawToken string

	t.Run("CreateToken_and_ListTokens", func(t *testing.T) {
		rec, err := agentTokenStore.CreateToken("hermes-1", nil, "hermes", nil)
		if err != nil {
			t.Fatalf("CreateToken: %v", err)
		}
		tokenID = rec.ID
		rawToken = rec.Token
		tokens, err := agentTokenStore.ListTokens()
		if err != nil {
			t.Fatalf("ListTokens: %v", err)
		}
		if len(tokens) == 0 {
			t.Error("expected at least one agent token")
		}
	})

	t.Run("ValidateToken", func(t *testing.T) {
		rec, err := agentTokenStore.ValidateToken(rawToken)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if rec == nil {
			t.Fatal("expected record, got nil")
		}
		if rec.Name != "hermes-1" {
			t.Errorf("Name = %q, want hermes-1", rec.Name)
		}
	})

	t.Run("SetAgentEnabled_false", func(t *testing.T) {
		if err := agentTokenStore.SetAgentEnabled(tokenID, false); err != nil {
			t.Fatalf("SetAgentEnabled: %v", err)
		}
	})

	t.Run("RevokeToken", func(t *testing.T) {
		if err := agentTokenStore.RevokeToken(tokenID); err != nil {
			t.Fatalf("RevokeToken: %v", err)
		}
		rec, _ := agentTokenStore.ValidateToken(rawToken)
		if rec != nil {
			t.Error("expected nil after revocation")
		}
	})
}

func TestAuditStore(t *testing.T) {
	t.Run("Log_and_Query", func(t *testing.T) {
		uid := uuid.New()
		auditStore.Log(AuditLoginSuccess, &uid, "alice", "1.2.3.4", "test", true, map[string]any{"key": "val"})
		time.Sleep(200 * time.Millisecond)

		records, err := auditStore.Query(map[string]any{"event": string(AuditLoginSuccess)})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(records) == 0 {
			t.Error("expected at least one audit record")
		}
	})

	t.Run("GetRecentEvents", func(t *testing.T) {
		records, err := auditStore.GetRecentEvents(10)
		if err != nil {
			t.Fatalf("GetRecentEvents: %v", err)
		}
		if len(records) == 0 {
			t.Error("expected at least one recent event")
		}
	})
}

func TestIntegrationStore(t *testing.T) {
	t.Run("Get_empty", func(t *testing.T) {
		got, err := integrationStore.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != nil {
			t.Error("expected nil for empty integration config")
		}
	})

	t.Run("Save_and_Get", func(t *testing.T) {
		cfg := IntegrationConfig{
			MattermostURL:       "https://mm.example.com",
			SlackDefaultChannel: "#alerts",
		}
		if err := integrationStore.Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := integrationStore.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got == nil {
			t.Fatal("expected config, got nil")
		}
		if got.MattermostURL != "https://mm.example.com" {
			t.Errorf("MattermostURL = %q, want https://mm.example.com", got.MattermostURL)
		}
		if got.SlackDefaultChannel != "#alerts" {
			t.Errorf("SlackDefaultChannel = %q, want #alerts", got.SlackDefaultChannel)
		}
	})

	t.Run("Update", func(t *testing.T) {
		cfg := IntegrationConfig{
			MattermostURL:       "https://mm2.example.com",
			SlackDefaultChannel: "#ops",
		}
		if err := integrationStore.Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, _ := integrationStore.Get()
		if got.MattermostURL != "https://mm2.example.com" {
			t.Errorf("MattermostURL = %q, want https://mm2.example.com", got.MattermostURL)
		}
	})

	t.Run("Telnyx_TTS_roundtrip", func(t *testing.T) {
		cfg := IntegrationConfig{
			TelnyxTTSVoice:     "ElevenLabs.eleven_flash_v2_5.iP95p4xoKVk53GoZ742B",
			TelnyxTTSLanguage:  "en-US",
			TelnyxTTSAPIKeyRef: "elevenlabs-prod",
		}
		if err := integrationStore.Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := integrationStore.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got == nil {
			t.Fatal("expected config, got nil")
		}
		if got.TelnyxTTSVoice != "ElevenLabs.eleven_flash_v2_5.iP95p4xoKVk53GoZ742B" {
			t.Errorf("TelnyxTTSVoice = %q, want ElevenLabs voice id", got.TelnyxTTSVoice)
		}
		if got.TelnyxTTSLanguage != "en-US" {
			t.Errorf("TelnyxTTSLanguage = %q, want en-US", got.TelnyxTTSLanguage)
		}
		if got.TelnyxTTSAPIKeyRef != "elevenlabs-prod" {
			t.Errorf("TelnyxTTSAPIKeyRef = %q, want elevenlabs-prod", got.TelnyxTTSAPIKeyRef)
		}
	})
}

func TestRouteRulesStore(t *testing.T) {
	t.Run("Get_empty", func(t *testing.T) {
		routes, err := routeRulesStore.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(routes) != 0 {
			t.Errorf("expected empty routes, got %d", len(routes))
		}
	})

	t.Run("Save_and_Get", func(t *testing.T) {
		routes := []config.RouteConfig{
			{
				MatchMode: "all",
				Conditions: []config.RouteCondition{
					{Source: "labels", Field: "severity", Operator: "exact", Value: "critical"},
				},
				Targets: []config.RouteTarget{
					{Provider: "mattermost", Channel: "#incidents"},
				},
			},
		}
		if err := routeRulesStore.Save(routes); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := routeRulesStore.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 route, got %d", len(got))
		}
		if got[0].Conditions[0].Value != "critical" {
			t.Errorf("Condition Value = %q, want critical", got[0].Conditions[0].Value)
		}
	})

	t.Run("Update", func(t *testing.T) {
		routes := []config.RouteConfig{
			{
				MatchMode: "any",
				Conditions: []config.RouteCondition{
					{Source: "labels", Field: "team", Operator: "exact", Value: "platform"},
				},
				Targets: []config.RouteTarget{
					{Provider: "slack", Channel: "#platform-alerts"},
				},
			},
		}
		if err := routeRulesStore.Save(routes); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, _ := routeRulesStore.Get()
		if len(got) != 1 || got[0].Conditions[0].Value != "platform" {
			t.Error("route update mismatch")
		}
	})
}

func TestNotificationStore(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New().String()
	var nid uuid.UUID

	t.Run("Create_and_ListByUser", func(t *testing.T) {
		rec, err := notificationStore.Create(ctx, &NotificationRecord{
			UserID:       userID,
			Type:         "mention",
			Title:        "You were mentioned",
			Message:      "alice mentioned you",
			ResourceType: "investigation",
			ResourceID:   "INV-1",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		nid = rec.ID
		recs, err := notificationStore.ListByUser(ctx, userID, 10, 0)
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		if len(recs) == 0 {
			t.Error("expected at least one notification")
		}
	})

	t.Run("GetUnreadCount", func(t *testing.T) {
		count, err := notificationStore.GetUnreadCount(ctx, userID)
		if err != nil {
			t.Fatalf("GetUnreadCount: %v", err)
		}
		if count < 1 {
			t.Errorf("UnreadCount = %d, want >= 1", count)
		}
	})

	t.Run("MarkRead", func(t *testing.T) {
		if err := notificationStore.MarkRead(ctx, userID, nid.String()); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}
		count, _ := notificationStore.GetUnreadCount(ctx, userID)
		if count != 0 {
			t.Errorf("UnreadCount = %d, want 0", count)
		}
	})

	t.Run("MarkAllRead", func(t *testing.T) {
		notificationStore.Create(ctx, &NotificationRecord{
			UserID:  userID,
			Type:    "info",
			Title:   "Info",
			Message: "test",
		})
		notificationStore.Create(ctx, &NotificationRecord{
			UserID:  userID,
			Type:    "info",
			Title:   "Info2",
			Message: "test2",
		})
		if err := notificationStore.MarkAllRead(ctx, userID); err != nil {
			t.Fatalf("MarkAllRead: %v", err)
		}
		count, _ := notificationStore.GetUnreadCount(ctx, userID)
		if count != 0 {
			t.Errorf("UnreadCount = %d, want 0 after mark all read", count)
		}
	})
}

func TestAlertInvestigationStore(t *testing.T) {
	ctx := context.Background()
	var invID string

	t.Run("CreateAlertInvestigation_and_GetAlertInvestigation", func(t *testing.T) {
		rec := AlertInvestigationRecord{
			CorrelationKey: "corr-key-1",
			Status:         AlertInvestigationStatusPending,
			Alerts: []rabbitmq.CorrelatedAlert{
				{
					Fingerprint:  "fp-inv-1",
					Labels:       map[string]string{"alertname": "HighCPU", "namespace": "prod"},
					Annotations:  map[string]string{"summary": "CPU is high"},
					Status:       "firing",
					StartsAt:     time.Now().Format(time.RFC3339),
					GeneratorURL: "http://grafana/1",
				},
			},
		}
		created, err := alertInvStore.CreateAlertInvestigation(ctx, rec)
		if err != nil {
			t.Fatalf("CreateAlertInvestigation: %v", err)
		}
		invID = created.AlertInvestigationID
		if invID == "" {
			t.Fatal("expected non-empty alert investigation ID")
		}

		got, err := alertInvStore.GetAlertInvestigation(ctx, invID)
		if err != nil {
			t.Fatalf("GetAlertInvestigation: %v", err)
		}
		if got == nil {
			t.Fatal("expected alert investigation, got nil")
		}
		if got.AlertInvestigationID != invID {
			t.Errorf("AlertInvestigationID = %q, want %q", got.AlertInvestigationID, invID)
		}
		if got.CorrelationKey != "corr-key-1" {
			t.Errorf("CorrelationKey = %q, want corr-key-1", got.CorrelationKey)
		}
		if len(got.Alerts) != 1 {
			t.Errorf("Alerts len = %d, want 1", len(got.Alerts))
		}
	})

	t.Run("UpdateAlertInvestigationStatus", func(t *testing.T) {
		if err := alertInvStore.UpdateAlertInvestigationStatus(ctx, invID, AlertInvestigationStatusInvestigating); err != nil {
			t.Fatalf("UpdateAlertInvestigationStatus: %v", err)
		}
		got, _ := alertInvStore.GetAlertInvestigation(ctx, invID)
		if got.Status != AlertInvestigationStatusInvestigating {
			t.Errorf("Status = %q, want %q", got.Status, AlertInvestigationStatusInvestigating)
		}
	})

	t.Run("AddAlertInvestigationUpdate", func(t *testing.T) {
		update := InvestigationUpdate{
			Type:    UpdateTypeFinding,
			Message: "Found root cause",
			Source:  UpdateSourceAgent,
		}
		if err := alertInvStore.AddAlertInvestigationUpdate(ctx, invID, update); err != nil {
			t.Fatalf("AddAlertInvestigationUpdate: %v", err)
		}
		got, _ := alertInvStore.GetAlertInvestigation(ctx, invID)
		if len(got.Updates) != 1 {
			t.Errorf("Updates len = %d, want 1", len(got.Updates))
		}
		if got.Updates[0].Message != "Found root cause" {
			t.Errorf("Update Message = %q, want 'Found root cause'", got.Updates[0].Message)
		}
	})

	t.Run("ListAlertInvestigations", func(t *testing.T) {
		recs, err := alertInvStore.ListAlertInvestigations(ctx, map[string]any{"status": AlertInvestigationStatusInvestigating})
		if err != nil {
			t.Fatalf("ListAlertInvestigations: %v", err)
		}
		if len(recs) == 0 {
			t.Error("expected at least one alert investigation")
		}
	})

	t.Run("ClaimPendingAlertInvestigation", func(t *testing.T) {
		created, err := alertInvStore.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
			CorrelationKey: "corr-claim",
			Status:         AlertInvestigationStatusPending,
		})
		if err != nil {
			t.Fatalf("CreateAlertInvestigation: %v", err)
		}
		claimed, err := alertInvStore.ClaimPendingAlertInvestigation(ctx, created.ID.String(), "agent-1", "Hermes", "hermes")
		if err != nil {
			t.Fatalf("ClaimPendingAlertInvestigation: %v", err)
		}
		if claimed == nil {
			t.Fatal("expected claimed alert investigation, got nil")
		}
		if claimed.Status != AlertInvestigationStatusAssigned {
			t.Errorf("Status = %q, want %q", claimed.Status, AlertInvestigationStatusAssigned)
		}
		if claimed.AgentName != "Hermes" {
			t.Errorf("AgentName = %q, want Hermes", claimed.AgentName)
		}
	})

	t.Run("FindSimilarAlertInvestigations", func(t *testing.T) {
		results, err := alertInvStore.FindSimilarAlertInvestigations(ctx, SimilarAlertInvestigationsQuery{
			CorrelationKey: "corr-key-1",
			Limit:          5,
		})
		if err != nil {
			t.Fatalf("FindSimilarAlertInvestigations: %v", err)
		}
		_ = results
	})

	t.Run("DeleteAlertInvestigation", func(t *testing.T) {
		if err := alertInvStore.DeleteAlertInvestigation(ctx, invID); err != nil {
			t.Fatalf("DeleteAlertInvestigation: %v", err)
		}
		got, _ := alertInvStore.GetAlertInvestigation(ctx, invID)
		if got != nil {
			t.Error("expected nil after delete")
		}
	})
}

func TestKnowledgeStore(t *testing.T) {
	ctx := context.Background()
	author, err := userStore.CreateUser("knowledge-test-"+uuid.NewString()+"@example.com", "P@ssw0rd!", "viewer")
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	authorID := author.ID
	var noteID uuid.UUID

	t.Run("Create_and_Get", func(t *testing.T) {
		note, err := knowledgeStore.Create(ctx, &KnowledgeNote{
			Kind:         KnowledgeKindRunbook,
			Title:        "Restart API Server",
			BodyMarkdown: "kubectl rollout restart deploy/api",
			Tags:         []string{"kubernetes", "api"},
			AuthorID:     &authorID,
			AuthorType:   KnowledgeAuthorUser,
			AuthorName:   "alice",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		noteID = note.ID
		got, err := knowledgeStore.Get(ctx, noteID.String())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got == nil || got.Title != "Restart API Server" {
			t.Errorf("Title = %q, want Restart API Server", got.Title)
		}
	})

	t.Run("Update", func(t *testing.T) {
		updated, err := knowledgeStore.Update(ctx, noteID.String(), &KnowledgeNote{
			BodyMarkdown: "kubectl rollout restart deploy/api -n production",
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.BodyMarkdown != "kubectl rollout restart deploy/api -n production" {
			t.Errorf("BodyMarkdown = %q, unexpected", updated.BodyMarkdown)
		}
	})

	t.Run("List", func(t *testing.T) {
		notes, total, err := knowledgeStore.List(ctx, KnowledgeQuery{Kind: KnowledgeKindRunbook})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total < 1 {
			t.Errorf("total = %d, want >= 1", total)
		}
		if len(notes) == 0 {
			t.Error("expected at least one note")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := knowledgeStore.Delete(ctx, noteID.String()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		got, _ := knowledgeStore.Get(ctx, noteID.String())
		if got != nil {
			t.Error("expected nil after delete")
		}
	})
}

func TestAgentAskStore(t *testing.T) {
	ctx := context.Background()
	fromToken, err := agentTokenStore.CreateToken("ask-from-agent", nil, "hermes", nil)
	if err != nil {
		t.Fatalf("create from-agent token: %v", err)
	}
	toToken, err := agentTokenStore.CreateToken("ask-to-agent", nil, "hermes", nil)
	if err != nil {
		t.Fatalf("create to-agent token: %v", err)
	}
	fromAgent := fromToken.ID
	toAgent := toToken.ID
	var askID uuid.UUID

	t.Run("Create_and_Get", func(t *testing.T) {
		rec, err := agentAskStore.Create(ctx, &AgentAskRecord{
			FromAgentID:   fromAgent,
			FromAgentName: "Hermes-1",
			FromAgentType: "hermes",
			ToAgentID:     &toAgent,
			ToAgentType:   "hermes",
			Question:      "What is the current error rate?",
			Status:        AgentAskPending,
			ExpiresAt:     time.Now().Add(10 * time.Minute),
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		askID = rec.ID
		got, err := agentAskStore.Get(ctx, askID.String())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got == nil || got.Question != "What is the current error rate?" {
			t.Errorf("Question = %q, unexpected", got.Question)
		}
	})

	t.Run("Reply", func(t *testing.T) {
		rec, err := agentAskStore.Reply(ctx, askID.String(), "Error rate is 2.3%", toAgent, "Hermes-2")
		if err != nil {
			t.Fatalf("Reply: %v", err)
		}
		if rec.Reply != "Error rate is 2.3%" {
			t.Errorf("Reply = %q, unexpected", rec.Reply)
		}
		if rec.Status != AgentAskAnswered {
			t.Errorf("Status = %q, want answered", rec.Status)
		}
	})

	t.Run("List", func(t *testing.T) {
		recs, total, err := agentAskStore.List(ctx, AgentAskQuery{
			FromAgentID: &fromAgent,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total < 1 {
			t.Errorf("total = %d, want >= 1", total)
		}
		if len(recs) == 0 {
			t.Error("expected at least one ask")
		}
	})

	t.Run("Cancel", func(t *testing.T) {
		rec, _ := agentAskStore.Create(ctx, &AgentAskRecord{
			FromAgentID:   fromAgent,
			FromAgentName: "Hermes-1",
			FromAgentType: "hermes",
			Question:      "Cancel test?",
			Status:        AgentAskPending,
			ExpiresAt:     time.Now().Add(10 * time.Minute),
		})
		if err := agentAskStore.Cancel(ctx, rec.ID.String(), fromAgent); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		got, _ := agentAskStore.Get(ctx, rec.ID.String())
		if got.Status != AgentAskCancelled {
			t.Errorf("Status = %q, want cancelled", got.Status)
		}
	})

	t.Run("ExpirePending", func(t *testing.T) {
		agentAskStore.Create(ctx, &AgentAskRecord{
			FromAgentID:   fromAgent,
			FromAgentName: "Hermes-1",
			FromAgentType: "hermes",
			Question:      "Expire test?",
			Status:        AgentAskPending,
			ExpiresAt:     time.Now().Add(-1 * time.Hour),
		})
		n, err := agentAskStore.ExpirePending(ctx)
		if err != nil {
			t.Fatalf("ExpirePending: %v", err)
		}
		if n < 1 {
			t.Errorf("expired = %d, want >= 1", n)
		}
	})
}

func TestDashboardStore(t *testing.T) {
	ctx := context.Background()
	t.Run("GetStats_empty", func(t *testing.T) {
		stats, err := dashboardStore.GetStats(ctx)
		if err != nil {
			t.Fatalf("GetStats: %v", err)
		}
		if stats == nil {
			t.Fatal("expected stats, got nil")
		}
		if stats.Alerts.Total < 0 {
			t.Error("Total should be >= 0")
		}
	})
}

func TestAgentDMStore(t *testing.T) {
	agentToken, err := agentTokenStore.CreateToken("dm-test-agent", nil, "hermes", nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	agentHex := agentToken.ID.String()
	var msgID uuid.UUID

	t.Run("AddMessage_and_ListMessages", func(t *testing.T) {
		msg, err := agentDMStore.AddMessage(agentHex, AgentDMRoleUser, "Hello agent", nil, nil)
		if err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
		msgID = msg.ID
		msgs, _, err := agentDMStore.ListMessages(agentHex, nil, 10)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(msgs) == 0 {
			t.Error("expected at least one message")
		}
	})

	t.Run("UpdateMessageBody", func(t *testing.T) {
		if err := agentDMStore.UpdateMessageBody(agentHex, msgID.String(), "Updated body", true); err != nil {
			t.Fatalf("UpdateMessageBody: %v", err)
		}
		msgs, _, _ := agentDMStore.ListMessages(agentHex, nil, 10)
		for _, m := range msgs {
			if m.ID == msgID {
				if m.Body != "Updated body" {
					t.Errorf("Body = %q, want Updated body", m.Body)
				}
				if !m.Edited {
					t.Errorf("Edited = false, want true")
				}
			}
		}
	})
}
