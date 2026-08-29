package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/ics"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

var validCoordinationAudiences = map[string]bool{
	"none":         true,
	"commander":    true,
	"communicator": true,
	"command":      true,
}

var validCoordinationUrgencies = map[string]bool{
	"info":            true,
	"needs_attention": true,
	"decision_needed": true,
}

var validCoordinationStatusLevels = map[string]bool{
	"investigating": true,
	"identified":    true,
	"mitigated":     true,
	"monitoring":    true,
	"resolved":      true,
}

func normalizeCoordinationAudience(audience string) string {
	audience = strings.TrimSpace(strings.ToLower(audience))
	if audience == "" {
		return "none"
	}
	return audience
}

func normalizeCoordinationUrgency(urgency string) string {
	urgency = strings.TrimSpace(strings.ToLower(urgency))
	if urgency == "" {
		return "info"
	}
	return urgency
}

func normalizeCoordinationStatusLevel(statusLevel string) string {
	statusLevel = strings.TrimSpace(strings.ToLower(statusLevel))
	if statusLevel == "" {
		return "investigating"
	}
	return statusLevel
}

func audienceRoles(audience string) []ics.RoleType {
	switch audience {
	case "commander":
		return []ics.RoleType{ics.RoleIncidentCommander}
	case "communicator":
		return []ics.RoleType{ics.RoleCommunicationsLead}
	case "command":
		return []ics.RoleType{ics.RoleIncidentCommander, ics.RoleCommunicationsLead}
	default:
		return nil
	}
}

func (e *AgentToolExecutor) resolveCoordinationAudienceMentions(ctx context.Context, incidentNumber int64, audience string, originAgentID uuid.UUID) ([]string, []string, []string, error) {
	roles := audienceRoles(audience)
	if len(roles) == 0 {
		return nil, nil, nil, nil
	}
	if e.icsRoleStore == nil {
		return nil, nil, nil, errors.New("ICS role store not configured")
	}
	activeRoles, err := e.icsRoleStore.GetActiveRoles(ctx, incidentNumber)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get active ICS roles: %w", err)
	}

	wanted := make(map[string]ics.RoleType, len(roles))
	for _, role := range roles {
		wanted[string(role)] = role
	}
	seenMentions := make(map[string]bool)
	resolvedRoles := make(map[string]bool)
	mentions := make([]string, 0, len(roles))

	for _, role := range activeRoles {
		roleType, ok := wanted[role.RoleType]
		if !ok || role.Status != string(ics.RoleStatusActive) {
			continue
		}
		resolvedRoles[string(roleType)] = true
		switch role.AssigneeType {
		case "agent":
			if role.AgentTokenID == nil || *role.AgentTokenID == originAgentID {
				continue
			}
			mention := "agent:" + role.AgentTokenID.String()
			if !seenMentions[mention] {
				mentions = append(mentions, mention)
				seenMentions[mention] = true
			}
		case "user":
			if role.UserID == nil {
				continue
			}
			mention := "user:" + role.UserID.String()
			if !seenMentions[mention] {
				mentions = append(mentions, mention)
				seenMentions[mention] = true
			}
		}
	}

	unresolved := make([]string, 0)
	for _, role := range roles {
		if !resolvedRoles[string(role)] {
			unresolved = append(unresolved, string(role))
		}
	}
	return mentions, unresolved, coordinationAudienceMentionLinks(activeRoles, roles, originAgentID), nil
}

func coordinationAudienceMentionLinks(activeRoles []store.ICSRoleRecord, roles []ics.RoleType, originAgentID uuid.UUID) []string {
	links := make([]string, 0, len(roles))
	seen := map[string]bool{}
	for _, wantedRole := range roles {
		for _, role := range activeRoles {
			if role.RoleType != string(wantedRole) || role.Status != string(ics.RoleStatusActive) {
				continue
			}
			switch role.AssigneeType {
			case "agent":
				if role.AgentTokenID == nil || *role.AgentTokenID == originAgentID {
					continue
				}
				mention := "agent:" + role.AgentTokenID.String()
				if seen[mention] {
					continue
				}
				links = append(links, fmt.Sprintf("[@%s](%s)", coordinationMentionLabel(role, wantedRole), mention))
				seen[mention] = true
			case "user":
				if role.UserID == nil {
					continue
				}
				mention := "user:" + role.UserID.String()
				if seen[mention] {
					continue
				}
				links = append(links, fmt.Sprintf("[@%s](%s)", coordinationMentionLabel(role, wantedRole), mention))
				seen[mention] = true
			}
		}
	}
	return links
}

