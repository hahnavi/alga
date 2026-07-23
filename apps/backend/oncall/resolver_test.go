package oncall

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/store"
)

type stubOnCallStore struct {
	schedule  *store.OnCallScheduleRecord
	overrides []store.ScheduleOverrideRecord
	schedErr  error
	overErr   error
}

func (s *stubOnCallStore) CreateSchedule(ctx context.Context, record *store.OnCallScheduleRecord) (*store.OnCallScheduleRecord, error) {
	return nil, nil
}

func (s *stubOnCallStore) GetSchedule(ctx context.Context, id uuid.UUID) (*store.OnCallScheduleRecord, error) {
	return s.schedule, s.schedErr
}

func (s *stubOnCallStore) GetScheduleByTeam(_ context.Context, _ uuid.UUID) (*store.OnCallScheduleRecord, error) {
	return s.schedule, s.schedErr
}

func (s *stubOnCallStore) UpdateSchedule(ctx context.Context, id uuid.UUID, record *store.OnCallScheduleRecord) (*store.OnCallScheduleRecord, error) {
	return nil, nil
}

func (s *stubOnCallStore) ListSchedules(ctx context.Context, limit, skip int) ([]store.OnCallScheduleRecord, int64, error) {
	return nil, 0, nil
}

func (s *stubOnCallStore) CreateOverride(ctx context.Context, record *store.ScheduleOverrideRecord) (*store.ScheduleOverrideRecord, error) {
	return nil, nil
}

func (s *stubOnCallStore) DeleteOverride(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (s *stubOnCallStore) ListOverrides(ctx context.Context, scheduleID uuid.UUID) ([]store.ScheduleOverrideRecord, error) {
	return s.overrides, s.overErr
}

func makeUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

func TestResolveWhoIsOnCall_WeeklyRotationTwoUsers(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	user2 := makeUUID("00000000-0000-0000-0000-000000000002")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{user1.String(), user2.String()},
				},
			},
		},
	}

	r := NewResolver(stub)

	cases := []struct {
		name    string
		at      time.Time
		wantUID uuid.UUID
	}{
		{"week0_user1", startDate, user1},
		{"week0_still_user1", startDate.Add(3 * 24 * time.Hour), user1},
		{"week1_user2", startDate.Add(7 * 24 * time.Hour), user2},
		{"week2_back_to_user1", startDate.Add(14 * 24 * time.Hour), user1},
		{"week3_user2", startDate.Add(21 * 24 * time.Hour), user2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.ResolveWhoIsOnCall(context.Background(), schedID, tc.at)
			if err != nil {
				t.Fatalf("ResolveWhoIsOnCall returned error: %v", err)
			}
			if got == nil {
				t.Fatal("ResolveWhoIsOnCall returned nil, want a user ID")
			}
			if *got != tc.wantUID {
				t.Fatalf("at %v: got %s, want %s", tc.at, got, tc.wantUID)
			}
		})
	}
}

func TestResolveWhoIsOnCall_EmptySchedule(t *testing.T) {
	t.Parallel()
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID:     schedID,
			Layers: nil,
		},
	}

	r := NewResolver(stub)
	got, err := r.ResolveWhoIsOnCall(context.Background(), schedID, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty schedule, got %s", got)
	}
}

func TestResolveWhoIsOnCall_NilSchedule(t *testing.T) {
	t.Parallel()
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	stub := &stubOnCallStore{
		schedule: nil,
	}

	r := NewResolver(stub)
	got, err := r.ResolveWhoIsOnCall(context.Background(), schedID, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for nil schedule, got %s", got)
	}
}

func TestResolveWhoIsOnCall_OverrideTakesPrecedence(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	user2 := makeUUID("00000000-0000-0000-0000-000000000002")
	overrideUser := makeUUID("00000000-0000-0000-0000-000000000099")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

	overrideStart := startDate.Add(2 * 24 * time.Hour)
	overrideEnd := overrideStart.Add(24 * time.Hour)

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{user1.String(), user2.String()},
				},
			},
		},
		overrides: []store.ScheduleOverrideRecord{
			{
				ID:         makeUUID("30000000-0000-0000-0000-000000000001"),
				ScheduleID: schedID,
				UserID:     overrideUser,
				StartAt:    overrideStart,
				EndAt:      overrideEnd,
			},
		},
	}

	r := NewResolver(stub)

	got, err := r.ResolveWhoIsOnCall(context.Background(), schedID, overrideStart.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected override user, got nil")
	}
	if *got != overrideUser {
		t.Fatalf("got %s, want override user %s", got, overrideUser)
	}

	beforeOverride := overrideStart.Add(-1 * time.Hour)
	got2, err := r.ResolveWhoIsOnCall(context.Background(), schedID, beforeOverride)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got2 == nil {
		t.Fatal("expected rotation user, got nil")
	}
	if *got2 == overrideUser {
		t.Fatal("before override window, should return rotation user not override user")
	}
}

