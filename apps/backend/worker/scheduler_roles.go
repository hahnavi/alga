// scheduler_roles.go contains the periodic ICS role assignment that maps
// online agents' capabilities to incident commander / comms lead / responder
// roles on active incidents.
package worker

import (
	"context"
	"errors"
	"strconv"

	"alga/capability"
	"alga/ics"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

type capRoleMapping struct {
	capability string
	roleType   ics.RoleType
	singleton  bool
}

var agentRoleMappings = []capRoleMapping{
	{capability.Command, ics.RoleIncidentCommander, true},
	{capability.Communicate, ics.RoleCommunicationsLead, true},
	{capability.Investigate, ics.RoleResponder, true},
}

func (s *InvestigationScheduler) tickAgentRoleAssignment(ctx context.Context) {
	if s.icsRoleStore == nil || s.incidentStore == nil {
		return
	}

	incidents, err := s.incidentStore.ListActiveIncidents(ctx)
	if err != nil {
		logger.Error("failed to list active incidents for agent role assignment",
			"component", "scheduler", "error", err)
		return
	}

	agents, err := s.agentTokenStore.ListActiveAgents()
	if err != nil {
		logger.Error("failed to list active agents for role assignment",
			"component", "scheduler", "error", err)
		return
	}

	for _, inc := range incidents {
		s.assignAgentRolesForIncident(ctx, inc.IncidentNumber, agents)
	}
}

func (s *InvestigationScheduler) assignAgentRolesForIncident(ctx context.Context, incidentNumber int64, agents []store.AgentTokenRecord) {
	incidentID := strconv.FormatInt(incidentNumber, 10)
	activeRoles, err := s.icsRoleStore.GetActiveRoles(ctx, incidentNumber)
	if err != nil {
		logger.Error("failed to get active ICS roles",
			"component", "scheduler", "incident_id", incidentID, "error", err)
		return
	}

	roleFilled := make(map[string]bool)
	agentAlreadyAssigned := make(map[string]bool)
	for _, r := range activeRoles {
		if r.AssigneeType == "agent" && r.AgentTokenID != nil {
			agentAlreadyAssigned[r.AgentTokenID.String()] = true
		}
		if r.Status == "active" {
			roleFilled[r.RoleType] = true
		}
	}

	hasHumanIC := false
	for _, r := range activeRoles {
		if r.RoleType == string(ics.RoleIncidentCommander) && r.AssigneeType == "user" && r.Status == "active" {
			hasHumanIC = true
			break
		}
	}

	onlineByCap := make(map[string]map[string]*store.AgentTokenRecord)
	getOnlineByCap := func(cap string) map[string]*store.AgentTokenRecord {
		if m, ok := onlineByCap[cap]; ok {
			return m
		}
		m := s.filterOnlineAgentsByCapability(ctx, agents, cap)
		onlineByCap[cap] = m
		return m
	}

	capCacheByCap := make(map[string]map[string]int)
	getCapCache := func(cap string) map[string]int {
		if cc, ok := capCacheByCap[cap]; ok {
			return cc
		}
		cc := s.buildCapacityCache(ctx, getOnlineByCap(cap))
		capCacheByCap[cap] = cc
		return cc
	}

	for _, mapping := range agentRoleMappings {
		if mapping.singleton && roleFilled[string(mapping.roleType)] {
			continue
		}

		if mapping.roleType == ics.RoleIncidentCommander && hasHumanIC {
			continue
		}

		capable := getOnlineByCap(mapping.capability)
		available := make(map[string]*store.AgentTokenRecord, len(capable))
		for id, a := range capable {
			if !agentAlreadyAssigned[id] {
				available[id] = a
			}
		}
		if len(available) == 0 {
			continue
		}

		if mapping.singleton {
			agent := pickLeastLoaded(available, getCapCache(mapping.capability))
			_, err := s.icsRoleStore.AssignAgentRole(ctx, incidentNumber, mapping.roleType, agent.ID, nil, nil)
			if err != nil {
				if errors.Is(err, ics.ErrActiveICExists) {
					continue
				}
				logger.Error("failed to assign agent ICS role",
					"component", "scheduler", "incident_id", incidentID,
					"role_type", string(mapping.roleType), "agent_id", agent.ID, "error", err)
				continue
			}
			roleFilled[string(mapping.roleType)] = true
			agentAlreadyAssigned[agent.ID.String()] = true

			s.addAgentRoleTimeline(ctx, incidentNumber, mapping.roleType, agent.Name)
			if s.ssePublisher != nil {
				s.ssePublisher.Publish(sse.Event{
					Type: "ics_role_assigned",
					Data: map[string]string{
						"incident_id": incidentID,
						"role_type":   string(mapping.roleType),
						"agent_name":  agent.Name,
					},
				})
			}
		} else {
			capCache := getCapCache(mapping.capability)
			for id, a := range available {
				if capCache[id] >= s.maxConcurrent {
					continue
				}
				_, err := s.icsRoleStore.AssignAgentRole(ctx, incidentNumber, mapping.roleType, a.ID, nil, nil)
				if err != nil {
					logger.Error("failed to assign agent ICS role",
						"component", "scheduler", "incident_id", incidentID,
						"role_type", string(mapping.roleType), "agent_id", a.ID, "error", err)
					continue
				}
				agentAlreadyAssigned[id] = true
				s.addAgentRoleTimeline(ctx, incidentNumber, mapping.roleType, a.Name)
			}
		}
	}
}

func (s *InvestigationScheduler) addAgentRoleTimeline(ctx context.Context, incidentNumber int64, roleType ics.RoleType, agentName string) {
	if s.incidentStore == nil {
		return
	}
	label := ics.RoleLabel(roleType)
	_ = s.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "ics_role_assigned",
		ActorType:      "system",
		Message:        "Auto-assigned " + label + " (agent: " + agentName + ")",
	})
}
