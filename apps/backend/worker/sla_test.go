package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/ics"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
)

func TestSLAWorkerQueue(t *testing.T) {
	t.Parallel()
	w := &SLAWorker{}
	if got := w.Queue(); got != rabbitmq.QueueSLASweep {
		t.Fatalf("Queue() = %q, want %q", got, rabbitmq.QueueSLASweep)
	}
}

func TestSLAWorkerPrefetchCount(t *testing.T) {
	t.Parallel()
	w := &SLAWorker{}
	if got := w.PrefetchCount(); got != 1 {
		t.Fatalf("PrefetchCount() = %d, want 1", got)
	}
}

func TestPriorityToSLATargets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		priority    string
		wantRespond time.Duration
		wantResolve time.Duration
	}{
		{"P1", 15 * time.Minute, 4 * time.Hour},
		{"P2", 30 * time.Minute, 8 * time.Hour},
		{"P3", 2 * time.Hour, 24 * time.Hour},
		{"P4", 8 * time.Hour, 72 * time.Hour},
		{"P5", 24 * time.Hour, 168 * time.Hour},
		{"unknown", 2 * time.Hour, 24 * time.Hour},
	}
	for _, tc := range cases {
		respond, resolve := PriorityToSLATargets(tc.priority)
		if respond != tc.wantRespond {
			t.Errorf("PriorityToSLATargets(%q) respond = %v, want %v", tc.priority, respond, tc.wantRespond)
		}
		if resolve != tc.wantResolve {
			t.Errorf("PriorityToSLATargets(%q) resolve = %v, want %v", tc.priority, resolve, tc.wantResolve)
		}
	}
}

func TestPriorityToSLATargets_HigherPriorityFasterResponse(t *testing.T) {
	t.Parallel()
	p1Respond, _ := PriorityToSLATargets("P1")
	p3Respond, _ := PriorityToSLATargets("P3")
	if p1Respond >= p3Respond {
		t.Errorf("P1 respond %v should be < P3 respond %v", p1Respond, p3Respond)
	}
}

func TestPriorityToSLATargets_HigherPriorityFasterResolve(t *testing.T) {
	t.Parallel()
	_, p1Resolve := PriorityToSLATargets("P1")
	_, p3Resolve := PriorityToSLATargets("P3")
	if p1Resolve >= p3Resolve {
		t.Errorf("P1 resolve %v should be < P3 resolve %v", p1Resolve, p3Resolve)
	}
}

func TestNewSLAWorker(t *testing.T) {
	t.Parallel()
	w := NewSLAWorker(nil, nil, nil)
	if w == nil {
		t.Fatal("NewSLAWorker returned nil")
	}
	if w.incidentStore != nil {
		t.Error("expected nil incidentStore")
	}
	if w.ssePublisher != nil {
		t.Error("expected nil ssePublisher")
	}
	if w.vkClient != nil {
		t.Error("expected nil vkClient")
	}
}

func newFakeIncidentStore() *schedulerIncidentStore {
	return &schedulerIncidentStore{}
}

type fakeCoordStore struct {
	statuses  []store.IncidentCoordinationMessageRecord
	agentRepl []store.IncidentCoordinationMessageRecord
}

func (f *fakeCoordStore) CreateMessage(context.Context, *store.IncidentCoordinationMessageRecord) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordStore) ListMessages(context.Context, int64, int, int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordStore) FindByProviderMessageID(context.Context, string) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordStore) SetSlackMessageTS(context.Context, uuid.UUID, string, string, string) error {
	return nil
}

func (f *fakeCoordStore) ListMessagesByKind(context.Context, int64, string, int, int) ([]store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordStore) CreateStatusUpdate(context.Context, int64, string, string, bool, uuid.UUID, string) (*store.IncidentCoordinationMessageRecord, error) {
	return nil, nil
}

func (f *fakeCoordStore) NewestStatusUpdate(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	if len(f.statuses) == 0 {
		return nil, nil
	}
	last := f.statuses[len(f.statuses)-1]
	return &last, nil
}

func (f *fakeCoordStore) NewestAgentCoordinationReply(context.Context, int64) (*store.IncidentCoordinationMessageRecord, error) {
	if len(f.agentRepl) == 0 {
		return nil, nil
	}
	last := f.agentRepl[len(f.agentRepl)-1]
	return &last, nil
}

type fakeICSStore struct {
	ic *store.ICSRoleRecord
}

func (f *fakeICSStore) AssignRole(context.Context, int64, ics.RoleType, uuid.UUID, *uuid.UUID, *string) (*store.ICSRoleRecord, error) {
	return nil, nil
}

func (f *fakeICSStore) AssignAgentRole(context.Context, int64, ics.RoleType, uuid.UUID, *uuid.UUID, *string) (*store.ICSRoleRecord, error) {
	return nil, nil
}

func (f *fakeICSStore) EndRole(context.Context, uuid.UUID, ics.EndReason) error { return nil }

func (f *fakeICSStore) GetActiveRoles(context.Context, int64) ([]store.ICSRoleRecord, error) {
	return nil, nil
}

