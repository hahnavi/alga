package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/rabbitmq"
	"alga/store"
)

type fakeAlertInvestigationStore struct {
	stalledAssigned      []store.AlertInvestigationRecord
	stalledInvestigating []store.AlertInvestigationRecord
	listAssignedHook     func(time.Duration)
}

func (s *fakeAlertInvestigationStore) ListStalledAssignedAlertInvestigations(_ context.Context, d time.Duration) ([]store.AlertInvestigationRecord, error) {
	if s.listAssignedHook != nil {
		s.listAssignedHook(d)
	}
	return s.stalledAssigned, nil
}

func (s *fakeAlertInvestigationStore) ListStalledInvestigatingAlertInvestigations(_ context.Context, _ time.Duration) ([]store.AlertInvestigationRecord, error) {
	return s.stalledInvestigating, nil
}

type fakeIncidentStoreForStuck struct {
	byID     map[uuid.UUID]*store.IncidentRecord
	timeline []store.IncidentTimelineEntryRecord
	getErr   error
}

func (s *fakeIncidentStoreForStuck) GetIncidentByID(_ context.Context, id uuid.UUID) (*store.IncidentRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if inc, ok := s.byID[id]; ok {
		return inc, nil
	}
	return nil, store.ErrNotFound
}

func (s *fakeIncidentStoreForStuck) AddTimelineEntry(_ context.Context, e *store.IncidentTimelineEntryRecord) error {
	s.timeline = append(s.timeline, *e)
	return nil
}

type fakeServiceStoreForStuck struct {
	byID   map[uuid.UUID]*store.ServiceRecord
	getErr error
}

func (s *fakeServiceStoreForStuck) GetService(_ context.Context, id string) (*store.ServiceRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	if svc, ok := s.byID[uid]; ok {
		return svc, nil
	}
	return nil, store.ErrNotFound
}

type fakeTeamStoreForStuck struct {
	byName map[string]*store.TeamRecord
	getErr error
}

func (s *fakeTeamStoreForStuck) GetTeamByName(_ context.Context, name string) (*store.TeamRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if t, ok := s.byName[name]; ok {
		return t, nil
	}
	return nil, store.ErrNotFound
}

type capturingPublisher struct {
	escalations []rabbitmq.EscalationMessage
	publishErr  error
}

func (p *capturingPublisher) PublishEscalation(_ context.Context, msg rabbitmq.EscalationMessage) error {
	if p.publishErr != nil {
		return p.publishErr
	}
	p.escalations = append(p.escalations, msg)
	return nil
}

type fakeValkeyForStuck struct {
	keys   map[string]struct{}
	hashes map[string]map[string]string
}

func newFakeValkeyForStuck() *fakeValkeyForStuck {
	return &fakeValkeyForStuck{
		keys:   map[string]struct{}{},
		hashes: map[string]map[string]string{},
	}
}

func (f *fakeValkeyForStuck) SetNX(_ context.Context, key, _ string, _ time.Duration) (bool, error) {
	if _, ok := f.keys[key]; ok {
		return false, nil
	}
	f.keys[key] = struct{}{}
	return true, nil
}

func (f *fakeValkeyForStuck) HGet(_ context.Context, key, field string) (string, error) {
	if h, ok := f.hashes[key]; ok {
		return h[field], nil
	}
	return "", nil
}

func (f *fakeValkeyForStuck) HSet(_ context.Context, key, field, value string) error {
	if _, ok := f.hashes[key]; !ok {
		f.hashes[key] = map[string]string{}
	}
	f.hashes[key][field] = value
	return nil
}

func newStuckWorker(aiStore stuckInvestigationLister, incStore *fakeIncidentStoreForStuck, svcStore *fakeServiceStoreForStuck, teamStore *fakeTeamStoreForStuck, vk *fakeValkeyForStuck, multiplier int) *StuckInvestigationEscalationWorker {
	var pub stuckEscalationPublisher
	if vk != nil {
		_ = pub
	}
	return NewStuckInvestigationEscalationWorker(
		aiStore, incStore, svcStore, teamStore, pub, vk, multiplier, time.Hour, 10*time.Minute, "ops-team",
	)
}

func TestStuckInvestigationEscalation_DisabledWhenMultiplierZero(t *testing.T) {
	t.Parallel()
	aiStore := &fakeAlertInvestigationStore{
		stalledAssigned: []store.AlertInvestigationRecord{
			{AlertInvestigationID: "inv-1", Status: "assigned"},
		},
	}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{}}
	teamStore := &fakeTeamStoreForStuck{}
	svcStore := &fakeServiceStoreForStuck{}
	w := newStuckWorker(aiStore, incStore, svcStore, teamStore, newFakeValkeyForStuck(), 0)
	w.tick(context.Background())
	if len(incStore.timeline) != 0 {
		t.Errorf("expected no timeline writes with multiplier=0, got %d", len(incStore.timeline))
	}
}

