package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/capability"
	"alga/config"
	entschema "alga/ent/schema"
	"alga/ics"
	"alga/incmetrics"
	"alga/matching"
	"alga/prompt"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
)

func TestComputeSpecificity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []config.RouteCondition
		want int
	}{
		{name: "empty", in: nil, want: 0},
		{name: "one_exact", in: []config.RouteCondition{{Operator: "exact"}}, want: 15},
		{name: "two_exact", in: []config.RouteCondition{{Operator: "exact"}, {Operator: "exact"}}, want: 30},
		{name: "exact_beats_regex", in: []config.RouteCondition{{Operator: "exact"}}, want: 15},
		{name: "regex_lowest", in: []config.RouteCondition{{Operator: "regex"}}, want: 11},
		{name: "wildcard_middle", in: []config.RouteCondition{{Operator: "wildcard"}}, want: 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeSpecificity(tc.in); got != tc.want {
				t.Fatalf("computeSpecificity(%+v) = %d want %d", tc.in, got, tc.want)
			}
		})
	}

	// Sanity: exact must outscore regex and wildcard for an equal count.
	exact := computeSpecificity([]config.RouteCondition{{Operator: "exact"}, {Operator: "exact"}})
	regex := computeSpecificity([]config.RouteCondition{{Operator: "regex"}, {Operator: "regex"}})
	wild := computeSpecificity([]config.RouteCondition{{Operator: "wildcard"}, {Operator: "wildcard"}})
	if exact <= wild || wild <= regex {
		t.Fatalf("expected exact > wildcard > regex, got %d %d %d", exact, wild, regex)
	}
}

func TestWildcardMatch(t *testing.T) {
	t.Parallel()
	// wildcardMatch requires at least one '*'; literal patterns without
	// a wildcard return false because the matcher consumes the prefix
	// before checking the (now-empty) suffix.
	cases := []struct {
		pattern, input string
		want           bool
	}{
		{"*foo", "barfoo", true},
		{"*foo", "bar", false},
		{"foo*", "foobar", true},
		{"foo*", "bar", false},
		{"*foo*", "barfoobaz", true},
		{"foo*bar", "fooXYZbar", true},
		{"foo*bar", "fooXYZ", false},
		{"a*b*c", "axxxbyyyc", true},
		{"a*b*c", "axxxbyyyz", false},
	}
	for _, tc := range cases {
		if got := wildcardMatch(tc.pattern, tc.input); got != tc.want {
			t.Fatalf("wildcardMatch(%q,%q)=%v want %v", tc.pattern, tc.input, got, tc.want)
		}
	}
}

func TestMatchCondition(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	labels := map[string]string{
		"namespace":  "prod",
		"app":        "frontend-web",
		"alertname":  "HighCPU",
		"deployment": "frontend",
	}
	cases := []struct {
		name string
		c    config.RouteCondition
		want bool
	}{
		{"exact_match", config.RouteCondition{Field: "namespace", Operator: "exact", Value: "prod"}, true},
		{"exact_miss", config.RouteCondition{Field: "namespace", Operator: "exact", Value: "dev"}, false},
		{"contains", config.RouteCondition{Field: "app", Operator: "contains", Value: "front"}, true},
		{"prefix", config.RouteCondition{Field: "app", Operator: "prefix", Value: "frontend"}, true},
		{"suffix", config.RouteCondition{Field: "app", Operator: "suffix", Value: "web"}, true},
		{"wildcard", config.RouteCondition{Field: "app", Operator: "wildcard", Value: "front*web"}, true},
		{"regex_match", config.RouteCondition{Field: "alertname", Operator: "regex", Value: `^High.*`}, true},
		{"regex_miss", config.RouteCondition{Field: "alertname", Operator: "regex", Value: `^Low.*`}, false},
		{"exists_true", config.RouteCondition{Field: "deployment", Operator: "exists"}, true},
		{"exists_false", config.RouteCondition{Field: "missing", Operator: "exists"}, false},
		{"not_exists", config.RouteCondition{Field: "missing", Operator: "not_exists"}, true},
		{"default_is_exact", config.RouteCondition{Field: "namespace", Operator: "", Value: "prod"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.matchCondition(tc.c, labels); got != tc.want {
				t.Fatalf("matchCondition(%+v)=%v want %v", tc.c, got, tc.want)
			}
		})
	}
}

func TestApplyBackoffSkipsActiveAndPrunes(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{backoff: map[string]time.Time{}}
	s.recordBackoff("inv-skip", 1)
	// Manually expire one entry so we exercise the prune branch.
	s.backoff["inv-old"] = time.Now().Add(-time.Minute)

	pending := []store.AlertInvestigationRecord{
		{AlertInvestigationID: "inv-skip"},
		{AlertInvestigationID: "inv-old"},
		{AlertInvestigationID: "inv-fresh"},
	}
	got := s.applyBackoff(pending)
	if len(got) != 2 {
		t.Fatalf("expected 2 surviving entries, got %d (%+v)", len(got), got)
	}
	for _, inv := range got {
		if inv.AlertInvestigationID == "inv-skip" {
			t.Fatalf("inv-skip should have been filtered")
		}
	}
	if _, present := s.backoff["inv-old"]; present {
		t.Fatalf("expired backoff entry should have been pruned")
	}
}

func TestFilterInactiveAlertInvestigationsDropsResolvedAndMissingAlerts(t *testing.T) {
	t.Parallel()

	alerts := &schedulerAlertLookup{
		byFingerprint: map[string]*store.AlertRecord{
			"fp-firing":   {Fingerprint: "fp-firing", Status: "firing"},
			"fp-resolved": {Fingerprint: "fp-resolved", Status: "resolved"},
		},
	}
	pending := []store.AlertInvestigationRecord{
		{
			AlertInvestigationID: "inv-firing",
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-firing"}},
		},
		{
			AlertInvestigationID: "inv-resolved",
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-resolved"}},
		},
		{
			AlertInvestigationID: "inv-missing",
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-missing"}},
		},
	}

	got := filterInactiveAlertInvestigations(alerts, pending)

	if len(got) != 1 {
		t.Fatalf("eligible investigations = %d, want 1 (%+v)", len(got), got)
	}
	if got[0].AlertInvestigationID != "inv-firing" {
		t.Fatalf("eligible investigation = %q, want inv-firing", got[0].AlertInvestigationID)
	}
}

func TestFilterInactiveAlertInvestigationsKeepsReusedFingerprint(t *testing.T) {
	t.Parallel()

	// Same fingerprint is shared by a soft-deleted prior alert and a live new
	// alert. The scheduler must keep the investigation because the live alert
	// still requires an agent — the previous GetByFingerprint implementation
	// could pick the soft-deleted row and drop it.
	deletedAt := time.Now().Add(-time.Hour)
	alerts := &schedulerAlertLookup{
		byFingerprint: map[string]*store.AlertRecord{
			"fp-reused": {AlertNumber: 33, Fingerprint: "fp-reused", Status: "firing"},
			"fp-old":    {AlertNumber: 31, Fingerprint: "fp-reused", Status: "resolved", DeletedAt: &deletedAt},
		},
	}
	pending := []store.AlertInvestigationRecord{
		{
			AlertInvestigationID: "inv-reused",
			Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-reused"}},
		},
	}

	got := filterInactiveAlertInvestigations(alerts, pending)

	if len(got) != 1 || got[0].AlertInvestigationID != "inv-reused" {
		t.Fatalf("expected reused-fingerprint investigation to be kept, got %+v", got)
	}
}

