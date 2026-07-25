package api

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/capability"
	entschema "alga/ent/schema"
	"alga/ics"
	"alga/incmetrics"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
)

type resolvingAlertStore struct {
	mockStore
}

func (s *resolvingAlertStore) ResolveAlertByUser(fingerprint string, actor *store.EventActor) error {
	if s.byFP != nil {
		if rec, ok := s.byFP[fingerprint]; ok {
			rec.Status = "resolved"
			s.byFP[fingerprint] = rec
		}
	}
	return nil
}

func (s *resolvingAlertStore) ResolveAlertByNumber(alertNumber int64, actor *store.EventActor) error {
	if s.byNumber != nil {
		if rec, ok := s.byNumber[alertNumber]; ok {
			rec.Status = "resolved"
			s.byNumber[alertNumber] = rec
			if s.byFP != nil && rec.Fingerprint != "" {
				s.byFP[rec.Fingerprint] = rec
			}
		}
	}
	return nil
}

type memoryExtractorSpy struct {
	called chan struct{}
}

func (s *memoryExtractorSpy) ExtractFromInvestigation(ctx context.Context, inv *store.AlertInvestigationRecord) error {
	select {
	case s.called <- struct{}{}:
	default:
	}
	return nil
}

func TestResolveAlertFinalizesInvestigationWhenAllAlertsResolved(t *testing.T) {
	agentID := uuid.New()
	invUUID := uuid.New()
	rootCause := "bad deploy"
	resolution := "rolled back"
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				AgentName:            "Hermes",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
			},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP: map[string]store.AlertRecord{
			"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1, Labels: map[string]string{"alertname": "HighCPU"}},
		},
		byNumber: map[int64]store.AlertRecord{
			1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1, Labels: map[string]string{"alertname": "HighCPU"}},
		},
	}}
	memory := &memoryExtractorSpy{called: make(chan struct{}, 1)}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetMemoryExtractor(memory)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_alert", RootCause: &rootCause, Resolution: &resolution})
	if !out.Ok {
		t.Fatalf("ExecuteInvTool ok=false error=%q", out.Error)
	}
	updates := invStore.statusUpdates["AINV-1"]
	if len(updates) == 0 || updates[len(updates)-1] != store.AlertInvestigationStatusComplete {
		t.Fatalf("status updates = %#v, want final complete", updates)
	}
	select {
	case <-memory.called:
	case <-time.After(time.Second):
		t.Fatal("expected memory extraction on finalization")
	}
}

func TestAlertOwnerResolveFinalizesAssignedLinkedInvestigation(t *testing.T) {
	agentID := uuid.New()
	invUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				AgentName:            "Hermes",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
			},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP: map[string]store.AlertRecord{
			"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1, Labels: map[string]string{"alertname": "HighCPU"}},
		},
		byNumber: map[int64]store.AlertRecord{
			1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1, Labels: map[string]string{"alertname": "HighCPU"}},
		},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_alert"})
	if !out.Ok {
		t.Fatalf("ExecuteInvTool ok=false error=%q", out.Error)
	}
	updates := invStore.statusUpdates["AINV-1"]
	if len(updates) == 0 || updates[len(updates)-1] != store.AlertInvestigationStatusComplete {
		t.Fatalf("status updates = %#v, want final complete", updates)
	}
}

func TestAlertOwnerResolveRejectsOtherAgentsInvestigation(t *testing.T) {
	agentID := uuid.New()
	otherAgentID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   uuid.New(),
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              otherAgentID.String(),
				AgentName:            "Other",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
			},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP: map[string]store.AlertRecord{
			"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1},
		},
		byNumber: map[int64]store.AlertRecord{
			1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1},
		},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_alert"})
	if out.Ok || out.Error != "not assigned to this investigation" {
		t.Fatalf("ExecuteInvTool = %#v, want not assigned error", out)
	}
	if updates := invStore.statusUpdates["AINV-1"]; len(updates) > 0 {
		t.Fatalf("status updates = %#v, want none", updates)
	}
	if rec := alerts.byNumber[1]; rec.Status != "firing" {
		t.Fatalf("alert status = %q, want firing", rec.Status)
	}
}

func TestCompleteInvestigationCommandIsRemoved(t *testing.T) {
	agentID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   uuid.New(),
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
			},
		},
	}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: &resolvingAlertStore{mockStore: mockStore{byFP: map[string]store.AlertRecord{
		"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1},
	}}}})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "complete_investigation"})
	if out.Ok || out.Error != "unknown op" {
		t.Fatalf("complete_investigation = %#v, want unknown op", out)
	}
}

type incidentStoreSpy struct {
	transitionTo     string
	transitionCalled bool
	timeline         []store.IncidentTimelineEntryRecord
	created          *store.IncidentRecord
	id               uuid.UUID
}

func (s *incidentStoreSpy) ReserveIncidentNumber(ctx context.Context) (int64, error) {
	return 1, nil
}

func (s *incidentStoreSpy) CreateIncident(ctx context.Context, record *store.IncidentRecord) (*store.IncidentRecord, error) {
	copy := *record
	s.created = &copy
	return record, nil
}

func (s *incidentStoreSpy) GetIncident(ctx context.Context, incidentNumber int64) (*store.IncidentRecord, error) {
	id := s.id
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &store.IncidentRecord{ID: id, IncidentNumber: incidentNumber, Status: s.transitionTo, Summary: "summary"}, nil
}

func (s *incidentStoreSpy) GetIncidentByID(ctx context.Context, id uuid.UUID) (*store.IncidentRecord, error) {
	return &store.IncidentRecord{ID: id, IncidentNumber: 1, Status: s.transitionTo, Summary: "summary"}, nil
}

func (s *incidentStoreSpy) UpdateIncident(ctx context.Context, incidentNumber int64, record *store.IncidentRecord) (*store.IncidentRecord, error) {
	return record, nil
}

func (s *incidentStoreSpy) DeleteIncident(ctx context.Context, incidentNumber int64) error {
	return nil
}
func (s *incidentStoreSpy) ListIncidents(ctx context.Context, filter store.IncidentListFilter) ([]store.IncidentRecord, int64, error) {
	return nil, 0, nil
}
func (s *incidentStoreSpy) ListSLAEligibleIncidents(ctx context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}
func (s *incidentStoreSpy) UpdateIncidentStatus(ctx context.Context, incidentNumber int64, status string) error {
	s.transitionTo = status
	return nil
}
func (s *incidentStoreSpy) TransitionIncidentStatus(ctx context.Context, incidentNumber int64, fromStatuses []string, toStatus string) error {
	s.transitionTo = toStatus
	s.transitionCalled = true
	return nil
}
func (s *incidentStoreSpy) AddTimelineEntry(ctx context.Context, record *store.IncidentTimelineEntryRecord) error {
	s.timeline = append(s.timeline, *record)
	return nil
}
func (s *incidentStoreSpy) GetTimeline(ctx context.Context, incidentNumber int64) ([]store.IncidentTimelineEntryRecord, error) {
	return s.timeline, nil
}
func (s *incidentStoreSpy) GetIncidentMetrics(ctx context.Context, startDate, endDate time.Time) (*incmetrics.Metrics, error) {
	return nil, nil
}
func (s *incidentStoreSpy) CountActiveByService(ctx context.Context) (map[string]int64, error) {
	return nil, nil
}
func (s *incidentStoreSpy) CountActiveByServiceID(ctx context.Context, serviceID string) (int, error) {
	return 0, nil
}
func (s *incidentStoreSpy) CountActiveByPriority(ctx context.Context, serviceID string) (map[string]int, error) {
	return nil, nil
}
func (s *incidentStoreSpy) ListActiveSummarizableIncidents(ctx context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}
func (s *incidentStoreSpy) ListActiveIncidents(ctx context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}
func (s *incidentStoreSpy) GetIncidentBySlackChannel(ctx context.Context, channelID string) (*store.IncidentRecord, error) {
	return nil, nil
}
func (s *incidentStoreSpy) SetIncidentWarRoomMeet(context.Context, int64, string, string) error {
	return nil
}

func (s *incidentStoreSpy) timelineContains(eventType string) bool {
	for _, entry := range s.timeline {
		if entry.EventType == eventType {
			return true
		}
	}
	return false
}

func TestResolveIncidentFinalizesRelatedInvestigation(t *testing.T) {
	agentID := uuid.New()
	incidentUUID := uuid.New()
	invUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   invUUID,
			AlertInvestigationID: "AINV-1",
			Status:               "reviewing",
			AgentID:              agentID.String(),
			PromotedIncidentID:   &incidentUUID,
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	incidentStore := &incidentStoreSpy{id: incidentUUID}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "impact", Version: 1},
		{Section: "actions_taken", Content: "actions", Version: 1},
		{Section: "root_cause", Content: "root cause", Version: 1},
		{Section: "resolution", Content: "resolution", Version: 1},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Command, capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_incident", IncidentNumber: 1})
	if !out.Ok {
		t.Fatalf("ExecuteInvTool ok=false error=%q", out.Error)
	}
	if incidentStore.transitionTo != "resolved" {
		t.Fatalf("incident transition = %q, want resolved", incidentStore.transitionTo)
	}
	if !incidentStore.timelineContains("incident_resolved") {
		t.Fatal("expected incident resolved timeline")
	}
	updates := invStore.statusUpdates["AINV-1"]
	if len(updates) == 0 || updates[len(updates)-1] != store.AlertInvestigationStatusComplete {
		t.Fatalf("related investigation status updates = %#v, want complete", updates)
	}
}

func TestMitigateIncidentFinalizesRelatedInvestigation(t *testing.T) {
	agentID := uuid.New()
	incidentUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   uuid.New(),
			AlertInvestigationID: "AINV-1",
			Status:               "investigating",
			AgentID:              agentID.String(),
			PromotedIncidentID:   &incidentUUID,
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	incidentStore := &incidentStoreSpy{id: incidentUUID}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Command, capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "mitigate_incident", IncidentNumber: 1})
	if !out.Ok {
		t.Fatalf("ExecuteInvTool ok=false error=%q", out.Error)
	}
	if incidentStore.transitionTo != "mitigated" {
		t.Fatalf("incident transition = %q, want mitigated", incidentStore.transitionTo)
	}
	updates := invStore.statusUpdates["AINV-1"]
	if len(updates) == 0 || updates[len(updates)-1] != store.AlertInvestigationStatusComplete {
		t.Fatalf("related investigation status updates = %#v, want complete", updates)
	}
}

func TestPromoteToIncidentCommand(t *testing.T) {
	agentID := uuid.New()
	invUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				Summary: &entschema.AlertInvestigationSummary{
					Summary: "This is a detailed summary of the alert investigation",
				},
				Alerts: []rabbitmq.CorrelatedAlert{{
					Fingerprint: "fp-1",
					AlertNumber: 1,
					Labels:      map[string]string{"alertname": "CPUTooHigh"},
					Annotations: map[string]string{"summary": "Alert CPUTooHigh firing"},
				}},
			},
		},
		statusUpdates: map[string][]string{},
	}
	incidentStore := &incidentStoreSpy{}
	incidentInvStore := &spyIncidentInvestigationStore{}
	alerts := &resolvingAlertStore{mockStore: mockStore{byFP: map[string]store.AlertRecord{
		"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1, Labels: map[string]string{"alertname": "CPUTooHigh"}},
	}}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "promote_to_incident", Title: "My Custom Title"})

	if !out.Ok {
		t.Fatalf("ExecuteInvTool ok=false error=%q", out.Error)
	}

	if out.IncidentNumber != 1 {
		t.Fatalf("expected incident_number 1, got %d", out.IncidentNumber)
	}
	if out.IncidentNumber != 1 {
		t.Fatalf("expected incident_number 1, got %d", out.IncidentNumber)
	}
	if incidentStore.created == nil {
		t.Fatal("expected incident to be created")
	}
	if incidentStore.created.Title != "My Custom Title" {
		t.Fatalf("incident title = %q, want custom title", incidentStore.created.Title)
	}
	if incidentStore.created.Description != "This is a detailed summary of the alert investigation" {
		t.Fatalf("incident description = %q, want investigation summary", incidentStore.created.Description)
	}
	wantInvID := incidentInvStore.created[0].ID.String()
	if out.IncidentInvestigationID != wantInvID {
		t.Fatalf("expected incident_investigation_id %q, got %q", wantInvID, out.IncidentInvestigationID)
	}
	commandUpdates := invStore.updatesAdded["AINV-1"]
	if len(commandUpdates) == 0 {
		t.Fatal("expected persisted command update")
	}
	commandMessage := commandUpdates[len(commandUpdates)-1].Message
	if !strings.Contains(commandMessage, "promoted the alert investigation to incident") {
		t.Fatalf("command update message = %q, want promotion wording", commandMessage)
	}
	if !strings.Contains(commandMessage, "[**#1**](/incidents/1)") {
		t.Fatalf("command update message = %q, want incident number link", commandMessage)
	}
	if !strings.Contains(commandMessage, "incident response team") {
		t.Fatalf("command update message = %q, want incident-response-team handoff wording", commandMessage)
	}
	if strings.Contains(strings.ToLower(commandMessage), "whether to continue") {
		t.Fatalf("command update message = %q, should not ask whether to continue", commandMessage)
	}

	if len(incidentInvStore.created) != 1 {
		t.Fatalf("expected 1 incident investigation created, got %d", len(incidentInvStore.created))
	}
	incInv := incidentInvStore.created[0]
	if incInv.IncidentNumber != 1 {
		t.Fatalf("expected incident number 1, got %d", incInv.IncidentNumber)
	}

	updates := invStore.statusUpdates["AINV-1"]
	if len(updates) == 0 || updates[len(updates)-1] != store.AlertInvestigationStatusPromoted {
		t.Fatalf("expected alert investigation status promoted, updates = %#v", updates)
	}

	rec := invStore.byID["AINV-1"]
	if rec.PromotedIncidentID == nil || rec.PromotedIncidentInvestigationID == nil {
		t.Fatalf("expected promoted incident/investigation UUIDs to be populated, got %#v, %#v", rec.PromotedIncidentID, rec.PromotedIncidentInvestigationID)
	}
	if incidentStore.timelineContains("investigation_created") {
		t.Fatalf("unexpected investigation_created timeline entry: %#v", incidentStore.timeline)
	}
}

