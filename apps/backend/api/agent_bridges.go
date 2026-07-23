// Package api: agent_bridges.go exposes *Server methods that return the
// cross-domain function-field closures consumed by agent.Service and
// AgentToolExecutor (package agent). The closures live in package api so they
// can call unexported Server methods and helpers (handleAlertResolve,
// runAlertCascade, buildPostMortemDraft, coordinationMessage*,
// writeAlertsQueryResponse, revokeTokenByID, handleIncidentSummaryFromAgent,
// scheduleDisplayName); app/wire.go passes them to agent.NewService /
// agentExecutor.Set* without either package reaching across the import
// boundary (agent must not import alga/api).
package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/api/platform"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

// PlatformAuthDeps returns the session/PAT auth dependencies the agent Service
// needs to compose platform.AuthMiddleware for the operator-facing
// /api/v1/agent-tokens* and /api/v1/agent/capabilities routes. Mirrors the deps
// *Server.authMiddleware uses.
func (s *Server) PlatformAuthDeps() platform.AuthDeps {
	return platform.AuthDeps{
		UserStore:                s.userStore,
		SessionStore:             s.sessionStore,
		PersonalAccessTokenStore: s.personalAccessTokenStore,
		AuditStore:               s.auditStore,
		IPExtractor:              s.ipExtractor,
	}
}

// agentAlertActorToAPI converts the agent-package actor into the unexported
// api alertActionActor consumed by handleAlertResolve/handleAlertReopen.
func agentAlertActorToAPI(a *agent.AlertActionActor) *alertActionActor {
	if a == nil {
		return nil
	}
	return &alertActionActor{
		actor:     a.Actor,
		isAgent:   a.IsAgent,
		agentName: a.AgentName,
	}
}

// AgentResolveAlertFn returns the alert-resolve callback for agent.Service.
func (s *Server) AgentResolveAlertFn() func(w http.ResponseWriter, r *http.Request, fingerprint string, a *agent.AlertActionActor) {
	return func(w http.ResponseWriter, r *http.Request, fingerprint string, a *agent.AlertActionActor) {
		s.handleAlertResolve(w, r, fingerprint, agentAlertActorToAPI(a))
	}
}

// AgentReopenAlertFn returns the alert-reopen callback for agent.Service.
func (s *Server) AgentReopenAlertFn() func(w http.ResponseWriter, r *http.Request, fingerprint string, a *agent.AlertActionActor) {
	return func(w http.ResponseWriter, r *http.Request, fingerprint string, a *agent.AlertActionActor) {
		s.handleAlertReopen(w, r, fingerprint, agentAlertActorToAPI(a))
	}
}

// AgentWriteAlertsQueryResponseFn returns the alert-list serializer for
// agent.Service.
func (s *Server) AgentWriteAlertsQueryResponseFn() func(w http.ResponseWriter, r *http.Request) {
	return s.writeAlertsQueryResponse
}

// AgentPostIncidentSummaryFn returns the incident-summary ingestion callback
// for agent.Service.
func (s *Server) AgentPostIncidentSummaryFn() func(ctx context.Context, agentRec *store.AgentTokenRecord, incidentID, text string) error {
	return s.handleIncidentSummaryFromAgent
}

// AgentScheduleDisplayNameFn returns the on-call schedule display-name
// resolver for agent.Service.
func (s *Server) AgentScheduleDisplayNameFn() func(ctx context.Context, sched *store.OnCallScheduleRecord) string {
	return s.scheduleDisplayName
}

// AgentRevokeTokenByIDFn returns the operator token-revocation helper for
// agent.Service.
func (s *Server) AgentRevokeTokenByIDFn() func(w http.ResponseWriter, r *http.Request, idHex string, revokeFn func(uuid.UUID) error, kind string) {
	return s.revokeTokenByID
}