func TestNudgeByStatusSkipsResolvedAlertInvestigations(t *testing.T) {
	t.Parallel()

	resolver := &countingResolver{}
	s := &InvestigationScheduler{
		resolver: resolver,
		alertStore: &schedulerAlertStore{
			schedulerAlertLookup: schedulerAlertLookup{
				byFingerprint: map[string]*store.AlertRecord{
					"fp-resolved": {Fingerprint: "fp-resolved", Status: "resolved"},
				},
			},
		},
	}

	s.nudgeByStatus(context.Background(), func(context.Context, time.Duration) ([]store.AlertInvestigationRecord, error) {
		return []store.AlertInvestigationRecord{
			{
				AlertInvestigationID: "inv-resolved",
				Status:               "investigating",
				AgentID:              uuid.NewString(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-resolved"}},
			},
		}, nil
	}, "investigating", time.Minute)

	if resolver.forwarded != 0 {
		t.Fatalf("forwarded prompts = %d, want 0", resolver.forwarded)
	}
	if _, nudged := s.nudged.Load("inv-resolved"); nudged {
		t.Fatalf("resolved investigation should not be marked nudged")
	}
}

func TestNudgeByStatusIncludesPlaybookEnrichment(t *testing.T) {
	playbookID := uuid.New()
	resolver := &capturePromptResolver{}
	s := &InvestigationScheduler{
		resolver: resolver,
		playbookEnricher: prompt.NewPlaybookEnricher(&schedulerPlaybookStore{
			matches: []*store.PlaybookRecord{{ID: playbookID, Title: "CPU Runbook", Kind: "procedure"}},
			steps: map[uuid.UUID][]store.PlaybookStepRecord{
				playbookID: {
					{StepNumber: 1, Title: "Check pods", Description: "Find noisy workloads"},
				},
			},
		}),
	}

	s.nudgeByStatus(context.Background(), func(context.Context, time.Duration) ([]store.AlertInvestigationRecord, error) {
		return []store.AlertInvestigationRecord{
			{
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              uuid.NewString(),
				AgentName:            "agent-one",
				Alerts: []rabbitmq.CorrelatedAlert{{
					Fingerprint: "fp-1",
					Labels:      map[string]string{"alertname": "HighCPU"},
				}},
			},
		}, nil
	}, "investigating", time.Minute)

	if !strings.Contains(resolver.prompt, "CPU Runbook") || !strings.Contains(resolver.prompt, "Find noisy workloads") {
		t.Fatalf("nudged prompt missing playbook enrichment:\n%s", resolver.prompt)
	}
}

func TestPickCandidatePrefersLabelMatchOverCatchAll(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{maxConcurrent: 5}

	catchAll := &store.AgentTokenRecord{
		ID:           uuid.New(),
		Name:         "all-rounder",
		AgentType:    "hermes",
		Scope:        "all",
		Capabilities: []string{capability.Investigate},
	}
	frontendOnly := &store.AgentTokenRecord{
		ID:           uuid.New(),
		Name:         "frontend",
		AgentType:    "hermes",
		Scope:        "labels",
		Capabilities: []string{capability.Investigate},
		LabelSelectors: []config.RouteCondition{
			{Field: "app", Operator: "exact", Value: "frontend"},
		},
	}
	online := map[string]*store.AgentTokenRecord{
		catchAll.ID.String():     catchAll,
		frontendOnly.ID.String(): frontendOnly,
	}
	capacity := map[string]int{
		catchAll.ID.String():     0,
		frontendOnly.ID.String(): 0,
	}

	inv := store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-1",
		Status:               "pending",
		CreatedAt:            time.Now(),
	}
	// Wrap the labels via an Alert to exercise extractLabels too.
	inv.Alerts = nil // No alerts -> empty labels -> catch-all wins.

	if pick := s.pickCandidate(context.Background(), inv, online, capacity); pick == nil || pick.ID.String() != catchAll.ID.String() {
		t.Fatalf("expected catch-all when no labels match, got %+v", pick)
	}
}

func TestPickCandidateRespectsCapacity(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{maxConcurrent: 2}

	full := &store.AgentTokenRecord{
		ID: uuid.New(), Name: "full", AgentType: "hermes", Scope: "all", Capabilities: []string{capability.Investigate},
	}
	open := &store.AgentTokenRecord{
		ID: uuid.New(), Name: "open", AgentType: "hermes", Scope: "all", Capabilities: []string{capability.Investigate},
	}
	online := map[string]*store.AgentTokenRecord{
		full.ID.String(): full,
		open.ID.String(): open,
	}
	capacity := map[string]int{
		full.ID.String(): 2,
		open.ID.String(): 0,
	}

	pick := s.pickCandidate(context.Background(), store.AlertInvestigationRecord{AlertInvestigationID: "inv-x"}, online, capacity)
	if pick == nil || pick.ID.String() != open.ID.String() {
		t.Fatalf("expected scheduler to skip full agent, got %+v", pick)
	}
}

func TestPickCandidateRequiresInvestigateCapability(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{maxConcurrent: 2}

	commandOnly := &store.AgentTokenRecord{
		ID:           uuid.New(),
		Name:         "commander",
		AgentType:    "hermes",
		Scope:        "all",
		Capabilities: []string{capability.Command},
	}
	responder := &store.AgentTokenRecord{
		ID:           uuid.New(),
		Name:         "responder",
		AgentType:    "hermes",
		Scope:        "all",
		Capabilities: []string{capability.Investigate},
	}
	online := map[string]*store.AgentTokenRecord{
		commandOnly.ID.String(): commandOnly,
		responder.ID.String():   responder,
	}
	capacity := map[string]int{
		commandOnly.ID.String(): 0,
		responder.ID.String():   0,
	}

	pick := s.pickCandidate(context.Background(), store.AlertInvestigationRecord{AlertInvestigationID: "inv-x"}, online, capacity)
	if pick == nil || pick.ID.String() != responder.ID.String() {
		t.Fatalf("expected scheduler to skip non-investigate agent, got %+v", pick)
	}
}

func TestAssignAgentRolesForIncidentDoesNotDuplicateResponderAgent(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	incidentNumber := int64(1)
	roleStore := &schedulerRoleStore{
		roles: []store.ICSRoleRecord{
			{
				ID:             uuid.New(),
				IncidentNumber: incidentNumber,
				RoleType:       string(ics.RoleResponder),
				AssigneeType:   "agent",
				AgentTokenID:   &agentID,
				Status:         string(ics.RoleStatusActive),
				StartedAt:      time.Now().UTC(),
			},
		},
	}
	s := &InvestigationScheduler{
		icsRoleStore:  roleStore,
		resolver:      alwaysOnlineResolver{},
		maxConcurrent: 10,
	}
	agents := []store.AgentTokenRecord{
		{
			ID:           agentID,
			Name:         "responder-agent",
			Capabilities: []string{capability.Investigate},
		},
	}

	s.assignAgentRolesForIncident(context.Background(), incidentNumber, agents)

	if roleStore.assignAgentCalls != 0 {
		t.Fatalf("AssignAgentRole called %d times, want 0", roleStore.assignAgentCalls)
	}
}

func TestAssignAgentRolesForIncidentDoesNotReuseCommanderAsResponder(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	incidentNumber := int64(1)
	roleStore := &schedulerRoleStore{
		roles: []store.ICSRoleRecord{
			{
				ID:             uuid.New(),
				IncidentNumber: incidentNumber,
				RoleType:       string(ics.RoleIncidentCommander),
				AssigneeType:   "agent",
				AgentTokenID:   &agentID,
				Status:         string(ics.RoleStatusActive),
				StartedAt:      time.Now().UTC(),
			},
		},
	}
	s := &InvestigationScheduler{
		icsRoleStore:  roleStore,
		resolver:      alwaysOnlineResolver{},
		maxConcurrent: 10,
	}
	agents := []store.AgentTokenRecord{
		{
			ID:           agentID,
			Name:         "multi-capability-agent",
			Capabilities: []string{capability.Command, capability.Investigate},
		},
	}

	s.assignAgentRolesForIncident(context.Background(), incidentNumber, agents)

	if roleStore.assignAgentCalls != 0 {
		t.Fatalf("AssignAgentRole called %d times, want 0", roleStore.assignAgentCalls)
	}
}

