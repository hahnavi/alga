package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

type createThreadMessageRequest struct {
	Message          string   `json:"message"`
	Mentions         []string `json:"mentions,omitempty"`
	ReplyToMessageID string   `json:"reply_to_message_id,omitempty"`
}

func (s *Server) handleAlertThread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	alertNumber, ok := alertNumberFromThreadPath(w, r, "/thread")
	if !ok {
		return
	}
	s.writeOwnerThread(w, r, store.ThreadOwnerAlert, alertNumber, http.StatusOK)
}

func (s *Server) handleAlertThreadTyping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	alertNumber, ok := alertNumberFromThreadPath(w, r, "/thread/typing")
	if !ok {
		return
	}
	s.forwardOwnerThreadTypingToAgent(store.ThreadOwnerAlert, alertNumber)
	writeStatus(w, "ok")
}

func (s *Server) handleAlertThreadMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	alertNumber, ok := alertNumberFromThreadPath(w, r, "/thread/messages")
	if !ok {
		return
	}
	if !s.createOwnerThreadMessage(w, r, store.ThreadOwnerAlert, alertNumber) {
		return
	}
	s.writeOwnerThread(w, r, store.ThreadOwnerAlert, alertNumber, http.StatusCreated)
}

func (s *Server) handleIncidentThread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	incidentID := ownerIDFromThreadPath(r, "incident_id", "/api/v1/incidents/", "/thread")
	if incidentID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
		return
	}
	s.writeOwnerThread(w, r, store.ThreadOwnerIncidentInvestigation, incidentID, http.StatusOK)
}

func (s *Server) handleIncidentThreadMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	incidentID := ownerIDFromThreadPath(r, "incident_id", "/api/v1/incidents/", "/thread/messages")
	if incidentID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
		return
	}
	if !s.createOwnerThreadMessage(w, r, store.ThreadOwnerIncidentInvestigation, incidentID) {
		return
	}
	s.writeOwnerThread(w, r, store.ThreadOwnerIncidentInvestigation, incidentID, http.StatusCreated)
}

func (s *Server) writeOwnerThread(w http.ResponseWriter, r *http.Request, ownerType string, ownerID string, status int) {
	if s.investigationThreadStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "investigation thread store not configured")
		return
	}
	limit, skip := parseLimitSkip(r, 50)
	thread, total, err := s.investigationThreadStore.GetThreadByOwner(r.Context(), ownerType, ownerID, limit, skip)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "investigation thread not found")
			return
		}
		writeInternalError(w, err, "failed to get investigation thread")
		return
	}
	if status == http.StatusOK {
		writePaginatedJSON(w, ensureSlice(thread.Messages), total)
		return
	}
	writeJSON(w, status, map[string]any{"items": ensureSlice(thread.Messages), "total": total})
}

func (s *Server) createOwnerThreadMessage(w http.ResponseWriter, r *http.Request, ownerType string, ownerID string) bool {
	if s.investigationThreadStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "investigation thread store not configured")
		return false
	}

	var req createThreadMessageRequest
	if !decodeJSON(w, r, &req) {
		return false
	}
	messageText := strings.TrimSpace(req.Message)
	if messageText == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "message is required")
		return false
	}

	thread, err := s.investigationThreadStore.EnsureThread(r.Context(), ownerType, ownerID)
	if err != nil {
		writeInternalError(w, err, "failed to get investigation thread")
		return false
	}

	message := store.InvestigationThreadMessage{
		Type:     string(store.UpdateTypeComment),
		Source:   string(store.UpdateSourceUser),
		Message:  messageText,
		Mentions: req.Mentions,
	}
	if user := userFromContext(r.Context()); user != nil {
		message.UserID = user.ID.String()
		message.Username = user.DisplayName()
	}
	if qid := strings.TrimSpace(req.ReplyToMessageID); qid != "" {
		message.ReplyToMessageID = qid
	}

	msgRec, err := s.investigationThreadStore.AddMessage(r.Context(), thread.ThreadID, message)
	if err != nil {
		writeInternalError(w, err, "failed to create investigation thread message")
		return false
	}

	replyToText, replyToAuthor := s.resolveReplyToMessage(thread.Messages, msgRec.ReplyToMessageID)

	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{
			Type: "owner_thread_message",
			Data: map[string]any{
				"owner_type": ownerType,
				"owner_id":   ownerID,
				"chat_id":    ownerType + "_" + ownerID,
				"message":    msgRec,
			},
		})
	}

	s.syncThreadMessageToExternalChat(ownerType, ownerID, messageText, userFromContext(r.Context()))

	s.forwardOwnerThreadMessageToAgent(ownerType, ownerID, messageText, userFromContext(r.Context()), req.Mentions, msgRec.ReplyToMessageID, replyToText, replyToAuthor)

	s.audit(r, store.AuditInvestigationUpdated, map[string]any{
		"owner_type": ownerType,
		"owner_id":   ownerID,
		"action":     "thread_message_created",
	})
	return true
}