// AgentAlertCascadeFn returns the incident→alert cascade runner for
// AgentToolExecutor. It adapts the agent (agentID, agentName) pair into the api
// CascadeActor and delegates to runAlertCascade.
func (s *Server) AgentAlertCascadeFn() func(ctx context.Context, alertStore store.Store, auditStore store.AuditStore, publisher *sse.DualPublisher, incidentNumber int64, agentID uuid.UUID, agentName string) store.AlertCascadeResult {
	return func(ctx context.Context, alertStore store.Store, auditStore store.AuditStore, publisher *sse.DualPublisher, incidentNumber int64, agentID uuid.UUID, agentName string) store.AlertCascadeResult {
		return runAlertCascade(ctx, alertStore, auditStore, cascadePublisherFromDual(publisher), incidentNumber, CascadeActor{
			ID:   agentID,
			Type: "agent",
			Name: agentName,
		})
	}
}

// AgentPostMortemBuilderFn returns the post-mortem draft builder for
// AgentToolExecutor. It adapts the loose store args into postMortemDraftDeps.
func (s *Server) AgentPostMortemBuilderFn() func(ctx context.Context, documentStore store.IncidentDocumentStore, coordinationStore store.IncidentCoordinationStore, incidentStore store.IncidentStore, alertStore store.Store, inc *store.IncidentRecord, summary string) *store.PostMortemRecord {
	return func(ctx context.Context, documentStore store.IncidentDocumentStore, coordinationStore store.IncidentCoordinationStore, incidentStore store.IncidentStore, alertStore store.Store, inc *store.IncidentRecord, summary string) *store.PostMortemRecord {
		return buildPostMortemDraft(ctx, postMortemDraftDeps{
			documentStore:     documentStore,
			coordinationStore: coordinationStore,
			incidentStore:     incidentStore,
			alertStore:        alertStore,
		}, inc, summary)
	}
}

// AgentCoordinationForwarderFn returns the coordination-update forwarder for
// AgentToolExecutor. It replicates the legacy executor method body using the
// Server's cross-domain stores and the unexported coordinationMessage* helpers
// so package agent does not need to import alga/api.
func (s *Server) AgentCoordinationForwarderFn() func(ctx context.Context, incidentNumber int64, messageText string, mentions []string, agentRec *store.AgentTokenRecord) {
	return func(ctx context.Context, incidentNumber int64, messageText string, mentions []string, agentRec *store.AgentTokenRecord) {
		if s.investigationForwarder == nil || s.incidentInvestigationStore == nil || agentRec == nil {
			return
		}
		investigations, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
		if err != nil {
			logger.WarnCtx(ctx, "failed to list incident investigations for coordination forwarding", "incident_number", incidentNumber, "error", err)
			return
		}
		activeRoles, err := s.icsRoleStore.GetActiveRoles(ctx, incidentNumber)
		if err != nil {
			logger.WarnCtx(ctx, "failed to list active ICS roles for coordination forwarding", "incident_number", incidentNumber, "error", err)
		}

		incidentStatus := ""
		if s.incidentStore != nil {
			if inc, err := s.incidentStore.GetIncident(ctx, incidentNumber); err == nil && inc != nil {
				incidentStatus = inc.Status
			}
		}

		chatID := "incident_coord_" + strconv.FormatInt(incidentNumber, 10)
		if coordinationMessageHasNoContentAfterMentions(messageText) {
			logger.InfoCtx(ctx, "coordination update is a bare mention, suppressing agent activation", "incident_number", incidentNumber, "mentions", len(mentions), "agent", agentRec.Name)
			return
		}
		for _, recipient := range coordinationAgentRecipients(investigations, activeRoles, mentions, agentRec.ID.String()) {
			event := coordinationMessageEvent(chatID, messageText, agentRec.ID.String(), agentRec.Name, recipient.Trigger, strconv.FormatInt(incidentNumber, 10), recipient.RoleType, incidentStatus)
			if err := s.investigationForwarder.ForwardEventToAgent(recipient.AgentID, event); err != nil {
				logger.WarnCtx(ctx, "failed to forward coordination update to agent", "incident_number", incidentNumber, "agent_id", recipient.AgentID, "trigger", recipient.Trigger, "error", err)
			}
		}
	}
}