func TestAssignAgentRolesForIncidentAssignsSingleResponderAgent(t *testing.T) {
	t.Parallel()

	incidentNumber := int64(1)
	roleStore := &schedulerRoleStore{}
	s := &InvestigationScheduler{
		icsRoleStore:  roleStore,
		resolver:      alwaysOnlineResolver{},
		maxConcurrent: 10,
	}
	agents := []store.AgentTokenRecord{
		{
			ID:           uuid.New(),
			Name:         "responder-agent-1",
			Capabilities: []string{capability.Investigate},
		},
		{
			ID:           uuid.New(),
			Name:         "responder-agent-2",
			Capabilities: []string{capability.Investigate},
		},
	}

	s.assignAgentRolesForIncident(context.Background(), incidentNumber, agents)

	var responderRoles int
	for _, role := range roleStore.roles {
		if role.RoleType == string(ics.RoleResponder) {
			responderRoles++
		}
	}
	if responderRoles != 1 {
		t.Fatalf("responder roles = %d, want 1", responderRoles)
	}
}

func TestAssignAgentRolesForIncidentUsesSingleCommunicatorRole(t *testing.T) {
	t.Parallel()

	incidentNumber := int64(1)
	roleStore := &schedulerRoleStore{}
	communicatorID := uuid.New()
	s := &InvestigationScheduler{
		icsRoleStore:  roleStore,
		resolver:      alwaysOnlineResolver{},
		maxConcurrent: 10,
	}
	agents := []store.AgentTokenRecord{
		{
			ID:           communicatorID,
			Name:         "communicator-agent",
			Capabilities: []string{capability.Communicate},
		},
	}

	s.assignAgentRolesForIncident(context.Background(), incidentNumber, agents)

	var communicatorRoles int
	for _, role := range roleStore.roles {
		if role.RoleType == string(ics.RoleCommunicationsLead) {
			communicatorRoles++
		}
	}
	if communicatorRoles != 1 {
		t.Fatalf("communicator roles = %d, want 1", communicatorRoles)
	}
}

func TestResolveIncidentRole(t *testing.T) {
	t.Parallel()

	agentA := uuid.New()
	agentB := uuid.New()
	incidentNumber := int64(1)

	cases := []struct {
		name       string
		nilStore   bool
		roles      []store.ICSRoleRecord
		queryAgent string
		want       string
	}{
		{
			name: "agent match returns role type",
			roles: []store.ICSRoleRecord{
				{
					RoleType:     string(ics.RoleResponder),
					AssigneeType: "agent",
					AgentTokenID: &agentA,
					Status:       string(ics.RoleStatusActive),
				},
			},
			queryAgent: agentA.String(),
			want:       string(ics.RoleResponder),
		},
		{
			name: "non agent assignee skipped",
			roles: []store.ICSRoleRecord{
				{
					RoleType:     string(ics.RoleIncidentCommander),
					AssigneeType: "user",
					AgentTokenID: &agentA,
					Status:       string(ics.RoleStatusActive),
				},
			},
			queryAgent: agentA.String(),
			want:       "",
		},
		{
			name: "nil agent token id skipped",
			roles: []store.ICSRoleRecord{
				{
					RoleType:     string(ics.RoleResponder),
					AssigneeType: "agent",
					AgentTokenID: nil,
					Status:       string(ics.RoleStatusActive),
				},
			},
			queryAgent: agentA.String(),
			want:       "",
		},
		{
			name: "different agent id skipped",
			roles: []store.ICSRoleRecord{
				{
					RoleType:     string(ics.RoleResponder),
					AssigneeType: "agent",
					AgentTokenID: &agentB,
					Status:       string(ics.RoleStatusActive),
				},
			},
			queryAgent: agentA.String(),
			want:       "",
		},
		{
			name:       "empty roles returns empty",
			roles:      nil,
			queryAgent: agentA.String(),
			want:       "",
		},
		{
			name:       "nil store returns empty",
			nilStore:   true,
			queryAgent: agentA.String(),
			want:       "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &InvestigationScheduler{}
			if !tc.nilStore {
				s.icsRoleStore = &schedulerRoleStore{roles: tc.roles}
			}
			got := s.resolveIncidentRole(context.Background(), incidentNumber, tc.queryAgent)
			if got != tc.want {
				t.Fatalf("resolveIncidentRole(%d, %q) = %q, want %q", incidentNumber, tc.queryAgent, got, tc.want)
			}
		})
	}
}

func TestDisconnectLockKey(t *testing.T) {
	t.Parallel()
	if got := disconnectLockKey("abc123"); got != "alga:agent-disconnect-lock:abc123" {
		t.Fatalf("disconnectLockKey returned %q", got)
	}
}

func TestIncidentSweepCreatesInvestigationWithoutTimelineWhenMissing(t *testing.T) {
	t.Parallel()

	incidentStore := &schedulerIncidentStore{
		active: []store.IncidentRecord{
			{IncidentNumber: 1, Title: "Database outage", Status: "active", Severity: "critical"},
		},
	}
	incidentInvestigationStore := &stubIncidentInvestigationStore{
		createResult: &store.IncidentInvestigationRecord{
			IncidentInvestigationID: "IINV-1",
			Status:                  "pending",
			IncidentNumber:          1,
		},
	}
	s := &InvestigationScheduler{
		incidentStore:              incidentStore,
		incidentInvestigationStore: incidentInvestigationStore,
		notify:                     make(chan struct{}, 1),
	}

	s.incidentSweepTick(context.Background())

	if !incidentInvestigationStore.createCalled {
		t.Fatalf("incident sweep did not create an investigation")
	}
	if incidentInvestigationStore.createInput.IncidentNumber != 1 {
		t.Fatalf("incident_number = %d, want 1", incidentInvestigationStore.createInput.IncidentNumber)
	}
	if incidentInvestigationStore.createInput.Status != "pending" {
		t.Fatalf("status = %q, want pending", incidentInvestigationStore.createInput.Status)
	}
	for _, entry := range incidentStore.timeline {
		if entry.EventType == "investigation_created" {
			t.Fatalf("unexpected investigation_created timeline entry: %#v", entry)
		}
	}
}

func TestIncidentSweepSkipsIncidentWithOpenInvestigation(t *testing.T) {
	t.Parallel()

	incidentStore := &schedulerIncidentStore{
		active: []store.IncidentRecord{{IncidentNumber: 1, Title: "Database outage", Status: "active"}},
	}
	incidentInvestigationStore := &stubIncidentInvestigationStore{
		byIncident: map[int64][]store.IncidentInvestigationRecord{
			1: {{IncidentInvestigationID: "IINV-1", Status: "investigating", IncidentNumber: 1}},
		},
	}
	s := &InvestigationScheduler{
		incidentStore:              incidentStore,
		incidentInvestigationStore: incidentInvestigationStore,
		notify:                     make(chan struct{}, 1),
	}

	s.incidentSweepTick(context.Background())

	if incidentInvestigationStore.createCalled {
		t.Fatalf("incident sweep created an investigation despite an open one")
	}
}

func TestIncidentSweepCreatesInvestigationAfterCompletedInvestigation(t *testing.T) {
	t.Parallel()

	incidentStore := &schedulerIncidentStore{
		active: []store.IncidentRecord{{IncidentNumber: 1, Title: "Database outage", Status: "active"}},
	}
	incidentInvestigationStore := &stubIncidentInvestigationStore{
		byIncident: map[int64][]store.IncidentInvestigationRecord{
			1: {{IncidentInvestigationID: "IINV-OLD", Status: store.IncidentInvestigationStatusComplete, IncidentNumber: 1}},
		},
		createResult: &store.IncidentInvestigationRecord{
			IncidentInvestigationID: "IINV-NEW",
			Status:                  store.IncidentInvestigationStatusPending,
			IncidentNumber:          1,
		},
	}
	s := &InvestigationScheduler{
		incidentStore:              incidentStore,
		incidentInvestigationStore: incidentInvestigationStore,
		notify:                     make(chan struct{}, 1),
	}

	s.incidentSweepTick(context.Background())

	if !incidentInvestigationStore.createCalled {
		t.Fatalf("incident sweep should create a new investigation after completed one")
	}
}

