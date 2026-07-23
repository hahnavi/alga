package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/auditlog"
	"alga/logger"
)

type AuditEvent string

const (
	AuditLoginSuccess                       AuditEvent = "login_success"
	AuditLoginFailed                        AuditEvent = "login_failed"
	AuditLogout                             AuditEvent = "logout"
	AuditPasswordChanged                    AuditEvent = "password_changed"
	AuditAccountLocked                      AuditEvent = "account_locked"
	AuditAccountUnlocked                    AuditEvent = "account_unlocked"
	AuditUserCreated                        AuditEvent = "user_created"
	AuditUserUpdated                        AuditEvent = "user_updated"
	AuditUserDeleted                        AuditEvent = "user_deleted"
	AuditTokenCreated                       AuditEvent = "token_created"
	AuditTokenRevoked                       AuditEvent = "token_revoked"
	AuditTokenRegenerated                   AuditEvent = "token_regenerated"
	AuditPATCreated                         AuditEvent = "pat_created"
	AuditPATRevoked                         AuditEvent = "pat_revoked"
	AuditRoutesUpdated                      AuditEvent = "routes_updated"
	AuditSessionRefreshed                   AuditEvent = "session_refreshed"
	AuditSuspiciousActivity                 AuditEvent = "suspicious_activity"
	AuditAlertCreated                       AuditEvent = "alert_created"
	AuditAlertAcknowledged                  AuditEvent = "alert_acknowledged"
	AuditAlertResolved                      AuditEvent = "alert_resolved"
	AuditAlertReopened                      AuditEvent = "alert_reopened"
	AuditAlertDeleted                       AuditEvent = "alert_deleted"
	AuditAlertInvestigated                  AuditEvent = "alert_investigated"
	AuditEmailChanged                       AuditEvent = "email_changed"
	AuditIntegrationsUpdated                AuditEvent = "integrations_updated"
	AuditInvestigationCreated               AuditEvent = "investigation_created"
	AuditInvestigationUpdated               AuditEvent = "investigation_updated"
	AuditInvestigationDeleted               AuditEvent = "investigation_deleted"
	AuditInvestigationAlertLinked           AuditEvent = "investigation_alert_linked"
	AuditInvestigationAlertUnlinked         AuditEvent = "investigation_alert_unlinked"
	AuditAgentEnabled                       AuditEvent = "agent_enabled"
	AuditAgentDisabled                      AuditEvent = "agent_disabled"
	AuditKnowledgeCreated                   AuditEvent = "knowledge_created"
	AuditKnowledgeUpdated                   AuditEvent = "knowledge_updated"
	AuditKnowledgeDeleted                   AuditEvent = "knowledge_deleted"
	AuditPeerAskCreated                     AuditEvent = "peer_ask_created"
	AuditPeerAskReplied                     AuditEvent = "peer_ask_replied"
	AuditPeerAskCancelled                   AuditEvent = "peer_ask_cancelled"
	AuditMaintenanceWindowCreated           AuditEvent = "maintenance_window_created"
	AuditMaintenanceWindowUpdated           AuditEvent = "maintenance_window_updated"
	AuditMaintenanceWindowDeleted           AuditEvent = "maintenance_window_deleted"
	AuditTriageCompleted                    AuditEvent = "triage_completed"
	AuditTriageOverridden                   AuditEvent = "triage_overridden"
	AuditTriageRuleCreated                  AuditEvent = "triage_rule_created"
	AuditTriageRuleUpdated                  AuditEvent = "triage_rule_updated"
	AuditTriageRuleDeleted                  AuditEvent = "triage_rule_deleted"
	AuditTriagePatternPromoted              AuditEvent = "triage_pattern_promoted"
	AuditAlertAutoResolved                  AuditEvent = "alert_auto_resolved"
	AuditAlertSuppressed                    AuditEvent = "alert_suppressed"
	AuditIncidentCreated                    AuditEvent = "incident_created"
	AuditIncidentUpdated                    AuditEvent = "incident_updated"
	AuditIncidentDeleted                    AuditEvent = "incident_deleted"
	AuditIncidentAcknowledged               AuditEvent = "incident_acknowledged"
	AuditIncidentMitigated                  AuditEvent = "incident_mitigated"
	AuditIncidentResolved                   AuditEvent = "incident_resolved"
	AuditIncidentClosed                     AuditEvent = "incident_closed"
	AuditIncidentReopened                   AuditEvent = "incident_reopened"
	AuditIncidentCancelled                  AuditEvent = "incident_cancelled"
	AuditIncidentRoleAssigned               AuditEvent = "incident_role_assigned"
	AuditIncidentRoleRemoved                AuditEvent = "incident_role_removed"
	AuditIncidentEscalated                  AuditEvent = "incident_escalated"
	AuditVoiceAck                           AuditEvent = "voice_ack"
	AuditVoiceSilence                       AuditEvent = "voice_silence"
	AuditIncidentCoordinationMessageCreated AuditEvent = "incident_coordination_message_created"
	AuditIncidentCoordinationMessageUpdated AuditEvent = "incident_coordination_message_updated"
	AuditIncidentCoordinationBridgeFailed   AuditEvent = "incident_coordination_bridge_failed"
	AuditIncidentCoordinationAgentRequested AuditEvent = "incident_coordination_agent_requested"
	AuditIncidentSlackChannelCreated        AuditEvent = "incident_slack_channel_created"
	AuditIncidentSlackChannelArchived       AuditEvent = "incident_slack_channel_archived"
	AuditIncidentSlackChannelUnarchived     AuditEvent = "incident_slack_channel_unarchived"
	AuditIncidentSlackChannelUnlinked       AuditEvent = "incident_slack_channel_unlinked"
	AuditIncidentGoogleMeetCreated          AuditEvent = "incident_google_meet_created"
	AuditIncidentGoogleMeetUnlinked         AuditEvent = "incident_google_meet_unlinked"
	AuditIncidentStatusUpdateCreated        AuditEvent = "incident_status_update_created"
	AuditServiceCreated                     AuditEvent = "service_created"
	AuditServiceUpdated                     AuditEvent = "service_updated"
	AuditServiceDeleted                     AuditEvent = "service_deleted"
	AuditTeamCreated                        AuditEvent = "team_created"
	AuditTeamUpdated                        AuditEvent = "team_updated"
	AuditTeamDeleted                        AuditEvent = "team_deleted"
	AuditEscalationPolicyCreated            AuditEvent = "escalation_policy_created"
	AuditEscalationPolicyUpdated            AuditEvent = "escalation_policy_updated"
	AuditEscalationPolicyDeleted            AuditEvent = "escalation_policy_deleted"
	AuditScheduleCreated                    AuditEvent = "schedule_created"
	AuditScheduleUpdated                    AuditEvent = "schedule_updated"
	AuditScheduleOverrideCreated            AuditEvent = "schedule_override_created"
	AuditScheduleOverrideRemoved            AuditEvent = "schedule_override_removed"
	AuditPostMortemCreated                  AuditEvent = "postmortem_created"
	AuditPostMortemUpdated                  AuditEvent = "postmortem_updated"
	AuditPostMortemStatusChanged            AuditEvent = "postmortem_status_changed"
	AuditPostMortemDeleted                  AuditEvent = "postmortem_deleted"
	AuditActionItemCreated                  AuditEvent = "action_item_created"
	AuditActionItemUpdated                  AuditEvent = "action_item_updated"
	AuditActionItemDeleted                  AuditEvent = "action_item_deleted"
	AuditNotifPrefsUpdated                  AuditEvent = "notification_preferences_updated"
	AuditGoogleLoginSuccess                 AuditEvent = "google_login_success"
	AuditGoogleLoginFailed                  AuditEvent = "google_login_failed"
	AuditSlackLoginSuccess                  AuditEvent = "slack_login_success"
	AuditSlackLoginFailed                   AuditEvent = "slack_login_failed"
	AuditUserSlackLinked                    AuditEvent = "user_slack_linked"
	AuditUserSlackUnlinked                  AuditEvent = "user_slack_unlinked"
	AuditUserGoogleLinked                   AuditEvent = "user_google_linked"
	AuditUserGoogleUnlinked                 AuditEvent = "user_google_unlinked"
	AuditSlackDisconnected                  AuditEvent = "slack_disconnected"
	AuditHandoffNotesSaved                  AuditEvent = "oncall_handoff_notes_saved"
	AuditHandoffAcknowledged                AuditEvent = "oncall_handoff_acknowledged"
	AuditAgentRoleAssigned                  AuditEvent = "agent_role_assigned"
	AuditAgentRoleEnded                     AuditEvent = "agent_role_ended"
	AuditHeartbeatCreated                   AuditEvent = "heartbeat_created"
	AuditHeartbeatUpdated                   AuditEvent = "heartbeat_updated"
	AuditHeartbeatDeleted                   AuditEvent = "heartbeat_deleted"
	AuditHeartbeatTokenRegenerated          AuditEvent = "heartbeat_token_regenerated"
	AuditHeartbeatAlert                     AuditEvent = "heartbeat_alert"
	AuditStatusPageCreated                  AuditEvent = "status_page_created"
	AuditStatusPageUpdated                  AuditEvent = "status_page_updated"
	AuditStatusPageDeleted                  AuditEvent = "status_page_deleted"
	AuditStatusPageComponentCreated         AuditEvent = "status_page_component_created"
	AuditStatusPageComponentUpdated         AuditEvent = "status_page_component_updated"
	AuditStatusPageComponentDeleted         AuditEvent = "status_page_component_deleted"
	AuditOIDCProviderCreated                AuditEvent = "oidc_provider_created"
	AuditOIDCProviderUpdated                AuditEvent = "oidc_provider_updated"
	AuditOIDCProviderDeleted                AuditEvent = "oidc_provider_deleted"
	AuditOIDCLoginSuccess                   AuditEvent = "oidc_login_success"
	AuditOIDCLoginFailed                    AuditEvent = "oidc_login_failed"
	AuditOIDCIdentityLinked                 AuditEvent = "oidc_identity_linked"
	AuditCredentialProviderCreated          AuditEvent = "credential_provider_created"
	AuditCredentialProviderUpdated          AuditEvent = "credential_provider_updated"
	AuditCredentialProviderDeleted          AuditEvent = "credential_provider_deleted"
	AuditSharedSecretCreated                AuditEvent = "shared_secret_created"
	AuditSharedSecretUpdated                AuditEvent = "shared_secret_updated"
	AuditSharedSecretDeleted                AuditEvent = "shared_secret_deleted"
	AuditSharedSecretAccessed               AuditEvent = "shared_secret_accessed"
)

