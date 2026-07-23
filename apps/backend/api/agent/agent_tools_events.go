package agent

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"alga/logger"
	"alga/slack"
	"alga/sse"
	"alga/store"

	"alga/api/platform"
)

func (e *AgentToolExecutor) afterAlertMutation(ctx context.Context, fp string, actor *store.EventActor) {
	if e.alertSideEffects == nil || e.alertSideEffects.Store == nil {
		return
	}
	rec, err := e.alertSideEffects.Store.GetByFingerprint(fp)
	if err != nil || rec == nil {
		logger.Warn("agent alert op: could not reload alert", "fingerprint", fp, "error", err)
		return
	}
	if e.alertSideEffects.SyncChat != nil {
		e.alertSideEffects.SyncChat(ctx, rec)
	}
	if e.alertSideEffects.Publish != nil {
		e.alertSideEffects.Publish(rec)
	}
}

func (e *AgentToolExecutor) postCommandUpdate(ctx context.Context, investigationID string, inv *store.AlertInvestigationRecord, humanMsg string, actor *store.EventActor) {
	update := store.InvestigationUpdate{
		Type:     store.UpdateTypeAction,
		Message:  humanMsg,
		Source:   store.UpdateSourceSystem,
		Internal: true,
	}
	if actor != nil {
		name := actor.Username
		update.Username = &name
	}

	if err := e.alertInvestigationStore.AddAlertInvestigationUpdate(ctx, investigationID, update); err != nil {
		logger.Error("Failed to add tool update to investigation", "investigation_id", investigationID, "error", err)
		return
	}

	e.chatSync.postToInvestigationThread(inv, humanMsg, slack.Mrkdwn(humanMsg), &ChatSyncOptions{
		saveMMPostID: func(postID string) {
			update.MMPostID = postID
		},
	})

	e.publishInvestigationEvent(investigationID, "investigation_update", map[string]any{
		"alert_investigation_id": investigationID,
		"update":                 update,
		"status":                 inv.Status,
	})
}

func (e *AgentToolExecutor) publishInvestigationPatch(investigationID string) {
	rec, err := e.alertInvestigationStore.GetAlertInvestigation(context.Background(), investigationID)
	if err != nil || rec == nil {
		logger.Warn("could not reload investigation for patch event", "investigation_id", investigationID, "error", err)
		return
	}
	e.publishInvestigationEvent(investigationID, "investigation_patch", map[string]any{
		"alert_investigation_id": investigationID,
		"summary":                rec.Summary,
		"status":                 rec.Status,
	})
}

func (e *AgentToolExecutor) publishInvestigationStatusChange(investigationID, newStatus string) {
	data := map[string]any{
		"alert_investigation_id": investigationID,
		"status":                 newStatus,
	}
	if rec, err := e.alertInvestigationStore.GetAlertInvestigation(context.Background(), investigationID); err == nil && rec != nil {
		if rec.CompletedAt != nil {
			data["completed_at"] = rec.CompletedAt.Format(time.RFC3339Nano)
		}
		data["updated_at"] = rec.UpdatedAt.Format(time.RFC3339Nano)
		if rec.StartedAt != nil {
			data["started_at"] = rec.StartedAt.Format(time.RFC3339Nano)
		}
		if rec.InvestigatingDurationMs != 0 {
			data["investigating_duration_ms"] = rec.InvestigatingDurationMs
		}
		if rec.Summary != nil {
			data["summary"] = rec.Summary
		}
	}
	e.publishInvestigationEvent(investigationID, "investigation_status_changed", data)
}