func TestResolveWhoIsOnCall_LayerWithEndDateExpired(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	endDate := startDate.Add(7 * 24 * time.Hour)

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					EndDate:          &endDate,
					UserIds:          []string{user1.String()},
				},
			},
		},
	}

	r := NewResolver(stub)

	got, err := r.ResolveWhoIsOnCall(context.Background(), schedID, endDate.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after layer end date, got %s", got)
	}
}

func TestResolveWhoIsOnCall_StoreError(t *testing.T) {
	t.Parallel()
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	stub := &stubOnCallStore{
		schedErr: context.DeadlineExceeded,
	}

	r := NewResolver(stub)
	_, err := r.ResolveWhoIsOnCall(context.Background(), schedID, time.Now())
	if err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

func TestMonthlyRotationCalendarAware(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	user2 := makeUUID("00000000-0000-0000-0000-000000000002")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	// Start on Jan 31, 2026
	startDate := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "monthly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{user1.String(), user2.String()},
				},
			},
		},
	}

	r := NewResolver(stub)

	// Feb 28, 2026 - should be user2 (1 month elapsed)
	result, err := r.ResolveWhoIsOnCall(context.Background(), schedID, time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a user to be on call")
	}
	if *result != user2 {
		t.Errorf("February should be user2, got %v", result)
	}

	// March 31, 2026 - should be user1 (2 months elapsed)
	result2, err := r.ResolveWhoIsOnCall(context.Background(), schedID, time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2 == nil {
		t.Fatal("expected a user to be on call")
	}
	if *result2 != user1 {
		t.Errorf("March should be user1, got %v", result2)
	}

	// April 30, 2026 - should be user2 (3 months elapsed)
	result3, err := r.ResolveWhoIsOnCall(context.Background(), schedID, time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result3 == nil {
		t.Fatal("expected a user to be on call")
	}
	if *result3 != user2 {
		t.Errorf("April should be user2, got %v", result3)
	}
}
func TestRotationDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		rotationType string
		interval     int
		want         time.Duration
	}{
		{"daily", 1, 24 * time.Hour},
		{"daily", 2, 48 * time.Hour},
		{"weekly", 1, 7 * 24 * time.Hour},
		{"weekly", 2, 14 * 24 * time.Hour},
		{"hourly", 1, time.Hour},
		{"hourly", 6, 6 * time.Hour},
		{"monthly", 1, 0}, // Monthly is handled separately in calculateSlot for calendar-aware rotation
		{"unknown", 1, 7 * 24 * time.Hour},
		{"weekly", 0, 7 * 24 * time.Hour},
		{"weekly", -1, 7 * 24 * time.Hour},
	}
	for _, tc := range cases {
		got := rotationDuration(tc.rotationType, tc.interval)
		if got != tc.want {
			t.Errorf("rotationDuration(%q, %d) = %v, want %v", tc.rotationType, tc.interval, got, tc.want)
		}
	}
}

func TestResolveWhoIsOnCall_HourlyRotation(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	user2 := makeUUID("00000000-0000-0000-0000-000000000002")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday midnight

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "hourly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{user1.String(), user2.String()},
				},
			},
		},
	}

	r := NewResolver(stub)

	cases := []struct {
		name    string
		at      time.Time
		wantUID uuid.UUID
	}{
		{"hour0_user1", startDate, user1},
		{"hour1_user2", startDate.Add(time.Hour), user2},
		{"hour2_user1", startDate.Add(2 * time.Hour), user1},
		{"hour3_user2", startDate.Add(3 * time.Hour), user2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.ResolveWhoIsOnCall(context.Background(), schedID, tc.at)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil || *got != tc.wantUID {
				t.Fatalf("at %v: got %v, want %s", tc.at, got, tc.wantUID)
			}
		})
	}
}

func TestResolveWhoIsOnCall_PriorityOrdering(t *testing.T) {
	t.Parallel()
	low := makeUUID("00000000-0000-0000-0000-000000000001")
	high := makeUUID("00000000-0000-0000-0000-000000000002")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

	// lowPriority layer is created first (created_at tiebreak) but has lower
	// priority; the higher-priority layer must win.
	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{low.String()},
					Priority:         0,
				},
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000002"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{high.String()},
					Priority:         5,
				},
			},
		},
	}

	r := NewResolver(stub)
	got, err := r.ResolveWhoIsOnCall(context.Background(), schedID, startDate.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != high {
		t.Fatalf("expected higher-priority layer user %s, got %v", high, got)
	}
}