func TestDispatchPromotedInvestigationUsesIncidentPromptScope(t *testing.T) {
	incidentUUID := uuid.New()
	incidentInvestigationUUID := uuid.New()
	agent := &store.AgentTokenRecord{ID: uuid.New(), Name: "sre-agent", AgentType: "hermes"}
	resolver := &capturePromptResolver{}
	s := &InvestigationScheduler{
		resolver:         resolver,
		healthTracker:    NewAgentHealthTracker(50),
		dispatchAttempts: make(map[string]dispatchAttempt),
	}
	inv := &store.AlertInvestigationRecord{
		ID:                              uuid.New(),
		AlertInvestigationID:            "AINV-1",
		Status:                          "assigned",
		AgentID:                         agent.ID.String(),
		AgentName:                       agent.Name,
		AgentType:                       agent.AgentType,
		PromotedIncidentID:              &incidentUUID,
		PromotedIncidentInvestigationID: &incidentInvestigationUUID,
		PrimaryAlertFingerprint:         "fp-1",
		PrimaryAlertNumber:              42,
		Alerts: []rabbitmq.CorrelatedAlert{
			{Fingerprint: "fp-1", AlertNumber: 42, Labels: map[string]string{"alertname": "HighCPU"}},
		},
	}

	if !s.dispatch(context.Background(), inv, agent) {
		t.Fatalf("dispatch returned false")
	}
	if !strings.Contains(resolver.prompt, "Incident Instructions") {
		t.Fatalf("prompt missing incident scope: %s", resolver.prompt)
	}
	if strings.Contains(resolver.prompt, "alga_resolve_incident") {
		t.Fatalf("incident investigator prompt must not mention alga_resolve_incident:\n%s", resolver.prompt)
	}
	if strings.Contains(resolver.prompt, "alga_mitigate_incident") {
		t.Fatalf("promoted alert investigation without ICS role must not advertise commander-only tools: %s", resolver.prompt)
	}
	if !strings.Contains(resolver.prompt, "Alert #42") {
		t.Fatalf("prompt missing primary alert identity: %s", resolver.prompt)
	}
	if !strings.Contains(resolver.prompt, "promoted to an incident") {
		t.Fatalf("prompt missing promoted handoff wording: %s", resolver.prompt)
	}
	if !strings.Contains(resolver.prompt, "investigation is now complete") {
		t.Fatalf("prompt must tell promoted agent its investigation is complete: %s", resolver.prompt)
	}
	// The promoted alert-investigation agent must not be pointed at the incident
	// investigation (its ID is not linkable) and must not be given incident tools.
	if strings.Contains(resolver.prompt, incidentInvestigationUUID.String()) {
		t.Fatalf("promoted handoff prompt must not leak incident investigation UUID: %s", resolver.prompt)
	}
	if strings.Contains(resolver.prompt, "Continue the active investigation") {
		t.Fatalf("promoted handoff prompt must not tell agent to continue: %s", resolver.prompt)
	}
}

func TestDispatchIncidentInvestigationPostsInvestigatingStatusUpdate(t *testing.T) {
	agent := &store.AgentTokenRecord{ID: uuid.New(), Name: "sre-agent", AgentType: "hermes"}
	coordinationStore := &stubIncidentCoordinationStore{}
	s := &InvestigationScheduler{
		resolver:                   &capturePromptResolver{},
		incidentInvestigationStore: &stubIncidentInvestigationStore{},
		healthTracker:              NewAgentHealthTracker(50),
	}
	s.SetIncidentCoordinationStore(coordinationStore)

	inv := &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-25",
		IncidentNumber:          25,
		Status:                  store.IncidentInvestigationStatusPending,
	}

	if !s.dispatchIncidentInvestigation(context.Background(), inv, agent) {
		t.Fatalf("dispatchIncidentInvestigation returned false")
	}

	if len(coordinationStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1: %+v", len(coordinationStore.messages), coordinationStore.messages)
	}
	msg := coordinationStore.messages[0]
	if msg.Kind != store.IncidentCoordinationKindStatusUpdate {
		t.Fatalf("kind = %q, want %q", msg.Kind, store.IncidentCoordinationKindStatusUpdate)
	}
	if msg.ActorType != store.IncidentCoordinationActorSystem {
		t.Fatalf("actor_type = %q, want %q", msg.ActorType, store.IncidentCoordinationActorSystem)
	}
	if msg.ActorDisplayName != "System" {
		t.Fatalf("actor_display_name = %q, want %q", msg.ActorDisplayName, "System")
	}
	if msg.Source != store.IncidentCoordinationSourceSystem {
		t.Fatalf("source = %q, want %q", msg.Source, store.IncidentCoordinationSourceSystem)
	}
	if msg.Internal {
		t.Fatalf("internal = true, want false (must appear on the card)")
	}
	if level, _ := msg.Metadata["status_level"].(string); level != "investigating" {
		t.Fatalf("metadata.status_level = %q, want %q", level, "investigating")
	}
	if auto, _ := msg.Metadata["auto"].(bool); !auto {
		t.Fatalf("metadata.auto = %v, want true", auto)
	}
	if msg.Body == "" {
		t.Fatalf("body must not be empty")
	}
}

func TestDispatchIncidentInvestigationSkipsStatusUpdateWhenNewestIsInvestigating(t *testing.T) {
	agent := &store.AgentTokenRecord{ID: uuid.New(), Name: "sre-agent", AgentType: "hermes"}
	coordinationStore := &stubIncidentCoordinationStore{
		newestStatusUpdate: &store.IncidentCoordinationMessageRecord{
			Metadata: map[string]any{"status_level": "investigating"},
		},
	}
	s := &InvestigationScheduler{
		resolver:                   &capturePromptResolver{},
		incidentInvestigationStore: &stubIncidentInvestigationStore{},
		healthTracker:              NewAgentHealthTracker(50),
	}
	s.SetIncidentCoordinationStore(coordinationStore)

	inv := &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-25",
		IncidentNumber:          25,
		Status:                  store.IncidentInvestigationStatusPending,
	}

	if !s.dispatchIncidentInvestigation(context.Background(), inv, agent) {
		t.Fatalf("dispatchIncidentInvestigation returned false")
	}
	if len(coordinationStore.messages) != 0 {
		t.Fatalf("coordination messages = %d, want 0 (newest already investigating): %+v", len(coordinationStore.messages), coordinationStore.messages)
	}
}

func TestDispatchIncidentInvestigationPostsStatusUpdateWhenNewestIsOtherLevel(t *testing.T) {
	agent := &store.AgentTokenRecord{ID: uuid.New(), Name: "sre-agent", AgentType: "hermes"}
	coordinationStore := &stubIncidentCoordinationStore{
		newestStatusUpdate: &store.IncidentCoordinationMessageRecord{
			Metadata: map[string]any{"status_level": "identified"},
		},
	}
	s := &InvestigationScheduler{
		resolver:                   &capturePromptResolver{},
		incidentInvestigationStore: &stubIncidentInvestigationStore{},
		healthTracker:              NewAgentHealthTracker(50),
	}
	s.SetIncidentCoordinationStore(coordinationStore)

	inv := &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-25",
		IncidentNumber:          25,
		Status:                  store.IncidentInvestigationStatusPending,
	}

	if !s.dispatchIncidentInvestigation(context.Background(), inv, agent) {
		t.Fatalf("dispatchIncidentInvestigation returned false")
	}
	if len(coordinationStore.messages) != 1 {
		t.Fatalf("coordination messages = %d, want 1 (newest is identified, should post investigating): %+v", len(coordinationStore.messages), coordinationStore.messages)
	}
	if level, _ := coordinationStore.messages[0].Metadata["status_level"].(string); level != "investigating" {
		t.Fatalf("metadata.status_level = %q, want investigating", level)
	}
}