func TestPromoteToIncidentRefusesWhenAllLinkedAlertsAreResolved(t *testing.T) {
	agentID := uuid.New()
	invUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				Alerts: []rabbitmq.CorrelatedAlert{{
					Fingerprint: "fp-1",
					AlertNumber: 1,
					Labels:      map[string]string{"alertname": "PostgreSQLDown"},
				}, {
					Fingerprint: "fp-2",
					AlertNumber: 2,
					Labels:      map[string]string{"alertname": "PostgreSQLDown"},
				}},
			},
		},
		statusUpdates: map[string][]string{},
	}
	incidentStore := &incidentStoreSpy{}
	incidentInvStore := &spyIncidentInvestigationStore{}
	alerts := &mockStore{byFP: map[string]store.AlertRecord{
		"fp-1": {Fingerprint: "fp-1", Status: "resolved", AlertNumber: 1},
		"fp-2": {Fingerprint: "fp-2", Status: "resolved", AlertNumber: 2},
	}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "promote_to_incident", Title: "Should not be created"})

	if out.Ok {
		t.Fatalf("expected promotion to be refused, got ok=true incident=%d", out.IncidentNumber)
	}
	if !strings.Contains(out.Error, "refusing to promote") {
		t.Fatalf("expected refusal error, got %q", out.Error)
	}
	if !strings.Contains(out.Error, "alga_set_outcome") {
		t.Fatalf("refusal error must redirect agent to alga_set_outcome, got %q", out.Error)
	}
	if !strings.Contains(out.Error, "fp-1") || !strings.Contains(out.Error, "fp-2") {
		t.Fatalf("refusal error must list resolved fingerprints, got %q", out.Error)
	}
	if incidentStore.created != nil {
		t.Fatalf("incident must not be created when all linked alerts are resolved, got %#v", incidentStore.created)
	}
	if len(incidentInvStore.created) != 0 {
		t.Fatalf("incident investigation must not be created when promotion is refused, got %d", len(incidentInvStore.created))
	}
	rec := invStore.byID["AINV-1"]
	if rec.PromotedIncidentID != nil {
		t.Fatalf("alert investigation must not be marked promoted, got %v", rec.PromotedIncidentID)
	}
}

func TestPromoteToIncidentProceedsWhenAtLeastOneLinkedAlertStillFiring(t *testing.T) {
	agentID := uuid.New()
	invUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				Alerts: []rabbitmq.CorrelatedAlert{{
					Fingerprint: "fp-1",
					AlertNumber: 1,
					Labels:      map[string]string{"alertname": "PostgreSQLDown"},
				}, {
					Fingerprint: "fp-2",
					AlertNumber: 2,
					Labels:      map[string]string{"alertname": "PostgreSQLDown"},
				}},
			},
		},
		statusUpdates: map[string][]string{},
	}
	incidentStore := &incidentStoreSpy{}
	incidentInvStore := &spyIncidentInvestigationStore{}
	alerts := &mockStore{byFP: map[string]store.AlertRecord{
		"fp-1": {Fingerprint: "fp-1", Status: "resolved", AlertNumber: 1},
		"fp-2": {Fingerprint: "fp-2", Status: "firing", AlertNumber: 2, Labels: map[string]string{"alertname": "PostgreSQLDown"}},
	}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "promote_to_incident"})

	if !out.Ok {
		t.Fatalf("expected promotion to proceed, got ok=false error=%q", out.Error)
	}
	if incidentStore.created == nil {
		t.Fatal("incident must be created when at least one linked alert is still firing")
	}
}

func TestPromoteToIncidentAuditLogsLiveStateMetadata(t *testing.T) {
	agentID := uuid.New()
	invUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				Alerts: []rabbitmq.CorrelatedAlert{{
					Fingerprint: "fp-1",
					AlertNumber: 1,
					Labels:      map[string]string{"alertname": "PostgreSQLDown"},
				}, {
					Fingerprint: "fp-2",
					AlertNumber: 2,
					Labels:      map[string]string{"alertname": "PostgreSQLDown"},
				}, {
					Fingerprint: "fp-3",
					AlertNumber: 3,
					Labels:      map[string]string{"alertname": "PostgreSQLDown"},
				}},
			},
		},
		statusUpdates: map[string][]string{},
	}
	incidentStore := &incidentStoreSpy{}
	incidentInvStore := &spyIncidentInvestigationStore{}
	alerts := &mockStore{byFP: map[string]store.AlertRecord{
		"fp-1": {Fingerprint: "fp-1", Status: "resolved", AlertNumber: 1},
		"fp-2": {Fingerprint: "fp-2", Status: "firing", AlertNumber: 2},
		"fp-3": {Fingerprint: "fp-3", Status: "firing", AlertNumber: 3},
	}}
	audit := &promoteAuditRecorder{}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetAuditStore(audit)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "Hermes",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "promote_to_incident"})

	if !out.Ok {
		t.Fatalf("expected promotion to succeed, got ok=false error=%q", out.Error)
	}
	creation := audit.findEvent(store.AuditIncidentCreated)
	if creation == nil {
		t.Fatalf("expected AuditIncidentCreated event, audit events: %v", audit.events)
	}
	if got := creation.Details["linked_alerts_total"]; got != 3 {
		t.Fatalf("audit linked_alerts_total = %v, want 3", got)
	}
	if got := creation.Details["linked_alerts_firing"]; got != 2 {
		t.Fatalf("audit linked_alerts_firing = %v, want 2", got)
	}
	if got := creation.Details["linked_alerts_resolved"]; got != 1 {
		t.Fatalf("audit linked_alerts_resolved = %v, want 1", got)
	}
	firing, _ := creation.Details["firing_fingerprints"].([]string)
	if len(firing) != 2 || !slices.Contains(firing, "fp-2") || !slices.Contains(firing, "fp-3") {
		t.Fatalf("audit firing_fingerprints = %v, want [fp-2 fp-3]", firing)
	}
}

type promoteAuditRecorder struct {
	mu     sync.Mutex
	events []store.AuditRecord
}

func (r *promoteAuditRecorder) Log(event store.AuditEvent, _ *uuid.UUID, _ string, _ string, _ string, _ bool, details map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, store.AuditRecord{Event: event, Details: details})
}

func (r *promoteAuditRecorder) LogEntity(event store.AuditEvent, _ *uuid.UUID, _ string, _ string, _ string, _ bool, details map[string]any, entityType string, entityID *uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, store.AuditRecord{Event: event, Details: details, EntityType: entityType, EntityID: entityID})
}

func (r *promoteAuditRecorder) Query(map[string]any) ([]store.AuditRecord, error) { return nil, nil }
func (r *promoteAuditRecorder) GetRecentEvents(int) ([]store.AuditRecord, error)  { return nil, nil }

func (r *promoteAuditRecorder) findEvent(event store.AuditEvent) *store.AuditRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.events {
		if r.events[i].Event == event {
			cp := r.events[i]
			return &cp
		}
	}
	return nil
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestAgentEnsurePostMortemTimelineMessageOmitsPostMortemID(t *testing.T) {
	incidentStore := &incidentStoreSpy{}
	postMortemStore := &trackingPostMortemStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetPostMortemStore(postMortemStore)
	executor.SetPostMortemBuilder(func(_ context.Context, _ store.IncidentDocumentStore, _ store.IncidentCoordinationStore, _ store.IncidentStore, _ store.Store, inc *store.IncidentRecord, summary string) *store.PostMortemRecord {
		return &store.PostMortemRecord{IncidentID: inc.ID, Title: inc.Summary, Summary: summary, Status: "draft"}
	})

	executor.EnsurePostMortem(context.Background(), 1, "summary")

	if len(postMortemStore.created) != 1 {
		t.Fatalf("created post-mortems = %d, want 1", len(postMortemStore.created))
	}
	for _, entry := range incidentStore.timeline {
		if entry.EventType == "postmortem_created" && entry.Message != "Post-mortem created" {
			t.Fatalf("postmortem timeline message = %q, want %q", entry.Message, "Post-mortem created")
		}
	}
}

type spyIncidentInvestigationStore struct {
	created []store.IncidentInvestigationRecord
	active  *store.IncidentInvestigationRecord
	list    []store.IncidentInvestigationRecord
	updates []store.InvestigationUpdate
}

func (m *spyIncidentInvestigationStore) CreateIncidentInvestigation(ctx context.Context, record store.IncidentInvestigationRecord) (*store.IncidentInvestigationRecord, error) {
	record.ID = uuid.New()
	m.created = append(m.created, record)
	return &record, nil
}

func (m *spyIncidentInvestigationStore) GetIncidentInvestigation(ctx context.Context, id string) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}

func (m *spyIncidentInvestigationStore) GetActiveIncidentInvestigationByIncident(ctx context.Context, incidentNumber int64) (*store.IncidentInvestigationRecord, error) {
	return m.active, nil
}

func (m *spyIncidentInvestigationStore) ListIncidentInvestigationsByIncident(ctx context.Context, incidentNumber int64) ([]store.IncidentInvestigationRecord, error) {
	return m.list, nil
}

func (m *spyIncidentInvestigationStore) AddIncidentInvestigationUpdate(ctx context.Context, id string, update store.InvestigationUpdate) error {
	m.updates = append(m.updates, update)
	if m.active != nil && m.active.IncidentInvestigationID == id {
		m.active.Updates = append(m.active.Updates, update)
	}
	return nil
}

func (m *spyIncidentInvestigationStore) UpdateIncidentInvestigationStatus(ctx context.Context, id string, status string) error {
	return nil
}

func (m *spyIncidentInvestigationStore) ClaimPendingIncidentInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}

func (m *spyIncidentInvestigationStore) ListPendingIncidentInvestigations(ctx context.Context, limit int64) ([]store.IncidentInvestigationRecord, error) {
	return nil, nil
}

func (m *spyIncidentInvestigationStore) SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *entschema.InvestigationSummary) error {
	return nil
}

func (m *spyIncidentInvestigationStore) SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	return nil
}

type agentToolCoordinationStore struct {
	messages []store.IncidentCoordinationMessageRecord
}

func (s *agentToolCoordinationStore) CreateMessage(_ context.Context, record *store.IncidentCoordinationMessageRecord) (*store.IncidentCoordinationMessageRecord, error) {
	record.ID = uuid.New()
	s.messages = append(s.messages, *record)
	return record, nil
}

func (s *agentToolCoordinationStore) UpdateMessageBody(_ context.Context, incidentNumber int64, messageID uuid.UUID, body string) (*store.IncidentCoordinationMessageRecord, error) {
	for i := range s.messages {
		if s.messages[i].IncidentNumber == incidentNumber && s.messages[i].ID == messageID {
			s.messages[i].Body = body
			return &s.messages[i], nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *agentToolCoordinationStore) ListMessages(context.Context, int64, int, int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *agentToolCoordinationStore) FindByProviderMessageID(context.Context, string) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *agentToolCoordinationStore) SetSlackMessageTS(context.Context, uuid.UUID, string, string, string) error {
	return nil
}

func (s *agentToolCoordinationStore) ListMessagesByKind(context.Context, int64, string, int, int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *agentToolCoordinationStore) NewestStatusUpdate(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *agentToolCoordinationStore) NewestAgentCoordinationReply(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *agentToolCoordinationStore) CreateStatusUpdate(ctx context.Context, incidentNumber int64, statusLevel string, body string, internal bool, actorID uuid.UUID, actorDisplayName string) (*store.IncidentCoordinationMessageRecord, error) {
	return s.CreateMessage(ctx, &store.IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             store.IncidentCoordinationKindStatusUpdate,
		ActorType:        store.IncidentCoordinationActorAgent,
		ActorID:          &actorID,
		ActorDisplayName: actorDisplayName,
		Body:             body,
		Internal:         internal,
		Source:           store.IncidentCoordinationSourceAgent,
		Metadata:         map[string]any{"status_level": statusLevel},
	})
}

type mockICSRoleStore struct {
	roles []store.ICSRoleRecord
}

func (m *mockICSRoleStore) AssignRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, userID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*store.ICSRoleRecord, error) {
	return nil, nil
}
func (m *mockICSRoleStore) AssignAgentRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, agentTokenID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*store.ICSRoleRecord, error) {
	return nil, nil
}
func (m *mockICSRoleStore) EndRole(ctx context.Context, assignmentID uuid.UUID, reason ics.EndReason) error {
	return nil
}
func (m *mockICSRoleStore) GetActiveRoles(ctx context.Context, incidentNumber int64) ([]store.ICSRoleRecord, error) {
	return m.roles, nil
}
func (m *mockICSRoleStore) GetActiveIC(ctx context.Context, incidentNumber int64) (*store.ICSRoleRecord, error) {
	return nil, nil
}
func (m *mockICSRoleStore) GetAllRoles(ctx context.Context, incidentNumber int64) ([]store.ICSRoleRecord, error) {
	return nil, nil
}
func (m *mockICSRoleStore) GetDelegationTree(ctx context.Context, incidentNumber int64) ([]store.ICSRoleRecord, error) {
	return nil, nil
}
func (m *mockICSRoleStore) GetActiveRolesForAgent(ctx context.Context, agentTokenID uuid.UUID) ([]store.ICSRoleRecord, error) {
	return nil, nil
}
func (m *mockICSRoleStore) EndAllRolesForIncident(ctx context.Context, incidentNumber int64, reason ics.EndReason) error {
	return nil
}
func (m *mockICSRoleStore) EndRolesForAgent(ctx context.Context, agentTokenID uuid.UUID, reason ics.EndReason) error {
	return nil
}

type coordinationForwarderSpy struct {
	events map[string][]sse.Event
}

func (f *coordinationForwarderSpy) AgentOnline(agentIDHex string) bool {
	return true
}

func (f *coordinationForwarderSpy) ForwardToAgent(agentIDHex, investigationID, senderID, senderName, message string) error {
	return nil
}

