// Code moved from http.go; see git history.

package api

import (
	"alga/sse"
	"alga/store"
)

func (s *Server) publishInvestigationEvent(investigationID, eventType string, data any) {
	if s.ssePublisher == nil {
		return
	}
	s.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

func (s *Server) publishAlertUpdated(record *store.AlertRecord) {
	if s.alertBroadcaster == nil || record == nil {
		return
	}
	s.alertBroadcaster.PublishAlertEvent("alert_updated", *record)
}

func (s *Server) publishToUser(userID string, eventType string, data any) {
	if s.ssePublisher == nil {
		return
	}
	s.ssePublisher.PublishToUser(userID, sse.Event{Type: eventType, Data: data})
}

func (s *Server) publishAlertDeleted(record *store.AlertRecord) {
	if s.alertBroadcaster == nil || record == nil {
		return
	}
	s.alertBroadcaster.PublishAlertEvent("alert_deleted", *record)
}
