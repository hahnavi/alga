package ics

import (
	"context"
	"errors"
	"testing"
)

func TestWarRoomProvisioner_ProvisionWarRoom(t *testing.T) {
	ds := newStubDocumentStore()
	is := newStubIncidentStore()
	rs := newStubRoleStore()
	dm := NewDocumentManager(ds)
	wp := NewWarRoomProvisioner(is, rs, dm, nil)

	ctx := context.Background()
	incidentNumber := int64(1)

	is.setIncident(&IncidentRecord{IncidentNumber: incidentNumber, Status: "active", TriageReport: map[string]any{"severity": "high"}})

	err := wp.ProvisionWarRoom(ctx, incidentNumber)
	if err != nil {
		t.Fatalf("ProvisionWarRoom: %v", err)
	}

	recs, err := ds.GetAllSections(ctx, incidentNumber)
	if err != nil {
		t.Fatalf("GetAllSections: %v", err)
	}
	if len(recs) == 0 {
		t.Error("expected sections to be created")
	}
}

func TestWarRoomProvisioner_ProvisionWarRoom_AlreadyProvisioned(t *testing.T) {
	ds := newStubDocumentStore()
	is := newStubIncidentStore()
	rs := newStubRoleStore()
	dm := NewDocumentManager(ds)
	wp := NewWarRoomProvisioner(is, rs, dm, nil)

	ctx := context.Background()
	incidentNumber := int64(2)

	is.setIncident(&IncidentRecord{
		IncidentNumber:   incidentNumber,
		Status:           "active",
		WarRoomChannelID: "C001",
	})

	err := wp.ProvisionWarRoom(ctx, incidentNumber)
	if err != nil {
		t.Fatalf("ProvisionWarRoom: %v", err)
	}

	recs, err := ds.GetAllSections(ctx, incidentNumber)
	if err != nil {
		t.Fatalf("GetAllSections: %v", err)
	}
	if len(recs) != 0 {
		t.Error("expected no sections when war room already provisioned")
	}
}

type stubMeetClient struct {
	result *SpaceResult
	err    error
	called int
}

func (s *stubMeetClient) CreateSpace(context.Context) (*SpaceResult, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestProvisionWarRoomNoClientIsNoop(t *testing.T) {
	is := newStubIncidentStore()
	is.setIncident(&IncidentRecord{IncidentNumber: 1, Status: "active"})
	p := NewWarRoomProvisioner(is, nil, nil, nil)
	if err := p.ProvisionWarRoom(context.Background(), 1); err != nil {
		t.Fatalf("expected no error when meetClient is nil, got %v", err)
	}
	if len(is.meetCalls) != 0 {
		t.Fatalf("expected no meet calls, got %d", len(is.meetCalls))
	}
}

func TestProvisionWarRoomCreatesMeet(t *testing.T) {
	is := newStubIncidentStore()
	is.setIncident(&IncidentRecord{IncidentNumber: 42, Status: "active"})
	meet := &stubMeetClient{result: &SpaceResult{SpaceName: "spaces/x", MeetingURI: "https://meet.google.com/x"}}
	p := NewWarRoomProvisioner(is, nil, nil, meet)
	if err := p.ProvisionWarRoom(context.Background(), 42); err != nil {
		t.Fatalf("ProvisionWarRoom: %v", err)
	}
	if meet.called != 1 {
		t.Fatalf("expected 1 CreateSpace call, got %d", meet.called)
	}
	if len(is.meetCalls) != 1 || is.meetCalls[0].space != "spaces/x" {
		t.Fatalf("unexpected meet calls: %+v", is.meetCalls)
	}
}

func TestProvisionWarRoomIdempotent(t *testing.T) {
	is := newStubIncidentStore()
	is.setIncident(&IncidentRecord{IncidentNumber: 1, Status: "active", GoogleMeetSpaceName: "spaces/already"})
	meet := &stubMeetClient{result: &SpaceResult{SpaceName: "spaces/x", MeetingURI: "https://meet.google.com/x"}}
	p := NewWarRoomProvisioner(is, nil, nil, meet)
	if err := p.ProvisionWarRoom(context.Background(), 1); err != nil {
		t.Fatalf("ProvisionWarRoom: %v", err)
	}
	if meet.called != 0 {
		t.Fatalf("expected no CreateSpace call when space exists, got %d", meet.called)
	}
}

func TestProvisionWarRoomMeetErrorPropagates(t *testing.T) {
	is := newStubIncidentStore()
	is.setIncident(&IncidentRecord{IncidentNumber: 1, Status: "active"})
	meet := &stubMeetClient{err: errors.New("google down")}
	p := NewWarRoomProvisioner(is, nil, nil, meet)
	if err := p.ProvisionWarRoom(context.Background(), 1); err == nil {
		t.Fatal("expected error when CreateSpace fails")
	}
}