func (f *coordinationForwarderSpy) ForwardEventToAgent(agentIDHex string, event sse.Event) error {
	if f.events == nil {
		f.events = map[string][]sse.Event{}
	}
	f.events[agentIDHex] = append(f.events[agentIDHex], event)
	return nil
}

// coordinationForwarderClosure builds the AgentToolExecutor coordination
// forwarder (the function injected via SetCoordinationForwarder) around a
// coordinationForwarderSpy so tests assert on delivered SSE events. It mirrors
// Server.AgentCoordinationForwarderFn without requiring a full Server wiring.
func coordinationForwarderClosure(
	forwarder *coordinationForwarderSpy,
	incidentInvStore store.IncidentInvestigationStore,
	roleStore store.ICSRoleStore,
	incidentStore store.IncidentStore,
) func(ctx context.Context, incidentNumber int64, messageText string, mentions []string, agentRec *store.AgentTokenRecord) {
	return func(ctx context.Context, incidentNumber int64, messageText string, mentions []string, agentRec *store.AgentTokenRecord) {
		if forwarder == nil || agentRec == nil {
			return
		}
		chatID := "incident_coord_" + strconv.FormatInt(incidentNumber, 10)

		var investigations []store.IncidentInvestigationRecord
		if incidentInvStore != nil {
			invs, err := incidentInvStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
			if err == nil {
				investigations = invs
			}
		}
		var activeRoles []store.ICSRoleRecord
		if roleStore != nil {
			roles, err := roleStore.GetActiveRoles(ctx, incidentNumber)
			if err == nil {
				activeRoles = roles
			}
		}
		incidentStatus := ""
		if incidentStore != nil {
			if inc, err := incidentStore.GetIncident(ctx, incidentNumber); err == nil && inc != nil {
				incidentStatus = inc.Status
			}
		}
		if coordinationMessageHasNoContentAfterMentions(messageText) {
			return
		}
		for _, recipient := range coordinationAgentRecipients(investigations, activeRoles, mentions, agentRec.ID.String()) {
			event := coordinationMessageEvent(chatID, messageText, agentRec.ID.String(), agentRec.Name, recipient.Trigger, strconv.FormatInt(incidentNumber, 10), recipient.RoleType, incidentStatus)
			_ = forwarder.ForwardEventToAgent(recipient.AgentID, event)
		}
	}
}

func TestInvestigatorCannotResolveIncidentDirectly(t *testing.T) {
	agentID := uuid.New()
	incidentID := "88"
	incidentNumber := int64(88)
	incidentStore := &incidentStoreSpy{transitionTo: "mitigated"}
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentNumber: incidentNumber,
		AgentID:        agentID.String(),
		AgentName:      "investigator",
		AgentType:      "hermes",
		Status:         "investigating",
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "investigator",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "resolve_incident", Reason: "fixed"})

	if out.Ok {
		t.Fatalf("resolve_incident succeeded for investigator")
	}
	if !strings.Contains(out.Error, "commander") {
		t.Fatalf("error = %q, want commander guidance", out.Error)
	}
	if incidentStore.transitionCalled {
		t.Fatalf("incident status transition should not be called")
	}
}

func TestPostHandoffAudienceNoneCreatesAgentReply(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	incidentNumber := int64(1)
	coordStore := &agentToolCoordinationStore{}
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1",
		IncidentNumber:          incidentNumber,
		AgentID:                 agentID.String(),
		AgentName:               "investigator",
		AgentType:               "hermes",
		Status:                  "investigating",
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(&mockICSRoleStore{})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "investigator",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{
		ChatID:   "incident_coord_" + incidentID,
		Op:       "post_handoff",
		Message:  "Investigating database failover impact.",
		Audience: "none",
		Urgency:  "info",
	})

	if !out.Ok {
		t.Fatalf("post_handoff failed: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1", len(coordStore.messages))
	}
	msg := coordStore.messages[0]
	if msg.Kind != store.IncidentCoordinationKindAgentReply {
		t.Fatalf("kind = %q, want agent_reply", msg.Kind)
	}
	if msg.LinkedInvestigationID != "IINV-1" {
		t.Fatalf("linked investigation = %q, want IINV-1", msg.LinkedInvestigationID)
	}
	if mentions, ok := msg.Metadata["mentions"]; ok {
		mentionList, ok := mentions.([]string)
		if !ok || len(mentionList) != 0 {
			t.Fatalf("mentions = %#v, want absent or empty []string", mentions)
		}
	}
	if msg.Metadata["source_tool"] != "post_handoff" || msg.Metadata["audience"] != "none" || msg.Metadata["urgency"] != "info" {
		t.Fatalf("metadata = %#v", msg.Metadata)
	}
	// status_level is no longer stored on the handoff — the agent must publish
	// milestones through alga_publish_status_update instead.
	if _, hasStatusLevel := msg.Metadata["status_level"]; hasStatusLevel {
		t.Fatalf("metadata must not carry status_level; alga_publish_status_update is the only path that creates a Status Updates card entry: %#v", msg.Metadata)
	}
}

func TestPostHandoffRejectsStatusLevel(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	incidentNumber := int64(1)
	coordStore := &agentToolCoordinationStore{}
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1",
		IncidentNumber:          incidentNumber,
		AgentID:                 agentID.String(),
		Status:                  store.IncidentInvestigationStatusInvestigating,
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(&mockICSRoleStore{})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "investigator",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{
		ChatID:      "incident_coord_" + incidentID,
		Op:          "post_handoff",
		Message:     "Investigation complete. Cluster healthy and ready for commander review.",
		Audience:    "commander",
		Urgency:     "info",
		StatusLevel: "monitoring",
	})

	if out.Ok {
		t.Fatalf("post_handoff with status_level must be rejected; got Ok")
	}
	if !strings.Contains(out.Error, "no longer accepts status_level") {
		t.Fatalf("expected rejection referencing alga_publish_status_update, got: %s", out.Error)
	}
	if len(coordStore.messages) != 0 {
		t.Fatalf("rejected call must not create a coordination message; got %d", len(coordStore.messages))
	}
}

func TestIncidentInvestigationThreadRejectsOutcomeAndFindingTools(t *testing.T) {
	agentID := uuid.New()
	incidentNumber := int64(1)
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1",
		IncidentNumber:          incidentNumber,
		AgentID:                 uuid.NewString(),
		Status:                  "investigating",
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetICSRoleStore(roleStore)

	for _, op := range []string{"add_finding", "set_outcome"} {
		out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
			ID:           agentID,
			Name:         "responder",
			Capabilities: []string{capability.Investigate},
		}, agent.InvTool{ChatID: "incident_coord_1", Op: op, Note: "test note"})

		if out.Ok {
			t.Fatalf("incident scope tool op %q must be rejected; got success", op)
		}
	}
}

func TestCommunicatorIncidentTextPostsCoordinationReply(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	messageID, err := executor.HandleIncomingMessage(&store.AgentTokenRecord{
		ID:           agentID,
		Name:         "communicator",
		Capabilities: []string{capability.Command, capability.Communicate, capability.Investigate},
	}, "incident_coord_"+incidentID, "Coordination response from communications.", "", "", []string{"commander"}, "")

	if err != nil {
		t.Fatalf("HandleIncomingMessage error = %v", err)
	}
	if messageID == "" {
		t.Fatal("messageID is empty")
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1", len(coordStore.messages))
	}
	msg := coordStore.messages[0]
	if msg.Kind != store.IncidentCoordinationKindAgentReply {
		t.Fatalf("kind = %q, want agent_reply", msg.Kind)
	}
	if msg.Body != "Coordination response from communications." {
		t.Fatalf("body = %q", msg.Body)
	}
}

// TestResponderIncidentCoordReplyRoutesToCoordination captures the fix for the
// dual-capability routing bug: when a Responder (who holds BOTH investigation-
// thread and coordination permissions) is activated in the coordination chat and
// replies with free text, the reply must land in the coordination thread — not
// the investigation thread. Previously the capability-precedence check routed
// all responder free text to the investigation thread, making its replies to a
// commander @mention invisible in the coordination conversation.
//
// The responder-only free-text gate allows the reply here because a human
// operator @mentioned this responder in the coordination thread first (the
// "human mention carve-out"). Without that prior human mention, the gate would
// reject the free-text reply.
func TestResponderIncidentCoordReplyRoutesToCoordination(t *testing.T) {
	agentID := uuid.New()
	userID := uuid.New()
	incidentID := "1"
	now := time.Now()
	coordStore := &responderGateCoordinationStore{
		preHumanMentions: []store.IncidentCoordinationMessageRecord{
			{
				IncidentNumber:   mustParseIncidentNumber(incidentID),
				Kind:             store.IncidentCoordinationKindChat,
				ActorType:        store.IncidentCoordinationActorUser,
				ActorID:          &userID,
				ActorDisplayName: "oncall",
				Body:             "@responder can you check pg3?",
				Source:           store.IncidentCoordinationSourceAlga,
				Metadata:         map[string]any{"mentions": []string{"agent:" + agentID.String()}},
				CreatedAt:        now.Add(-5 * time.Minute),
			},
		},
	}
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1",
		IncidentNumber:          mustParseIncidentNumber(incidentID),
		AgentID:                 agentID.String(),
		AgentName:               "responder",
		Status:                  store.IncidentInvestigationStatusInvestigating,
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	messageID, err := executor.HandleIncomingMessage(&store.AgentTokenRecord{
		ID:           agentID,
		Name:         "responder",
		Capabilities: []string{capability.Investigate},
	}, "incident_coord_"+incidentID, "Acknowledging commander, proceeding with rollback.", "", "", []string{"commander"}, "")

	if err != nil {
		t.Fatalf("HandleIncomingMessage error = %v", err)
	}
	if messageID == "" {
		t.Fatal("expected coordination message id")
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1 (responder coord reply must land in coordination thread)", len(coordStore.messages))
	}
	if len(incidentInvStore.updates) != 0 {
		t.Fatalf("investigation updates = %d, want 0 (responder coord reply must NOT land in investigation thread)", len(incidentInvStore.updates))
	}
}

func TestCommunicatorCanEditIncidentCoordinationReply(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	messageID, err := executor.HandleIncomingMessage(&store.AgentTokenRecord{
		ID:           agentID,
		Name:         "communicator",
		Capabilities: []string{capability.Communicate},
	}, "incident_coord_"+incidentID, "Original coordination response.", "", "", nil, "")
	if err != nil {
		t.Fatalf("HandleIncomingMessage error = %v", err)
	}

	err = executor.HandleEditMessage("incident_coord_"+incidentID, messageID, "Edited coordination response.", &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "communicator",
		Capabilities: []string{capability.Communicate},
	})

	if err != nil {
		t.Fatalf("HandleEditMessage error = %v", err)
	}
	if coordStore.messages[0].Body != "Edited coordination response." {
		t.Fatalf("body = %q, want edited coordination response", coordStore.messages[0].Body)
	}
}

func TestCommunicatorCannotPublishIncidentInvestigationThreadTyping(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)

	if executor.HandleAgentTyping(&store.AgentTokenRecord{ID: agentID, Name: "communicator"}, "incident_inv_"+incidentID, true) {
		t.Fatal("typing event authorized for communicator, want blocked")
	}
}

func TestAgentCannotPostOtherAgentsAlertInvestigationThreadText(t *testing.T) {
	agentID := uuid.New()
	otherAgentID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   uuid.New(),
			AlertInvestigationID: "AINV-1",
			Status:               "investigating",
			AgentID:              otherAgentID.String(),
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)

	messageID, err := executor.HandleIncomingMessage(&store.AgentTokenRecord{
		ID:           agentID,
		Name:         "wrong-agent",
		Capabilities: []string{capability.Investigate},
	}, "alert_1", "I should not write another agent's investigation.", "", "", nil, "")

	if err == nil || !strings.Contains(err.Error(), "not assigned") {
		t.Fatalf("HandleIncomingMessage messageID=%q err=%v, want not assigned failure", messageID, err)
	}
}

func TestAgentCannotPublishOtherAgentsAlertInvestigationTyping(t *testing.T) {
	agentID := uuid.New()
	otherAgentID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   uuid.New(),
			AlertInvestigationID: "AINV-1",
			Status:               "investigating",
			AgentID:              otherAgentID.String(),
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)

	if executor.HandleAgentTyping(&store.AgentTokenRecord{ID: agentID, Name: "wrong-agent"}, "alert_1", true) {
		t.Fatal("typing event authorized for wrong agent, want blocked")
	}
}

func TestCommunicatorCannotEditIncidentInvestigationThreadMessage(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	threadStore := newMemoryInvestigationThreadStore()
	thread, err := threadStore.EnsureThread(context.Background(), store.ThreadOwnerIncidentInvestigation, incidentID)
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	msg, err := threadStore.AddMessage(context.Background(), thread.ThreadID, store.InvestigationThreadMessage{Message: "original", Source: "agent"})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetThreadStore(threadStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)

	err = executor.HandleEditMessage("incident_inv_"+incidentID, msg.ID.String(), "edited", &store.AgentTokenRecord{ID: agentID, Name: "communicator"})

	if err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("HandleEditMessage err=%v, want role authorization failure", err)
	}
}

func TestAgentCannotEditOtherAgentsAlertInvestigationMessage(t *testing.T) {
	agentID := uuid.New()
	otherAgentID := uuid.New()
	messageID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   uuid.New(),
			AlertInvestigationID: "AINV-1",
			Status:               "investigating",
			AgentID:              otherAgentID.String(),
			Updates:              []store.InvestigationUpdate{{ID: messageID, Message: "original"}},
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)

	err := executor.HandleEditMessage("alert_1", messageID.String(), "edited", &store.AgentTokenRecord{ID: agentID, Name: "wrong-agent"})

	if err == nil || !strings.Contains(err.Error(), "not assigned") {
		t.Fatalf("HandleEditMessage err=%v, want not assigned failure", err)
	}
}

