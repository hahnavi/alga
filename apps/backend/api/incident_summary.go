package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alga/logger"
	"alga/metrics"
	"alga/store"
)

func (s *Server) handleIncidentSummaryFromAgent(ctx context.Context, agentRec *store.AgentTokenRecord, incidentID string, text string) error {
	if s.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	if s.incidentChannelManager == nil {
		return errors.New("incident channel manager not configured")
	}

	incidentNumber := mustParseIncidentNumber(incidentID)
	inc, err := s.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil {
		return fmt.Errorf("get incident: %w", err)
	}
	if inc == nil {
		return fmt.Errorf("incident %s not found", incidentID)
	}
	if inc.SlackChannelID == "" {
		return fmt.Errorf("incident %s has no slack channel", incidentID)
	}

	agentIDHex := agentRec.ID.String()
	invs, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
	if err != nil {
		return fmt.Errorf("list investigations for incident: %w", err)
	}
	authorized := false
	for _, inv := range invs {
		if inv.AgentID == agentIDHex {
			authorized = true
			break
		}
	}
	if !authorized {
		return fmt.Errorf("agent %s is not assigned to any investigation in incident %s", agentRec.Name, incidentID)
	}

	agentName := agentRec.Name
	if agentName == "" {
		agentName = "Agent"
	}

	inc.Summary = text
	if _, err := s.incidentStore.UpdateIncident(ctx, incidentNumber, inc); err != nil {
		logger.WarnCtx(ctx, "failed to save incident summary", "incident_number", incidentID, "error", err)
	}

	if err := s.incidentChannelManager.PostAgentSummary(ctx, inc, agentName, text); err != nil {
		return fmt.Errorf("post agent summary: %w", err)
	}

	if s.vkClient != nil {
		if err := s.vkClient.Do(ctx, s.vkClient.Builder().Set().Key("alga:summary:last:"+incidentID).Value("1").ExSeconds(int64(15*time.Minute/time.Second)).Build()).Error(); err != nil {
			logger.WarnCtx(ctx, "failed to set summary last key for incident", "incident_number", incidentID, "error", err)
		}
		if err := s.vkClient.Del(ctx, "alga:summary:pending:"+incidentID); err != nil {
			logger.WarnCtx(ctx, "failed to del summary pending key for incident", "incident_number", incidentID, "error", err)
		}
	}

	timelineEntry := &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "summary_posted",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s posted a status summary", agentName),
	}
	if err := s.incidentStore.AddTimelineEntry(ctx, timelineEntry); err != nil {
		logger.WarnCtx(ctx, "failed to add summary_posted timeline entry", "incident_number", incidentID, "error", err)
	}

	metrics.SummaryPostedTotal.Add(1)
	return nil
}
