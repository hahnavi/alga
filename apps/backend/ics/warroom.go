package ics

import (
	"context"
	"fmt"

	"alga/logger"
)

type WarRoomProvisioner struct {
	incStore   IncidentStore
	roleStore  RoleStore
	docManager *DocumentManager
	meetClient MeetSpaceCreator
}

func NewWarRoomProvisioner(incStore IncidentStore, roleStore RoleStore, docManager *DocumentManager, meetClient MeetSpaceCreator) *WarRoomProvisioner {
	return &WarRoomProvisioner{incStore: incStore, roleStore: roleStore, docManager: docManager, meetClient: meetClient}
}

func (p *WarRoomProvisioner) ProvisionWarRoom(ctx context.Context, incidentNumber int64) error {
	incident, err := p.incStore.GetIncident(ctx, incidentNumber)
	if err != nil {
		return fmt.Errorf("get incident for war room provisioning: %w", err)
	}
	if incident.WarRoomChannelID != "" {
		logger.Info("War room already provisioned", "component", "ics-warroom", "incident_number", incidentNumber)
		return nil
	}

	if p.meetClient != nil && incident.GoogleMeetSpaceName == "" {
		space, err := p.meetClient.CreateSpace(ctx)
		if err != nil {
			return fmt.Errorf("create google meet space: %w", err)
		}
		if err := p.incStore.SetWarRoomMeet(ctx, incidentNumber, space.SpaceName, space.MeetingURI); err != nil {
			return fmt.Errorf("persist google meet war room: %w", err)
		}
		logger.Info("Google Meet war room created", "component", "ics-warroom", "incident_number", incidentNumber, "space", space.SpaceName)
	}

	if p.docManager != nil {
		err = p.docManager.InitializeForIncident(ctx, incidentNumber, incident.TriageReport)
		if err != nil {
			logger.Warn("Failed to initialize incident document", "component", "ics-warroom", "incident_number", incidentNumber, "error", err)
		} else {
			logger.Info("War room document initialized", "component", "ics-warroom", "incident_number", incidentNumber)
		}
	}
	return nil
}