func TestCommunicatorCannotDeleteIncidentInvestigationThreadMessage(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	threadStore := newMemoryInvestigationThreadStore()
	thread, err := threadStore.EnsureThread(context.Background(), store.ThreadOwnerIncidentInvestigation, incidentID)
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	msg, err := threadStore.AddMessage(context.Background(), thread.ThreadID, store.InvestigationThreadMessage{Message: "original", Source: "agent"})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetThreadStore(threadStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)

	err = executor.HandleDeleteMessage("incident_inv_"+incidentID, msg.ID.String(), &store.AgentTokenRecord{ID: agentID, Name: "communicator"})

	if err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("HandleDeleteMessage err=%v, want role authorization failure", err)
	}
}

func TestAgentCannotDeleteOtherAgentsAlertInvestigationMessage(t *testing.T) {
	agentID := uuid.New()
	otherAgentID := uuid.New()
	messageID := uuid.New()
	invStore := &trackingAlertInvestigationStore{byID: map[string]*store.AlertInvestigationRecord{
		"AINV-1": {
			ID:                   uuid.New(),
			AlertInvestigationID: "AINV-1",
			Status:               "investigating",
			AgentID:              otherAgentID.String(),
			Updates:              []store.InvestigationUpdate{{ID: messageID, Message: "original"}},
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
		},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)

	err := executor.HandleDeleteMessage("alert_1", messageID.String(), &store.AgentTokenRecord{ID: agentID, Name: "wrong-agent"})

	if err == nil || !strings.Contains(err.Error(), "not assigned") {
		t.Fatalf("HandleDeleteMessage err=%v, want not assigned failure", err)
	}
}

func TestCommunicatorCanPostHandoff(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "communicator",
		Capabilities: []string{capability.Command, capability.Communicate, capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "Command update sent.", Audience: "none"})

	if !out.Ok {
		t.Fatalf("post_handoff failed: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1", len(coordStore.messages))
	}
}

func TestAgentIncidentTextRoutesToInvestigation(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1",
		IncidentNumber:          mustParseIncidentNumber(incidentID),
		AgentID:                 agentID.String(),
		AgentName:               "investigator",
		AgentType:               "hermes",
		Status:                  "investigating",
	}}
	coordStore := &agentToolCoordinationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)

	messageID, err := executor.HandleIncomingMessage(
		&store.AgentTokenRecord{ID: agentID, Name: "investigator", Capabilities: []string{capability.Investigate}},
		"incident_inv_"+incidentID,
		"pg3 rebuilt from pg1; verifying database-level availability",
		agentID.String(),
		"investigator",
		[]string{"agent:" + agentID.String()},
		"",
	)

	if err != nil {
		t.Fatalf("HandleIncomingMessage: %v", err)
	}
	if messageID == "" {
		t.Fatal("expected investigation update message id")
	}
	if len(incidentInvStore.updates) != 1 {
		t.Fatalf("incident investigation updates = %d, want 1", len(incidentInvStore.updates))
	}
	update := incidentInvStore.updates[0]
	if update.Type != store.UpdateTypeComment || update.Source != store.UpdateSourceAgent {
		t.Fatalf("update type/source = %q/%q, want comment/agent", update.Type, update.Source)
	}
	if update.Message != "pg3 rebuilt from pg1; verifying database-level availability" {
		t.Fatalf("update message = %q", update.Message)
	}
	if update.Username == nil || *update.Username != "investigator" {
		t.Fatalf("update username = %#v, want investigator", update.Username)
	}
	if len(update.Mentions) != 1 || update.Mentions[0] != "agent:"+agentID.String() {
		t.Fatalf("update mentions = %#v", update.Mentions)
	}
	if len(coordStore.messages) != 0 {
		t.Fatalf("coordination messages = %d, want 0", len(coordStore.messages))
	}
}

func TestAgentIncidentTextRoutesToOwnerThreadWhenConfigured(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1",
		IncidentNumber:          mustParseIncidentNumber(incidentID),
		AgentID:                 agentID.String(),
		AgentName:               "wad1D4w",
		AgentType:               "hermes",
		Status:                  "investigating",
	}}
	threadStore := newMemoryInvestigationThreadStore()
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetThreadStore(threadStore)

	messageID, err := executor.HandleIncomingMessage(
		&store.AgentTokenRecord{ID: agentID, Name: "wad1D4w", Capabilities: []string{capability.Investigate}},
		"incident_inv_"+incidentID,
		"I checked the database nodes and posted findings.",
		agentID.String(),
		"wad1D4w",
		nil,
		"",
	)

	if err != nil {
		t.Fatalf("HandleIncomingMessage: %v", err)
	}
	if messageID == "" {
		t.Fatal("expected owner thread message id")
	}
	if len(incidentInvStore.updates) != 0 {
		t.Fatalf("incident investigation updates = %d, want 0", len(incidentInvStore.updates))
	}
	thread, _, err := threadStore.GetThreadByOwner(context.Background(), store.ThreadOwnerIncidentInvestigation, incidentID, 50, 0)
	if err != nil {
		t.Fatalf("GetThreadByOwner: %v", err)
	}
	if len(thread.Messages) != 1 {
		t.Fatalf("owner thread messages = %d, want 1", len(thread.Messages))
	}
	msg := thread.Messages[0]
	if msg.ID.String() != messageID {
		t.Fatalf("message id = %q, want %q", msg.ID.String(), messageID)
	}
	if msg.Source != string(store.UpdateSourceAgent) || msg.Username != "wad1D4w" || msg.Message != "I checked the database nodes and posted findings." {
		t.Fatalf("owner thread message = %#v", msg)
	}
}

func TestAgentTypingSourceUsesAgentName(t *testing.T) {
	if got := agent.AgentTypingSource(&store.AgentTokenRecord{Name: "wad1D4w"}); got != "wad1D4w" {
		t.Fatalf("agentTypingSource = %q, want wad1D4w", got)
	}
	if got := agent.AgentTypingSource(&store.AgentTokenRecord{}); got != "Agent" {
		t.Fatalf("agentTypingSource empty = %q, want Agent", got)
	}
}

func TestPostHandoffCommandAudienceResolvesCommanderAndCommunicator(t *testing.T) {
	agentID := uuid.New()
	commanderID := uuid.New()
	communicatorUserID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1",
		IncidentNumber:          mustParseIncidentNumber(incidentID),
		AgentID:                 agentID.String(),
		Status:                  "investigating",
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, AgentName: "pr1k1Ti3w", Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "user", UserID: &communicatorUserID, UserName: "Comms Lead", Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "investigator",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "Need command decision on rollback.", Audience: "command", Urgency: "decision_needed"})

	if !out.Ok {
		t.Fatalf("post_handoff failed: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1", len(coordStore.messages))
	}
	mentions, ok := coordStore.messages[0].Metadata["mentions"].([]string)
	if !ok {
		t.Fatalf("mentions metadata missing: %#v", coordStore.messages[0].Metadata)
	}
	want := map[string]bool{"agent:" + commanderID.String(): true, "user:" + communicatorUserID.String(): true}
	if len(mentions) != len(want) {
		t.Fatalf("mentions = %#v, want exactly %#v", mentions, want)
	}
	for _, mention := range mentions {
		if !want[mention] {
			t.Fatalf("mentions = %#v, unexpected %q", mentions, mention)
		}
		delete(want, mention)
	}
	if len(want) != 0 {
		t.Fatalf("mentions = %#v, missing %#v", mentions, want)
	}
	wantBody := "[@pr1k1Ti3w](agent:" + commanderID.String() + ") [@Comms Lead](user:" + communicatorUserID.String() + ") Need command decision on rollback."
	if coordStore.messages[0].Body != wantBody {
		t.Fatalf("body = %q, want %q", coordStore.messages[0].Body, wantBody)
	}
}

func TestPostHandoffForwardsResolvedAgentMention(t *testing.T) {
	agentID := uuid.New()
	commanderID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	incidentInvStore := &spyIncidentInvestigationStore{
		active: &store.IncidentInvestigationRecord{
			IncidentInvestigationID: "IINV-1",
			IncidentNumber:          mustParseIncidentNumber(incidentID),
			AgentID:                 agentID.String(),
			Status:                  "investigating",
		},
		list: []store.IncidentInvestigationRecord{
			{IncidentInvestigationID: "IINV-1", IncidentNumber: mustParseIncidentNumber(incidentID), AgentID: agentID.String(), Status: "investigating"},
		},
	}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	forwarder := &coordinationForwarderSpy{}
	incidentStore := &incidentStoreSpy{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetInvestigationForwarder(forwarder)
	executor.SetCoordinationForwarder(coordinationForwarderClosure(forwarder, incidentInvStore, roleStore, incidentStore))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "investigator",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "Need command decision on rollback.", Audience: "commander"})

	if !out.Ok {
		t.Fatalf("post_handoff failed: %s", out.Error)
	}
	if got := len(forwarder.events[agentID.String()]); got != 0 {
		t.Fatalf("origin agent forwarded events = %d, want 0", got)
	}
	events := forwarder.events[commanderID.String()]
	if len(events) != 1 {
		t.Fatalf("commander forwarded events = %d, want 1", len(events))
	}
	if events[0].Type != "message" {
		t.Fatalf("event type = %q, want message", events[0].Type)
	}
	data, ok := events[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("event data = %#v, want map[string]any", events[0].Data)
	}
	if data["trigger"] != "mention" {
		t.Fatalf("trigger = %#v, want mention", data["trigger"])
	}
	if data["chat_id"] != "incident_coord_"+incidentID {
		t.Fatalf("chat_id = %#v, want incident_%s", data["chat_id"], incidentID)
	}
}

func TestExtractAgentMentionsFromText(t *testing.T) {
	id1 := "c3460e2c-25ec-4300-8c7f-c563af400aaf"
	id2 := "40275326-3f72-4efb-b404-15cda75261c3"
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "single valid mention",
			text: "Over to you, [@pr1k1Ti3w](agent:" + id1 + ").",
			want: []string{"agent:" + id1},
		},
		{
			name: "malformed angle-bracket mention from copied template",
			text: "Over to you, [@pr1k1Ti3w](agent:<" + id1 + ">).",
			want: []string{"agent:" + id1},
		},
		{
			name: "multiple mentions deduplicated and ordered",
			text: "[@a](agent:" + id1 + ") and [@b](agent:" + id2 + ") and again [@c](agent:" + id1 + ")",
			want: []string{"agent:" + id1, "agent:" + id2},
		},
		{
			name: "no mentions",
			text: "Just a regular update with no pings.",
			want: nil,
		},
		{
			name: "non-agent mentions ignored",
			text: "[@bob](user:" + id1 + ") and [@team](group:abc) hi",
			want: nil,
		},
		{
			name: "truncated uuid ignored",
			text: "[@bob](agent:c3460e2c) partial",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := agent.ExtractAgentMentions(tc.text)
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("extractAgentMentions = %#v, want nil/empty", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("extractAgentMentions = %#v, want %#v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("extractAgentMentions[%d] = %q, want %q (got %#v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestCommunicatorFreeTextMentionForwardsToCommander(t *testing.T) {
	commanderID := uuid.New()
	communicatorID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, AgentName: "pr1k1Ti3w", Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &communicatorID, AgentName: "sp0r4", Status: string(ics.RoleStatusActive)},
	}}
	forwarder := &coordinationForwarderSpy{}
	incidentStore := &incidentStoreSpy{}
	incidentInvStore := &spyIncidentInvestigationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetInvestigationForwarder(forwarder)
	executor.SetCoordinationForwarder(coordinationForwarderClosure(forwarder, incidentInvStore, roleStore, incidentStore))

	text := "Published ✅ — Resolved public status update is live. Over to you, [@pr1k1Ti3w](agent:" + commanderID.String() + ")."
	messageID, err := executor.HandleIncomingMessage(
		&store.AgentTokenRecord{ID: communicatorID, Name: "sp0r4", Capabilities: []string{capability.Communicate}},
		"incident_coord_"+incidentID,
		text,
		communicatorID.String(),
		"sp0r4",
		nil,
		"",
	)

	if err != nil {
		t.Fatalf("HandleIncomingMessage: %v", err)
	}
	if messageID == "" {
		t.Fatal("expected coordination message id")
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1", len(coordStore.messages))
	}
	mentions, ok := coordStore.messages[0].Metadata["mentions"].([]string)
	if !ok || len(mentions) != 1 || mentions[0] != "agent:"+commanderID.String() {
		t.Fatalf("stored mentions = %#v, want [agent:%s]", mentions, commanderID.String())
	}
	if got := len(forwarder.events[communicatorID.String()]); got != 0 {
		t.Fatalf("sender forwarded events = %d, want 0", got)
	}
	events := forwarder.events[commanderID.String()]
	if len(events) != 1 {
		t.Fatalf("commander forwarded events = %d, want 1", len(events))
	}
	data, ok := events[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("event data = %#v, want map[string]any", events[0].Data)
	}
	if data["trigger"] != "mention" {
		t.Fatalf("trigger = %#v, want mention", data["trigger"])
	}
	if data["incident_role"] != string(ics.RoleIncidentCommander) {
		t.Fatalf("incident_role = %#v, want %q", data["incident_role"], ics.RoleIncidentCommander)
	}
}

func TestCommunicatorFreeTextMalformedMentionForwardsToCommander(t *testing.T) {
	commanderID := uuid.New()
	communicatorID := uuid.New()
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &communicatorID, Status: string(ics.RoleStatusActive)},
	}}
	forwarder := &coordinationForwarderSpy{}
	incidentStore := &incidentStoreSpy{}
	incidentInvStore := &spyIncidentInvestigationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetInvestigationForwarder(forwarder)
	executor.SetCoordinationForwarder(coordinationForwarderClosure(forwarder, incidentInvStore, roleStore, incidentStore))

	text := "Over to you, [@pr1k1Ti3w](agent:<" + commanderID.String() + ">)."
	if _, err := executor.HandleIncomingMessage(
		&store.AgentTokenRecord{ID: communicatorID, Name: "sp0r4", Capabilities: []string{capability.Communicate}},
		"incident_coord_1",
		text,
		communicatorID.String(),
		"sp0r4",
		nil,
		"",
	); err != nil {
		t.Fatalf("HandleIncomingMessage: %v", err)
	}
	if len(forwarder.events[commanderID.String()]) != 1 {
		t.Fatalf("commander forwarded events = %d, want 1 even with malformed angle brackets", len(forwarder.events[commanderID.String()]))
	}
}