func TestStuckInvestigationEscalation_ResolvesIncidentDirectPolicy(t *testing.T) {
	t.Parallel()
	incidentID := uuid.New()
	policyID := uuid.New()
	inc := &store.IncidentRecord{ID: incidentID, IncidentNumber: 42, EscalationPolicyID: &policyID}
	inv := store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-direct",
		Status:               "assigned",
		PromotedIncidentID:   &incidentID,
	}
	aiStore := &fakeAlertInvestigationStore{}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{incidentID: inc}}
	svcStore := &fakeServiceStoreForStuck{}
	teamStore := &fakeTeamStoreForStuck{}
	w := newStuckWorker(aiStore, incStore, svcStore, teamStore, newFakeValkeyForStuck(), 2)
	got, ok := w.resolvePolicyID(context.Background(), inv)
	if !ok {
		t.Fatalf("resolvePolicyID ok=false, want true")
	}
	if got != policyID {
		t.Errorf("policyID = %s, want %s (incident direct)", got, policyID)
	}
}

func TestStuckInvestigationEscalation_FallsBackToServicePolicy(t *testing.T) {
	t.Parallel()
	incidentID := uuid.New()
	svcID := uuid.New()
	svcPolicyID := uuid.New()
	inc := &store.IncidentRecord{ID: incidentID, IncidentNumber: 7, ServiceID: &svcID}
	svc := &store.ServiceRecord{ID: svcID, Name: "svc-a", EscalationPolicyID: &svcPolicyID}
	inv := store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-svc",
		Status:               "investigating",
		PromotedIncidentID:   &incidentID,
	}
	aiStore := &fakeAlertInvestigationStore{}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{incidentID: inc}}
	svcStore := &fakeServiceStoreForStuck{byID: map[uuid.UUID]*store.ServiceRecord{svcID: svc}}
	teamStore := &fakeTeamStoreForStuck{}
	w := newStuckWorker(aiStore, incStore, svcStore, teamStore, newFakeValkeyForStuck(), 2)
	got, ok := w.resolvePolicyID(context.Background(), inv)
	if !ok {
		t.Fatalf("resolvePolicyID ok=false, want true")
	}
	if got != svcPolicyID {
		t.Errorf("policyID = %s, want %s (service)", got, svcPolicyID)
	}
}

func TestStuckInvestigationEscalation_FallsBackToOpsTeamPolicy(t *testing.T) {
	t.Parallel()
	incidentID := uuid.New()
	svcID := uuid.New()
	inc := &store.IncidentRecord{ID: incidentID, IncidentNumber: 99, ServiceID: &svcID}
	inv := store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-ops",
		Status:               "assigned",
		PromotedIncidentID:   &incidentID,
	}
	aiStore := &fakeAlertInvestigationStore{}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{incidentID: inc}}
	svcStore := &fakeServiceStoreForStuck{byID: map[uuid.UUID]*store.ServiceRecord{}} // service has no policy
	teamStore := &fakeTeamStoreForStuck{byName: map[string]*store.TeamRecord{
		"ops-team": {ID: uuid.New(), Name: "ops-team"},
	}}
	w := newStuckWorker(aiStore, incStore, svcStore, teamStore, newFakeValkeyForStuck(), 2)
	// Teams no longer carry an escalation policy; with neither the incident
	// nor its service configured, no policy is resolved.
	_, ok := w.resolvePolicyID(context.Background(), inv)
	if ok {
		t.Errorf("resolvePolicyID ok=true, want false (no ops-team fallback)")
	}
}

func TestStuckInvestigationEscalation_NoPolicyNoEscalation(t *testing.T) {
	t.Parallel()
	incidentID := uuid.New()
	svcID := uuid.New()
	inc := &store.IncidentRecord{ID: incidentID, IncidentNumber: 1, ServiceID: &svcID}
	inv := store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-none",
		Status:               "investigating",
		PromotedIncidentID:   &incidentID,
	}
	aiStore := &fakeAlertInvestigationStore{}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{incidentID: inc}}
	svcStore := &fakeServiceStoreForStuck{byID: map[uuid.UUID]*store.ServiceRecord{}}
	teamStore := &fakeTeamStoreForStuck{byName: map[string]*store.TeamRecord{}}
	w := newStuckWorker(aiStore, incStore, svcStore, teamStore, newFakeValkeyForStuck(), 2)
	_, ok := w.resolvePolicyID(context.Background(), inv)
	if ok {
		t.Errorf("resolvePolicyID ok=true, want false (no policy anywhere)")
	}
}

func TestStuckInvestigationEscalation_DedupPreventsRefire(t *testing.T) {
	t.Parallel()
	aiStore := &fakeAlertInvestigationStore{}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{}}
	teamStore := &fakeTeamStoreForStuck{}
	svcStore := &fakeServiceStoreForStuck{}
	vk := newFakeValkeyForStuck()
	w := newStuckWorker(aiStore, incStore, svcStore, teamStore, vk, 2)
	if !w.claimDedup(context.Background(), "inv-dup") {
		t.Fatalf("first claimDedup should return true")
	}
	if w.claimDedup(context.Background(), "inv-dup") {
		t.Errorf("second claimDedup should return false")
	}
}