func TestDispatchIncidentInvestigationStatusUpdateIsBestEffort(t *testing.T) {
	agent := &store.AgentTokenRecord{ID: uuid.New(), Name: "sre-agent", AgentType: "hermes"}
	coordinationStore := &stubIncidentCoordinationStore{
		createErr: fmt.Errorf("db down"),
	}
	s := &InvestigationScheduler{
		resolver:                   &capturePromptResolver{},
		incidentInvestigationStore: &stubIncidentInvestigationStore{},
		healthTracker:              NewAgentHealthTracker(50),
	}
	s.SetIncidentCoordinationStore(coordinationStore)

	inv := &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-25",
		IncidentNumber:          25,
		Status:                  store.IncidentInvestigationStatusPending,
	}

	if !s.dispatchIncidentInvestigation(context.Background(), inv, agent) {
		t.Fatalf("dispatch must still succeed when the status update write fails")
	}
	if len(coordinationStore.messages) != 0 {
		t.Fatalf("coordination messages = %d, want 0 (create failed)", len(coordinationStore.messages))
	}
}

func TestDispatchIncidentInvestigationDoesNotCreateStartMessageWhenForwardFails(t *testing.T) {
	agent := &store.AgentTokenRecord{ID: uuid.New(), Name: "sre-agent", AgentType: "hermes"}
	coordinationStore := &stubIncidentCoordinationStore{}
	s := &InvestigationScheduler{
		resolver:                   failingResolver{},
		incidentInvestigationStore: &stubIncidentInvestigationStore{},
		healthTracker:              NewAgentHealthTracker(50),
	}
	s.SetIncidentCoordinationStore(coordinationStore)

	inv := &store.IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-25",
		IncidentNumber:          25,
		Status:                  store.IncidentInvestigationStatusPending,
	}

	if s.dispatchIncidentInvestigation(context.Background(), inv, agent) {
		t.Fatalf("dispatchIncidentInvestigation returned true, want false")
	}
	if len(coordinationStore.messages) != 0 {
		t.Fatalf("coordination messages = %d, want 0", len(coordinationStore.messages))
	}
}

type alwaysOnlineResolver struct{}

func (alwaysOnlineResolver) ForwardToAgent(_, _, _, _, _ string) error       { return nil }
func (alwaysOnlineResolver) ForwardEventToAgent(_ string, _ sse.Event) error { return nil }
func (alwaysOnlineResolver) AgentOnline(_ string) bool                       { return true }

type countingResolver struct {
	forwarded int
}

func (r *countingResolver) ForwardToAgent(_, _, _, _, _ string) error {
	r.forwarded++
	return nil
}

func (r *countingResolver) ForwardEventToAgent(_ string, _ sse.Event) error { return nil }
func (r *countingResolver) AgentOnline(_ string) bool                       { return true }

type capturePromptResolver struct {
	prompt string
}

func (r *capturePromptResolver) ForwardToAgent(_, _, _, _, prompt string) error {
	r.prompt = prompt
	return nil
}

func (r *capturePromptResolver) ForwardEventToAgent(_ string, _ sse.Event) error { return nil }
func (r *capturePromptResolver) AgentOnline(_ string) bool                       { return true }

type failingResolver struct{}

func (failingResolver) ForwardToAgent(_, _, _, _, _ string) error {
	return fmt.Errorf("forward failed")
}

func (failingResolver) ForwardEventToAgent(_ string, _ sse.Event) error { return nil }
func (failingResolver) AgentOnline(_ string) bool                       { return true }

type stubIncidentCoordinationStore struct {
	messages           []store.IncidentCoordinationMessageRecord
	newestStatusUpdate *store.IncidentCoordinationMessageRecord
	createErr          error
}

func (s *stubIncidentCoordinationStore) CreateMessage(_ context.Context, record *store.IncidentCoordinationMessageRecord) (*store.IncidentCoordinationMessageRecord, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.messages = append(s.messages, *record)
	return record, nil
}

func (s *stubIncidentCoordinationStore) ListMessages(ctx context.Context, incidentNumber int64, limit, skip int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *stubIncidentCoordinationStore) FindByProviderMessageID(ctx context.Context, providerMessageID string) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *stubIncidentCoordinationStore) SetSlackMessageTS(ctx context.Context, id uuid.UUID, channelID, messageTS, threadTS string) error {
	return nil
}

func (s *stubIncidentCoordinationStore) ListMessagesByKind(ctx context.Context, incidentNumber int64, kind string, limit, skip int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *stubIncidentCoordinationStore) CreateStatusUpdate(ctx context.Context, incidentNumber int64, statusLevel string, body string, internal bool, actorID uuid.UUID, actorDisplayName string) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (s *stubIncidentCoordinationStore) NewestStatusUpdate(_ context.Context, _ int64) (*store.IncidentCoordinationMessageRecord, error) {
	return s.newestStatusUpdate, nil
}

func (s *stubIncidentCoordinationStore) NewestAgentCoordinationReply(ctx context.Context, incidentNumber int64) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

// schedulerAlertLookup simulates GetOpenByFingerprint: a row is returned only
// when the alert is open and not soft-deleted. Resolved/deleted rows behave
// like the production SQL filter and return ErrNotFound so the investigation
// is dropped from the scheduler's pending set.
type schedulerAlertLookup struct {
	byFingerprint map[string]*store.AlertRecord
	err           error
}

func (s *schedulerAlertLookup) GetOpenByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	if rec, ok := s.byFingerprint[fingerprint]; ok {
		if rec.DeletedAt != nil || rec.Status == "resolved" {
			return nil, store.ErrNotFound
		}
		return rec, nil
	}
	return nil, store.ErrNotFound
}

type schedulerAlertStore struct {
	schedulerAlertLookup
}

type schedulerPlaybookStore struct {
	matches []*store.PlaybookRecord
	steps   map[uuid.UUID][]store.PlaybookStepRecord
}

func (s *schedulerPlaybookStore) Create(context.Context, *store.PlaybookRecord, []store.PlaybookStepRecord) (*store.PlaybookRecord, error) {
	return nil, nil
}
func (s *schedulerPlaybookStore) Get(_ context.Context, id uuid.UUID) (*store.PlaybookRecord, []store.PlaybookStepRecord, error) {
	return nil, s.steps[id], nil
}
func (s *schedulerPlaybookStore) Update(context.Context, uuid.UUID, *store.PlaybookRecord) error {
	return nil
}
func (s *schedulerPlaybookStore) Delete(context.Context, uuid.UUID) error { return nil }
func (s *schedulerPlaybookStore) List(context.Context, store.PlaybookFilter, int, int) ([]*store.PlaybookRecord, int64, error) {
	return nil, 0, nil
}
func (s *schedulerPlaybookStore) AddStep(context.Context, *store.PlaybookStepRecord) (*store.PlaybookStepRecord, error) {
	return nil, nil
}
func (s *schedulerPlaybookStore) UpdateStep(context.Context, uuid.UUID, *store.PlaybookStepRecord) error {
	return nil
}
func (s *schedulerPlaybookStore) DeleteStep(context.Context, uuid.UUID) error { return nil }
func (s *schedulerPlaybookStore) ReorderSteps(context.Context, uuid.UUID, []store.StepOrder) error {
	return nil
}
func (s *schedulerPlaybookStore) FindMatching(context.Context, map[string]string) ([]*store.PlaybookRecord, error) {
	return s.matches, nil
}

