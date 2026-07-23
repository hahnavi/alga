package ics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type stubRoleStore struct {
	mu    sync.Mutex
	roles map[uuid.UUID]RoleRecord
}

func newStubRoleStore() *stubRoleStore {
	return &stubRoleStore{
		roles: make(map[uuid.UUID]RoleRecord),
	}
}

func (s *stubRoleStore) AssignRole(_ context.Context, incidentNumber int64, roleType RoleType, userID uuid.UUID, parentID *uuid.UUID, scope *string) (*RoleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.roles {
		if r.IncidentNumber == incidentNumber && r.RoleType == string(roleType) && r.Status == string(RoleStatusActive) {
			if roleType == RoleIncidentCommander {
				return nil, fmt.Errorf("duplicate active IC")
			}
		}
	}

	id := uuid.New()
	rec := RoleRecord{
		ID:                 id,
		IncidentNumber:     incidentNumber,
		RoleType:           string(roleType),
		AssigneeType:       "user",
		UserID:             &userID,
		ParentAssignmentID: parentID,
		ScopeDescription:   scope,
		Status:             string(RoleStatusActive),
		StartedAt:          time.Now().UTC(),
	}
	s.roles[id] = rec
	return &rec, nil
}

func (s *stubRoleStore) EndRole(_ context.Context, assignmentID uuid.UUID, reason EndReason) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.roles[assignmentID]
	if !ok {
		return fmt.Errorf("not found")
	}
	r.Status = string(RoleStatusEnded)
	reasonStr := string(reason)
	r.EndedReason = &reasonStr
	now := time.Now().UTC()
	r.EndedAt = &now
	s.roles[assignmentID] = r
	return nil
}

func (s *stubRoleStore) GetActiveRoles(_ context.Context, incidentNumber int64) ([]RoleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []RoleRecord
	for _, r := range s.roles {
		if r.IncidentNumber == incidentNumber && r.Status == string(RoleStatusActive) {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *stubRoleStore) GetActiveIC(_ context.Context, incidentNumber int64) (*RoleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.roles {
		if r.IncidentNumber == incidentNumber && r.RoleType == string(RoleIncidentCommander) && r.Status == string(RoleStatusActive) {
			return &r, nil
		}
	}
	return nil, nil
}

func (s *stubRoleStore) EndAllRolesForIncident(_ context.Context, incidentNumber int64, reason EndReason) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reasonStr := string(reason)
	now := time.Now().UTC()
	for id, r := range s.roles {
		if r.IncidentNumber == incidentNumber && r.Status == string(RoleStatusActive) {
			r.Status = string(RoleStatusEnded)
			r.EndedReason = &reasonStr
			r.EndedAt = &now
			s.roles[id] = r
		}
	}
	return nil
}

func (s *stubRoleStore) AssignAgentRole(_ context.Context, incidentNumber int64, roleType RoleType, agentTokenID uuid.UUID, parentID *uuid.UUID, scope *string) (*RoleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.roles {
		if r.IncidentNumber == incidentNumber && r.RoleType == string(roleType) && r.Status == string(RoleStatusActive) {
			if roleType == RoleIncidentCommander {
				return nil, fmt.Errorf("duplicate active IC")
			}
		}
	}

	id := uuid.New()
	rec := RoleRecord{
		ID:                 id,
		IncidentNumber:     incidentNumber,
		RoleType:           string(roleType),
		AssigneeType:       "agent",
		AgentTokenID:       &agentTokenID,
		ParentAssignmentID: parentID,
		ScopeDescription:   scope,
		Status:             string(RoleStatusActive),
		StartedAt:          time.Now().UTC(),
	}
	s.roles[id] = rec
	return &rec, nil
}

func (s *stubRoleStore) GetActiveRolesForAgent(_ context.Context, agentTokenID uuid.UUID) ([]RoleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []RoleRecord
	for _, r := range s.roles {
		if r.AgentTokenID != nil && *r.AgentTokenID == agentTokenID && r.Status == string(RoleStatusActive) {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *stubRoleStore) EndRolesForAgent(_ context.Context, agentTokenID uuid.UUID, reason EndReason) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reasonStr := string(reason)
	now := time.Now().UTC()
	for id, r := range s.roles {
		if r.AgentTokenID != nil && *r.AgentTokenID == agentTokenID && r.Status == string(RoleStatusActive) {
			r.Status = string(RoleStatusEnded)
			r.EndedReason = &reasonStr
			r.EndedAt = &now
			s.roles[id] = r
		}
	}
	return nil
}

func (s *stubRoleStore) hasActiveIC(incidentNumber int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.roles {
		if r.IncidentNumber == incidentNumber && r.RoleType == string(RoleIncidentCommander) && r.Status == string(RoleStatusActive) {
			return true
		}
	}
	return false
}

type warRoomMeetCall struct {
	num   int64
	space string
	conf  string
}

type stubIncidentStore struct {
	mu        sync.Mutex
	incidents map[int64]*IncidentRecord
	timeline  []TimelineEntry
	meetCalls []warRoomMeetCall
}

func newStubIncidentStore() *stubIncidentStore {
	return &stubIncidentStore{
		incidents: make(map[int64]*IncidentRecord),
	}
}

func (s *stubIncidentStore) GetIncident(_ context.Context, incidentNumber int64) (*IncidentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inc, ok := s.incidents[incidentNumber]
	if !ok {
		return nil, fmt.Errorf("incident not found: %d", incidentNumber)
	}
	return inc, nil
}

func (s *stubIncidentStore) AddTimelineEntry(_ context.Context, entry *TimelineEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.timeline = append(s.timeline, *entry)
	return nil
}

func (s *stubIncidentStore) SetWarRoomMeet(_ context.Context, num int64, space, conf string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.meetCalls = append(s.meetCalls, warRoomMeetCall{num: num, space: space, conf: conf})
	return nil
}

func (s *stubIncidentStore) setIncident(inc *IncidentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.incidents[inc.IncidentNumber] = inc
}

type stubDocumentStore struct {
	mu      sync.Mutex
	records map[string]DocumentRecord
}

func newStubDocumentStore() *stubDocumentStore {
	return &stubDocumentStore{records: make(map[string]DocumentRecord)}
}

func (s *stubDocumentStore) GetAllSections(_ context.Context, incidentNumber int64) ([]DocumentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []DocumentRecord
	for _, rec := range s.records {
		if rec.IncidentNumber == incidentNumber {
			result = append(result, rec)
		}
	}
	return result, nil
}

func (s *stubDocumentStore) UpsertSection(_ context.Context, incidentNumber int64, section DocumentSection, content string, version int, userID uuid.UUID) (*DocumentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%d:%s", incidentNumber, section)
	existing, ok := s.records[key]
	if ok && existing.Version != version {
		return nil, fmt.Errorf("version conflict")
	}
	rec := DocumentRecord{
		ID:             uuid.New(),
		IncidentNumber: incidentNumber,
		Section:        string(section),
		Content:        content,
		Version:        existing.Version + 1,
		UpdatedBy:      &userID,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	s.records[key] = rec
	return &rec, nil
}

func (s *stubDocumentStore) InitializeDocument(_ context.Context, incidentNumber int64, sections map[DocumentSection]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for sec, content := range sections {
		key := fmt.Sprintf("%d:%s", incidentNumber, sec)
		s.records[key] = DocumentRecord{
			ID:             uuid.New(),
			IncidentNumber: incidentNumber,
			Section:        string(sec),
			Content:        content,
			Version:        1,
			UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
		}
	}
	return nil
}