func TestResolveWhoIsOnCall_DaysOfWeekWindow(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{user1.String()},
					DaysOfWeek:       []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday"},
				},
			},
		},
	}

	r := NewResolver(stub)

	// Monday noon -> active
	if got, _ := r.ResolveWhoIsOnCall(context.Background(), schedID, startDate.Add(12*time.Hour)); got == nil {
		t.Fatal("expected active on Monday")
	}
	// Saturday noon -> inactive (start_date + 5 days = Saturday)
	if got, _ := r.ResolveWhoIsOnCall(context.Background(), schedID, startDate.Add(5*24*time.Hour+12*time.Hour)); got != nil {
		t.Fatalf("expected inactive on Saturday, got %s", got)
	}
}

func TestResolveWhoIsOnCall_TimeOfDayWindow(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{user1.String()},
					StartTime:        "09:00",
					EndTime:          "17:00",
				},
			},
		},
	}

	r := NewResolver(stub)

	// Within window
	if got, _ := r.ResolveWhoIsOnCall(context.Background(), schedID, startDate.Add(12*time.Hour)); got == nil {
		t.Fatal("expected active at noon (within 09:00-17:00)")
	}
	// Before window (08:00)
	if got, _ := r.ResolveWhoIsOnCall(context.Background(), schedID, startDate.Add(8*time.Hour)); got != nil {
		t.Fatalf("expected inactive at 08:00 (before 09:00), got %s", got)
	}
	// After window (18:00)
	if got, _ := r.ResolveWhoIsOnCall(context.Background(), schedID, startDate.Add(18*time.Hour)); got != nil {
		t.Fatalf("expected inactive at 18:00 (after 17:00), got %s", got)
	}
}

func TestGenerateShifts_CoalescesAndMarksSource(t *testing.T) {
	t.Parallel()
	user1 := makeUUID("00000000-0000-0000-0000-000000000001")
	user2 := makeUUID("00000000-0000-0000-0000-000000000002")
	overrideUser := makeUUID("00000000-0000-0000-0000-000000000099")
	schedID := makeUUID("10000000-0000-0000-0000-000000000001")

	startDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday
	overrideStart := startDate.Add(2 * 24 * time.Hour)       // Wednesday
	overrideEnd := overrideStart.Add(24 * time.Hour)         // Thursday

	stub := &stubOnCallStore{
		schedule: &store.OnCallScheduleRecord{
			ID: schedID,
			Layers: []store.ScheduleLayerRecord{
				{
					ID:               makeUUID("20000000-0000-0000-0000-000000000001"),
					ScheduleID:       schedID,
					RotationType:     "weekly",
					RotationInterval: 1,
					StartDate:        startDate,
					UserIds:          []string{user1.String(), user2.String()},
				},
			},
		},
		overrides: []store.ScheduleOverrideRecord{
			{ScheduleID: schedID, UserID: overrideUser, StartAt: overrideStart, EndAt: overrideEnd},
		},
	}

	r := NewResolver(stub)
	from := startDate
	to := startDate.Add(7 * 24 * time.Hour)
	shifts := r.GenerateShifts(context.Background(), schedID, from, to)

	if len(shifts) != 3 {
		t.Fatalf("expected 3 shifts (rotation->override->rotation), got %d", len(shifts))
	}

	// First shift: user1 rotation from Monday to Wednesday (override start).
	if shifts[0].UserID != user1 {
		t.Errorf("shift 0 user = %s, want %s", shifts[0].UserID, user1)
	}
	if shifts[0].Source != ShiftSourceRotation {
		t.Errorf("shift 0 source = %q, want rotation", shifts[0].Source)
	}
	if !shifts[0].End.Equal(overrideStart) {
		t.Errorf("shift 0 end = %v, want %v (override start)", shifts[0].End, overrideStart)
	}

	// Middle shift: override user.
	if shifts[1].UserID != overrideUser {
		t.Errorf("shift 1 user = %s, want override %s", shifts[1].UserID, overrideUser)
	}
	if shifts[1].Source != ShiftSourceOverride {
		t.Errorf("shift 1 source = %q, want override", shifts[1].Source)
	}

	// Last shift resumes rotation.
	if shifts[2].Source != ShiftSourceRotation {
		t.Errorf("shift 2 source = %q, want rotation", shifts[2].Source)
	}
}