func alertNumberFromThreadPath(w http.ResponseWriter, r *http.Request, routeSuffix string) (string, bool) {
	alertNumber := ownerIDFromThreadPath(r, "alert_number", "/api/v1/alerts/", routeSuffix)
	if alertNumber == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing alert number")
		return "", false
	}
	if _, err := strconv.ParseInt(alertNumber, 10, 64); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid alert number")
		return "", false
	}
	return alertNumber, true
}

func ownerIDFromThreadPath(r *http.Request, pathValueKey string, routePrefix string, routeSuffix string) string {
	if ownerID := r.PathValue(pathValueKey); ownerID != "" {
		return ownerID
	}
	ownerID := strings.TrimPrefix(r.URL.Path, routePrefix)
	return strings.TrimSuffix(ownerID, routeSuffix)
}

func (s *Server) syncThreadMessageToExternalChat(ownerType, ownerID, messageText string, user *store.UserRecord) {
	if s.chatSync == nil {
		return
	}
	ctx := context.Background()

	switch ownerType {
	case store.ThreadOwnerAlert:
		alertNum, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil {
			return
		}
		investigations, err := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(ctx, alertNum)
		if err != nil || len(investigations) == 0 {
			return
		}
		for i := len(investigations) - 1; i >= 0; i-- {
			inv := investigations[i]
			if inv.SlackChannelID != "" && inv.SlackThreadTS != "" {
				slMsg, cz := s.chatSync.UserSlackThreadMessage(user, messageText)
				s.chatSync.PostToSlackThreadWithCustomize(inv.SlackChannelID, inv.SlackThreadTS, slMsg, cz)
				return
			}
			mmThread := inv.PrimaryThreadID
			if mmThread == "" {
				mmThread = inv.MMThreadID
			}
			if mmThread != "" {
				displayName := "User"
				if user != nil {
					displayName = user.DisplayName()
					if displayName == "" {
						displayName = user.Email
					}
				}
				mmMsg := "**" + displayName + "**: " + messageText
				s.chatSync.PostToMattermostThread(mmThread, mmMsg)
				return
			}
		}

	case store.ThreadOwnerIncidentInvestigation:
		incidentNumber, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil {
			return
		}
		inc, err := s.incidentStore.GetIncident(ctx, incidentNumber)
		if err != nil || inc == nil {
			return
		}
		if inc.SlackChannelID != "" {
			slMsg, cz := s.chatSync.UserSlackThreadMessage(user, messageText)
			s.chatSync.PostToSlackThreadWithCustomize(inc.SlackChannelID, "", slMsg, cz)
			return
		}
		investigations, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
		if err != nil || len(investigations) == 0 {
			return
		}
		for i := len(investigations) - 1; i >= 0; i-- {
			inv := investigations[i]
			if inv.SlackChannelID != "" && inv.SlackThreadTS != "" {
				slMsg, cz := s.chatSync.UserSlackThreadMessage(user, messageText)
				s.chatSync.PostToSlackThreadWithCustomize(inv.SlackChannelID, inv.SlackThreadTS, slMsg, cz)
				return
			}
			mmThread := inv.PrimaryThreadID
			if mmThread == "" {
				mmThread = inv.MMThreadID
			}
			if mmThread != "" {
				displayName := "User"
				if user != nil {
					displayName = user.DisplayName()
					if displayName == "" {
						displayName = user.Email
					}
				}
				mmMsg := "**" + displayName + "**: " + messageText
				s.chatSync.PostToMattermostThread(mmThread, mmMsg)
				return
			}
		}
	}
}

