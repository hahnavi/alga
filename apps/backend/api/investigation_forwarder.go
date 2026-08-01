package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/api/platform"
	"alga/logger"
	"alga/sse"
	"alga/store"
	"alga/valkey"
	"alga/webhook"
)

type DefaultInvestigationForwarder struct {
	AgentTokens                store.AgentTokenStore
	AgentSSE                   *agent.AgentSSEHandler
	Presence                   *valkey.Presence
	AlertInvestigationStore    store.AlertInvestigationStore
	IncidentInvestigationStore store.IncidentInvestigationStore
	IncidentStore              store.IncidentStore
}

var _ webhook.InvestigationAgentForwarder = (*DefaultInvestigationForwarder)(nil)

func (f *DefaultInvestigationForwarder) ForwardToAgent(agentIDHex, investigationID, senderID, senderName, message string) error {
	return f.ForwardDispatchToAgent(agentIDHex, investigationID, senderID, senderName, message, "")
}

func (f *DefaultInvestigationForwarder) ForwardDispatchToAgent(agentIDHex, investigationID, senderID, senderName, message, systemContext string) error {
	if f == nil || f.AgentTokens == nil {
		return errors.New("investigation forwarder not configured")
	}
	logger.Info("ForwardDispatchToAgent", "agent_id", agentIDHex, "investigation_id", investigationID, "sender_id", senderID, "sender_name", senderName, "message_len", len(message))

	var chatID string
	if f.AlertInvestigationStore != nil {
		if inv, err := f.AlertInvestigationStore.GetAlertInvestigation(context.Background(), investigationID); err == nil && inv != nil {
			if inv.PromotedIncidentID != nil {
				incidentNumberStr := ""
				if f.IncidentStore != nil {
					if inc, err := f.IncidentStore.GetIncidentByID(context.Background(), *inv.PromotedIncidentID); err == nil && inc != nil {
						incidentNumberStr = strconv.FormatInt(inc.IncidentNumber, 10)
					}
				}
				if incidentNumberStr != "" {
					chatID = platform.BuildOwnerChatID(store.ThreadOwnerIncidentInvestigation, incidentNumberStr)
				}
			} else if len(inv.Alerts) > 0 && inv.Alerts[0].AlertNumber > 0 {
				chatID = platform.BuildOwnerChatID(store.ThreadOwnerAlert, strconv.FormatInt(inv.Alerts[0].AlertNumber, 10))
			}
		}
	}
	if chatID == "" && f.IncidentInvestigationStore != nil {
		if inv, err := f.IncidentInvestigationStore.GetIncidentInvestigation(context.Background(), investigationID); err == nil && inv != nil && inv.IncidentNumber != 0 {
			chatID = platform.BuildOwnerChatID(store.ThreadOwnerIncidentInvestigation, strconv.FormatInt(inv.IncidentNumber, 10))
		}
	}
	if chatID == "" {
		return fmt.Errorf("could not resolve owner-scoped chat ID for investigation %s", investigationID)
	}

	data := map[string]any{
		"type":        "message",
		"message_id":  uuid.NewString(),
		"chat_id":     chatID,
		"text":        message,
		"sender_id":   senderID,
		"sender_name": senderName,
		"trigger":     "dispatch",
	}
	if systemContext != "" {
		data["system_context"] = systemContext
	}

	event := sse.Event{
		Type: "message",
		Data: data,
	}

	if f.AgentSSE != nil {
		return f.AgentSSE.PublishToAgent(agentIDHex, event)
	}

	return errors.New("agent SSE handler not configured")
}

func (f *DefaultInvestigationForwarder) ForwardEventToAgent(agentIDHex string, event sse.Event) error {
	if f == nil || f.AgentTokens == nil {
		return errors.New("investigation forwarder not configured")
	}
	logger.Info("ForwardEventToAgent", "agent_id", agentIDHex, "event_type", event.Type)

	if f.AgentSSE != nil {
		return f.AgentSSE.PublishToAgent(agentIDHex, event)
	}

	return errors.New("agent SSE handler not configured")
}

func (f *DefaultInvestigationForwarder) AgentOnline(agentIDHex string) bool {
	if f == nil || f.AgentTokens == nil {
		return false
	}
	if f.AgentSSE != nil {
		return f.AgentSSE.AgentOnline(agentIDHex)
	}
	if f.Presence != nil && f.Presence.Available() {
		return f.Presence.IsAgentOnline(context.Background(), agentIDHex)
	}
	return false
}

func (f *DefaultInvestigationForwarder) BackfillThreadToAgent(ctx context.Context, agentIDHex, ownerType, ownerID string, threadStore store.InvestigationThreadStore) {
	if f.AgentSSE == nil || threadStore == nil {
		return
	}
	thread, _, err := threadStore.GetThreadByOwner(ctx, ownerType, ownerID, 200, 0)
	if err != nil || thread == nil {
		return
	}
	chatID := platform.BuildOwnerChatID(ownerType, ownerID)
	for _, msg := range thread.Messages {
		event := sse.Event{
			Type: "message",
			Data: map[string]any{
				"type":        "message",
				"message_id":  msg.ID.String(),
				"chat_id":     chatID,
				"text":        msg.Message,
				"sender_id":   msg.UserID,
				"sender_name": msg.Username,
				"trigger":     "observe",
			},
		}
		if err := f.AgentSSE.PublishToAgent(agentIDHex, event); err != nil {
			logger.Warn("failed to backfill thread message to agent", "agent_id", agentIDHex, "message_id", msg.ID, "error", err)
		}
	}
}
