package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"alga/logger"
	"alga/sse"
	"alga/store"
)

var agentMentionRe = regexp.MustCompile(`\[@[^\]]*\]\(agent:<?([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})>?\)`)

// ExtractAgentMentions parses inline `[@Name](agent:<uuid>)` mention syntax from
// free-form agent text and returns the deduplicated "agent:<uuid>" tokens in
// order of first appearance. Agent integration plugins post plain text replies
// without an explicit mentions field; without this recovery, ICS-role agents
// who are not incident-investigation agents (e.g. an incident commander) never
// receive @mentions embedded in teammate free text. Stray angle brackets around
// the UUID are tolerated because agents occasionally echo the prompt template
// placeholder `[@Name](agent:<id>)` verbatim.
func ExtractAgentMentions(text string) []string {
	matches := agentMentionRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		mention := "agent:" + m[1]
		if seen[mention] {
			continue
		}
		seen[mention] = true
		out = append(out, mention)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (e *AgentToolExecutor) HandleIncomingMessage(agentRec *store.AgentTokenRecord, chatID, text, senderID, senderName string, mentions []string, replyToMessageID string) (string, error) {
	if chatID == "" || text == "" {
		return "", errors.New("missing chat_id or text")
	}

	replyToMessageID = strings.TrimSpace(replyToMessageID)

	// Agent integration plugins post plain-text replies without an explicit
	// mentions field. Recover inline `[@Name](agent:<uuid>)` mentions from the
	// text so ICS-role agents (commander/communicator) who are not incident
	// investigation agents still get forwarded @mentions embedded in teammate
	// free text. Stray angle brackets around the UUID are tolerated because
	// agents occasionally echo the prompt template placeholder verbatim.
	if len(mentions) == 0 {
		mentions = ExtractAgentMentions(text)
	}

	if store.IsAlgaAgentDMChatID(chatID) && e.agentDMStore != nil {
		return e.handleAgentDMMessage(agentRec, chatID, text, senderID, senderName)
	}

	ownerType, ownerID := parseOwnerFromChatID(chatID)

	if ownerType != "" && ownerID != "" {
		displayName := senderName
		if displayName == "" {
			displayName = agentRec.Name
		}

		if store.IsIncidentThreadOwner(ownerType) {
			if ownerNum, err := strconv.ParseInt(ownerID, 10, 64); err == nil {
				return e.handleIncidentAgentTextMessage(agentRec, ownerType, ownerNum, text, displayName, mentions, replyToMessageID)
			}
		}

		if ownerType == store.ThreadOwnerAlert {
			alertNumber, parseErr := strconv.ParseInt(ownerID, 10, 64)
			if parseErr != nil || alertNumber <= 0 {
				return "", fmt.Errorf("invalid alert number: %q", ownerID)
			}
			if e.alertInvestigationStore == nil {
				return "", errors.New("alert investigation store not configured")
			}
			inv, err := e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(context.Background(), alertNumber)
			if err != nil {
				return "", fmt.Errorf("failed to get investigation: %w", err)
			}
			if err := authorizeAssignedAlertInvestigationAgent(agentRec, inv); err != nil {
				return "", err
			}
			if inv.Status == store.AlertInvestigationStatusAssigned {
				if err := e.alertInvestigationStore.TransitionAlertInvestigationStatus(context.Background(), inv.ID.String(), []string{store.AlertInvestigationStatusAssigned}, store.AlertInvestigationStatusInvestigating); err != nil {
					logger.Warn("agent message: assigned→investigating transition failed", "investigation_id", inv.AlertInvestigationID, "error", err)
					return "", errors.New("investigation status conflict, will be rescheduled")
				}
				inv.Status = store.AlertInvestigationStatusInvestigating
				e.publishInvestigationStatusChange(inv.AlertInvestigationID, store.AlertInvestigationStatusInvestigating)
			}
		}

		if e.threadStore == nil {
			return "", errors.New("investigation thread store not configured")
		}
		thread, err := e.threadStore.EnsureThread(context.Background(), ownerType, ownerID)
		if err != nil {
			logger.Warn("agent message: failed to ensure thread", "owner_type", ownerType, "owner_id", ownerID, "error", err)
			return "", fmt.Errorf("failed to ensure investigation thread: %w", err)
		}
		threadMsg := store.InvestigationThreadMessage{
			Type:      "comment",
			Source:    "agent",
			Message:   text,
			Username:  displayName,
			AgentType: agentRec.AgentType,
			Mentions:  mentions,
		}
		// Prefer an explicit sender_id (e.g. forwarded operator id) when present;
		// otherwise attribute the message to the authenticated agent token, so
		// every agent text message has a stable user_id. Without this fallback,
		// messages posted without sender_id (the common case for agent plugins)
		// get an empty user_id and appear as a distinct participant in the
		// thread UI. This mirrors HandleToolCallMessage's agentRec.ID fallback.
		if senderID != "" {
			threadMsg.UserID = senderID
		} else if agentRec.ID != uuid.Nil {
			threadMsg.UserID = agentRec.ID.String()
		}
		if replyToMessageID != "" {
			threadMsg.ReplyToMessageID = replyToMessageID
		}
		threadMsgRec, threadErr := e.threadStore.AddMessage(context.Background(), thread.ThreadID, threadMsg)
		if threadErr != nil {
			logger.Warn("agent message: failed to add thread message", "owner_type", ownerType, "owner_id", ownerID, "error", threadErr)
			return "", fmt.Errorf("failed to add investigation thread message: %w", threadErr)
		}
		logger.Info("agent message posted to owner thread", "owner_type", ownerType, "owner_id", ownerID, "thread_msg_id", threadMsgRec.ID.String())
		e.publishOwnerThreadEvent(ownerType, ownerID, "owner_thread_message", map[string]any{
			"message": threadMsgRec,
		})
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("goroutine panic recovered", "panic", r, "location", "sync-owner-thread-agent-message")
				}
			}()
			e.chatSyncSem <- struct{}{}
			defer func() { <-e.chatSyncSem }()
			e.syncOwnerThreadAgentMessage(ownerType, ownerID, displayName, text)
		}()
		return threadMsgRec.ID.String(), nil
	}

	return "", fmt.Errorf("invalid or unsupported chat_id format: %q (only owner-scoped threads are supported)", chatID)
}