func coordinationMentionLabel(role store.ICSRoleRecord, roleType ics.RoleType) string {
	switch role.AssigneeType {
	case "agent":
		if strings.TrimSpace(role.AgentName) != "" {
			return strings.TrimSpace(role.AgentName)
		}
	case "user":
		if strings.TrimSpace(role.UserName) != "" {
			return strings.TrimSpace(role.UserName)
		}
		if strings.TrimSpace(role.UserEmail) != "" {
			return strings.TrimSpace(role.UserEmail)
		}
	}
	return ics.RoleLabel(roleType)
}

func (e *AgentToolExecutor) performPostHandoff(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if e.incidentCoordinationStore == nil {
		return errors.New("incident coordination store not configured")
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	if e.incidentInvestigationStore == nil {
		return errors.New("incident investigation store not configured")
	}
	if e.icsRoleStore == nil {
		return errors.New("ICS role store not configured")
	}
	message := strings.TrimSpace(cmd.Message)
	if message == "" {
		return errors.New("message is required")
	}
	audience := normalizeCoordinationAudience(cmd.Audience)
	if !validCoordinationAudiences[audience] {
		return errors.New("audience must be one of: none, commander, communicator, command")
	}
	// NOTE: the previous restriction that forced the commander to use
	// alga_request_status_update for communicator audience has been removed; the
	// commander may now post a coordinator update with audience=communicator
	// directly when they need to.
	urgency := normalizeCoordinationUrgency(cmd.Urgency)
	if !validCoordinationUrgencies[urgency] {
		return errors.New("urgency must be one of: info, needs_attention, decision_needed")
	}
	// status_level is no longer accepted on alga_post_handoff. The handoff is a
	// coordination tool, not a status-update tool; status milestones must be
	// published via alga_publish_status_update. Reject any caller that still
	// tries to pass status_level here so the agent doesn't fall back to using
	// this field as a fake status update.
	if strings.TrimSpace(cmd.StatusLevel) != "" {
		return errors.New("alga_post_handoff no longer accepts status_level; publish milestones via alga_publish_status_update (which writes to the Status Updates card) — do not pass status_level on the handoff")
	}
	if !e.agentCanActOnIncident(ctx, agentRec, agent, incidentNumber) {
		return errors.New("agent is not assigned to this incident")
	}

	// Responder-only gate: the responder must not post coordination updates
	// during active investigation work — every call activates other agents
	// (commander/communicator) and causes ping-pong loops. The responder may
	// post at most ONE coordination update, and only AFTER they have already
	// published BOTH status_level='identified' (root cause found) AND
	// status_level='monitoring' (recovery verified) status updates via
	// alga_publish_status_update. The Status Updates card must reflect the full
	// milestone sequence (investigating → identified → monitoring) before the
	// handoff so the commander has paper evidence that the responder actually
	// followed the workflow. Agents that also hold the commander or
	// communications-lead role (e.g. a commander who is also assigned as
	// responder) are exempt because they have a legitimate need to coordinate.
	if err := e.responderPostHandoffGate(ctx, agentRec.ID, incidentNumber); err != nil {
		return err
	}

	inv, _ := e.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(ctx, incidentNumber)
	mentions, unresolvedRoles, mentionLinks, err := e.resolveCoordinationAudienceMentions(ctx, incidentNumber, audience, agentRec.ID)
	if err != nil {
		return err
	}
	messageBody := message
	if len(mentionLinks) > 0 {
		messageBody = strings.Join(mentionLinks, " ") + " " + message
	}

	metadata := map[string]any{
		"source_tool":      "post_handoff",
		"audience":         audience,
		"urgency":          urgency,
		"origin_agent_id":  agentRec.ID.String(),
		"mentions":         mentions,
		"unresolved_roles": unresolvedRoles,
		"agent_type":       agentRec.AgentType,
	}
	record := &store.IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             store.IncidentCoordinationKindAgentReply,
		ActorType:        store.IncidentCoordinationActorAgent,
		ActorID:          &agentRec.ID,
		ActorDisplayName: agentRec.Name,
		Body:             messageBody,
		Source:           store.IncidentCoordinationSourceAgent,
		Metadata:         metadata,
	}
	if inv != nil {
		record.LinkedInvestigationID = inv.IncidentInvestigationID
	}
	created, err := e.incidentCoordinationStore.CreateMessage(ctx, record)
	if err != nil {
		return fmt.Errorf("create coordination update: %w", err)
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_coordination_message_created", Data: map[string]string{"incident_number": incID, "message_id": created.ID.String()}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentCoordinationMessageCreated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"message_id":      created.ID.String(),
			"kind":            created.Kind,
			"source_tool":     "post_handoff",
		})
	}
	e.auditCoordinationAgentMentions(agentRec, created, mentions)
	e.forwardCoordinationUpdateToAgents(ctx, incidentNumber, messageBody, mentions, agentRec)
	return nil
}