type AuditRecord struct {
	ID        uuid.UUID      `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Event     AuditEvent     `json:"event"`
	UserID    *uuid.UUID     `json:"user_id,omitempty"`
	Username  string         `json:"username"`
	IP        string         `json:"ip"`
	UserAgent string         `json:"user_agent"`
	Success   bool           `json:"success"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

type AuditStore interface {
	Log(event AuditEvent, userID *uuid.UUID, username, ip, userAgent string, success bool, details map[string]any)
	Query(filter map[string]any) ([]AuditRecord, error)
	GetRecentEvents(limit int) ([]AuditRecord, error)
}

type pgAuditStore struct {
	pgStoreBase
	queue     chan AuditRecord
	stop      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
}

func newPGAuditStore(client *ent.Client) AuditStore {
	return &pgAuditStore{
		pgStoreBase: pgStoreBase{client: client},
		queue:       make(chan AuditRecord, 1024),
		stop:        make(chan struct{}),
	}
}

func (s *pgAuditStore) Log(event AuditEvent, userID *uuid.UUID, username, ip, userAgent string, success bool, details map[string]any) {
	record := AuditRecord{
		Timestamp: time.Now().UTC(),
		Event:     event,
		UserID:    userID,
		Username:  username,
		IP:        ip,
		UserAgent: userAgent,
		Success:   success,
		Details:   details,
	}

	s.startConsumers()

	select {
	case s.queue <- record:
	default:
		logger.Warn("audit queue full, dropping event", "event", record.Event)
	}
}

func (s *pgAuditStore) persist(rec AuditRecord) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	b := s.client.AuditLog.Create().
		SetTimestamp(rec.Timestamp).
		SetEvent(string(rec.Event)).
		SetUsername(rec.Username).
		SetIP(rec.IP).
		SetUserAgent(rec.UserAgent).
		SetSuccess(rec.Success)

	if rec.UserID != nil {
		b.SetUserID(*rec.UserID)
	}

	if rec.Details != nil {
		b.SetDetails(rec.Details)
	}

	if _, err := b.Save(ctx); err != nil {
		logger.Error("Failed to persist audit event", "event", rec.Event, "error", err)
	}
}