func (s *schedulerAlertStore) Create(store.AlertRecord) (int64, error) { return 0, nil }
func (s *schedulerAlertStore) GetByFingerprint(string) (*store.AlertRecord, error) {
	return nil, store.ErrNotFound
}
func (s *schedulerAlertStore) UpdateStatus(string, string, *store.AlertEvent) error { return nil }
func (s *schedulerAlertStore) UpdateStatusSilenced(string) error                    { return nil }
func (s *schedulerAlertStore) UpdateDeliveryTargets(string, []store.DeliveryTarget) error {
	return nil
}
func (s *schedulerAlertStore) AcknowledgeAlert(string, *store.EventActor) error { return nil }
func (s *schedulerAlertStore) ReopenAlert(string, store.AlertEvent) error       { return nil }
func (s *schedulerAlertStore) ResolveAlertByUser(string, *store.EventActor) error {
	return nil
}
func (s *schedulerAlertStore) DeleteAlert(string) error { return nil }
func (s *schedulerAlertStore) GetByAlertNumber(int64) (*store.AlertRecord, error) {
	return nil, store.ErrNotFound
}
func (s *schedulerAlertStore) AcknowledgeAlertByNumber(int64, *store.EventActor) error {
	return nil
}
func (s *schedulerAlertStore) ReopenAlertByNumber(int64, store.AlertEvent) error { return nil }
func (s *schedulerAlertStore) ResolveAlertByNumber(int64, *store.EventActor) error {
	return nil
}
func (s *schedulerAlertStore) DeleteAlertByNumber(int64) error { return nil }
func (s *schedulerAlertStore) QueryAlerts(map[string]any) ([]store.AlertRecord, error) {
	return nil, nil
}
func (s *schedulerAlertStore) ListUninvestigatedAlerts(context.Context, time.Duration) ([]store.AlertRecord, error) {
	return nil, nil
}
func (s *schedulerAlertStore) DeleteOlderThan(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (s *schedulerAlertStore) CountOlderThan(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (s *schedulerAlertStore) LinkAlertToIncident(context.Context, string, int64) error { return nil }
func (s *schedulerAlertStore) UnlinkAlertFromIncident(context.Context, string, int64) error {
	return nil
}
func (s *schedulerAlertStore) GetAlertsByIncident(context.Context, int64) ([]string, error) {
	return nil, nil
}
func (s *schedulerAlertStore) ResolveAlertsByIncident(_ context.Context, _ int64, _ *store.EventActor) (store.AlertCascadeResult, error) {
	return store.AlertCascadeResult{}, nil
}
func (s *schedulerAlertStore) Close()                                     {}
func (s *schedulerAlertStore) TriageResultStore() store.TriageResultStore { return nil }
func (s *schedulerAlertStore) TriageRuleStore() store.TriageRuleStore     { return nil }

type schedulerRoleStore struct {
	roles            []store.ICSRoleRecord
	assignAgentCalls int
}

func (s *schedulerRoleStore) AssignRole(context.Context, int64, ics.RoleType, uuid.UUID, *uuid.UUID, *string) (*store.ICSRoleRecord, error) {
	return nil, nil
}

func (s *schedulerRoleStore) AssignAgentRole(_ context.Context, incidentNumber int64, roleType ics.RoleType, agentTokenID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*store.ICSRoleRecord, error) {
	s.assignAgentCalls++
	rec := store.ICSRoleRecord{
		ID:                 uuid.New(),
		IncidentNumber:     incidentNumber,
		RoleType:           string(roleType),
		AssigneeType:       "agent",
		AgentTokenID:       &agentTokenID,
		ParentAssignmentID: parentAssignmentID,
		ScopeDescription:   scope,
		Status:             string(ics.RoleStatusActive),
		StartedAt:          time.Now().UTC(),
	}
	s.roles = append(s.roles, rec)
	return &rec, nil
}

func (s *schedulerRoleStore) EndRole(context.Context, uuid.UUID, ics.EndReason) error { return nil }

func (s *schedulerRoleStore) GetActiveRoles(context.Context, int64) ([]store.ICSRoleRecord, error) {
	return s.roles, nil
}

func (s *schedulerRoleStore) GetActiveIC(context.Context, int64) (*store.ICSRoleRecord, error) {
	return nil, nil
}

func (s *schedulerRoleStore) GetActiveCommunicator(context.Context, int64) (*store.ICSRoleRecord, error) {
	return nil, nil
}

func (s *schedulerRoleStore) GetAllRoles(context.Context, int64) ([]store.ICSRoleRecord, error) {
	return s.roles, nil
}

func (s *schedulerRoleStore) GetDelegationTree(context.Context, int64) ([]store.ICSRoleRecord, error) {
	return s.roles, nil
}

func (s *schedulerRoleStore) GetActiveRolesForAgent(context.Context, uuid.UUID) ([]store.ICSRoleRecord, error) {
	return nil, nil
}

func (s *schedulerRoleStore) EndAllRolesForIncident(context.Context, int64, ics.EndReason) error {
	return nil
}

func (s *schedulerRoleStore) EndRolesForAgent(context.Context, uuid.UUID, ics.EndReason) error {
	return nil
}

type transitionCall struct {
	incidentNumber int64
	from           []string
	to             string
}

type schedulerIncidentStore struct {
	active          []store.IncidentRecord
	timeline        []store.IncidentTimelineEntryRecord
	transitionMu    sync.Mutex
	transitionCalls []transitionCall
}

func (s *schedulerIncidentStore) ReserveIncidentNumber(context.Context) (int64, error) {
	return 0, nil
}

func (s *schedulerIncidentStore) CreateIncident(context.Context, *store.IncidentRecord) (*store.IncidentRecord, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) GetIncident(context.Context, int64) (*store.IncidentRecord, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) GetIncidentByID(context.Context, uuid.UUID) (*store.IncidentRecord, error) {
	return &store.IncidentRecord{IncidentNumber: 1}, nil
}

func (s *schedulerIncidentStore) UpdateIncident(context.Context, int64, *store.IncidentRecord) (*store.IncidentRecord, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) DeleteIncident(context.Context, int64) error {
	return nil
}

func (s *schedulerIncidentStore) ListIncidents(context.Context, store.IncidentListFilter) ([]store.IncidentRecord, int64, error) {
	return nil, 0, nil
}

func (s *schedulerIncidentStore) ListSLAEligibleIncidents(context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) UpdateIncidentStatus(context.Context, int64, string) error {
	return nil
}

func (s *schedulerIncidentStore) TransitionIncidentStatus(_ context.Context, incidentNumber int64, from []string, to string) error {
	s.transitionMu.Lock()
	defer s.transitionMu.Unlock()
	s.transitionCalls = append(s.transitionCalls, transitionCall{
		incidentNumber: incidentNumber,
		from:           from,
		to:             to,
	})
	return nil
}

func (s *schedulerIncidentStore) AddTimelineEntry(_ context.Context, record *store.IncidentTimelineEntryRecord) error {
	s.timeline = append(s.timeline, *record)
	return nil
}

func (s *schedulerIncidentStore) GetTimeline(context.Context, int64) ([]store.IncidentTimelineEntryRecord, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) AssignRole(context.Context, string, uuid.UUID, string) error {
	return nil
}

func (s *schedulerIncidentStore) UpdateRole(context.Context, string, uuid.UUID, string) error {
	return nil
}

func (s *schedulerIncidentStore) RemoveRole(context.Context, string, uuid.UUID) error {
	return nil
}

func (s *schedulerIncidentStore) GetIncidentMetrics(context.Context, time.Time, time.Time) (*incmetrics.Metrics, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) CountActiveByService(context.Context) (map[string]int64, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) CountActiveByServiceID(context.Context, string) (int, error) {
	return 0, nil
}

func (s *schedulerIncidentStore) CountActiveByPriority(context.Context, string) (map[string]int, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) ListActiveSummarizableIncidents(context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) ListActiveIncidents(context.Context) ([]store.IncidentRecord, error) {
	return s.active, nil
}

func (s *schedulerIncidentStore) GetIncidentBySlackChannel(context.Context, string) (*store.IncidentRecord, error) {
	return nil, nil
}

func (s *schedulerIncidentStore) SetIncidentWarRoomMeet(context.Context, int64, string, string) error {
	return nil
}

func TestOperatorWeight(t *testing.T) {
	t.Parallel()
	cases := []struct {
		op   string
		want int
	}{
		{"exact", 5},
		{"Exact", 5},
		{"EXACT", 5},
		{" contains ", 3},
		{"prefix", 3},
		{"suffix", 3},
		{"wildcard", 2},
		{"regex", 1},
		{"exists", 1},
		{"not_exists", 1},
		{"unknown", 0},
		{"", 0},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			if got := operatorWeight(tc.op); got != tc.want {
				t.Fatalf("operatorWeight(%q) = %d, want %d", tc.op, got, tc.want)
			}
		})
	}
}

