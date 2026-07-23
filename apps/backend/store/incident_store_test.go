package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIncidentStoreGetIncidentByIDAcceptsIncidentUUID(t *testing.T) {
	client := newEntTestClient(t)
	incidentStore := newPGIncidentStore(client)

	created, err := incidentStore.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: 12,
		Title:          "Database outage",
		Description:    "test incident",
		Status:         "active",
		Severity:       "critical",
		ImpactLevel:    "medium",
		Priority:       "P1",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}

	got, err := incidentStore.GetIncidentByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get incident by uuid: %v", err)
	}
	if got == nil {
		t.Fatalf("expected incident for uuid %s", created.ID)
	}
	if got.IncidentNumber != 12 {
		t.Fatalf("incident number = %d, want 12", got.IncidentNumber)
	}
}

func TestSetIncidentWarRoomMeet(t *testing.T) {
	client := newEntTestClient(t)
	s := newPGIncidentStore(client)

	incidentNumber := int64(7777)
	_, err := s.CreateIncident(context.Background(), &IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          "Meet war room test",
		Status:         "active",
		Severity:       "high",
		ImpactLevel:    "medium",
		Priority:       "P2",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}

	if err := s.SetIncidentWarRoomMeet(context.Background(), incidentNumber, "spaces/abc-123", "https://meet.google.com/abc-123"); err != nil {
		t.Fatalf("SetIncidentWarRoomMeet: %v", err)
	}

	got, err := s.GetIncident(context.Background(), incidentNumber)
	if err != nil {
		t.Fatalf("GetIncident: %v", err)
	}
	if got.GoogleMeetSpaceName != "spaces/abc-123" {
		t.Fatalf("google_meet_space_name = %q, want spaces/abc-123", got.GoogleMeetSpaceName)
	}
	if got.ConferenceURL != "https://meet.google.com/abc-123" {
		t.Fatalf("conference_url = %q, want meet URL", got.ConferenceURL)
	}

	// Overwrite: re-setting replaces previous values.
	if err := s.SetIncidentWarRoomMeet(context.Background(), incidentNumber, "spaces/zzz", "https://meet.google.com/zzz"); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	got2, _ := s.GetIncident(context.Background(), incidentNumber)
	if got2.GoogleMeetSpaceName != "spaces/zzz" {
		t.Fatalf("overwrite google_meet_space_name = %q, want spaces/zzz", got2.GoogleMeetSpaceName)
	}
}

func TestSetIncidentWarRoomMeetMissingIncident(t *testing.T) {
	client := newEntTestClient(t)
	s := newPGIncidentStore(client)

	err := s.SetIncidentWarRoomMeet(context.Background(), 999999, "spaces/x", "https://meet.google.com/x")
	if !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}