func (s *pgAuditStore) startConsumers() {
	s.startOnce.Do(func() {
		for range 2 {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer func() {
					if r := recover(); r != nil {
						logger.Error("audit consumer panic recovered", "panic", r)
					}
				}()
				for {
					select {
					case <-s.stop:
						return
					case rec := <-s.queue:
						s.persist(rec)
					}
				}
			}()
		}
	})
}

func (s *pgAuditStore) Close() {
	if s.stop != nil {
		close(s.stop)
		s.wg.Wait()
	}
}

func pgAuditLogsToRecords(logs []*ent.AuditLog) []AuditRecord {
	records := make([]AuditRecord, 0, len(logs))
	for _, l := range logs {
		records = append(records, AuditRecord{
			ID:        l.ID,
			Timestamp: l.Timestamp,
			Event:     AuditEvent(l.Event),
			UserID:    l.UserID,
			Username:  l.Username,
			IP:        l.IP,
			UserAgent: l.UserAgent,
			Success:   l.Success,
			Details:   l.Details,
			RequestID: l.RequestID,
		})
	}
	return records
}

func (s *pgAuditStore) Query(filter map[string]any) ([]AuditRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := s.client.AuditLog.Query().Order(ent.Desc(auditlog.FieldTimestamp))

	if ev, ok := filter["event"].(string); ok {
		query = query.Where(auditlog.Event(ev))
	}

	limit, _ := extractLimitSkip(filter, 500)
	query = query.Limit(limit)

	logs, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}

	return pgAuditLogsToRecords(logs), nil
}

func (s *pgAuditStore) GetRecentEvents(limit int) ([]AuditRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logs, err := s.client.AuditLog.Query().
		Order(ent.Desc(auditlog.FieldTimestamp)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent events: %w", err)
	}

	return pgAuditLogsToRecords(logs), nil
}