func validatePublicStatusUpdateMessage(message string) error {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "alert #") || strings.Contains(lower, "alert#") {
		return errors.New("public-facing status updates must not reference internal alert numbers; describe user-visible impact and current service state instead")
	}
	if strings.Contains(lower, "](agent:") || strings.Contains(lower, "](user:") || strings.Contains(lower, "agent:") || strings.Contains(lower, "user:") {
		return errors.New("public-facing status updates must not include internal agent or user mentions")
	}
	return nil
}

func (e *AgentToolExecutor) auditCoordinationAgentMentions(agentRec *store.AgentTokenRecord, message *store.IncidentCoordinationMessageRecord, mentions []string) {
	if e.auditStore == nil || agentRec == nil || message == nil || len(mentions) == 0 {
		return
	}
	originAgentID := agentRec.ID.String()
	for _, mention := range mentions {
		agentIDHex, ok := strings.CutPrefix(mention, "agent:")
		if !ok || agentIDHex == "" || agentIDHex == originAgentID {
			continue
		}
		e.auditStore.Log(store.AuditIncidentCoordinationAgentRequested, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_id": message.IncidentNumber,
			"message_id":  message.ID.String(),
			"agent":       agentIDHex,
		})
	}
}

func (e *AgentToolExecutor) forwardCoordinationUpdateToAgents(ctx context.Context, incidentNumber int64, messageText string, mentions []string, agentRec *store.AgentTokenRecord) {
	if e.forwardCoordinationUpdateFn == nil {
		return
	}
	e.forwardCoordinationUpdateFn(ctx, incidentNumber, messageText, mentions, agentRec)
}

func (e *AgentToolExecutor) performIncidentInvestigationPause(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if e.incidentInvestigationStore == nil {
		return errors.New("incident investigation store not configured")
	}
	inv, err := e.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(ctx, incidentNumber)
	if err != nil || inv == nil {
		return fmt.Errorf("no active investigation for incident %s", incID)
	}
	if err := e.incidentInvestigationStore.UpdateIncidentInvestigationStatus(ctx, inv.IncidentInvestigationID, store.IncidentInvestigationStatusPaused); err != nil {
		return fmt.Errorf("pause investigation: %w", err)
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "investigation_paused",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s paused investigation", agentRec.Name),
	})
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "investigation_status_changed", Data: map[string]string{"incident_number": incID, "investigation_id": inv.IncidentInvestigationID, "status": "paused"}})
	}
	return nil
}