func TestPostHandoffRecordsUnresolvedRoles(t *testing.T) {
	agentID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{IncidentNumber: mustParseIncidentNumber(incidentID), AgentID: agentID.String(), Status: "investigating"}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(&mockICSRoleStore{})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{ID: agentID, Name: "investigator", Capabilities: []string{capability.Investigate}}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "Need comms support.", Audience: "communicator"})
	if !out.Ok {
		t.Fatalf("post_handoff failed: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1", len(coordStore.messages))
	}
	unresolved, ok := coordStore.messages[0].Metadata["unresolved_roles"].([]string)
	if !ok || len(unresolved) != 1 || unresolved[0] != string(ics.RoleCommunicationsLead) {
		t.Fatalf("unresolved_roles = %#v", coordStore.messages[0].Metadata["unresolved_roles"])
	}
}

func TestPostHandoffAllowsCommanderCommunicatorAudience(t *testing.T) {
	commanderID := uuid.New()
	communicatorID := uuid.New()
	incidentID := "1"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, AgentName: "commander", Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &communicatorID, AgentName: "comms", Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "FYI to comms team.", Audience: "communicator"})
	if !out.Ok {
		t.Fatalf("commander post_handoff to communicator was rejected; want allowed (deferral rule removed): %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1", len(coordStore.messages))
	}
}

func TestPostHandoffRejectsUnassignedInvestigator(t *testing.T) {
	agentID := uuid.New()
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetIncidentCoordinationStore(&agentToolCoordinationStore{})
	executor.SetICSRoleStore(&mockICSRoleStore{})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{ID: agentID, Name: "investigator", Capabilities: []string{capability.Investigate}}, agent.InvTool{ChatID: "incident_coord_1", Op: "post_handoff", Message: "I should not be allowed.", Audience: "none"})
	if out.Ok || !strings.Contains(out.Error, "not assigned") {
		t.Fatalf("outcome = %#v, want not assigned failure", out)
	}
}

func TestCommanderAgentCanResolveIncident(t *testing.T) {
	agentID := uuid.New()
	incidentID := "88"
	incidentStore := &incidentStoreSpy{transitionTo: "mitigated"}
	incidentInvStore := &spyIncidentInvestigationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "Elevated error rate", Version: 1},
		{Section: "actions_taken", Content: "Reverted build", Version: 1},
		{Section: "root_cause", Content: "Bad deploy bypassed canary", Version: 1},
		{Section: "resolution", Content: "Rolled back the deploy", Version: 1},
	}}

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "commander-agent",
		Capabilities: []string{capability.Command, capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "resolve_incident", Reason: "fixed"})

	if !out.Ok {
		t.Fatalf("resolve_incident failed for commander: %s", out.Error)
	}
	if incidentStore.transitionTo != "resolved" {
		t.Fatalf("incident status = %s, want resolved", incidentStore.transitionTo)
	}
}

func TestCommanderCanResolveIncidentWithCommandCapabilityOnly(t *testing.T) {
	agentID := uuid.New()
	incidentID := "88"
	incidentStore := &incidentStoreSpy{transitionTo: "mitigated"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "Elevated error rate", Version: 1},
		{Section: "actions_taken", Content: "Reverted build", Version: 1},
		{Section: "root_cause", Content: "Bad deploy bypassed canary", Version: 1},
		{Section: "resolution", Content: "Rolled back the deploy", Version: 1},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "commander-agent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "resolve_incident", Reason: "fixed"})

	if !out.Ok {
		t.Fatalf("resolve_incident failed for command-only commander: %s", out.Error)
	}
}

func TestResponderCanSetIncidentSeverity(t *testing.T) {
	agentID := uuid.New()
	incidentID := "88"
	incidentStore := &incidentStoreSpy{transitionTo: "active"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleResponder),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "responder-agent",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "set_incident_severity", Severity: "high"})

	if !out.Ok {
		t.Fatalf("set_incident_severity failed for responder: %s", out.Error)
	}
}

func TestCommanderCanSetIncidentSeverity(t *testing.T) {
	agentID := uuid.New()
	incidentStore := &incidentStoreSpy{transitionTo: "active"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "commander-agent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "incident_coord_88", Op: "set_incident_severity", Severity: "high"})

	if !out.Ok {
		t.Fatalf("set_incident_severity failed for commander: %s", out.Error)
	}
}

func TestCommanderCanBeginTriage(t *testing.T) {
	agentID := uuid.New()
	incidentStore := &incidentStoreSpy{transitionTo: "triaging"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID: agentID, Name: "commander-agent", Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "incident_coord_5", Op: "begin_triage"})

	if !out.Ok {
		t.Fatalf("begin_triage failed for commander: %s", out.Error)
	}
}

func TestCommanderCanPromoteIncident(t *testing.T) {
	agentID := uuid.New()
	incidentStore := &incidentStoreSpy{transitionTo: "active"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID: agentID, Name: "commander-agent", Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "incident_coord_5", Op: "promote_incident"})

	if !out.Ok {
		t.Fatalf("promote_incident failed for commander: %s", out.Error)
	}
}

func TestCommanderResolveIncidentCascadesLinkedAlerts(t *testing.T) {
	agentID := uuid.New()
	incidentID := "89"
	incidentStore := &incidentStoreSpy{transitionTo: "mitigated"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "Elevated error rate", Version: 1},
		{Section: "actions_taken", Content: "Reverted build", Version: 1},
		{Section: "root_cause", Content: "Bad deploy bypassed canary", Version: 1},
		{Section: "resolution", Content: "Rolled back the deploy", Version: 1},
	}}
	alerts := &mockStore{resolveResult: store.AlertCascadeResult{
		Resolved: []store.AlertRecord{{AlertNumber: 7, Fingerprint: "fp-7", Status: "resolved"}},
	}}

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetAlertCascade(func(ctx context.Context, alertStore store.Store, _ store.AuditStore, _ *sse.DualPublisher, incidentNumber int64, _ uuid.UUID, _ string) store.AlertCascadeResult {
		alerts.resolveCalls++
		alerts.lastCascadeID = strconv.FormatInt(incidentNumber, 10)
		return alerts.resolveResult
	})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "commander-agent",
		Capabilities: []string{capability.Command, capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "resolve_incident", Reason: "fixed"})

	if !out.Ok {
		t.Fatalf("resolve_incident failed: %s", out.Error)
	}
	if alerts.resolveCalls != 1 {
		t.Fatalf("ResolveAlertsByIncident calls = %d, want 1", alerts.resolveCalls)
	}
	if alerts.lastCascadeID != incidentID {
		t.Fatalf("cascade incident id = %q, want %q", alerts.lastCascadeID, incidentID)
	}
}

// statusUpdateCoordinationStore embeds the default test coordination store and
// lets a test control how many status-update messages exist for an incident.
type statusUpdateCoordinationStore struct {
	agentToolCoordinationStore
	statusUpdates []store.IncidentCoordinationMessageRecord
}

func (s *statusUpdateCoordinationStore) ListMessagesByKind(_ context.Context, _ int64, kind string, _ int, _ int) ([]store.IncidentCoordinationMessageRecord, error) {
	if kind == store.IncidentCoordinationKindStatusUpdate {
		return s.statusUpdates, nil
	}
	return nil, nil
}

func TestCommanderResolveBlockedUntilCommunicatorPublishesStatusUpdate(t *testing.T) {
	commanderID := uuid.New()
	communicatorID := uuid.New()
	incidentID := "91"
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &communicatorID, Status: string(ics.RoleStatusActive)},
	}}
	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "Elevated error rate", Version: 1},
		{Section: "actions_taken", Content: "Reverted build", Version: 1},
		{Section: "root_cause", Content: "Bad deploy bypassed canary", Version: 1},
		{Section: "resolution", Content: "Rolled back the deploy", Version: 1},
	}}
	incidentStore := &summaryTrackingIncidentStore{summary: "Resolved summary"}
	incidentStore.transitionTo = "mitigated"

	coordStore := &statusUpdateCoordinationStore{}

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)
	executor.SetIncidentCoordinationStore(coordStore)

	cmd := &store.AgentTokenRecord{ID: commanderID, Name: "commander-agent", Capabilities: []string{capability.Command, capability.Investigate}}

	// No status update published yet -> resolution must be blocked.
	out := executor.ExecuteInvTool(context.Background(), cmd, agent.InvTool{
		ChatID: "incident_coord_" + incidentID, Op: "resolve_incident", Reason: "fixed", Summary: "Resolved summary",
	})
	if out.Ok {
		t.Fatalf("resolve_incident must be blocked when the communicator has not published a status update")
	}
	if !strings.Contains(out.Error, "status update") {
		t.Fatalf("resolve error should mention a status update requirement, got %q", out.Error)
	}

	// An investigating-only update is not enough for final closure.
	coordStore.statusUpdates = []store.IncidentCoordinationMessageRecord{{Body: "investigating", Metadata: map[string]any{"status_level": "investigating"}}}
	out2 := executor.ExecuteInvTool(context.Background(), cmd, agent.InvTool{
		ChatID: "incident_coord_" + incidentID, Op: "resolve_incident", Reason: "fixed", Summary: "Resolved summary",
	})
	if out2.Ok {
		t.Fatalf("resolve_incident must be blocked until the communicator publishes a resolved status update")
	}
	if !strings.Contains(out2.Error, "resolved status update") {
		t.Fatalf("resolve error should mention a resolved status update requirement, got %q", out2.Error)
	}

	// After the communicator publishes a resolved update, the same commander may resolve.
	coordStore.statusUpdates = []store.IncidentCoordinationMessageRecord{{Body: "resolved", Metadata: map[string]any{"status_level": "resolved"}}}
	out3 := executor.ExecuteInvTool(context.Background(), cmd, agent.InvTool{
		ChatID: "incident_coord_" + incidentID, Op: "resolve_incident", Reason: "fixed", Summary: "Resolved summary",
	})
	if !out3.Ok {
		t.Fatalf("resolve_incident should succeed once a resolved status update is published: %s", out3.Error)
	}
}

// scopedAlertInvStore embeds the no-op mock and returns a configurable record for
// GetCurrentAlertInvestigationByAlertNumber.
type scopedAlertInvStore struct {
	mockAlertInvestigationStore
	inv *store.AlertInvestigationRecord
	err error
}

func (s *scopedAlertInvStore) GetCurrentAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*store.AlertInvestigationRecord, error) {
	return s.inv, s.err
}

func TestCommanderOwnsAlertIncident(t *testing.T) {
	commanderID := uuid.New()
	incidentID := uuid.New()
	invStore := &scopedAlertInvStore{inv: &store.AlertInvestigationRecord{PromotedIncidentID: &incidentID}}
	incStore := &incidentStoreSpy{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetIncidentStore(incStore)
	executor.SetICSRoleStore(roleStore)

	// Active commander of the incident that owns the alert -> allowed.
	if err := executor.CommanderOwnsAlertIncident(context.Background(), commanderID, 56); err != nil {
		t.Fatalf("expected commander to be authorized on its incident's alert, got: %v", err)
	}

	// A different agent (not the commander) -> denied.
	if err := executor.CommanderOwnsAlertIncident(context.Background(), uuid.New(), 56); err == nil {
		t.Fatalf("expected non-commander agent to be denied")
	}

	// Alert not linked to any incident (no promoted incident) -> denied.
	invStore.inv = &store.AlertInvestigationRecord{}
	if err := executor.CommanderOwnsAlertIncident(context.Background(), commanderID, 56); err == nil {
		t.Fatalf("expected denial when alert is not linked to an incident")
	}
}

// summaryTrackingIncidentStore embeds incidentStoreSpy but returns a
// customizable summary and records UpdateIncident so resolution-document
// population is observable in tests.
type summaryTrackingIncidentStore struct {
	incidentStoreSpy
	summary    string
	updatedSum *string
}

func (s *summaryTrackingIncidentStore) GetIncident(_ context.Context, incidentNumber int64) (*store.IncidentRecord, error) {
	return &store.IncidentRecord{ID: uuid.New(), IncidentNumber: incidentNumber, Status: s.transitionTo, Summary: s.summary}, nil
}

func (s *summaryTrackingIncidentStore) UpdateIncident(_ context.Context, _ int64, record *store.IncidentRecord) (*store.IncidentRecord, error) {
	cp := record.Summary
	s.updatedSum = &cp
	return record, nil
}

// recordingDocStore is a self-consistent in-memory document store that records
// UpsertSection calls (including the editor userID) so resolution-document
// population is observable. GetAllSections/GetSection reflect upserted content.
type recordingDocStore struct {
	sections map[string]string
	upserts  []recordedDocUpsert
}

type recordedDocUpsert struct {
	section ics.DocumentSection
	content string
	version int
	userID  uuid.UUID
}

func (r *recordingDocStore) ensure() {
	if r.sections == nil {
		r.sections = map[string]string{}
	}
}

func (r *recordingDocStore) GetAllSections(_ context.Context, _ int64) ([]store.IncidentDocumentRecord, error) {
	r.ensure()
	out := make([]store.IncidentDocumentRecord, 0, len(r.sections))
	for sec, content := range r.sections {
		out = append(out, store.IncidentDocumentRecord{Section: sec, Content: content, Version: 1})
	}
	return out, nil
}

func (r *recordingDocStore) GetSection(_ context.Context, _ int64, section ics.DocumentSection) (*store.IncidentDocumentRecord, error) {
	r.ensure()
	if content, ok := r.sections[string(section)]; ok {
		return &store.IncidentDocumentRecord{Section: string(section), Content: content, Version: 1}, nil
	}
	return nil, nil
}

func (r *recordingDocStore) UpsertSection(_ context.Context, _ int64, section ics.DocumentSection, content string, version int, userID uuid.UUID) (*store.IncidentDocumentRecord, error) {
	r.ensure()
	r.sections[string(section)] = content
	r.upserts = append(r.upserts, recordedDocUpsert{section: section, content: content, version: version, userID: userID})
	return nil, nil
}

func (r *recordingDocStore) InitializeDocument(_ context.Context, _ int64, sections map[ics.DocumentSection]string) error {
	r.ensure()
	for sec, content := range sections {
		r.sections[string(sec)] = content
	}
	return nil
}

func TestCommanderCanSetIncidentResolutionDocs(t *testing.T) {
	agentID := uuid.New()
	incidentID := "91"
	incidentStore := &summaryTrackingIncidentStore{summary: ""}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	docStore := &recordingDocStore{}

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)

	rootCause := "Synthetic null finding; no underlying defect."
	resolution := "Closed once investigation confirmed no impact."
	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "commander-agent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{
		ChatID:           "incident_coord_" + incidentID,
		Op:               "set_incident_resolution_docs",
		Summary:          "Synthetic test incident; no production impact.",
		ImpactAssessment: "None — no services or users affected.",
		ActionsTaken:     "Investigation confirmed null finding; outcome recorded.",
		RootCause:        &rootCause,
		Resolution:       &resolution,
	})

	if !out.Ok {
		t.Fatalf("set_incident_resolution_docs failed: %s", out.Error)
	}
	if incidentStore.updatedSum == nil || *incidentStore.updatedSum != "Synthetic test incident; no production impact." {
		t.Fatalf("incident summary not updated, got: %v", incidentStore.updatedSum)
	}
	if len(docStore.upserts) != 4 {
		t.Fatalf("expected 4 doc upserts (impact_assessment, actions_taken, root_cause, resolution), got %d", len(docStore.upserts))
	}
	sections := map[string]string{}
	for _, u := range docStore.upserts {
		sections[string(u.section)] = u.content
		if u.userID != uuid.Nil {
			t.Fatalf("agent-authored upsert should not attribute a user editor, got %s for %s", u.userID, u.section)
		}
	}
	if sections["impact_assessment"] == "" || sections["actions_taken"] == "" || sections["root_cause"] == "" || sections["resolution"] == "" {
		t.Fatalf("missing required section upserts: %#v", sections)
	}
}