func TestStuckInvestigationEscalation_NoValkeyAllowsRefire(t *testing.T) {
	t.Parallel()
	aiStore := &fakeAlertInvestigationStore{}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{}}
	teamStore := &fakeTeamStoreForStuck{}
	svcStore := &fakeServiceStoreForStuck{}
	w := NewStuckInvestigationEscalationWorker(
		aiStore, incStore, svcStore, teamStore, nil, nil, 2, time.Hour, 10*time.Minute, "ops-team",
	)
	if !w.claimDedup(context.Background(), "inv-x") {
		t.Errorf("claimDedup with nil vk should return true")
	}
}

func TestStuckInvestigationEscalation_ThresholdScalesWithMultiplier(t *testing.T) {
	t.Parallel()
	var observedThreshold time.Duration
	aiStore := &fakeAlertInvestigationStore{listAssignedHook: func(d time.Duration) { observedThreshold = d }}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{}}
	teamStore := &fakeTeamStoreForStuck{}
	svcStore := &fakeServiceStoreForStuck{}
	w := NewStuckInvestigationEscalationWorker(
		aiStore, incStore, svcStore, teamStore, nil, nil, 3, time.Hour, 10*time.Minute, "ops-team",
	)
	w.tick(context.Background())
	if observedThreshold != 30*time.Minute {
		t.Errorf("threshold = %v, want 30m (3 * 10m)", observedThreshold)
	}
}

func TestStuckInvestigationEscalation_ProcessOneFiresEscalation(t *testing.T) {
	t.Parallel()
	incidentID := uuid.New()
	policyID := uuid.New()
	inc := &store.IncidentRecord{ID: incidentID, IncidentNumber: 555, EscalationPolicyID: &policyID}
	inv := store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-fire",
		Status:               "assigned",
		PromotedIncidentID:   &incidentID,
		StartedAt:            ptrTime(time.Now().Add(-25 * time.Minute)),
	}
	aiStore := &fakeAlertInvestigationStore{stalledAssigned: []store.AlertInvestigationRecord{inv}}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{incidentID: inc}}
	svcStore := &fakeServiceStoreForStuck{}
	teamStore := &fakeTeamStoreForStuck{}

	pub := &capturingPublisher{}
	w := NewStuckInvestigationEscalationWorker(
		aiStore, incStore, svcStore, teamStore, pub, nil, 2, time.Hour, 10*time.Minute, "ops-team",
	)
	w.tick(context.Background())

	if len(pub.escalations) != 1 {
		t.Fatalf("expected 1 escalation, got %d", len(pub.escalations))
	}
	if pub.escalations[0].PolicyID != policyID {
		t.Errorf("PolicyID = %s, want %s", pub.escalations[0].PolicyID, policyID)
	}
	if pub.escalations[0].IncidentNumber != 555 {
		t.Errorf("IncidentNumber = %d, want 555", pub.escalations[0].IncidentNumber)
	}
	if pub.escalations[0].Level != 1 {
		t.Errorf("Level = %d, want 1", pub.escalations[0].Level)
	}
	if len(incStore.timeline) != 1 {
		t.Fatalf("expected 1 timeline entry, got %d", len(incStore.timeline))
	}
	if incStore.timeline[0].EventType != "stuck_investigation_escalation" {
		t.Errorf("EventType = %q, want %q", incStore.timeline[0].EventType, "stuck_investigation_escalation")
	}
}

func TestStuckInvestigationEscalation_ProcessOneDedupesSecondPass(t *testing.T) {
	t.Parallel()
	incidentID := uuid.New()
	policyID := uuid.New()
	inc := &store.IncidentRecord{ID: incidentID, IncidentNumber: 600, EscalationPolicyID: &policyID}
	inv := store.AlertInvestigationRecord{
		AlertInvestigationID: "inv-dedupe",
		Status:               "assigned",
		PromotedIncidentID:   &incidentID,
	}
	aiStore := &fakeAlertInvestigationStore{stalledAssigned: []store.AlertInvestigationRecord{inv}}
	incStore := &fakeIncidentStoreForStuck{byID: map[uuid.UUID]*store.IncidentRecord{incidentID: inc}}
	svcStore := &fakeServiceStoreForStuck{}
	teamStore := &fakeTeamStoreForStuck{}
	vk := newFakeValkeyForStuck()
	pub := &capturingPublisher{}
	w := NewStuckInvestigationEscalationWorker(
		aiStore, incStore, svcStore, teamStore, pub, vk, 2, time.Hour, 10*time.Minute, "ops-team",
	)
	w.tick(context.Background())
	w.tick(context.Background())
	if len(pub.escalations) != 1 {
		t.Errorf("expected 1 escalation across 2 ticks (dedup), got %d", len(pub.escalations))
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