func (e *AgentToolExecutor) publishInvestigationEvent(investigationID, eventType string, data any) {
	if e.ssePublisher == nil {
		return
	}
	e.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

func (e *AgentToolExecutor) PublishAgentDMEvent(eventType string, data map[string]any) {
	if e.ssePublisher == nil {
		return
	}
	e.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

func (e *AgentToolExecutor) PublishAgentPresence(agentIDHex string, online bool) {
	if e.ssePublisher == nil {
		return
	}
	e.ssePublisher.Publish(sse.Event{
		Type: "agent_presence",
		Data: map[string]any{
			"agent_id": agentIDHex,
			"online":   online,
		},
	})
}

func (e *AgentToolExecutor) syncOwnerThreadAgentMessage(ownerType, ownerID, senderName, text string) {
	if e.chatSync == nil {
		return
	}
	ctx := context.Background()

	switch ownerType {
	case store.ThreadOwnerAlert:
		alertNum, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil {
			return
		}
		investigations, err := e.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(ctx, alertNum)
		if err != nil || len(investigations) == 0 {
			return
		}
		for i := len(investigations) - 1; i >= 0; i-- {
			inv := investigations[i]
			if inv.SlackChannelID != "" && inv.SlackThreadTS != "" {
				e.chatSync.syncAgentMessage("", "", senderName, text, &inv)
				return
			}
			mmThread := inv.PrimaryThreadID
			if mmThread == "" {
				mmThread = inv.MMThreadID
			}
			if mmThread != "" {
				mmMsg := fmt.Sprintf("**%s**: %s", senderName, text)
				e.chatSync.PostToMattermostThread(mmThread, mmMsg)
				return
			}
		}

	case store.ThreadOwnerIncidentInvestigation:
		ownerNum, _ := strconv.ParseInt(ownerID, 10, 64)
		inc, err := e.incidentStore.GetIncident(ctx, ownerNum)
		if err != nil || inc == nil {
			return
		}
		if inc.SlackChannelID != "" {
			slMsg := slack.Mrkdwn(text)
			cz := slack.PostCustomize{
				Username: senderName,
				IconURL:  fmt.Sprintf("https://api.dicebear.com/9.x/bottts-neutral/png?seed=%s&size=128", url.QueryEscape(senderName)),
			}
			e.chatSync.PostToSlackThreadWithCustomize(inc.SlackChannelID, "", slMsg, &cz)
		}
	}
}

func (e *AgentToolExecutor) logAudit(op, agentName, investigationID, fingerprint string) {
	if e.auditStore == nil {
		return
	}
	auditEvent := mapToolAuditEvent(op)
	details := map[string]any{
		"alert_investigation_id": investigationID,
		"agent_source":           agentName,
	}
	if fingerprint != "" {
		details["fingerprint"] = fingerprint
	}
	e.auditStore.Log(auditEvent, nil, "agent:"+agentName, "", "", true, map[string]any(details))
}

func agentActor(agentRec *store.AgentTokenRecord) *store.EventActor {
	name := strings.TrimSpace(agentRec.Name)
	if name == "" {
		name = "agent"
	}
	return &store.EventActor{Username: name, Source: "agent"}
}

func (e *AgentToolExecutor) allInvestigationAlertsResolved(inv *store.AlertInvestigationRecord) bool {
	if e.alertSideEffects == nil || e.alertSideEffects.Store == nil {
		return false
	}
	if len(inv.Alerts) == 0 {
		return false
	}
	for _, a := range inv.Alerts {
		rec, err := e.alertSideEffects.Store.GetByFingerprint(a.Fingerprint)
		if err != nil {
			return false
		}
		if rec != nil && rec.Status != "resolved" {
			return false
		}
	}
	return true
}

func investigationFingerprint(inv *store.AlertInvestigationRecord, fp string) (string, error) {
	fp = strings.TrimSpace(fp)
	if fp != "" {
		for _, a := range inv.Alerts {
			if a.Fingerprint == fp {
				return fp, nil
			}
		}
		return "", fmt.Errorf("fingerprint %q is not linked to this investigation", fp)
	}
	if len(inv.Alerts) == 0 {
		return "", errors.New("investigation has no linked alerts")
	}
	return inv.Alerts[0].Fingerprint, nil
}

func resolveAlertLabel(alertStore store.Store, fp string) (string, int64) {
	alertName := fp
	var alertNumber int64
	alert, _ := alertStore.GetByFingerprint(fp)
	if alert != nil {
		if name, ok := alert.Labels["alertname"]; ok && name != "" {
			alertName = name
		}
		alertNumber = alert.AlertNumber
	}
	if alertNumber > 0 {
		alertName = fmt.Sprintf("#%d %s", alertNumber, alertName)
	}
	return alertName, alertNumber
}

func parseOwnerFromChatID(chatID string) (ownerType, ownerID string) {
	if strings.HasPrefix(chatID, "alert_") {
		return store.ThreadOwnerAlert, strings.TrimPrefix(chatID, "alert_")
	}
	if id, ok := incidentNumberFromIncidentChatIDRaw(chatID); ok {
		return incidentOwnerTypeFromChatID(chatID), id
	}
	return "", ""
}

// incidentOwnerTypeFromChatID returns the incident owner type implied by the
// chat_id prefix (incident_inv_ or incident_coord_). Caller must ensure chatID
// is incident-scoped.
func incidentOwnerTypeFromChatID(chatID string) string {
	if strings.HasPrefix(chatID, "incident_coord_") {
		return store.ThreadOwnerIncidentCoordination
	}
	return store.ThreadOwnerIncidentInvestigation
}

// incidentNumberFromIncidentChatID extracts the incident number from an
// incident-scoped chat_id (incident_inv_<n> or incident_coord_<n>).
func incidentNumberFromIncidentChatID(chatID string) (int64, bool) {
	id, ok := incidentNumberFromIncidentChatIDRaw(chatID)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func incidentNumberFromIncidentChatIDRaw(chatID string) (string, bool) {
	for _, p := range []string{"incident_coord_", "incident_inv_"} {
		if strings.HasPrefix(chatID, p) {
			return strings.TrimPrefix(chatID, p), true
		}
	}
	return "", false
}

func (e *AgentToolExecutor) publishOwnerThreadEvent(ownerType, ownerID, eventType string, data map[string]any) {
	if e.ssePublisher == nil {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["owner_type"] = ownerType
	data["owner_id"] = ownerID
	data["chat_id"] = platform.BuildOwnerChatID(ownerType, ownerID)
	e.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

func (e *AgentToolExecutor) ensureThreadMessage(ctx context.Context, ownerType, ownerID, msgType, message string) {
	if e.threadStore == nil {
		return
	}
	thread, err := e.threadStore.EnsureThread(ctx, ownerType, ownerID)
	if err != nil {
		logger.WarnCtx(ctx, "failed to ensure thread for owner", "owner_type", ownerType, "owner_id", ownerID, "error", err)
		return
	}
	_, _ = e.threadStore.AddMessage(ctx, thread.ThreadID, store.InvestigationThreadMessage{
		Type:     msgType,
		Source:   "system",
		Message:  message,
		Username: "System",
	})
}