// HandleToolCallMessage records a compact tool-invocation indicator (icon +
// tool name only, no arguments or result) in the owner-scoped investigation
// thread. Unlike HandleIncomingMessage it performs no status transition and
// does not sync to Mattermost/Slack — tool calls are Alga-internal visibility.
func (e *AgentToolExecutor) HandleToolCallMessage(agentRec *store.AgentTokenRecord, chatID, toolName string) (string, error) {
	if chatID == "" || toolName == "" {
		return "", errors.New("missing chat_id or tool name")
	}
	ownerType, ownerID := parseOwnerFromChatID(chatID)
	if ownerType == "" || ownerID == "" {
		return "", fmt.Errorf("invalid chat_id format: %q (only owner-scoped threads are supported)", chatID)
	}
	if e.threadStore == nil {
		return "", errors.New("investigation thread store not configured")
	}
	thread, err := e.threadStore.EnsureThread(context.Background(), ownerType, ownerID)
	if err != nil {
		return "", fmt.Errorf("failed to ensure investigation thread: %w", err)
	}
	threadMsg := store.InvestigationThreadMessage{
		Type:      "tool_call",
		Source:    "agent",
		Message:   toolName,
		Username:  agentRec.Name,
		AgentType: agentRec.AgentType,
	}
	if agentRec.ID != uuid.Nil {
		threadMsg.UserID = agentRec.ID.String()
	}
	threadMsgRec, err := e.threadStore.AddMessage(context.Background(), thread.ThreadID, threadMsg)
	if err != nil {
		return "", fmt.Errorf("failed to add tool_call thread message: %w", err)
	}
	logger.Info("agent tool_call posted to owner thread", "owner_type", ownerType, "owner_id", ownerID, "tool", toolName, "thread_msg_id", threadMsgRec.ID.String())
	e.publishOwnerThreadEvent(ownerType, ownerID, "owner_thread_message", map[string]any{
		"message": threadMsgRec,
	})
	return threadMsgRec.ID.String(), nil
}

