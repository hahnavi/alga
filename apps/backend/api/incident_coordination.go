package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"alga/ics"
	"alga/logger"
	"alga/rbac"
	"alga/sse"
	"alga/store"
)

// coordinationMessageHasNoContentAfterMentions reports whether a coordination
// message body is empty (or whitespace-only) once @mention links are removed.
// This is the only structural signal the system uses to suppress agent
// activation: a "message" that is just a mention (e.g. "[@wad1D4w](agent:...)")
// with no surrounding text is unambiguously a bare mention, and a bare mention
// is not worth waking an agent for. All other message-text classification is
// the agent's job and lives in the prompt, not in this file.
func coordinationMessageHasNoContentAfterMentions(message string) bool {
	if message == "" {
		return false
	}
	body := coordinationMentionRegex.ReplaceAllString(message, "")
	return strings.TrimSpace(body) == ""
}

var coordinationMentionRegex = regexp.MustCompile(`\[@[^\]]+\]\((?:agent|user):[^\)]+\)`)

type incidentCoordinationMessageRequest struct {
	Kind                  string         `json:"kind"`
	Body                  string         `json:"body"`
	Internal              bool           `json:"internal,omitempty"`
	Mentions              []string       `json:"mentions,omitempty"`
	LinkedInvestigationID string         `json:"linked_investigation_id,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
}

func (s *Server) handleIncidentCoordinationMessages(w http.ResponseWriter, r *http.Request, incidentID string) {
	if s.incidentCoordinationStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "incident coordination store not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListIncidentCoordinationMessages(w, r, incidentID)
	case http.MethodPost:
		s.handleCreateIncidentCoordinationMessage(w, r, incidentID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListIncidentCoordinationMessages(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	limit, skip := parseLimitSkip(r, 100)
	messages, err := s.incidentCoordinationStore.ListMessages(r.Context(), mustParseIncidentNumber(incidentID), int(limit), int(skip))
	if err != nil {
		writeInternalError(w, err, "failed to list incident coordination messages")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(messages))
}

func (s *Server) handleCreateIncidentCoordinationMessage(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	var req incidentCoordinationMessageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "body is required")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = store.IncidentCoordinationKindChat
	}
	user := userFromContext(r.Context())
	actorName := "User"
	var actorID *uuid.UUID
	if user != nil {
		actorID = &user.ID
		actorName = user.DisplayName()
		if actorName == "" {
			actorName = user.Email
		}
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	if len(req.Mentions) > 0 {
		metadata["mentions"] = req.Mentions
	}
	record := &store.IncidentCoordinationMessageRecord{
		IncidentNumber:        mustParseIncidentNumber(incidentID),
		Kind:                  kind,
		ActorType:             store.IncidentCoordinationActorUser,
		ActorID:               actorID,
		ActorDisplayName:      actorName,
		Body:                  body,
		Internal:              req.Internal,
		Source:                store.IncidentCoordinationSourceAlga,
		LinkedInvestigationID: req.LinkedInvestigationID,
		Metadata:              metadata,
	}
	created, err := s.incidentCoordinationStore.CreateMessage(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create incident coordination message")
		return
	}
	s.syncIncidentCoordinationMessageToSlack(r, created, user)
	s.publishIncidentEvent("incident_coordination_message_created", map[string]string{"incident_number": incidentID, "message_id": created.ID.String()})
	s.audit(r, store.AuditIncidentCoordinationMessageCreated, map[string]any{"incident_number": incidentID, "message_id": created.ID.String(), "kind": kind})
	s.handleIncidentCoordinationAgentMentions(r, created, req.Mentions)
	s.forwardCoordinationMessageToAgents(r.Context(), incidentID, body, userFromContext(r.Context()), req.Mentions, "")
	writeData(w, http.StatusCreated, created)
}

func (s *Server) syncIncidentCoordinationMessageToSlack(r *http.Request, message *store.IncidentCoordinationMessageRecord, user *store.UserRecord) {
	if message == nil || message.Internal || s.slackClient == nil || !s.slackClient.Enabled() || s.incidentStore == nil {
		return
	}
	incident, err := s.incidentStore.GetIncident(r.Context(), message.IncidentNumber)
	if err != nil || incident == nil || incident.SlackChannelID == "" || incident.SlackChannelArchived {
		return
	}
	displayName := "User"
	if user != nil {
		displayName = user.DisplayName()
		if displayName == "" {
			displayName = user.Email
		}
	}
	text := fmt.Sprintf("*%s*: %s", displayName, message.Body)
	var ts string
	ts, err = s.slackClient.PostMessage(r.Context(), incident.SlackChannelID, text)
	if err != nil {
		s.audit(r, store.AuditIncidentCoordinationBridgeFailed, map[string]any{"incident_id": message.IncidentNumber, "message_id": message.ID.String(), "provider": "slack", "error": err.Error()})
		return
	}
	if ts != "" {
		if err := s.incidentCoordinationStore.SetSlackMessageTS(r.Context(), message.ID, incident.SlackChannelID, ts, ts); err != nil {
			s.audit(r, store.AuditIncidentCoordinationBridgeFailed, map[string]any{"incident_id": message.IncidentNumber, "message_id": message.ID.String(), "provider": "slack", "error": fmt.Sprintf("save slack ts: %v", err)})
		}
	}
}

func (s *Server) createInvestigationSummaryCoordinationMessage(ctx context.Context, incidentID, investigationID, summary string) (*store.IncidentCoordinationMessageRecord, error) {
	if s.incidentCoordinationStore == nil {
		return nil, errors.New("incident coordination store not configured")
	}
	body := strings.TrimSpace(summary)
	if body == "" {
		return nil, errors.New("summary is required")
	}
	return s.incidentCoordinationStore.CreateMessage(ctx, &store.IncidentCoordinationMessageRecord{
		IncidentNumber:        mustParseIncidentNumber(incidentID),
		Kind:                  store.IncidentCoordinationKindInvestigationSummary,
		ActorType:             store.IncidentCoordinationActorSystem,
		ActorDisplayName:      "System",
		Body:                  body,
		Source:                store.IncidentCoordinationSourceSystem,
		LinkedInvestigationID: investigationID,
		Metadata:              map[string]any{"summary_source": "investigation"},
	})
}

func (s *Server) handleIncidentCoordinationAgentMentions(r *http.Request, message *store.IncidentCoordinationMessageRecord, mentions []string) {
	if message == nil || len(mentions) == 0 {
		return
	}
	for _, mention := range mentions {
		if !strings.HasPrefix(mention, "agent:") {
			continue
		}
		s.audit(r, store.AuditIncidentCoordinationAgentRequested, map[string]any{
			"incident_id": message.IncidentNumber,
			"message_id":  message.ID.String(),
			"agent":       strings.TrimPrefix(mention, "agent:"),
		})
	}
}

func (s *Server) forwardCoordinationMessageToAgents(ctx context.Context, incidentID, messageText string, user *store.UserRecord, mentions []string, excludeAgentID string) {
	if s.agentSSE == nil || s.incidentInvestigationStore == nil {
		return
	}
	investigations, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, mustParseIncidentNumber(incidentID))
	if err != nil {
		return
	}
	var activeRoles []store.ICSRoleRecord
	if s.icsRoleStore != nil {
		activeRoles, err = s.icsRoleStore.GetActiveRoles(ctx, mustParseIncidentNumber(incidentID))
		if err != nil {
			logger.WarnCtx(ctx, "failed to list active ICS roles for coordination forwarding", "incident_number", incidentID, "error", err)
		}
	}

	incidentStatus := ""
	if s.incidentStore != nil {
		if inc, err := s.incidentStore.GetIncident(ctx, mustParseIncidentNumber(incidentID)); err == nil && inc != nil {
			incidentStatus = inc.Status
		}
	}

	senderID := ""
	senderName := "User"
	if user != nil {
		senderID = user.ID.String()
		senderName = user.DisplayName()
		if senderName == "" {
			senderName = user.Email
		}
	}

	chatID := "incident_coord_" + incidentID
	if coordinationMessageHasNoContentAfterMentions(messageText) {
		logger.InfoCtx(ctx, "coordination message is a bare mention, suppressing agent activation", "incident_number", incidentID, "mentions", len(mentions))
		return
	}
	for _, recipient := range coordinationAgentRecipients(investigations, activeRoles, mentions, excludeAgentID) {
		event := coordinationMessageEvent(chatID, messageText, senderID, senderName, recipient.Trigger, incidentID, recipient.RoleType, incidentStatus)
		if err := s.agentSSE.PublishToAgent(recipient.AgentID, event); err != nil {
			logger.Warn("failed to forward coordination message to agent", "incident_number", incidentID, "agent_id", recipient.AgentID, "trigger", recipient.Trigger, "error", err)
		}
	}
}

type coordinationAgentRecipient struct {
	AgentID  string
	Trigger  string
	RoleType string
}

func coordinationAgentRecipients(investigations []store.IncidentInvestigationRecord, activeRoles []store.ICSRoleRecord, mentions []string, excludeAgentID string) []coordinationAgentRecipient {
	seen := make(map[string]bool)
	allowedMentionAgents := make(map[string]bool)
	roleByAgent := make(map[string]string)
	for _, role := range activeRoles {
		if role.Status != string(ics.RoleStatusActive) || role.AssigneeType != "agent" || role.AgentTokenID == nil {
			continue
		}
		agentHex := role.AgentTokenID.String()
		allowedMentionAgents[agentHex] = true
		if roleByAgent[agentHex] == "" {
			roleByAgent[agentHex] = role.RoleType
		}
	}

	recipients := make([]coordinationAgentRecipient, 0, len(investigations)+len(mentions))
	add := func(agentIDHex, trigger string) {
		agentIDHex = strings.TrimSpace(agentIDHex)
		if agentIDHex == "" || agentIDHex == excludeAgentID || seen[agentIDHex] {
			return
		}
		seen[agentIDHex] = true
		recipients = append(recipients, coordinationAgentRecipient{AgentID: agentIDHex, Trigger: trigger, RoleType: roleByAgent[agentIDHex]})
	}

	for i := len(investigations) - 1; i >= 0; i-- {
		if !isActiveIncidentInvestigationForCoordination(investigations[i].Status) {
			continue
		}
		agentIDHex := investigations[i].AgentID
		if strings.TrimSpace(agentIDHex) != "" {
			allowedMentionAgents[strings.TrimSpace(agentIDHex)] = true
		}
		add(agentIDHex, triggerForAgent(mentions, agentIDHex))
	}
	for _, mention := range mentions {
		if agentIDHex, ok := strings.CutPrefix(mention, "agent:"); ok {
			agentIDHex = strings.TrimSpace(agentIDHex)
			add(agentIDHex, "mention")
		}
	}
	return recipients
}

func isActiveIncidentInvestigationForCoordination(status string) bool {
	switch status {
	case store.IncidentInvestigationStatusPending, store.IncidentInvestigationStatusAssigned, store.IncidentInvestigationStatusInvestigating, store.IncidentInvestigationStatusPaused:
		return true
	default:
		return false
	}
}

func coordinationMessageEvent(chatID, messageText, senderID, senderName, trigger, incidentNumber, incidentRole, incidentStatus string) sse.Event {
	data := map[string]any{
		"type":        "message",
		"message_id":  uuid.NewString(),
		"chat_id":     chatID,
		"text":        messageText,
		"sender_id":   senderID,
		"sender_name": senderName,
		"trigger":     trigger,
	}
	if incidentNumber != "" {
		data["incident_number"] = incidentNumber
	}
	if incidentRole != "" {
		data["incident_role"] = incidentRole
	}
	if incidentStatus != "" {
		data["incident_status"] = incidentStatus
	}
	return sse.Event{
		Type: "message",
		Data: data,
	}
}
