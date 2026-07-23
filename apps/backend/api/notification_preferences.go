package api

import (
	"net/http"

	"alga/logger"
	"alga/store"
)

func (s *Server) handleNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getNotificationPreferences(w, r, user.ID.String())
	case http.MethodPut:
		s.updateNotificationPreferences(w, r, user.ID.String())
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) getNotificationPreferences(w http.ResponseWriter, r *http.Request, userID string) {
	prefs, err := s.userStore.GetNotificationPreferences(r.Context(), userID)
	if err != nil {
		writeInternalError(w, err, "failed to get notification preferences")
		return
	}
	if prefs == nil {
		prefs = map[string]any{}
	}
	writeData(w, http.StatusOK, prefs)
}

func (s *Server) updateNotificationPreferences(w http.ResponseWriter, r *http.Request, userID string) {
	var prefs map[string]any
	if !decodeJSON(w, r, &prefs) {
		return
	}

	if rulesRaw, ok := prefs["rules"]; ok {
		rules, ok := rulesRaw.([]any)
		if !ok {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "rules must be an array")
			return
		}
		for i, ruleRaw := range rules {
			rule, ok := ruleRaw.(map[string]any)
			if !ok {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "each rule must be an object")
				return
			}
			nt, ok := rule["notification_type"].(string)
			if !ok || nt == "" {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "each rule must have a notification_type string")
				return
			}
			channels, ok := rule["channels"].([]any)
			if !ok {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "each rule must have a channels array")
				return
			}
			for _, ch := range channels {
				if _, ok := ch.(string); !ok {
					writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "channels must be an array of strings")
					return
				}
			}
			_ = i
		}
	}

	if err := s.userStore.UpdateNotificationPreferences(r.Context(), userID, prefs); err != nil {
		writeInternalError(w, err, "failed to update notification preferences")
		return
	}

	logger.InfoCtx(r.Context(), "notification preferences updated", "component", "api", "user_id", userID)
	s.audit(r, store.AuditNotifPrefsUpdated, map[string]any{
		"user_id": userID,
	})
	writeData(w, http.StatusOK, prefs)
}

func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}

	if !s.requireStore(w, s.notificationStore, "notification store") {
		return
	}

	n := &store.NotificationRecord{
		UserID:       user.ID.String(),
		Type:         "test",
		Title:        "Test notification",
		Message:      "This is a test notification from Alga.",
		ResourceType: "system",
		ResourceID:   "test",
		Read:         false,
	}

	created, err := s.notificationStore.Create(r.Context(), n)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to create test notification", "component", "api", "user_id", user.ID.String(), "error", err)
		writeInternalError(w, err, "failed to create test notification")
		return
	}

	s.publishToUser(user.ID.String(), "notification_new", created)

	logger.InfoCtx(r.Context(), "test notification sent", "component", "api", "user_id", user.ID.String())

	unreadCount, _ := s.notificationStore.GetUnreadCount(r.Context(), user.ID.String())
	s.publishToUser(user.ID.String(), "notification_unread_count", map[string]any{
		"count": unreadCount,
	})

	writeStatus(w, "sent")
}