// handleIncidentAgentTextMessage routes an incident-scoped free-text reply to the
// thread identified by its chat_id: incident_coord_<n> → coordination thread,
// incident_inv_<n> → incident investigation thread. The chat_id alone determines
// the destination; capability precedence is not used to disambiguate, so a
// dual-capability agent (e.g. the Responder) replies in whichever thread it was
// activated in.
func (e *AgentToolExecutor) handleIncidentAgentTextMessage(agentRec *store.AgentTokenRecord, ownerType string, incidentNumber int64, text, displayName string, mentions []string, replyToMessageID string) (string, error) {
	ctx := context.Background()
	incID := strconv.FormatInt(incidentNumber, 10)

	if ownerType == store.ThreadOwnerIncidentCoordination {
		if err := e.authorizeIncidentTool(ctx, agentRec, incidentNumber, "post_handoff"); err != nil {
			return "", err
		}
		// Apply the same responder-only gate as the post_handoff tool. A
		// free-text reply in the coordination thread is functionally
		// equivalent to a post_handoff call — it forwards to other agents via
		// forwardCoordinationUpdateToAgents and can start ping-pong loops —
		// so the same "monitoring-first" and one-shot constraints apply. The
		// human-mention carve-out is applied inside the gate.
		if err := e.responderPostHandoffGate(ctx, agentRec.ID, incidentNumber); err != nil {
			return "", err
		}
		return e.handleIncidentCoordinationTextMessage(ctx, agentRec, incidentNumber, text, displayName, mentions, replyToMessageID)
	}

	if err := e.authorizeIncidentTool(ctx, agentRec, incidentNumber, "post_investigation_thread_message"); err != nil {
		return "", err
	}
	if e.incidentInvestigationStore == nil {
		return "", errors.New("incident investigation store not configured")
	}

	inv, err := e.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(ctx, incidentNumber)
	if err != nil || inv == nil {
		return "", fmt.Errorf("no active investigation for incident %s", incID)
	}
	if e.threadStore != nil {
		thread, err := e.threadStore.EnsureThread(ctx, store.ThreadOwnerIncidentInvestigation, incID)
		if err != nil {
			return "", fmt.Errorf("failed to ensure incident owner thread: %w", err)
		}
		threadMsg := store.InvestigationThreadMessage{
			Type:      string(store.UpdateTypeComment),
			Source:    string(store.UpdateSourceAgent),
			Message:   text,
			Username:  displayName,
			AgentType: agentRec.AgentType,
			Mentions:  mentions,
		}
		if agentRec.ID != uuid.Nil {
			threadMsg.UserID = agentRec.ID.String()
		}
		if replyToMessageID != "" {
			threadMsg.ReplyToMessageID = replyToMessageID
		}
		created, err := e.threadStore.AddMessage(ctx, thread.ThreadID, threadMsg)
		if err != nil {
			return "", fmt.Errorf("failed to add incident owner thread message: %w", err)
		}
		e.publishOwnerThreadEvent(store.ThreadOwnerIncidentInvestigation, incID, "owner_thread_message", map[string]any{"message": created})
		logger.Info("agent text message posted to incident owner thread", "incident_number", incidentNumber, "message_id", created.ID.String())
		return created.ID.String(), nil
	}
	update := store.InvestigationUpdate{
		Type:     store.UpdateTypeComment,
		Message:  text,
		Source:   store.UpdateSourceAgent,
		Mentions: mentions,
	}
	if agentRec.ID != uuid.Nil {
		userID := agentRec.ID.String()
		update.UserID = &userID
	}
	if displayName != "" {
		update.Username = &displayName
	}
	if err := e.incidentInvestigationStore.AddIncidentInvestigationUpdate(ctx, inv.IncidentInvestigationID, update); err != nil {
		return "", fmt.Errorf("failed to persist incident investigation update: %w", err)
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_investigation_update", Data: map[string]any{
			"incident_number":               incidentNumber,
			"incident_investigation_id":     inv.IncidentInvestigationID,
			"incident_investigation_status": inv.Status,
			"update":                        update,
		}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditInvestigationUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number":           incidentNumber,
			"incident_investigation_id": inv.IncidentInvestigationID,
			"action":                    "incident_investigation_update_created",
		})
	}

	logger.Info("agent text message posted to incident investigation", "incident_number", incidentNumber, "incident_investigation_id", inv.IncidentInvestigationID)
	return inv.IncidentInvestigationID, nil
}