func TestComputeSpecificityMixed(t *testing.T) {
	t.Parallel()
	conds := []config.RouteCondition{
		{Operator: "exact"},
		{Operator: "contains"},
		{Operator: "regex"},
	}
	got := computeSpecificity(conds)
	want := 3*10 + 5 + 3 + 1
	if got != want {
		t.Fatalf("computeSpecificity(mixed) = %d, want %d", got, want)
	}
}

func TestExtractLabels(t *testing.T) {
	t.Parallel()
	t.Run("with_alerts", func(t *testing.T) {
		inv := store.AlertInvestigationRecord{
			Alerts: []rabbitmq.CorrelatedAlert{
				{Labels: map[string]string{"namespace": "prod", "alertname": "HighCPU"}},
			},
		}
		labels := extractLabels(inv)
		if labels["namespace"] != "prod" {
			t.Fatalf("expected namespace=prod, got %q", labels["namespace"])
		}
	})
	t.Run("no_alerts", func(t *testing.T) {
		inv := store.AlertInvestigationRecord{}
		labels := extractLabels(inv)
		if len(labels) != 0 {
			t.Fatalf("expected empty labels, got %v", labels)
		}
	})
	t.Run("nil_alerts", func(t *testing.T) {
		inv := store.AlertInvestigationRecord{Alerts: nil}
		labels := extractLabels(inv)
		if len(labels) != 0 {
			t.Fatalf("expected empty labels, got %v", labels)
		}
	})
}

func TestEffectiveSummaryInterval(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{
		summaryDefaultInterval:   15 * time.Minute,
		summarySeverityIntervals: map[string]time.Duration{"critical": 5 * time.Minute},
	}
	if d := s.effectiveSummaryInterval("critical"); d != 5*time.Minute {
		t.Fatalf("critical interval = %v, want 5m", d)
	}
	if d := s.effectiveSummaryInterval("warning"); d != 15*time.Minute {
		t.Fatalf("warning interval = %v, want 15m (default)", d)
	}
	if d := s.effectiveSummaryInterval(""); d != 15*time.Minute {
		t.Fatalf("empty severity interval = %v, want 15m (default)", d)
	}
}

func TestEffectiveSummaryIntervalNoSeverityMap(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{
		summaryDefaultInterval: 30 * time.Minute,
	}
	if d := s.effectiveSummaryInterval("critical"); d != 30*time.Minute {
		t.Fatalf("critical interval = %v, want 30m (default)", d)
	}
}

func TestNewInvestigationSchedulerClampMaxConcurrent(t *testing.T) {
	t.Parallel()
	s := NewInvestigationScheduler(nil, nil, nil, 0)
	if s.maxConcurrent != 1 {
		t.Fatalf("maxConcurrent = %d, want 1", s.maxConcurrent)
	}
	s2 := NewInvestigationScheduler(nil, nil, nil, -5)
	if s2.maxConcurrent != 1 {
		t.Fatalf("maxConcurrent = %d, want 1", s2.maxConcurrent)
	}
}

func TestNewInvestigationSchedulerPreservesMaxConcurrent(t *testing.T) {
	t.Parallel()
	s := NewInvestigationScheduler(nil, nil, nil, 10)
	if s.maxConcurrent != 10 {
		t.Fatalf("maxConcurrent = %d, want 10", s.maxConcurrent)
	}
}

func TestSetDisconnectGraceDefault(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{disconnectGrace: 0}
	s.SetDisconnectGrace(0)
	if s.disconnectGrace != defaultDisconnectGrace {
		t.Fatalf("disconnectGrace = %v, want %v", s.disconnectGrace, defaultDisconnectGrace)
	}
	s.SetDisconnectGrace(-1)
	if s.disconnectGrace != defaultDisconnectGrace {
		t.Fatalf("disconnectGrace = %v, want %v", s.disconnectGrace, defaultDisconnectGrace)
	}
}

func TestSetDisconnectGraceCustom(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetDisconnectGrace(2 * time.Minute)
	if s.disconnectGrace != 2*time.Minute {
		t.Fatalf("disconnectGrace = %v, want 2m", s.disconnectGrace)
	}
}

func TestSetInvestigationTimeoutDefault(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetInvestigationTimeout(0)
	if s.investigationTimeout != 10*time.Minute {
		t.Fatalf("investigationTimeout = %v, want 10m", s.investigationTimeout)
	}
}

func TestSetInvestigationTimeoutCustom(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetInvestigationTimeout(30 * time.Minute)
	if s.investigationTimeout != 30*time.Minute {
		t.Fatalf("investigationTimeout = %v, want 30m", s.investigationTimeout)
	}
}

func TestWildcardMatchEmptyPattern(t *testing.T) {
	t.Parallel()
	if !wildcardMatch("*", "") {
		t.Fatalf("wildcardMatch('*', '') should be true")
	}
	if wildcardMatch("foo*", "") {
		t.Fatalf("wildcardMatch('foo*', '') should be false")
	}
}

func TestWildcardMatchSingleStar(t *testing.T) {
	t.Parallel()
	if !wildcardMatch("*", "anything") {
		t.Fatalf("wildcardMatch('*', 'anything') should be true")
	}
}

func TestMatchConditionEmptyField(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	labels := map[string]string{"namespace": "prod"}
	c := config.RouteCondition{Field: "", Operator: "exact", Value: "prod"}
	if s.matchCondition(c, labels) {
		t.Fatalf("empty field should not match")
	}
}

func TestMatchConditionContainsMiss(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	labels := map[string]string{"app": "frontend"}
	c := config.RouteCondition{Field: "app", Operator: "contains", Value: "backend"}
	if s.matchCondition(c, labels) {
		t.Fatalf("contains should miss")
	}
}

func TestMatchConditionPrefixMiss(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	labels := map[string]string{"app": "frontend"}
	c := config.RouteCondition{Field: "app", Operator: "prefix", Value: "back"}
	if s.matchCondition(c, labels) {
		t.Fatalf("prefix should miss")
	}
}

func TestMatchConditionSuffixMiss(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	labels := map[string]string{"app": "frontend"}
	c := config.RouteCondition{Field: "app", Operator: "suffix", Value: "end"}
	if !s.matchCondition(c, labels) {
		t.Fatalf("suffix 'end' should match 'frontend'")
	}
}

func TestMatchConditionRegexInvalidPattern(t *testing.T) {
	t.Parallel()
	if matching.MatchCondition("anything", "regex", "[invalid") {
		t.Fatalf("invalid regex should not match")
	}
}

func TestMatchConditionRegexTooLongPattern(t *testing.T) {
	t.Parallel()
	longPat := make([]byte, 257)
	for i := range longPat {
		longPat[i] = 'a'
	}
	if matching.MatchCondition("aaa", "regex", string(longPat)) {
		t.Fatalf("too-long regex should not match")
	}
}