func (f *fakeICSStore) GetActiveIC(context.Context, int64) (*store.ICSRoleRecord, error) {
	return f.ic, nil
}

func (f *fakeICSStore) GetAllRoles(context.Context, int64) ([]store.ICSRoleRecord, error) {
	return nil, nil
}

func (f *fakeICSStore) GetDelegationTree(context.Context, int64) ([]store.ICSRoleRecord, error) {
	return nil, nil
}

func (f *fakeICSStore) GetActiveRolesForAgent(context.Context, uuid.UUID) ([]store.ICSRoleRecord, error) {
	return nil, nil
}

func (f *fakeICSStore) EndAllRolesForIncident(context.Context, int64, ics.EndReason) error {
	return nil
}

func (f *fakeICSStore) EndRolesForAgent(context.Context, uuid.UUID, ics.EndReason) error {
	return nil
}

type captureForwarder struct {
	got []string
}

func (c *captureForwarder) ForwardToAgent(string, string, string, string, string) error { return nil }

func (c *captureForwarder) ForwardEventToAgent(agentIDHex string, _ sse.Event) error {
	c.got = append(c.got, agentIDHex)
	return nil
}

func (c *captureForwarder) AgentOnline(string) bool { return true }

func TestSweepCommsStalenessNudgesCommander(t *testing.T) {
	incidentNumber := int64(1)
	commanderID := uuid.New()
	incStore := newFakeIncidentStore()
	coord := &fakeCoordStore{agentRepl: []store.IncidentCoordinationMessageRecord{{
		IncidentNumber: incidentNumber,
		Kind:           store.IncidentCoordinationKindAgentReply,
		ActorType:      store.IncidentCoordinationActorAgent,
		CreatedAt:      time.Now().Add(-30 * time.Minute),
	}}}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetICSRoleStore(&fakeICSStore{ic: &store.ICSRoleRecord{RoleType: string(ics.RoleIncidentCommander), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commanderID}})
	w.SetForwarder(fwd)
	w.SetStatusUpdateInterval(15 * time.Minute)

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "active"})

	if len(fwd.got) != 1 || fwd.got[0] != commanderID.String() {
		t.Fatalf("expected commander nudge to %s, got %+v", commanderID, fwd.got)
	}
}

func TestSweepCommsStalenessSkipsWhenStatusUpdateExists(t *testing.T) {
	incidentNumber := int64(2)
	incStore := newFakeIncidentStore()
	reportAt := time.Now().Add(-30 * time.Minute)
	coord := &fakeCoordStore{
		agentRepl: []store.IncidentCoordinationMessageRecord{{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindAgentReply, ActorType: store.IncidentCoordinationActorAgent, CreatedAt: reportAt}},
		statuses:  []store.IncidentCoordinationMessageRecord{{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, CreatedAt: reportAt.Add(5 * time.Minute)}},
	}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetForwarder(fwd)
	w.SetStatusUpdateInterval(15 * time.Minute)

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "active"})

	if len(fwd.got) != 0 {
		t.Fatalf("expected no nudge when a status update exists after the agent reply, got %+v", fwd.got)
	}
}

func TestSweepCommsStalenessSkipsResolvedIncidents(t *testing.T) {
	incidentNumber := int64(3)
	incStore := newFakeIncidentStore()
	coord := &fakeCoordStore{agentRepl: []store.IncidentCoordinationMessageRecord{{
		IncidentNumber: incidentNumber,
		Kind:           store.IncidentCoordinationKindAgentReply,
		ActorType:      store.IncidentCoordinationActorAgent,
		CreatedAt:      time.Now().Add(-2 * time.Hour),
	}}}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetForwarder(fwd)
	w.SetStatusUpdateInterval(15 * time.Minute)

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "resolved"})

	if len(fwd.got) != 0 {
		t.Fatalf("expected no nudge for resolved incident, got %+v", fwd.got)
	}
}

func TestSweepCommsStalenessSkipsUserAssignedCommander(t *testing.T) {
	incidentNumber := int64(4)
	incStore := newFakeIncidentStore()
	coord := &fakeCoordStore{agentRepl: []store.IncidentCoordinationMessageRecord{{
		IncidentNumber: incidentNumber,
		Kind:           store.IncidentCoordinationKindAgentReply,
		ActorType:      store.IncidentCoordinationActorAgent,
		CreatedAt:      time.Now().Add(-2 * time.Hour),
	}}}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetForwarder(fwd)
	w.SetStatusUpdateInterval(15 * time.Minute)
	userID := uuid.New()
	w.SetICSRoleStore(&fakeICSStore{ic: &store.ICSRoleRecord{RoleType: string(ics.RoleIncidentCommander), Status: string(ics.RoleStatusActive), AssigneeType: "user", UserID: &userID}})

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "active"})

	if len(fwd.got) != 0 {
		t.Fatalf("expected no forward for user-assigned commander, got %+v", fwd.got)
	}
}