func TestCommanderResolveIncidentPopulatesInlineDocs(t *testing.T) {
	agentID := uuid.New()
	incidentID := "92"
	incidentStore := &summaryTrackingIncidentStore{summary: ""}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	docStore := &recordingDocStore{}

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)

	rootCause := "Misconfigured replica promoted stale WAL."
	resolution := "Reinitialized replica via patronictl reinit."
	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "commander-agent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{
		ChatID:           "incident_coord_" + incidentID,
		Op:               "resolve_incident",
		Reason:           "fixed",
		Summary:          "Resolved by commander inline.",
		ImpactAssessment: "No impact.",
		ActionsTaken:     "Verified and closed.",
		RootCause:        &rootCause,
		Resolution:       &resolution,
	})

	if !out.Ok {
		t.Fatalf("resolve_incident failed with inline docs: %s", out.Error)
	}
	if incidentStore.transitionTo != "resolved" {
		t.Fatalf("incident status = %s, want resolved", incidentStore.transitionTo)
	}
	if len(docStore.upserts) != 4 {
		t.Fatalf("expected 4 doc upserts from inline resolve (impact_assessment, actions_taken, root_cause, resolution), got %d", len(docStore.upserts))
	}
}

// TestResolveIncidentRequiresRootCauseAndResolutionSections locks in that the
// root_cause and resolution incident document sections are independently
// mandatory before an incident can be resolved. actions_taken no longer
// satisfies the resolution requirement, and root_cause is a new gate.
func TestResolveIncidentRequiresRootCauseAndResolutionSections(t *testing.T) {
	commanderID := uuid.New()
	incidentID := "95"
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	// impact_assessment + actions_taken present, but root_cause and resolution
	// sections are empty — resolution must be blocked.
	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "Elevated error rate", Version: 1},
		{Section: "actions_taken", Content: "Reverted build", Version: 1},
	}}
	coordStore := &statusUpdateCoordinationStore{statusUpdates: []store.IncidentCoordinationMessageRecord{
		{Body: "resolved", Metadata: map[string]any{"status_level": "resolved"}},
	}}
	incidentStore := &summaryTrackingIncidentStore{summary: "Resolved summary"}
	incidentStore.transitionTo = "mitigated"

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)
	executor.SetIncidentCoordinationStore(coordStore)

	cmd := &store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}}
	out := executor.ExecuteInvTool(context.Background(), cmd, agent.InvTool{
		ChatID:  "incident_coord_" + incidentID,
		Op:      "resolve_incident",
		Reason:  "fixed",
		Summary: "Resolved summary",
	})
	if out.Ok {
		t.Fatalf("resolve_incident must be blocked when root_cause and resolution sections are empty")
	}
	if !strings.Contains(out.Error, "root_cause") {
		t.Fatalf("resolve error should require root_cause, got %q", out.Error)
	}
	if !strings.Contains(out.Error, "resolution") {
		t.Fatalf("resolve error should require resolution, got %q", out.Error)
	}
	if incidentStore.transitionTo == "resolved" {
		t.Fatalf("incident must not transition to resolved when root_cause/resolution are missing")
	}
}

func TestResponderCannotSetIncidentResolutionDocs(t *testing.T) {
	agentID := uuid.New()
	incidentID := "93"
	incidentStore := &summaryTrackingIncidentStore{summary: ""}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleResponder),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(&recordingDocStore{})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "responder-agent",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{
		ChatID:           "incident_coord_" + incidentID,
		Op:               "set_incident_resolution_docs",
		Summary:          "should not be allowed",
		ImpactAssessment: "no",
		ActionsTaken:     "no",
	})

	if out.Ok {
		t.Fatalf("responder must not be able to set resolution docs")
	}
	if !strings.Contains(out.Error, "not authorized") {
		t.Fatalf("expected authorization error, got: %s", out.Error)
	}
}

func TestSetIncidentResolutionDocsRequiresAtLeastOneField(t *testing.T) {
	agentID := uuid.New()
	incidentID := "94"
	incidentStore := &summaryTrackingIncidentStore{summary: ""}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(&recordingDocStore{})

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           agentID,
		Name:         "commander-agent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "set_incident_resolution_docs"})

	if out.Ok {
		t.Fatalf("set_incident_resolution_docs must require at least one field")
	}
	if !strings.Contains(out.Error, "at least one of") {
		t.Fatalf("expected missing-field error, got: %s", out.Error)
	}
}

func TestPublishStatusUpdateAllowsResponder(t *testing.T) {
	responderID := uuid.New()
	incidentID := "21"
	incidentInvStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1", IncidentNumber: mustParseIncidentNumber(incidentID), AgentID: responderID.String(), Status: "investigating",
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &responderID, AgentName: "responder"},
	}}
	coordStore := &agentToolCoordinationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(incidentInvStore)
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "public update from responder", StatusLevel: "identified"})

	if !out.Ok {
		t.Fatalf("publish_status_update failed for responder: %s", out.Error)
	}
	if len(coordStore.messages) != 1 || coordStore.messages[0].Body != "public update from responder" {
		t.Fatalf("expected status update from responder to be recorded, got %+v", coordStore.messages)
	}
}

func TestPublishStatusUpdateRejectsUnauthorizedAgent(t *testing.T) {
	agentID := uuid.New()
	incidentID := "21"
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentCoordinationStore(&agentToolCoordinationStore{})
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: agentID, Name: "unauthorized", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "public update", StatusLevel: "identified"})

	if out.Ok {
		t.Fatalf("publish_status_update allowed for unauthorized agent; want forbidden")
	}
	if !strings.Contains(out.Error, "authorized") && !strings.Contains(out.Error, "assigned") {
		t.Fatalf("expected authorization error, got: %s", out.Error)
	}
}

func TestPublishStatusUpdateCommunicatorCreatesStatusUpdate(t *testing.T) {
	commsID := uuid.New()
	incidentID := "22"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commsID, AgentName: "comms"},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commsID, Name: "comms", Capabilities: []string{capability.Communicate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "Checkout is recovering, ETA 5m", StatusLevel: "identified"})
	if !out.Ok {
		t.Fatalf("publish_status_update failed: %s", out.Error)
	}
	if len(coordStore.messages) != 1 || coordStore.messages[0].Kind != store.IncidentCoordinationKindStatusUpdate {
		t.Fatalf("expected one status_update record, got %+v", coordStore.messages)
	}
	if coordStore.messages[0].Metadata["status_level"] != "identified" {
		t.Fatalf("status_level not recorded: %+v", coordStore.messages[0].Metadata)
	}
	if coordStore.messages[0].ActorType != store.IncidentCoordinationActorAgent {
		t.Fatalf("actor type = %q, want agent", coordStore.messages[0].ActorType)
	}
}

func TestPublishStatusUpdateRecordsSourceCoordinationMessageID(t *testing.T) {
	commsID := uuid.New()
	incidentID := "23"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commsID, AgentName: "comms"},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commsID, Name: "comms", Capabilities: []string{capability.Communicate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "Public update", StatusLevel: "identified", SourceCoordinationMessageID: "msg-123"})
	if !out.Ok {
		t.Fatalf("publish_status_update failed: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(coordStore.messages))
	}
	if coordStore.messages[0].Metadata["source_coordination_message_id"] != "msg-123" {
		t.Fatalf("source_coordination_message_id not propagated: %+v", coordStore.messages[0].Metadata)
	}
}

func TestPublishStatusUpdateRejectsInternalAlertReference(t *testing.T) {
	commsID := uuid.New()
	incidentID := "25"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commsID, AgentName: "comms"},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commsID, Name: "comms", Capabilities: []string{capability.Communicate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "Incident resolved; Alert #54 was a synthetic test.", StatusLevel: "resolved"})
	if out.Ok {
		t.Fatalf("publish_status_update accepted alert reference; want rejected")
	}
	if !strings.Contains(out.Error, "public-facing") {
		t.Fatalf("error = %q, want public-facing guidance", out.Error)
	}
	if len(coordStore.messages) != 0 {
		t.Fatalf("expected no message created, got %d", len(coordStore.messages))
	}
}

func TestPublishStatusUpdateRejectsInvalidStatusLevel(t *testing.T) {
	commsID := uuid.New()
	incidentID := "24"
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleCommunicationsLead), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commsID, AgentName: "comms"},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commsID, Name: "comms", Capabilities: []string{capability.Communicate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "Public update", StatusLevel: "bogus"})
	if out.Ok {
		t.Fatalf("publish_status_update accepted invalid status_level; want rejected")
	}
	if !strings.Contains(out.Error, "status_level") {
		t.Fatalf("expected status_level validation error, got: %s", out.Error)
	}
	if len(coordStore.messages) != 0 {
		t.Fatalf("expected no message created on invalid status_level, got %d", len(coordStore.messages))
	}
}

func TestPromotedAlertResolutionBoundaries(t *testing.T) {
	commanderID := uuid.New()
	responderID := uuid.New()
	incidentID := uuid.New()
	invUUID := uuid.New()

	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              responderID.String(),
				AgentName:            "ResponderAgent",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
				PromotedIncidentID:   &incidentID,
			},
		},
	}
	alertRec := store.AlertRecord{Fingerprint: "fp-1", Status: "firing", AlertNumber: 1, Labels: map[string]string{"alertname": "HighCPU"}}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": alertRec},
		byNumber: map[int64]store.AlertRecord{1: alertRec},
	}}
	incidentStore := &incidentStoreSpy{}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	// Responder tries to resolve via alert chat -> denied (alert closure is owned by the commander in incident scope).
	outResponder := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           responderID,
		Name:         "ResponderAgent",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_alert"})
	if outResponder.Ok {
		t.Fatalf("expected alert owner resolve to be blocked for responder (commander owns alert closure in incident scope)")
	}
	if !strings.Contains(outResponder.Error, "only the incident commander is authorized") {
		t.Fatalf("expected commander authorization error message, got: %s", outResponder.Error)
	}

	// Commander tries to resolve via alert chat -> allowed (alert closure is part of incident closure).
	out2 := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           commanderID,
		Name:         "CommanderAgent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_alert"})
	if !out2.Ok {
		t.Fatalf("expected commander to be allowed to resolve the alert as part of incident closure, got error: %s", out2.Error)
	}
}

// TestResolveAlertDoesNotWriteIncidentSummaryOnPromotedAlert locks in the rule
// that the incident Summary card is owned by the incident commander
// (alga_resolve_incident -> populateIncidentResolutionDocuments). Even when the
// alert has been promoted to an active incident and the resolver passes
// root_cause / resolution in alga_resolve_alert, the executor must NOT propagate
// those values into the incident summary; doing so overwrites or competes with
// the commander's authoritative executive summary.
func TestResolveAlertDoesNotWriteIncidentSummaryOnPromotedAlert(t *testing.T) {
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := uuid.New()
	invUUID := uuid.New()

	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              responderID.String(),
				AgentName:            "ResponderAgent",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
				PromotedIncidentID:   &incidentID,
			},
		},
	}
	alertRec := store.AlertRecord{Fingerprint: "fp-1", Status: "firing", AlertNumber: 1, Labels: map[string]string{"alertname": "HighCPU"}}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": alertRec},
		byNumber: map[int64]store.AlertRecord{1: alertRec},
	}}
	incidentStore := &summaryTrackingIncidentStore{summary: "commander-authored executive summary"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetIncidentStore(incidentStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	rootCause := "pg3 data directory corrupted"
	resolution := "reinitialized pg3 via patronictl reinit"

	// Resolver tries first: must be rejected at the role guard (alert closure is
	// commander-only in incident scope).
	outResponder := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           responderID,
		Name:         "ResponderAgent",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{
		ChatID:     "alert_1",
		Op:         "resolve_alert",
		RootCause:  &rootCause,
		Resolution: &resolution,
	})
	if outResponder.Ok {
		t.Fatalf("resolver must be rejected at the role guard; got Ok with no error")
	}
	if !strings.Contains(outResponder.Error, "only the incident commander is authorized") {
		t.Fatalf("expected role-guard rejection pointing at the commander, got: %s", outResponder.Error)
	}
	if alerts.byNumber[1].Status == "resolved" {
		t.Fatalf("resolver's rejected call must not have closed the alert")
	}

	// Commander calls next: must succeed at the role guard, must close the alert,
	// and must NOT write the incident Summary card.
	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           commanderID,
		Name:         "CommanderAgent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{
		ChatID:     "alert_1",
		Op:         "resolve_alert",
		RootCause:  &rootCause,
		Resolution: &resolution,
	})
	if !out.Ok {
		t.Fatalf("commander must be allowed to close the alert as part of incident closure, got error: %s", out.Error)
	}
	if got := alerts.byNumber[1].Status; got != "resolved" {
		t.Fatalf("alert status = %q, want resolved", got)
	}
	if incidentStore.updatedSum != nil {
		t.Fatalf("incident summary was written by alga_resolve_alert (got %q); the Summary card is commander-only via alga_resolve_incident and must remain %q", *incidentStore.updatedSum, "commander-authored executive summary")
	}
	for _, entry := range incidentStore.timeline {
		if entry.Message == "Incident summary updated from alert investigation outcome" {
			t.Fatalf("unexpected summary-update timeline entry from alga_resolve_alert: %+v", entry)
		}
	}
}