func (e *AgentToolExecutor) performIncidentInvestigationCancel(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if e.incidentInvestigationStore == nil {
		return errors.New("incident investigation store not configured")
	}
	inv, err := e.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(ctx, incidentNumber)
	if err != nil || inv == nil {
		return fmt.Errorf("no active investigation for incident %s", incID)
	}
	if err := e.incidentInvestigationStore.UpdateIncidentInvestigationStatus(ctx, inv.IncidentInvestigationID, store.IncidentInvestigationStatusCancelled); err != nil {
		return fmt.Errorf("cancel investigation: %w", err)
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "investigation_cancelled",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s cancelled investigation", agentRec.Name),
	})
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "investigation_status_changed", Data: map[string]string{"incident_number": incID, "investigation_id": inv.IncidentInvestigationID, "status": "cancelled"}})
	}
	return nil
}

func (e *AgentToolExecutor) authorizeIncidentTool(ctx context.Context, agentRec *store.AgentTokenRecord, incidentNumber int64, op string) error {
	if agentRec == nil {
		return errors.New("agent is not authorized for this incident tool")
	}
	roles := e.activeAgentIncidentRoles(ctx, agentRec.ID, incidentNumber)
	assignedInvestigator := e.agentAssignedToActiveIncidentInvestigation(ctx, agentRec.ID.String(), incidentNumber)
	hasRole := func(role ics.RoleType) bool {
		return roles[string(role)]
	}

	switch op {
	case "post_handoff":
		if assignedInvestigator || hasRole(ics.RoleResponder) || hasRole(ics.RoleIncidentCommander) || hasRole(ics.RoleCommunicationsLead) {
			return nil
		}
	case "publish_status_update":
		if assignedInvestigator || hasRole(ics.RoleResponder) || hasRole(ics.RoleIncidentCommander) || hasRole(ics.RoleCommunicationsLead) {
			return nil
		}
	case "set_incident_severity":
		if assignedInvestigator || hasRole(ics.RoleResponder) || hasRole(ics.RoleIncidentCommander) {
			return nil
		}
	case "pause_investigation", "cancel_investigation", "post_investigation_thread_message":
		if assignedInvestigator || hasRole(ics.RoleResponder) {
			return nil
		}
	case "set_incident_priority", "trigger_escalation", "mitigate_incident", "resolve_incident", "begin_triage", "promote_incident", "assign_incident_role", "set_incident_resolution_docs":
		if hasRole(ics.RoleIncidentCommander) {
			return nil
		}
		if op == "resolve_incident" {
			return errors.New("investigator agents cannot resolve incidents directly; notify the incident commander for verification")
		}
	}
	return fmt.Errorf("agent is not assigned or not authorized for incident tool %q by active incident role", op)
}

func (e *AgentToolExecutor) activeAgentIncidentRoles(ctx context.Context, agentID uuid.UUID, incidentNumber int64) map[string]bool {
	roles := make(map[string]bool)
	if e.icsRoleStore == nil {
		return roles
	}
	activeRoles, err := e.icsRoleStore.GetActiveRoles(ctx, incidentNumber)
	if err != nil {
		logger.WarnCtx(ctx, "failed to check active incident roles for agent tool", "incident_number", incidentNumber, "agent_id", agentID.String(), "error", err)
		return roles
	}
	for _, role := range activeRoles {
		if role.Status == string(ics.RoleStatusActive) && role.AssigneeType == "agent" && role.AgentTokenID != nil && *role.AgentTokenID == agentID {
			roles[role.RoleType] = true
		}
	}
	return roles
}

func (e *AgentToolExecutor) agentAssignedToActiveIncidentInvestigation(ctx context.Context, agentID string, incidentNumber int64) bool {
	if e.incidentInvestigationStore == nil {
		return false
	}
	inv, err := e.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(ctx, incidentNumber)
	return err == nil && inv != nil && inv.AgentID == agentID
}