func (e *AgentToolExecutor) handleIncidentCoordinationTextMessage(ctx context.Context, agentRec *store.AgentTokenRecord, incidentNumber int64, text, displayName string, mentions []string, replyToMessageID string) (string, error) {
	if e.incidentCoordinationStore == nil {
		return "", errors.New("incident coordination store not configured")
	}
	incID := strconv.FormatInt(incidentNumber, 10)

	metadata := map[string]any{
		"source":     "agent_text_reply",
		"mentions":   mentions,
		"agent_type": agentRec.AgentType,
	}
	if replyToMessageID != "" {
		metadata["reply_to_message_id"] = replyToMessageID
	}

	record := &store.IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             store.IncidentCoordinationKindAgentReply,
		ActorType:        store.IncidentCoordinationActorAgent,
		ActorID:          &agentRec.ID,
		ActorDisplayName: displayName,
		Body:             text,
		Source:           store.IncidentCoordinationSourceAgent,
		Metadata:         metadata,
	}
	if record.ActorDisplayName == "" {
		record.ActorDisplayName = agentRec.Name
	}
	if e.incidentInvestigationStore != nil {
		if inv, err := e.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(ctx, incidentNumber); err == nil && inv != nil {
			record.LinkedInvestigationID = inv.IncidentInvestigationID
		}
	}

	created, err := e.incidentCoordinationStore.CreateMessage(ctx, record)
	if err != nil {
		return "", fmt.Errorf("create coordination text message: %w", err)
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_coordination_message_created", Data: map[string]string{"incident_number": incID, "message_id": created.ID.String()}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentCoordinationMessageCreated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incidentNumber,
			"message_id":      created.ID.String(),
			"kind":            created.Kind,
			"source":          "agent_text_reply",
		})
	}
	e.auditCoordinationAgentMentions(agentRec, created, mentions)
	e.forwardCoordinationUpdateToAgents(ctx, incidentNumber, text, mentions, agentRec)
	logger.Info("agent text message posted to incident coordination", "incident_number", incidentNumber, "message_id", created.ID.String())
	return created.ID.String(), nil
}

func (e *AgentToolExecutor) HandleEditMessage(chatID, messageID, text string, agentRec *store.AgentTokenRecord) error {
	if chatID == "" || messageID == "" || text == "" {
		return errors.New("missing chat_id, message_id, or text")
	}

	ownerType, ownerID := parseOwnerFromChatID(chatID)
	if ownerType != "" && ownerID != "" {
		if ownerType == store.ThreadOwnerIncidentCoordination {
			ownerNum, _ := strconv.ParseInt(ownerID, 10, 64)
			return e.editIncidentCoordinationMessage(context.Background(), agentRec, ownerNum, messageID, text)
		}
		if ownerType == store.ThreadOwnerIncidentInvestigation {
			ownerNum, _ := strconv.ParseInt(ownerID, 10, 64)
			if err := e.authorizeIncidentTool(context.Background(), agentRec, ownerNum, "post_investigation_thread_message"); err != nil {
				return err
			}
		}
		if ownerType == store.ThreadOwnerAlert {
			alertNumber, parseErr := strconv.ParseInt(ownerID, 10, 64)
			if parseErr != nil || alertNumber <= 0 {
				return fmt.Errorf("invalid alert number: %q", ownerID)
			}
			if e.alertInvestigationStore == nil {
				return errors.New("alert investigation store not configured")
			}
			inv, err := e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(context.Background(), alertNumber)
			if err != nil {
				return fmt.Errorf("failed to get investigation: %w", err)
			}
			if err := authorizeAssignedAlertInvestigationAgent(agentRec, inv); err != nil {
				return err
			}
		}
		if e.threadStore == nil {
			return errors.New("investigation thread store not configured")
		}
		msg, err := e.threadStore.UpdateMessage(context.Background(), ownerType, ownerID, messageID, text, false)
		if err != nil {
			return fmt.Errorf("failed to update investigation thread message: %w", err)
		}
		e.publishOwnerThreadEvent(ownerType, ownerID, "owner_thread_message_edited", map[string]any{
			"message_id": msg.ID.String(),
			"message":    msg.Message,
			"edited":     msg.Edited,
		})
		return nil
	}

	if store.IsAlgaAgentDMChatID(chatID) && e.agentDMStore != nil {
		tokenHex := agentRec.ID.String()
		if err := e.agentDMStore.UpdateMessageBody(tokenHex, messageID, text, false); err != nil {
			return err
		}
		e.PublishAgentDMEvent("agent_dm_message_edited", map[string]any{
			"agent_token_id": tokenHex,
			"chat_id":        store.AlgaAgentDMChatID(),
			"message_id":     messageID,
			"message":        text,
			"edited":         false,
		})
		return nil
	}

	return fmt.Errorf("invalid or unsupported chat_id format: %q (only owner-scoped threads are supported)", chatID)
}