// TestResponderCannotResolveAlertInIncidentScope locks in the rule that
// alga_resolve_alert on a promoted alert is commander-only. The resolver must
// be rejected at the role guard so the agent does not race the commander on
// alert closure or try to push the Summary card from the alert thread.
func TestResponderCannotResolveAlertInIncidentScope(t *testing.T) {
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := uuid.New()
	invUUID := uuid.New()

	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              responderID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
				PromotedIncidentID:   &incidentID,
			},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
		byNumber: map[int64]store.AlertRecord{1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetICSRoleStore(roleStore)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           responderID,
		Name:         "ResponderAgent",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_alert"})
	if out.Ok {
		t.Fatalf("resolver must be rejected at the role guard; got Ok")
	}
	if !strings.Contains(out.Error, "only the incident commander is authorized") {
		t.Fatalf("expected commander-only authorization error, got: %s", out.Error)
	}
	if alerts.byNumber[1].Status == "resolved" {
		t.Fatalf("rejected resolver call must not have closed the alert")
	}
}

func TestCommanderCanResolveAlertInIncidentScopeWithoutInvestigate(t *testing.T) {
	commanderID := uuid.New()
	incidentID := uuid.New()
	invUUID := uuid.New()

	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              commanderID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
				PromotedIncidentID:   &incidentID,
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{
					ID:                   invUUID,
					AlertInvestigationID: "AINV-1",
					Status:               "investigating",
					AgentID:              commanderID.String(),
					Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
					PromotedIncidentID:   &incidentID,
				},
			},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
		byNumber: map[int64]store.AlertRecord{1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetICSRoleStore(roleStore)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           commanderID,
		Name:         "CommanderAgent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "alert_1", Op: "resolve_alert"})
	if !out.Ok {
		t.Fatalf("expected commander to successfully resolve the alert, got error: %s", out.Error)
	}
	if alerts.byNumber[1].Status != "resolved" {
		t.Fatalf("expected alert to be resolved")
	}
}

func TestCommanderCanResolveAlertFromIncidentCoordinationThread(t *testing.T) {
	commanderID := uuid.New()
	incidentUUID := uuid.New()
	invUUID := uuid.New()

	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              commanderID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
				PromotedIncidentID:   &incidentUUID,
				PrimaryAlertNumber:   1,
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{
					ID:                   invUUID,
					AlertInvestigationID: "AINV-1",
					Status:               "investigating",
					AgentID:              commanderID.String(),
					Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
					PromotedIncidentID:   &incidentUUID,
					PrimaryAlertNumber:   1,
				},
			},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
		byNumber: map[int64]store.AlertRecord{1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	incStore := &incidentStoreSpy{
		id: incidentUUID,
	}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetIncidentStore(incStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           commanderID,
		Name:         "CommanderAgent",
		Capabilities: []string{capability.Command},
	}, agent.InvTool{ChatID: "incident_coord_123", Op: "resolve_alert"})
	if !out.Ok {
		t.Fatalf("expected commander to successfully resolve the alert via incident_coord_123, got error: %s", out.Error)
	}
	if alerts.byNumber[1].Status != "resolved" {
		t.Fatalf("expected alert to be resolved")
	}
}

func TestResponderCannotResolveAlertFromIncidentCoordinationThread(t *testing.T) {
	responderID := uuid.New()
	incidentUUID := uuid.New()
	invUUID := uuid.New()

	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              responderID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
				PromotedIncidentID:   &incidentUUID,
				PrimaryAlertNumber:   1,
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{
					ID:                   invUUID,
					AlertInvestigationID: "AINV-1",
					Status:               "investigating",
					AgentID:              responderID.String(),
					Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
					PromotedIncidentID:   &incidentUUID,
					PrimaryAlertNumber:   1,
				},
			},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
		byNumber: map[int64]store.AlertRecord{1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
	}}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}
	incStore := &incidentStoreSpy{
		id: incidentUUID,
	}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetIncidentStore(incStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))

	out := executor.ExecuteInvTool(context.Background(), &store.AgentTokenRecord{
		ID:           responderID,
		Name:         "ResponderAgent",
		Capabilities: []string{capability.Investigate},
	}, agent.InvTool{ChatID: "incident_coord_123", Op: "resolve_alert"})
	if out.Ok {
		t.Fatalf("expected responder to be rejected when resolving the alert via incident_coord_123, but got success")
	}
	if !strings.Contains(out.Error, "only the incident commander is authorized") {
		t.Fatalf("expected commander authorization error, got: %s", out.Error)
	}
	if alerts.byNumber[1].Status == "resolved" {
		t.Fatalf("alert status must not be resolved")
	}
}

// publishStatusUpdateFixture wires up the executor + coordination + role stores
// for the publish-status-update defer tests. It returns the executor and the
// stores so individual tests can assert against them.
func publishStatusUpdateFixture(t *testing.T, incidentNumber int64, roles []store.ICSRoleRecord) (*agent.AgentToolExecutor, *agentToolCoordinationStore, *summaryTrackingIncidentStore, *spyForwarder) {
	t.Helper()
	coordStore := &agentToolCoordinationStore{}
	roleStore := &mockICSRoleStore{roles: roles}
	spy := &spyForwarder{}
	incidentStore := &summaryTrackingIncidentStore{summary: "summary"}
	incidentStore.transitionTo = "active"
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetInvestigationForwarder(spy)
	return executor, coordStore, incidentStore, spy
}

type spyForwarder struct{}

func (s *spyForwarder) ForwardToAgent(agentIDHex, investigationID, senderID, senderName, message string) error {
	return nil
}

func (s *spyForwarder) ForwardEventToAgent(agentIDHex string, event sse.Event) error {
	return nil
}

func (s *spyForwarder) AgentOnline(agentIDHex string) bool {
	return true
}

func TestPublishStatusUpdateCommanderPublishesDirectlyWhenCommunicatorAssigned(t *testing.T) {
	commanderID := uuid.New()
	communicatorID := uuid.New()
	incidentID := "100"
	executor, coordStore, _, _ := publishStatusUpdateFixture(t, 100, []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &communicatorID, Status: string(ics.RoleStatusActive)},
	})

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "investigating root cause", StatusLevel: "investigating"})

	if !out.Ok {
		t.Fatalf("commander must be able to publish directly even with a Communications Lead assigned; got error: %s", out.Error)
	}
	if len(coordStore.messages) != 1 || coordStore.messages[0].Kind != store.IncidentCoordinationKindStatusUpdate {
		t.Fatalf("expected one status_update coordination message published by the commander, got %+v", coordStore.messages)
	}
}

func TestPublishStatusUpdateCommanderAllowedForResolvedWhenCommunicatorAssigned(t *testing.T) {
	commanderID := uuid.New()
	communicatorID := uuid.New()
	incidentID := "101"
	executor, coordStore, _, _ := publishStatusUpdateFixture(t, 101, []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &communicatorID, Status: string(ics.RoleStatusActive)},
	})

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "cluster fully recovered", StatusLevel: "resolved"})

	if !out.Ok {
		t.Fatalf("commander should be allowed to publish the resolved status update even with a Communications Lead assigned; got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 || coordStore.messages[0].Kind != store.IncidentCoordinationKindStatusUpdate {
		t.Fatalf("expected one status_update coordination message, got %+v", coordStore.messages)
	}
}

func TestPublishStatusUpdateCommanderAllowedWhenNoCommunicator(t *testing.T) {
	commanderID := uuid.New()
	incidentID := "102"
	executor, coordStore, _, _ := publishStatusUpdateFixture(t, 102, []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	})

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "investigating", StatusLevel: "investigating"})

	if !out.Ok {
		t.Fatalf("commander should be able to publish when no Communications Lead is assigned; got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one status_update message, got %d", len(coordStore.messages))
	}
}

func TestPublishStatusUpdateResponderBypassesDeferral(t *testing.T) {
	responderID := uuid.New()
	communicatorID := uuid.New()
	incidentID := "103"
	executor, coordStore, _, _ := publishStatusUpdateFixture(t, 103, []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleCommunicationsLead), AssigneeType: "agent", AgentTokenID: &communicatorID, Status: string(ics.RoleStatusActive)},
	})

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "publishing for the responder", StatusLevel: "identified"})

	if !out.Ok {
		t.Fatalf("responder should not be blocked by the commander-deferral guard; got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one status_update message, got %d", len(coordStore.messages))
	}
}

// responderGateCoordinationStore is a test coordination store that lets a test
// pre-seed status updates and coordination replies (keyed by kind) and observes
// new writes via CreateMessage. It is used by the responder-only server-side
// gate tests.
type responderGateCoordinationStore struct {
	agentToolCoordinationStore
	preStatusUpdates []store.IncidentCoordinationMessageRecord
	preCoordReplies  []store.IncidentCoordinationMessageRecord
	preHumanMentions []store.IncidentCoordinationMessageRecord
}

func (s *responderGateCoordinationStore) ListMessagesByKind(_ context.Context, _ int64, kind string, _ int, _ int) ([]store.IncidentCoordinationMessageRecord, error) {
	switch kind {
	case store.IncidentCoordinationKindStatusUpdate:
		// Combine pre-seeded with newly created ones (status_update kind).
		out := append([]store.IncidentCoordinationMessageRecord{}, s.preStatusUpdates...)
		for _, m := range s.messages {
			if m.Kind == store.IncidentCoordinationKindStatusUpdate {
				out = append(out, m)
			}
		}
		return out, nil
	case store.IncidentCoordinationKindAgentReply:
		out := append([]store.IncidentCoordinationMessageRecord{}, s.preCoordReplies...)
		for _, m := range s.messages {
			if m.Kind == store.IncidentCoordinationKindAgentReply {
				out = append(out, m)
			}
		}
		return out, nil
	}
	return nil, nil
}

func (s *responderGateCoordinationStore) ListMessages(_ context.Context, _ int64, _ int, _ int) ([]store.IncidentCoordinationMessageRecord, error) {
	// Return pre-seeded human mentions plus any newly created non-status messages.
	out := append([]store.IncidentCoordinationMessageRecord{}, s.preHumanMentions...)
	for _, m := range s.messages {
		if m.Kind != store.IncidentCoordinationKindStatusUpdate {
			out = append(out, m)
		}
	}
	return out, nil
}

func TestPublishStatusUpdateResponderCannotPublishResolved(t *testing.T) {
	responderID := uuid.New()
	incidentID := "200"
	incidentNumber := int64(200)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-200", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "all clear", StatusLevel: "resolved"})

	if out.Ok {
		t.Fatalf("responder must NOT be allowed to publish status_level=resolved; expected rejection, got Ok")
	}
	if !strings.Contains(out.Error, "resolved") {
		t.Fatalf("rejection error should mention resolved, got: %s", out.Error)
	}
	if !strings.Contains(out.Error, "commander") {
		t.Fatalf("rejection error should point at the commander, got: %s", out.Error)
	}
	if len(coordStore.messages) != 0 {
		t.Fatalf("no status update should have been recorded for rejected resolved publish; got %d", len(coordStore.messages))
	}
}

func TestPublishStatusUpdateResponderCannotPublishInvestigating(t *testing.T) {
	responderID := uuid.New()
	incidentID := "201"
	incidentNumber := int64(201)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-201", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "starting up", StatusLevel: "investigating"})

	if out.Ok {
		t.Fatalf("responder must NOT be allowed to publish status_level=investigating; expected rejection, got Ok")
	}
	if !strings.Contains(out.Error, "investigating") {
		t.Fatalf("rejection error should mention investigating, got: %s", out.Error)
	}
}

func TestPublishStatusUpdateAgentWithBothResponderAndCommanderRolesCanPublishResolved(t *testing.T) {
	// An agent that holds BOTH responder and commander roles is not "responder-only"
	// and may publish resolved as the commander. This covers the case where a
	// single agent plays both roles on a small incident.
	agentID := uuid.New()
	incidentID := "202"
	incidentNumber := int64(202)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &agentID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-202", IncidentNumber: incidentNumber, AgentID: agentID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: agentID, Name: "dual-role", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "resolved", StatusLevel: "resolved"})

	if !out.Ok {
		t.Fatalf("agent holding both responder and commander roles must be allowed to publish resolved (commander-hat): %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one status_update recorded, got %d", len(coordStore.messages))
	}
}

func TestPublishStatusUpdateCommanderCanPublishResolved(t *testing.T) {
	// Sanity: a pure commander (no responder role) must still be allowed to
	// publish status_level=resolved. The new gate must not regress this.
	commanderID := uuid.New()
	incidentID := "203"
	executor, coordStore, _, _ := publishStatusUpdateFixture(t, 203, []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	})

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "publish_status_update", Message: "incident resolved", StatusLevel: "resolved"})

	if !out.Ok {
		t.Fatalf("commander must be allowed to publish resolved; got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one status_update recorded, got %d", len(coordStore.messages))
	}
}

