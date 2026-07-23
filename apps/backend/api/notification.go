// Code moved from http.go; see git history.

package api

import (
	"errors"
	"net/http"
	"strings"

	"alga/store"
)

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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

	limit, skip := parseLimitSkip(r, 20)

	records, err := s.notificationStore.ListByUser(r.Context(), user.ID.String(), limit, skip)
	if err != nil {
		writeInternalError(w, err, "failed to list notifications")
		return
	}
	writeData(w, http.StatusOK, records)
}

func (s *Server) handleUnreadCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}

	if s.notificationStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"count": 0})
		return
	}

	count, err := s.notificationStore.GetUnreadCount(r.Context(), user.ID.String())
	if err != nil {
		writeInternalError(w, err, "failed to get unread count")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
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

	if err := s.notificationStore.MarkAllRead(r.Context(), user.ID.String()); err != nil {
		writeInternalError(w, err, "failed to mark all notifications read")
		return
	}

	s.publishToUser(user.ID.String(), "notification_unread_count", map[string]any{"count": 0})
	writeStatus(w, "ok")
}

func (s *Server) handleNotificationByID(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/notifications/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing notification id")
		return
	}

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

	if !strings.HasSuffix(suffix, "/read") {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	notificationID := strings.TrimSuffix(suffix, "/read")
	if err := s.notificationStore.MarkRead(r.Context(), user.ID.String(), notificationID); err != nil {
		if errors.Is(err, store.ErrNotificationNotFound) {
			writeError(w, ErrorCodeNotFound, "notification not found")
			return
		}
		writeInternalError(w, err, "failed to mark notification read")
		return
	}

	unreadCount, _ := s.notificationStore.GetUnreadCount(r.Context(), user.ID.String())
	s.publishToUser(user.ID.String(), "notification_unread_count", map[string]any{"count": unreadCount})

	writeStatus(w, "ok")
}