func TestSweepCommsStalenessTreatsOlderStatusUpdateAsStale(t *testing.T) {
	incidentNumber := int64(5)
	commanderID := uuid.New()
	incStore := newFakeIncidentStore()
	reportAt := time.Now().Add(-30 * time.Minute)
	coord := &fakeCoordStore{
		agentRepl: []store.IncidentCoordinationMessageRecord{{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindAgentReply, ActorType: store.IncidentCoordinationActorAgent, CreatedAt: reportAt}},
		statuses:  []store.IncidentCoordinationMessageRecord{{IncidentNumber: incidentNumber, Kind: store.IncidentCoordinationKindStatusUpdate, CreatedAt: reportAt.Add(-20 * time.Minute)}},
	}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetForwarder(fwd)
	w.SetICSRoleStore(&fakeICSStore{ic: &store.ICSRoleRecord{RoleType: string(ics.RoleIncidentCommander), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commanderID}})
	w.SetStatusUpdateInterval(15 * time.Minute)

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "active"})

	if len(fwd.got) != 1 || fwd.got[0] != commanderID.String() {
		t.Fatalf("expected commander nudge when status update is older than the agent reply, got %+v", fwd.got)
	}
}

// TestSweepCommsStalenessNudgesOnResponderHandoffWithoutCommsTask covers the
// incident #10 regression: the responder handed off via post_handoff
// (audience=commander) but never called report_to_communicator, so no
// comms_task_report was created. The SLA worker must still detect the stale
// public-update cadence and nudge the commander.
func TestSweepCommsStalenessNudgesOnResponderHandoffWithoutCommsTask(t *testing.T) {
	incidentNumber := int64(6)
	commanderID := uuid.New()
	incStore := newFakeIncidentStore()
	handoffAt := time.Now().Add(-30 * time.Minute)
	coord := &fakeCoordStore{agentRepl: []store.IncidentCoordinationMessageRecord{{
		IncidentNumber: incidentNumber,
		Kind:           store.IncidentCoordinationKindAgentReply,
		ActorType:      store.IncidentCoordinationActorAgent,
		Metadata:       map[string]any{"audience": "commander"},
		CreatedAt:      handoffAt,
	}}}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetICSRoleStore(&fakeICSStore{ic: &store.ICSRoleRecord{RoleType: string(ics.RoleIncidentCommander), Status: string(ics.RoleStatusActive), AssigneeType: "agent", AgentTokenID: &commanderID}})
	w.SetForwarder(fwd)
	w.SetStatusUpdateInterval(15 * time.Minute)

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "active"})

	if len(fwd.got) != 1 || fwd.got[0] != commanderID.String() {
		t.Fatalf("expected commander nudge for stale responder handoff (incident #10 case), got %+v", fwd.got)
	}
}

// TestSweepCommsStalenessSkipsRecentResponderHandoff ensures a fresh
// responder handoff inside the staleness window does NOT nudge.
func TestSweepCommsStalenessSkipsRecentResponderHandoff(t *testing.T) {
	incidentNumber := int64(7)
	incStore := newFakeIncidentStore()
	coord := &fakeCoordStore{agentRepl: []store.IncidentCoordinationMessageRecord{{
		IncidentNumber: incidentNumber,
		Kind:           store.IncidentCoordinationKindAgentReply,
		ActorType:      store.IncidentCoordinationActorAgent,
		CreatedAt:      time.Now().Add(-2 * time.Minute),
	}}}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetICSRoleStore(&fakeICSStore{ic: &store.ICSRoleRecord{RoleType: string(ics.RoleIncidentCommander), Status: string(ics.RoleStatusActive), AssigneeType: "agent"}})
	w.SetForwarder(fwd)
	w.SetStatusUpdateInterval(15 * time.Minute)

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "active"})

	if len(fwd.got) != 0 {
		t.Fatalf("expected no nudge inside staleness window, got %+v", fwd.got)
	}
}

// TestSweepCommsStalenessNoAgentActivitySkips ensures a fresh incident with
// only a system-posted status update and no agent coordination reply does not
// trigger a stale nudge.
func TestSweepCommsStalenessNoAgentActivitySkips(t *testing.T) {
	incidentNumber := int64(8)
	incStore := newFakeIncidentStore()
	coord := &fakeCoordStore{statuses: []store.IncidentCoordinationMessageRecord{{
		IncidentNumber: incidentNumber,
		Kind:           store.IncidentCoordinationKindStatusUpdate,
		CreatedAt:      time.Now().Add(-2 * time.Hour),
	}}}
	fwd := &captureForwarder{}
	w := NewSLAWorker(incStore, nil, nil)
	w.SetCoordinationStore(coord)
	w.SetForwarder(fwd)
	w.SetICSRoleStore(&fakeICSStore{ic: &store.ICSRoleRecord{RoleType: string(ics.RoleIncidentCommander), Status: string(ics.RoleStatusActive), AssigneeType: "agent"}})
	w.SetStatusUpdateInterval(15 * time.Minute)

	w.sweepCommsStaleness(context.Background(), store.IncidentRecord{IncidentNumber: incidentNumber, Status: "active"})

	if len(fwd.got) != 0 {
		t.Fatalf("expected no nudge without any agent coordination reply, got %+v", fwd.got)
	}
}