func TestPostHandoffResponderBlockedBeforeIdentifiedAndMonitoring(t *testing.T) {
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := "300"
	incidentNumber := int64(300)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		// No status updates seeded — the responder has not yet published identified or monitoring.
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-300", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "investigation findings", Audience: "commander"})

	if out.Ok {
		t.Fatalf("responder must NOT be allowed to post coordination updates before publishing identified status")
	}
	if !strings.Contains(out.Error, "identified") {
		t.Fatalf("rejection error should mention identified (first gate), got: %s", out.Error)
	}
	if !strings.Contains(out.Error, "alga_publish_status_update") {
		t.Fatalf("rejection error should point at alga_publish_status_update, got: %s", out.Error)
	}
	if len(coordStore.messages) != 0 {
		t.Fatalf("no coordination message should have been recorded when the gate rejects; got %d", len(coordStore.messages))
	}
}

func TestPostHandoffResponderBlockedBeforeMonitoring(t *testing.T) {
	// With identified seeded but monitoring missing, the gate must reject the handoff.
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := "1301"
	incidentNumber := int64(1301)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		preStatusUpdates: []store.IncidentCoordinationMessageRecord{
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "identified"}},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1301", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "still working", Audience: "commander"})

	if out.Ok {
		t.Fatalf("responder must NOT be allowed to post the handoff with only identified seeded (monitoring missing)")
	}
	if !strings.Contains(out.Error, "mitigated") {
		t.Fatalf("rejection error should mention mitigated (required since monitoring is missing), got: %s", out.Error)
	}
	if !strings.Contains(out.Error, "monitoring") {
		t.Fatalf("rejection error should mention monitoring (as the alternative to mitigated), got: %s", out.Error)
	}
}

func TestPostHandoffResponderAllowedAfterMitigatedStatus(t *testing.T) {
	// Fast mitigation path: identified + mitigated is sufficient for handoff (monitoring optional).
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := "1320"
	incidentNumber := int64(1320)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		preStatusUpdates: []store.IncidentCoordinationMessageRecord{
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "identified"}},
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "mitigated"}},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1320", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "ready for commander verification", Audience: "commander"})

	if !out.Ok {
		t.Fatalf("responder should be allowed to post the handoff after identified + mitigated (monitoring optional); got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one coordination message recorded, got %d", len(coordStore.messages))
	}
}

func TestPostHandoffResponderAllowedAfterMitigatedAndMonitoring(t *testing.T) {
	// Slow verification path: identified + mitigated + monitoring is also valid.
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := "1321"
	incidentNumber := int64(1321)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		preStatusUpdates: []store.IncidentCoordinationMessageRecord{
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "identified"}},
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "mitigated"}},
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "monitoring"}},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-1321", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "ready for commander verification", Audience: "commander"})

	if !out.Ok {
		t.Fatalf("responder should be allowed to post the handoff after identified + mitigated + monitoring; got: %s", out.Error)
	}
}

func TestPostHandoffResponderAllowedAfterMonitoringStatus(t *testing.T) {
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := "301"
	incidentNumber := int64(301)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		preStatusUpdates: []store.IncidentCoordinationMessageRecord{
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "identified"}},
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "monitoring"}},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-301", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "ready for commander verification", Audience: "commander"})

	if !out.Ok {
		t.Fatalf("responder should be allowed to post the single final handoff after publishing identified + monitoring; got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one coordination message recorded, got %d", len(coordStore.messages))
	}
}

func TestPostHandoffResponderOneShotRule(t *testing.T) {
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := "302"
	incidentNumber := int64(302)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		preStatusUpdates: []store.IncidentCoordinationMessageRecord{
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "identified"}},
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, ActorID: &responderID, Metadata: map[string]any{"status_level": "monitoring"}},
		},
		preCoordReplies: []store.IncidentCoordinationMessageRecord{
			{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindAgentReply, ActorID: &responderID, Metadata: map[string]any{"source_tool": "post_handoff"}},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-302", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "another update", Audience: "commander"})

	if out.Ok {
		t.Fatalf("responder must NOT be allowed to post a SECOND coordination update (one-shot rule)")
	}
	if !strings.Contains(out.Error, "ONE") && !strings.Contains(out.Error, "at most") {
		t.Fatalf("rejection error should mention the one-shot rule, got: %s", out.Error)
	}
}

func TestPostHandoffCommanderNotAffectedByResponderGate(t *testing.T) {
	// Sanity: a pure commander (no responder role) must be able to post
	// coordination updates freely — the responder gate must not regress this.
	commanderID := uuid.New()
	incidentID := "303"
	incidentNumber := int64(303)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		// No monitoring status update, but commander should be unaffected.
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-303", IncidentNumber: incidentNumber, AgentID: commanderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "commander update", Audience: "none"})

	if !out.Ok {
		t.Fatalf("commander must be able to post coordination updates without the responder gate firing; got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one coordination message recorded, got %d", len(coordStore.messages))
	}
}

func TestPostHandoffResponderAllowedWhenHumanMentionCarveOutApplies(t *testing.T) {
	// A responder who has NOT published monitoring AND has NOT done the
	// single-handoff should still be allowed to post a handoff if a human
	// operator @mentioned them in the coordination thread within the last
	// hour. The carve-out exists so a human can ask a direct question and
	// get a direct answer instead of being routed through status updates.
	responderID := uuid.New()
	humanID := uuid.New()
	incidentID := "400"
	incidentNumber := int64(400)
	now := time.Now()
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		// No monitoring status update, no prior handoff — but a recent human
		// @mention of this responder unlocks the gate.
		preHumanMentions: []store.IncidentCoordinationMessageRecord{
			{
				IncidentNumber:   incidentNumber,
				Kind:             store.IncidentCoordinationKindChat,
				ActorType:        store.IncidentCoordinationActorUser,
				ActorID:          &humanID,
				ActorDisplayName: "oncall",
				Body:             "@responder what's the status?",
				Source:           store.IncidentCoordinationSourceAlga,
				Metadata:         map[string]any{"mentions": []string{"agent:" + responderID.String()}},
				CreatedAt:        now.Add(-2 * time.Minute),
			},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-400", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "still investigating, will update shortly", Audience: "commander"})

	if !out.Ok {
		t.Fatalf("responder should be allowed to post a handoff when a human recently @mentioned them (carve-out); got: %s", out.Error)
	}
	if len(coordStore.messages) != 1 {
		t.Fatalf("expected one coordination message recorded, got %d", len(coordStore.messages))
	}
}

func TestPostHandoffHumanMentionCarveOutIgnoresAgentMentions(t *testing.T) {
	// A prior AGENT @mention (not a human one) must NOT unlock the carve-out.
	// Otherwise the responder and commander could ping-pong each other via
	// agent-to-agent @mentions forever, which is the exact behavior the
	// gate is meant to prevent.
	responderID := uuid.New()
	commanderID := uuid.New()
	incidentID := "401"
	incidentNumber := int64(401)
	now := time.Now()
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		// Pre-seed an AGENT message (commander) that @mentions the responder.
		// This should NOT trigger the carve-out because the actor is an agent,
		// not a human.
		preHumanMentions: []store.IncidentCoordinationMessageRecord{
			{
				IncidentNumber:   incidentNumber,
				Kind:             store.IncidentCoordinationKindAgentReply,
				ActorType:        store.IncidentCoordinationActorAgent,
				ActorID:          &commanderID,
				ActorDisplayName: "commander",
				Body:             "@responder please provide an update",
				Source:           store.IncidentCoordinationSourceAgent,
				Metadata:         map[string]any{"mentions": []string{"agent:" + responderID.String()}},
				CreatedAt:        now.Add(-2 * time.Minute),
			},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-401", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "ok on it", Audience: "commander"})

	if out.Ok {
		t.Fatalf("an agent @mention must NOT trigger the human-mention carve-out; got Ok with no error")
	}
	if !strings.Contains(out.Error, "identified") {
		t.Fatalf("rejection should still require identified (first gate) when carve-out does not apply, got: %s", out.Error)
	}
}

func TestPostHandoffHumanMentionCarveOutExpiresAfterOneHour(t *testing.T) {
	// A human @mention older than 1 hour must NOT unlock the carve-out —
	// otherwise the gate would be permanently bypassed after a single human
	// ping.
	responderID := uuid.New()
	humanID := uuid.New()
	incidentID := "402"
	incidentNumber := int64(402)
	now := time.Now()
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}
	coordStore := &responderGateCoordinationStore{
		preHumanMentions: []store.IncidentCoordinationMessageRecord{
			{
				IncidentNumber:   incidentNumber,
				Kind:             store.IncidentCoordinationKindChat,
				ActorType:        store.IncidentCoordinationActorUser,
				ActorID:          &humanID,
				ActorDisplayName: "oncall",
				Body:             "@responder ping",
				Source:           store.IncidentCoordinationSourceAlga,
				Metadata:         map[string]any{"mentions": []string{"agent:" + responderID.String()}},
				CreatedAt:        now.Add(-2 * time.Hour), // well past the 1h window
			},
		},
	}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(&incidentStoreSpy{})
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-402", IncidentNumber: incidentNumber, AgentID: responderID.String(), Status: "investigating",
	}})
	executor.SetIncidentCoordinationStore(coordStore)
	executor.SetICSRoleStore(roleStore)

	out := executor.ExecuteInvTool(context.Background(),
		&store.AgentTokenRecord{ID: responderID, Name: "responder", Capabilities: []string{capability.Investigate}},
		agent.InvTool{ChatID: "incident_coord_" + incidentID, Op: "post_handoff", Message: "still here", Audience: "commander"})

	if out.Ok {
		t.Fatalf("a human mention older than 1h must NOT unlock the carve-out; got Ok")
	}
	if !strings.Contains(out.Error, "identified") {
		t.Fatalf("rejection should still require identified (first gate) when carve-out expired, got: %s", out.Error)
	}
}

// TestResolveIncidentDoesNotRequireResponderFinding verifies that the
// alga_add_finding gate has been removed: a commander can resolve an incident
// when the required documents (summary, resolved status update,
// impact_assessment, actions_taken) are present, even if the responder never
// called alga_add_finding on the active incident investigation. Evidence in
// the coordination thread and alert investigation is sufficient.
func TestResolveIncidentDoesNotRequireResponderFinding(t *testing.T) {
	commanderID := uuid.New()
	responderID := uuid.New()
	incidentID := int64(110)
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
		{RoleType: string(ics.RoleResponder), AssigneeType: "agent", AgentTokenID: &responderID, Status: string(ics.RoleStatusActive)},
	}}
	// Active investigation with no updates (no alga_add_finding calls yet).
	investigationStore := &spyIncidentInvestigationStore{active: &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-110",
		IncidentNumber:          incidentID,
		AgentID:                 responderID.String(),
		AgentName:               "responder",
		Status:                  "investigating",
	}}
	docStore := &recordingDocStore{}
	coordStore := &statusUpdateCoordinationStore{statusUpdates: []store.IncidentCoordinationMessageRecord{
		{Body: "resolved", Metadata: map[string]any{"status_level": "resolved"}},
	}}
	incidentStore := &summaryTrackingIncidentStore{summary: "Resolved summary"}
	incidentStore.transitionTo = "mitigated"

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(investigationStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)
	executor.SetIncidentCoordinationStore(coordStore)

	cmd := &store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}}
	rootCause := "Patroni promoted a stale replica after the leader lost quorum."
	resolution := "Reinitialized the stale replica via patronictl reinit and verified replication."
	resolveCmd := agent.InvTool{
		ChatID:           "incident_coord_110",
		Op:               "resolve_incident",
		Reason:           "fixed",
		Summary:          "Resolved summary",
		ImpactAssessment: "Replica recovered via patronictl reinit; leader and other replica unaffected.",
		ActionsTaken:     "Reinitialized pg2 and verified replication with pg_isready and pg_stat_replication.",
		RootCause:        &rootCause,
		Resolution:       &resolution,
	}

	// Commander resolves with no prior alga_add_finding — must succeed.
	out := executor.ExecuteInvTool(context.Background(), cmd, resolveCmd)
	if !out.Ok {
		t.Fatalf("resolve_incident must succeed when summary, status update, and documents are present, even without alga_add_finding; got: %s", out.Error)
	}
}

// TestResolveIncidentNoActiveInvestigationDoesNotBlock ensures the finding
// requirement is only enforced when an active investigation exists. Without an
// investigation, there's nothing to gate against — the other resolution
// requirements still apply.
func TestResolveIncidentNoActiveInvestigationDoesNotBlock(t *testing.T) {
	commanderID := uuid.New()
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{RoleType: string(ics.RoleIncidentCommander), AssigneeType: "agent", AgentTokenID: &commanderID, Status: string(ics.RoleStatusActive)},
	}}
	investigationStore := &spyIncidentInvestigationStore{} // active is nil -> guard skips
	docStore := &recordingDocStore{}
	coordStore := &statusUpdateCoordinationStore{statusUpdates: []store.IncidentCoordinationMessageRecord{
		{Body: "resolved", Metadata: map[string]any{"status_level": "resolved"}},
	}}
	incidentStore := &summaryTrackingIncidentStore{summary: "Resolved summary"}
	incidentStore.transitionTo = "mitigated"

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(investigationStore)
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)
	executor.SetIncidentCoordinationStore(coordStore)

	cmd := &store.AgentTokenRecord{ID: commanderID, Name: "commander", Capabilities: []string{capability.Command}}
	rootCause := "Bad config rollouts bypassed the canary stage."
	resolution := "Reverted the configuration change and confirmed service health."
	out := executor.ExecuteInvTool(context.Background(), cmd, agent.InvTool{
		ChatID:           "incident_coord_111",
		Op:               "resolve_incident",
		Reason:           "fixed",
		Summary:          "Resolved summary",
		ImpactAssessment: "No impact — operational recovery completed.",
		ActionsTaken:     "Reverted configuration and confirmed service health.",
		RootCause:        &rootCause,
		Resolution:       &resolution,
	})
	if !out.Ok {
		t.Fatalf("resolve_incident should not block on finding when no active investigation exists; got: %s", out.Error)
	}
}
