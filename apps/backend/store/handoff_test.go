package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/ent"
)

func TestHandoffCreate(t *testing.T) {
	s := newStubHandoffStore()
	ctx := context.Background()

	record, err := s.Create(ctx, &HandoffRecordRecord{
		ScheduleID: uuid.New(),
		HandoffAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if record.ID == uuid.Nil {
		t.Fatal("expected non-nil ID")
	}
	if record.Status != "pending" {
		t.Fatalf("expected pending, got %s", record.Status)
	}
}

func TestPGHandoffCreateDefaultsStatusToPending(t *testing.T) {
	client := newHandoffEntTestClient(t)
	scheduleID := uuid.New()
	createHandoffTestSchedule(t, client, scheduleID)
	s := newPGHandoffStore(client)
	record, err := s.Create(context.Background(), &HandoffRecordRecord{
		ScheduleID: scheduleID,
		HandoffAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if record.Status != "pending" {
		t.Fatalf("status = %q, want pending", record.Status)
	}
}

func TestHandoffAcknowledge(t *testing.T) {
	s := newStubHandoffStore()
	ctx := context.Background()
	scheduleID := uuid.New()

	record, _ := s.Create(ctx, &HandoffRecordRecord{
		ScheduleID: scheduleID,
		HandoffAt:  time.Now(),
	})

	err := s.Acknowledge(ctx, record.ID)
	if err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	updated, _ := s.Get(ctx, record.ID)
	if updated.Status != "acknowledged" {
		t.Fatalf("expected acknowledged, got %s", updated.Status)
	}
	if updated.IncomingAcknowledgedAt == nil {
		t.Fatal("expected acknowledged_at to be set")
	}
}

func TestHandoffGetPendingForUser(t *testing.T) {
	s := newStubHandoffStore()
	ctx := context.Background()
	userID := uuid.New()

	_, _ = s.Create(ctx, &HandoffRecordRecord{
		ScheduleID:     uuid.New(),
		IncomingUserID: &userID,
		HandoffAt:      time.Now(),
	})

	pending, err := s.GetPendingForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetPendingForUser: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
}

func TestHandoffGetPendingForUserIncludesOutgoingUser(t *testing.T) {
	client := newHandoffEntTestClient(t)
	scheduleID := uuid.New()
	createHandoffTestSchedule(t, client, scheduleID)
	s := newPGHandoffStore(client)
	ctx := context.Background()
	userID := mustCreateUser(t, client)

	_, err := s.Create(ctx, &HandoffRecordRecord{
		ScheduleID:     scheduleID,
		OutgoingUserID: &userID,
		HandoffAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pending, err := s.GetPendingForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetPendingForUser: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected outgoing user pending handoff, got %d", len(pending))
	}
}

func TestHandoffUpdateNotes(t *testing.T) {
	s := newStubHandoffStore()
	ctx := context.Background()

	record, _ := s.Create(ctx, &HandoffRecordRecord{
		ScheduleID: uuid.New(),
		HandoffAt:  time.Now(),
	})

	err := s.UpdateOutgoingNotes(ctx, record.ID, "outgoing notes here")
	if err != nil {
		t.Fatalf("UpdateOutgoingNotes: %v", err)
	}
	err = s.UpdateIncomingNotes(ctx, record.ID, "incoming notes here")
	if err != nil {
		t.Fatalf("UpdateIncomingNotes: %v", err)
	}

	updated, _ := s.Get(ctx, record.ID)
	if updated.OutgoingNotes != "outgoing notes here" {
		t.Fatalf("expected outgoing notes, got %s", updated.OutgoingNotes)
	}
	if updated.IncomingNotes != "incoming notes here" {
		t.Fatalf("expected incoming notes, got %s", updated.IncomingNotes)
	}
}

func TestHandoffGetLatestForSchedule(t *testing.T) {
	s := newStubHandoffStore()
	ctx := context.Background()
	scheduleID := uuid.New()

	_, _ = s.Create(ctx, &HandoffRecordRecord{
		ScheduleID: scheduleID,
		HandoffAt:  time.Now().Add(-time.Hour),
	})
	second, _ := s.Create(ctx, &HandoffRecordRecord{
		ScheduleID: scheduleID,
		HandoffAt:  time.Now(),
	})

	latest, err := s.GetLatestForSchedule(ctx, scheduleID)
	if err != nil {
		t.Fatalf("GetLatestForSchedule: %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("expected latest to be second record")
	}
}

func TestPGHandoffGetLatestForScheduleUsesHandoffAt(t *testing.T) {
	client := newHandoffEntTestClient(t)
	s := newPGHandoffStore(client)
	ctx := context.Background()
	scheduleID := uuid.New()
	createHandoffTestSchedule(t, client, scheduleID)

	newerHandoff, err := s.Create(ctx, &HandoffRecordRecord{
		ScheduleID: scheduleID,
		HandoffAt:  time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create newer handoff: %v", err)
	}
	_, err = s.Create(ctx, &HandoffRecordRecord{
		ScheduleID: scheduleID,
		HandoffAt:  time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("Create older handoff: %v", err)
	}

	latest, err := s.GetLatestForSchedule(ctx, scheduleID)
	if err != nil {
		t.Fatalf("GetLatestForSchedule: %v", err)
	}
	if latest.ID != newerHandoff.ID {
		t.Fatalf("latest ID = %s, want %s", latest.ID, newerHandoff.ID)
	}
}

func TestPGHandoffGetLatestForScheduleReturnsErrNotFound(t *testing.T) {
	s := newPGHandoffStore(newHandoffEntTestClient(t))
	_, err := s.GetLatestForSchedule(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestHandoffListWithFilters(t *testing.T) {
	s := newStubHandoffStore()
	ctx := context.Background()
	scheduleID := uuid.New()
	userID := uuid.New()

	_, _ = s.Create(ctx, &HandoffRecordRecord{
		ScheduleID:     scheduleID,
		IncomingUserID: &userID,
		HandoffAt:      time.Now(),
		Status:         "pending",
	})
	_, _ = s.Create(ctx, &HandoffRecordRecord{
		ScheduleID:     uuid.New(),
		IncomingUserID: &userID,
		HandoffAt:      time.Now(),
		Status:         "acknowledged",
	})

	// Filter by status
	filter := HandoffFilter{Status: "pending"}
	records, total, err := s.List(ctx, filter, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 total, got %d", total)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

// stubHandoffStore is an in-memory implementation of HandoffStore for testing.
type stubHandoffStore struct {
	records map[string]*HandoffRecordRecord
}

func newStubHandoffStore() *stubHandoffStore {
	return &stubHandoffStore{records: make(map[string]*HandoffRecordRecord)}
}

func (s *stubHandoffStore) Create(ctx context.Context, r *HandoffRecordRecord) (*HandoffRecordRecord, error) {
	r.ID = uuid.New()
	if r.Status == "" {
		r.Status = "pending"
	}
	r.CreatedAt = time.Now()
	r.UpdatedAt = time.Now()
	s.records[r.ID.String()] = r
	return r, nil
}

func (s *stubHandoffStore) Get(ctx context.Context, id uuid.UUID) (*HandoffRecordRecord, error) {
	r, ok := s.records[id.String()]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (s *stubHandoffStore) List(ctx context.Context, filter HandoffFilter, limit, skip int) ([]*HandoffRecordRecord, int64, error) {
	var result []*HandoffRecordRecord
	for _, r := range s.records {
		if filter.ScheduleID != nil && r.ScheduleID != *filter.ScheduleID {
			continue
		}
		if filter.UserID != nil {
			if r.OutgoingUserID == nil && r.IncomingUserID == nil {
				continue
			}
			if r.OutgoingUserID != nil && *r.OutgoingUserID == *filter.UserID {
				// match
			} else if r.IncomingUserID != nil && *r.IncomingUserID == *filter.UserID {
				// match
			} else {
				continue
			}
		}
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		result = append(result, r)
	}
	if skip < len(result) {
		result = result[skip:]
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, int64(len(result)), nil
}

func (s *stubHandoffStore) GetPendingForUser(ctx context.Context, userID uuid.UUID) ([]*HandoffRecordRecord, error) {
	var result []*HandoffRecordRecord
	for _, r := range s.records {
		if r.Status == "pending" && ((r.IncomingUserID != nil && *r.IncomingUserID == userID) || (r.OutgoingUserID != nil && *r.OutgoingUserID == userID)) {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *stubHandoffStore) GetLatestForSchedule(ctx context.Context, scheduleID uuid.UUID) (*HandoffRecordRecord, error) {
	var latest *HandoffRecordRecord
	for _, r := range s.records {
		if r.ScheduleID == scheduleID {
			if latest == nil || r.HandoffAt.After(latest.HandoffAt) {
				latest = r
			}
		}
	}
	return latest, nil
}

func (s *stubHandoffStore) UpdateOutgoingNotes(ctx context.Context, id uuid.UUID, notes string) error {
	r, ok := s.records[id.String()]
	if !ok {
		return nil
	}
	r.OutgoingNotes = notes
	r.UpdatedAt = time.Now()
	return nil
}

func (s *stubHandoffStore) UpdateIncomingNotes(ctx context.Context, id uuid.UUID, notes string) error {
	r, ok := s.records[id.String()]
	if !ok {
		return nil
	}
	r.IncomingNotes = notes
	r.UpdatedAt = time.Now()
	return nil
}

func (s *stubHandoffStore) Acknowledge(ctx context.Context, id uuid.UUID) error {
	r, ok := s.records[id.String()]
	if !ok {
		return nil
	}
	r.Status = "acknowledged"
	now := time.Now()
	r.IncomingAcknowledgedAt = &now
	r.UpdatedAt = now
	return nil
}

func newHandoffEntTestClient(t *testing.T) *ent.Client {
	t.Helper()
	return newTestEntClient(t)
}

func createHandoffTestSchedule(t *testing.T, client *ent.Client, id uuid.UUID) {
	t.Helper()
	_, err := client.OnCallSchedule.Create().
		SetID(id).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
}