// agentIsResponderOnly reports whether the agent is assigned the Responder role
// on the incident WITHOUT also holding the Incident Commander or Communications
// Lead role. Agents that hold multiple roles (e.g. a commander who is also
// assigned as responder) are NOT considered responder-only because they have a
// legitimate need to use commander/comms tools, including publishing the
// resolved status update and posting coordination updates freely.
func (e *AgentToolExecutor) agentIsResponderOnly(ctx context.Context, agentID uuid.UUID, incidentNumber int64) bool {
	roles := e.activeAgentIncidentRoles(ctx, agentID, incidentNumber)
	if !roles[string(ics.RoleResponder)] {
		return false
	}
	if roles[string(ics.RoleIncidentCommander)] || roles[string(ics.RoleCommunicationsLead)] {
		return false
	}
	return true
}

// responderForbiddenStatusLevelOwner returns a short human-readable description
// of who owns a status_level the responder is not allowed to publish. Used in
// the rejection error message so the agent knows where to route the action.
func responderForbiddenStatusLevelOwner(statusLevel string) string {
	switch statusLevel {
	case "resolved":
		return "published by the incident commander when resolving the incident"
	case "investigating":
		return "posted automatically by the system when an investigation begins"
	default:
		return "reserved"
	}
}

// responderPostHandoffGate enforces the responder-only constraints on
// alga_post_handoff. It returns nil if the call is allowed, or an
// error explaining why it is rejected. Non-responder agents (commanders,
// communicators, assigned investigators without a responder role, etc.) bypass
// this gate entirely.
//
// Rules:
//  1. The responder must have ALREADY published a status_level=monitoring
//     status update via alga_publish_status_update. This signals that recovery
//     has been verified and the responder is ready for the commander handoff.
//  2. The responder may post AT MOST ONE coordination update per incident —
//     the single final handoff. Subsequent calls are rejected to prevent
//     mid-investigation noise that activates other agents.
func (e *AgentToolExecutor) responderPostHandoffGate(ctx context.Context, agentID uuid.UUID, incidentNumber int64) error {
	if !e.agentIsResponderOnly(ctx, agentID, incidentNumber) {
		return nil
	}
	// Carve-out: a human (actor_type=user) @mentioning this responder within
	// the last hour unlocks both the post_handoff tool and free-text replies
	// in the coordination thread, so a human operator can ask a direct
	// question and get a direct answer. Mentions from other agents do NOT
	// unlock the carve-out — they would restart ping-pong loops.
	if e.responderRecentlyMentionedByHuman(ctx, agentID, incidentNumber) {
		return nil
	}
	if e.incidentCoordinationStore == nil {
		// The handler already guards against a nil store before reaching here,
		// but be defensive: if there is no store, do not block the call.
		return nil
	}
	// Look at recent status updates and coordination replies on this incident.
	// We fetch a bounded window; the responder is expected to publish a small
	// number of status updates (identified + mitigated/monitoring) before the handoff.
	statusUpdates, err := e.incidentCoordinationStore.ListMessagesByKind(ctx, incidentNumber, store.IncidentCoordinationKindStatusUpdate, 50, 0)
	if err != nil {
		logger.WarnCtx(ctx, "failed to list status updates for responder coordination gate; allowing call", "incident_number", incidentNumber, "agent_id", agentID.String(), "error", err)
		return nil
	}
	publishedIdentified := false
	publishedMitigated := false
	publishedMonitoring := false
	for i := range statusUpdates {
		su := &statusUpdates[i]
		if su.ActorID == nil || *su.ActorID != agentID {
			continue
		}
		switch level, _ := su.Metadata["status_level"].(string); level {
		case "identified":
			publishedIdentified = true
		case "mitigated":
			publishedMitigated = true
		case "monitoring":
			publishedMonitoring = true
		}
	}
	if !publishedIdentified {
		return fmt.Errorf("responders must publish status_level=%q via alga_publish_status_update (root cause found) BEFORE posting any coordination update; alga_post_handoff activates other agents and is reserved for the single final commander handoff. Use alga_publish_status_update for ALL status communication during investigation, identification, mitigation, and verification", "identified")
	}
	// The handoff gate accepts EITHER `mitigated` (impact reduced, hand off to
	// commander for verification + resolution) OR `monitoring` (impact
	// reduced AND verification is in progress over time). `monitoring` is
	// intentionally optional — fast mitigation paths skip it and go straight
	// from `identified` to `mitigated`.
	if !publishedMitigated && !publishedMonitoring {
		return fmt.Errorf("responders must publish status_level=%q (impact reduced / partial fix applied) OR %q (verification in progress) via alga_publish_status_update BEFORE posting any coordination update; alga_post_handoff activates other agents and is reserved for the single final commander handoff. Use alga_publish_status_update for ALL status communication during investigation, identification, mitigation, and verification", "mitigated", "monitoring")
	}
	// One-shot rule: count prior coordination replies from this responder.
	coordReplies, err := e.incidentCoordinationStore.ListMessagesByKind(ctx, incidentNumber, store.IncidentCoordinationKindAgentReply, 50, 0)
	if err != nil {
		logger.WarnCtx(ctx, "failed to list coordination replies for responder one-shot gate; allowing call", "incident_number", incidentNumber, "agent_id", agentID.String(), "error", err)
		return nil
	}
	for i := range coordReplies {
		cr := &coordReplies[i]
		if cr.ActorID == nil || *cr.ActorID != agentID {
			continue
		}
		if sourceTool, _ := cr.Metadata["source_tool"].(string); sourceTool == "post_handoff" {
			return errors.New("responders may post at most ONE alga_post_handoff per incident (the final commander handoff). You have already posted one; the commander monitors the Status Updates card and will act on your monitoring update. Do not post additional coordination updates")
		}
	}
	return nil
}

