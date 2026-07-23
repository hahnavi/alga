package store

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/ent"
)

// TestToICSRoleRecordPopulatesAgentRevoked proves that when an agent token
// is soft-deleted, the ICS role record returned to the API exposes
// agent_revoked=true so the frontend can gray the avatar and italicize
// the name.
func TestToICSRoleRecordPopulatesAgentRevoked(t *testing.T) {
	s := &pgICSRoleStore{}
	assignmentID := uuid.New()
	agentID := uuid.New()
	incidentID := uuid.New()
	now := time.Now()

	revokedAgent := &ent.AgentToken{ID: agentID, Name: "old-bot", AgentType: "hermes", Revoked: true, Enabled: false}
	liveAgent := &ent.AgentToken{ID: agentID, Name: "live-bot", AgentType: "hermes", Revoked: false, Enabled: true}

	revoked := &ent.ICSRoleAssignment{
		ID:           assignmentID,
		RoleType:     "responder",
		AssigneeType: "agent",
		Status:       "active",
		StartedAt:    now,
		Edges:        ent.ICSRoleAssignmentEdges{Incident: &ent.Incident{ID: incidentID, IncidentNumber: 7}, AgentToken: revokedAgent},
	}
	rec := s.toICSRoleRecord(revoked, 7)
	if !rec.AgentRevoked {
		t.Errorf("AgentRevoked = false, want true for revoked agent token")
	}

	live := &ent.ICSRoleAssignment{
		ID:           assignmentID,
		RoleType:     "responder",
		AssigneeType: "agent",
		Status:       "active",
		StartedAt:    now,
		Edges:        ent.ICSRoleAssignmentEdges{Incident: &ent.Incident{ID: incidentID, IncidentNumber: 7}, AgentToken: liveAgent},
	}
	liveRec := s.toICSRoleRecord(live, 7)
	if liveRec.AgentRevoked {
		t.Errorf("AgentRevoked = true, want false for live agent token")
	}
}