type incidentCoordinationMessageUpdater interface {
	UpdateMessageBody(ctx context.Context, incidentNumber int64, messageID uuid.UUID, body string) (*store.IncidentCoordinationMessageRecord, error)
}

func (e *AgentToolExecutor) editIncidentCoordinationMessage(ctx context.Context, agentRec *store.AgentTokenRecord, incidentNumber int64, messageID, text string) error {
	if err := e.authorizeIncidentTool(ctx, agentRec, incidentNumber, "post_handoff"); err != nil {
		return err
	}
	updater, ok := e.incidentCoordinationStore.(incidentCoordinationMessageUpdater)
	if !ok || updater == nil {
		return errors.New("incident coordination store does not support message edits")
	}
	msgUUID, err := uuid.Parse(messageID)
	if err != nil {
		return fmt.Errorf("invalid coordination message id: %w", err)
	}
	updated, err := updater.UpdateMessageBody(ctx, incidentNumber, msgUUID, text)
	if err != nil {
		return err
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_coordination_message_updated", Data: map[string]string{"incident_number": strconv.FormatInt(incidentNumber, 10), "message_id": messageID}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentCoordinationMessageUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incidentNumber,
			"message_id":      messageID,
			"kind":            updated.Kind,
		})
	}
	return nil
}

func (e *AgentToolExecutor) HandleDeleteMessage(chatID, messageID string, agentRec *store.AgentTokenRecord) error {
	if chatID == "" || messageID == "" {
		return errors.New("missing chat_id or message_id")
	}

	ownerType, ownerID := parseOwnerFromChatID(chatID)
	if ownerType != "" && ownerID != "" {
		if ownerType == store.ThreadOwnerIncidentCoordination {
			return errors.New("coordination messages cannot be deleted via this endpoint")
		}
		if ownerType == store.ThreadOwnerIncidentInvestigation {
			ownerNum, _ := strconv.ParseInt(ownerID, 10, 64)
			if err := e.authorizeIncidentTool(context.Background(), agentRec, ownerNum, "post_investigation_thread_message"); err != nil {
				return err
			}
		}
		if ownerType == store.ThreadOwnerAlert {
			alertNumber, parseErr := strconv.ParseInt(ownerID, 10, 64)
			if parseErr != nil || alertNumber <= 0 {
				return fmt.Errorf("invalid alert number: %q", ownerID)
			}
			if e.alertInvestigationStore == nil {
				return errors.New("alert investigation store not configured")
			}
			inv, err := e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(context.Background(), alertNumber)
			if err != nil {
				return fmt.Errorf("failed to get investigation: %w", err)
			}
			if err := authorizeAssignedAlertInvestigationAgent(agentRec, inv); err != nil {
				return err
			}
		}
		if e.threadStore == nil {
			return errors.New("investigation thread store not configured")
		}
		if err := e.threadStore.DeleteMessage(context.Background(), ownerType, ownerID, messageID); err != nil {
			return fmt.Errorf("failed to delete investigation thread message: %w", err)
		}
		e.publishOwnerThreadEvent(ownerType, ownerID, "owner_thread_message_deleted", map[string]any{
			"message_id": messageID,
			"deleted_by": agentRec.Name,
		})
		return nil
	}

	if store.IsAlgaAgentDMChatID(chatID) && e.agentDMStore != nil {
		tokenHex := agentRec.ID.String()
		if err := e.agentDMStore.DeleteMessage(tokenHex, messageID); err != nil {
			return err
		}
		e.PublishAgentDMEvent("agent_dm_message_deleted", map[string]any{
			"agent_token_id": tokenHex,
			"chat_id":        store.AlgaAgentDMChatID(),
			"message_id":     messageID,
			"deleted_by":     agentRec.Name,
		})
		return nil
	}

	return fmt.Errorf("invalid or unsupported chat_id format: %q (only owner-scoped threads are supported)", chatID)
}