func (s *Server) resolveThreadAgentIDs(ctx context.Context, ownerType, ownerID string) []string {
	seen := make(map[string]bool)
	var ids []string

	switch ownerType {
	case store.ThreadOwnerAlert:
		alertNum, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil {
			return nil
		}
		investigations, err := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(ctx, alertNum)
		if err != nil {
			return nil
		}
		for i := len(investigations) - 1; i >= 0; i-- {
			if aid := investigations[i].AgentID; aid != "" && !seen[aid] {
				seen[aid] = true
				ids = append(ids, aid)
			}
		}
	case store.ThreadOwnerIncidentInvestigation:
		incidentNumber, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil {
			return nil
		}
		investigations, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
		if err != nil {
			return nil
		}
		for i := len(investigations) - 1; i >= 0; i-- {
			if aid := investigations[i].AgentID; aid != "" && !seen[aid] {
				seen[aid] = true
				ids = append(ids, aid)
			}
		}
	}
	return ids
}

func triggerForAgent(mentions []string, agentIDHex string) string {
	for _, m := range mentions {
		if m == "agent:"+agentIDHex {
			return "mention"
		}
	}
	return "observe"
}

func (s *Server) resolveReplyToMessage(messages []store.InvestigationThreadMessage, replyToMessageID string) (text, author string) {
	replyToMessageID = strings.TrimSpace(replyToMessageID)
	if replyToMessageID == "" {
		return "", ""
	}
	for _, m := range messages {
		if m.ID.String() == replyToMessageID {
			return m.Message, m.Username
		}
	}
	return "", ""
}

func (s *Server) forwardOwnerThreadMessageToAgent(ownerType, ownerID, messageText string, user *store.UserRecord, mentions []string, replyToMessageID, replyToText, replyToAuthor string) {
	if s.agentSSE == nil {
		return
	}

	ctx := context.Background()
	agentIDs := s.resolveThreadAgentIDs(ctx, ownerType, ownerID)
	if len(agentIDs) == 0 {
		return
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

	chatID := platform.BuildOwnerChatID(ownerType, ownerID)
	for _, agentIDHex := range agentIDs {
		trigger := triggerForAgent(mentions, agentIDHex)
		data := map[string]any{
			"type":        "message",
			"message_id":  uuid.NewString(),
			"chat_id":     chatID,
			"text":        messageText,
			"sender_id":   senderID,
			"sender_name": senderName,
			"trigger":     trigger,
		}
		if replyToMessageID != "" {
			data["reply_to_message_id"] = replyToMessageID
			if replyToText != "" {
				data["reply_to_text"] = replyToText
			}
			if replyToAuthor != "" {
				data["reply_to_sender_name"] = replyToAuthor
			}
		}
		event := sse.Event{
			Type: "message",
			Data: data,
		}
		if err := s.agentSSE.PublishToAgent(agentIDHex, event); err != nil {
			logger.Warn("failed to forward owner thread message to agent", "owner_type", ownerType, "owner_id", ownerID, "agent_id", agentIDHex, "trigger", trigger, "error", err)
		}
	}
}

func (s *Server) forwardOwnerThreadTypingToAgent(ownerType, ownerID string) {
	if s.agentSSE == nil {
		return
	}

	ctx := context.Background()
	var agentIDHex string

	switch ownerType {
	case store.ThreadOwnerAlert:
		alertNum, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil {
			return
		}
		investigations, err := s.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(ctx, alertNum)
		if err != nil || len(investigations) == 0 {
			return
		}
		for i := len(investigations) - 1; i >= 0; i-- {
			if investigations[i].AgentID != "" {
				agentIDHex = investigations[i].AgentID
				break
			}
		}

	case store.ThreadOwnerIncidentInvestigation:
		incidentNumber, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil {
			return
		}
		investigations, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
		if err != nil || len(investigations) == 0 {
			return
		}
		for i := len(investigations) - 1; i >= 0; i-- {
			if investigations[i].AgentID != "" {
				agentIDHex = investigations[i].AgentID
				break
			}
		}
	}

	if agentIDHex == "" {
		return
	}

	chatID := platform.BuildOwnerChatID(ownerType, ownerID)
	event := sse.Event{
		Type: "typing",
		Data: map[string]any{
			"type":    "typing",
			"chat_id": chatID,
			"active":  true,
		},
	}

	if err := s.agentSSE.PublishToAgent(agentIDHex, event); err != nil {
		logger.Warn("failed to forward owner thread typing to agent", "owner_type", ownerType, "owner_id", ownerID, "agent_id", agentIDHex, "error", err)
	}
}