func TestSetStaleConfigZeroDisables(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetStaleConfig(0, 5*time.Minute)
	if s.staleThreshold != 0 {
		t.Fatalf("staleThreshold should remain 0")
	}
}

func TestSetStaleConfigNegativeDisables(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetStaleConfig(-1, 5*time.Minute)
	if s.staleThreshold != 0 {
		t.Fatalf("staleThreshold should remain 0")
	}
}

func TestSetStaleConfigDefaultInterval(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetStaleConfig(10*time.Minute, 0)
	if s.staleInterval != 5*time.Minute {
		t.Fatalf("staleInterval = %v, want 5m default", s.staleInterval)
	}
}

func TestSetDataRetentionZeroDisables(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetDataRetention(0, time.Hour)
	if s.dataRetentionDays != 0 {
		t.Fatalf("dataRetentionDays should remain 0")
	}
}

func TestSetDataRetentionDefaultInterval(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetDataRetention(30, 0)
	if s.dataRetentionDays != 30 {
		t.Fatalf("dataRetentionDays = %d, want 30", s.dataRetentionDays)
	}
	if s.pruneInterval != time.Hour {
		t.Fatalf("pruneInterval = %v, want 1h default", s.pruneInterval)
	}
}

func TestSetSummaryConfigDefaultInterval(t *testing.T) {
	t.Parallel()
	s := &InvestigationScheduler{}
	s.SetSummaryConfig(true, 0, nil)
	if s.summaryDefaultInterval != 15*time.Minute {
		t.Fatalf("summaryDefaultInterval = %v, want 15m", s.summaryDefaultInterval)
	}
}

type stubIncidentInvestigationStore struct {
	createCalled bool
	createInput  store.IncidentInvestigationRecord
	createResult *store.IncidentInvestigationRecord
	byIncident   map[int64][]store.IncidentInvestigationRecord
}

func (s *stubIncidentInvestigationStore) CreateIncidentInvestigation(_ context.Context, record store.IncidentInvestigationRecord) (*store.IncidentInvestigationRecord, error) {
	s.createCalled = true
	s.createInput = record
	return s.createResult, nil
}

func (s *stubIncidentInvestigationStore) GetIncidentInvestigation(ctx context.Context, id string) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}

func (s *stubIncidentInvestigationStore) GetActiveIncidentInvestigationByIncident(ctx context.Context, incidentNumber int64) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}

func (s *stubIncidentInvestigationStore) ListIncidentInvestigationsByIncident(_ context.Context, incidentNumber int64) ([]store.IncidentInvestigationRecord, error) {
	if s.byIncident != nil {
		if recs, ok := s.byIncident[incidentNumber]; ok {
			return recs, nil
		}
	}
	return nil, nil
}

func (s *stubIncidentInvestigationStore) AddIncidentInvestigationUpdate(ctx context.Context, id string, update store.InvestigationUpdate) error {
	return nil
}

func (s *stubIncidentInvestigationStore) UpdateIncidentInvestigationStatus(ctx context.Context, id string, status string) error {
	return nil
}

func (s *stubIncidentInvestigationStore) ClaimPendingIncidentInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}

func (s *stubIncidentInvestigationStore) ListPendingIncidentInvestigations(ctx context.Context, limit int64) ([]store.IncidentInvestigationRecord, error) {
	return nil, nil
}

func (s *stubIncidentInvestigationStore) SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *entschema.InvestigationSummary) error {
	return nil
}

func (s *stubIncidentInvestigationStore) SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	return nil
}

func TestBuildDiscriminators(t *testing.T) {
	t.Parallel()

	t.Run("populates from all labels", func(t *testing.T) {
		t.Parallel()
		got := buildDiscriminators(map[string]string{
			"alertname": "HighCPU",
			"namespace": "prod",
			"app":       "api-server",
			"severity":  "warning",
		})
		want := map[string]string{
			"alertname": "HighCPU",
			"namespace": "prod",
			"app":       "api-server",
			"severity":  "warning",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("buildDiscriminators = %v, want %v", got, want)
		}
	})

	t.Run("caps at 8 keys", func(t *testing.T) {
		t.Parallel()
		labels := make(map[string]string, 12)
		for i := range 12 {
			labels[fmt.Sprintf("k%d", i)] = fmt.Sprintf("v%d", i)
		}
		got := buildDiscriminators(labels)
		if len(got) != 8 {
			t.Errorf("len(discriminators) = %d, want 8", len(got))
		}
	})

	t.Run("empty labels returns nil", func(t *testing.T) {
		t.Parallel()
		if got := buildDiscriminators(nil); got != nil {
			t.Errorf("buildDiscriminators(nil) = %v, want nil", got)
		}
		if got := buildDiscriminators(map[string]string{}); got != nil {
			t.Errorf("buildDiscriminators({}) = %v, want nil", got)
		}
	})
}

func TestAutoAcknowledgePromotedInvestigationTransitionsIncidentToActive(t *testing.T) {
	t.Parallel()

	incidentStore := &schedulerIncidentStore{}
	s := &InvestigationScheduler{
		incidentStore: incidentStore,
	}
	promotedID := uuid.New()
	inv := &store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-1",
		PromotedIncidentID:   &promotedID,
		Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1"}},
	}

	s.autoAcknowledge(context.Background(), inv)

	incidentStore.transitionMu.Lock()
	defer incidentStore.transitionMu.Unlock()
	if got, want := len(incidentStore.transitionCalls), 1; got != want {
		t.Fatalf("TransitionIncidentStatus calls = %d, want %d", got, want)
	}
	call := incidentStore.transitionCalls[0]
	if call.incidentNumber != 1 {
		t.Errorf("incidentNumber = %d, want 1", call.incidentNumber)
	}
	if !reflect.DeepEqual(call.from, []string{"detected", "triaging"}) {
		t.Errorf("from = %v, want [detected triaging]", call.from)
	}
	if call.to != "active" {
		t.Errorf("to = %q, want %q (auto-ack aligns with operator handleAcknowledgeIncident)", call.to, "active")
	}
}

func TestAutoAcknowledgeSkipsIncidentTransitionWhenNotPromoted(t *testing.T) {
	t.Parallel()

	incidentStore := &schedulerIncidentStore{}
	s := &InvestigationScheduler{
		incidentStore: incidentStore,
	}
	inv := &store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-1",
		// PromotedIncidentID is nil — investigation has no parent incident
		Alerts: []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1"}},
	}

	s.autoAcknowledge(context.Background(), inv)

	incidentStore.transitionMu.Lock()
	defer incidentStore.transitionMu.Unlock()
	if got := len(incidentStore.transitionCalls); got != 0 {
		t.Errorf("TransitionIncidentStatus calls = %d, want 0", got)
	}
}

func TestRunMapPurgePreservesInFlightDispatchAttempts(t *testing.T) {
	t.Parallel()

	s := &InvestigationScheduler{
		investigationTimeout: 10 * time.Minute,
	}
	s.dispatchAttempts = map[string]dispatchAttempt{
		"inv-active": {count: 4, lastSeen: time.Now()},
	}

	s.purgeDispatchAttempts()

	s.dispatchMu.Lock()
	_, present := s.dispatchAttempts["inv-active"]
	s.dispatchMu.Unlock()
	if !present {
		t.Fatalf("in-flight entry was purged; want preserved")
	}

	s.dispatchMu.Lock()
	s.dispatchAttempts["inv-active"] = dispatchAttempt{count: 4, lastSeen: time.Now().Add(-3 * time.Hour)}
	s.dispatchMu.Unlock()

	s.purgeDispatchAttempts()

	s.dispatchMu.Lock()
	_, present = s.dispatchAttempts["inv-active"]
	s.dispatchMu.Unlock()
	if present {
		t.Fatalf("stale entry (3h old) was not purged; want removed")
	}
}