// responderRecentlyMentionedByHuman reports whether a human (actor_type=user)
// has recently @mentioned the given responder agent in the incident's
// coordination thread. "Recently" means within the last hour. This is the
// carve-out that lets a human operator pull a responder into the coordination
// thread with a direct question and get a free-text reply back, even before
// the responder has published status_level=monitoring. Mentions from other
// agents do NOT trigger this carve-out — they would restart ping-pong loops.
func (e *AgentToolExecutor) responderRecentlyMentionedByHuman(ctx context.Context, agentID uuid.UUID, incidentNumber int64) bool {
	if e.incidentCoordinationStore == nil {
		return false
	}
	mention := "agent:" + agentID.String()
	cutoff := time.Now().Add(-1 * time.Hour)
	messages, err := e.incidentCoordinationStore.ListMessages(ctx, incidentNumber, 50, 0)
	if err != nil {
		logger.WarnCtx(ctx, "failed to list coordination messages for human-mention carve-out; denying", "incident_number", incidentNumber, "agent_id", agentID.String(), "error", err)
		return false
	}
	for i := range messages {
		m := &messages[i]
		if m.ActorType != store.IncidentCoordinationActorUser {
			continue
		}
		if m.CreatedAt.Before(cutoff) {
			continue
		}
		rawMentions, _ := m.Metadata["mentions"].([]string)
		if slices.Contains(rawMentions, mention) {
			return true
		}
	}
	return false
}

// CommanderOwnsAlertIncident verifies that the agent is the active incident
// commander for an incident that owns the given alert. Ownership is established
// via the alert's current alert-investigation being promoted to that incident.
// Returns nil if authorized, an error otherwise.
func (e *AgentToolExecutor) CommanderOwnsAlertIncident(ctx context.Context, agentID uuid.UUID, alertNumber int64) error {
	if e.alertInvestigationStore == nil || e.incidentStore == nil || e.icsRoleStore == nil {
		return fmt.Errorf("unable to verify incident scope for alert #%d", alertNumber)
	}
	inv, err := e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, alertNumber)
	if err != nil || inv == nil || inv.PromotedIncidentID == nil {
		return fmt.Errorf("alert #%d is not linked to an incident you are assigned to", alertNumber)
	}
	inc, err := e.incidentStore.GetIncidentByID(ctx, *inv.PromotedIncidentID)
	if err != nil || inc == nil {
		return fmt.Errorf("alert #%d is not linked to an incident you are assigned to", alertNumber)
	}
	roles := e.activeAgentIncidentRoles(ctx, agentID, inc.IncidentNumber)
	if !roles[string(ics.RoleIncidentCommander)] {
		return fmt.Errorf("agent is not the active incident commander for incident #%d", inc.IncidentNumber)
	}
	return nil
}