func (e *AgentToolExecutor) handleAgentDMMessage(agentRec *store.AgentTokenRecord, chatID, text, senderID, senderName string) (string, error) {
	if e.agentDMStore == nil {
		return "", errors.New("agent dm store not configured")
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("missing text")
	}
	tokenHex := agentRec.ID.String()
	var uidPtr, namePtr *string
	if strings.TrimSpace(senderID) != "" {
		s := strings.TrimSpace(senderID)
		uidPtr = &s
	}
	if strings.TrimSpace(senderName) != "" {
		s := strings.TrimSpace(senderName)
		namePtr = &s
	}
	rec, err := e.agentDMStore.AddMessage(tokenHex, store.AgentDMRoleAgent, text, uidPtr, namePtr)
	if err != nil {
		return "", err
	}
	e.PublishAgentDMEvent("agent_dm_message", map[string]any{
		"agent_token_id": tokenHex,
		"chat_id":        store.AlgaAgentDMChatID(),
		"message":        rec,
	})
	return rec.ID.String(), nil
}

func (e *AgentToolExecutor) HandleAgentTyping(agentRec *store.AgentTokenRecord, chatID string, active bool) bool {
	if store.IsAlgaAgentDMChatID(chatID) {
		if active {
			e.PublishAgentDMEvent("agent_dm_typing", map[string]any{
				"agent_token_id": agentRec.ID.String(),
				"chat_id":        store.AlgaAgentDMChatID(),
				"source":         AgentTypingSource(agentRec),
			})
		} else {
			e.PublishAgentDMEvent("agent_dm_typing_stop", map[string]any{
				"agent_token_id": agentRec.ID.String(),
				"chat_id":        store.AlgaAgentDMChatID(),
				"source":         AgentTypingSource(agentRec),
			})
		}
		return true
	}
	ownerType, ownerID := parseOwnerFromChatID(chatID)
	if ownerType != "" && ownerID != "" {
		if !e.authorizeOwnerThreadTyping(agentRec, ownerType, ownerID) {
			return false
		}
		eventType := "owner_thread_typing"
		if !active {
			eventType = "owner_thread_typing_stop"
		}
		e.publishOwnerThreadEvent(ownerType, ownerID, eventType, map[string]any{
			"source":     AgentTypingSource(agentRec),
			"agent_type": strings.TrimSpace(agentRec.AgentType),
		})
		return true
	}
	return false
}

func AgentTypingSource(agentRec *store.AgentTokenRecord) string {
	if agentRec != nil && strings.TrimSpace(agentRec.Name) != "" {
		return strings.TrimSpace(agentRec.Name)
	}
	return "Agent"
}

func (e *AgentToolExecutor) HandleAgentDraft(agentRec *store.AgentTokenRecord, chatID, draftID, text string) bool {
	if draftID == "" {
		return false
	}
	if store.IsAlgaAgentDMChatID(chatID) {
		e.PublishAgentDMEvent("agent_dm_draft", map[string]any{
			"agent_token_id": agentRec.ID.String(),
			"chat_id":        store.AlgaAgentDMChatID(),
			"draft_id":       draftID,
			"source":         "agent",
			"message":        text,
		})
		return true
	}
	ownerType, ownerID := parseOwnerFromChatID(chatID)
	if ownerType != "" && ownerID != "" {
		if !e.authorizeOwnerThreadTyping(agentRec, ownerType, ownerID) {
			return false
		}
		e.publishOwnerThreadEvent(ownerType, ownerID, "owner_thread_draft", map[string]any{
			"draft_id": draftID,
			"source":   "agent",
			"message":  text,
		})
		return true
	}
	return false
}

func (e *AgentToolExecutor) authorizeOwnerThreadTyping(agentRec *store.AgentTokenRecord, ownerType string, ownerID string) bool {
	switch ownerType {
	case store.ThreadOwnerIncidentInvestigation:
		ownerNum, _ := strconv.ParseInt(ownerID, 10, 64)
		return e.authorizeIncidentTool(context.Background(), agentRec, ownerNum, "post_investigation_thread_message") == nil
	case store.ThreadOwnerIncidentCoordination:
		ownerNum, _ := strconv.ParseInt(ownerID, 10, 64)
		return e.authorizeIncidentTool(context.Background(), agentRec, ownerNum, "post_handoff") == nil
	case store.ThreadOwnerAlert:
		if e.alertInvestigationStore == nil {
			return false
		}
		alertNumber, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil || alertNumber <= 0 {
			return false
		}
		record, err := e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(context.Background(), alertNumber)
		return err == nil && authorizeAssignedAlertInvestigationAgent(agentRec, record) == nil
	default:
		return false
	}
}