func (e *AgentToolExecutor) agentCanActOnIncident(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64) bool {
	if agentRec == nil {
		return false
	}
	if len(e.activeAgentIncidentRoles(ctx, agentRec.ID, incidentNumber)) > 0 {
		return true
	}
	agentID := agentRec.ID.String()
	if e.agentAssignedToActiveIncidentInvestigation(ctx, agentID, incidentNumber) {
		return true
	}
	if e.alertInvestigationStore == nil || agentRec == nil {
		return false
	}
	inc, incErr := e.incidentStore.GetIncident(ctx, incidentNumber)
	if incErr != nil || inc == nil {
		return false
	}
	inv, err := e.alertInvestigationStore.ListAlertInvestigations(ctx, map[string]any{"promoted_incident_id": inc.ID.String()})
	if err != nil {
		logger.WarnCtx(ctx, "failed to check incident assignment", "incident_number", incidentNumber, "agent_id", agentID, "error", err)
		return false
	}
	for _, i := range inv {
		if i.AgentID == agentID && !store.IsTerminalInvestigationStatus(i.Status) {
			return true
		}
	}
	return false
}

func (e *AgentToolExecutor) performPublishStatusUpdate(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if e.incidentCoordinationStore == nil {
		return errors.New("incident coordination store not configured")
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	message := strings.TrimSpace(cmd.Message)
	if message == "" {
		return errors.New("message is required")
	}
	statusLevel := normalizeCoordinationStatusLevel(cmd.StatusLevel)
	if !validCoordinationStatusLevels[statusLevel] {
		return errors.New("status_level must be one of: investigating, identified, monitoring, resolved")
	}
	if err := validatePublicStatusUpdateMessage(message); err != nil {
		return err
	}
	if !e.agentCanActOnIncident(ctx, agentRec, agent, incidentNumber) {
		return errors.New("agent is not assigned to this incident")
	}
	// Responder-only gate: responders may only publish `identified` or
	// `monitoring` status updates. The initial `investigating` update is posted
	// automatically by the system, and the `resolved` update is the commander's
	// sole responsibility. An agent that also holds the commander or
	// communications-lead role is exempt.
	if statusLevel == "resolved" || statusLevel == "investigating" {
		if e.agentIsResponderOnly(ctx, agentRec.ID, incidentNumber) {
			return fmt.Errorf("responders cannot publish status_level=%q; use status_level=\"identified\" or status_level=\"monitoring\". The %q update is %s — direct it to the incident commander", statusLevel, statusLevel, responderForbiddenStatusLevelOwner(statusLevel))
		}
	}
	// NOTE: a Communications Lead may still be auto-assigned to the incident, but
	// per current operating policy the commander publishes status updates directly.
	// The deferral that previously routed milestone publishes through the
	// communicator (alga_request_status_update -> ForwardEventToAgent) has been
	// disabled; the commander can publish any status_level they are entitled to.
	metadata := map[string]any{"status_level": statusLevel, "source_tool": "publish_status_update", "agent_type": agentRec.AgentType}
	if src := strings.TrimSpace(cmd.SourceCoordinationMessageID); src != "" {
		metadata["source_coordination_message_id"] = src
	}
	created, err := e.incidentCoordinationStore.CreateMessage(ctx, &store.IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             store.IncidentCoordinationKindStatusUpdate,
		ActorType:        store.IncidentCoordinationActorAgent,
		ActorID:          &agentRec.ID,
		ActorDisplayName: agentRec.Name,
		Body:             message,
		Internal:         cmd.Internal,
		Source:           store.IncidentCoordinationSourceAgent,
		Metadata:         metadata,
	})
	if err != nil {
		return fmt.Errorf("publish status update: %w", err)
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "action": "status_update"}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentStatusUpdateCreated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"message_id":      created.ID.String(),
			"source_tool":     "publish_status_update",
		})
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "status_update",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s published a status update", agentRec.Name),
	})
	return nil
}
