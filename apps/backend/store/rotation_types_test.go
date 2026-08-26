//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"alga/db"
	"alga/db/models"
)

// TestScheduleLayerRotationTypes verifies the migrated rotation_type CHECK
// (migration 00016) accepts every resolver-supported type and rejects the
// legacy `custom` value that the migration folded to weekly.
func TestScheduleLayerRotationTypes(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	now := time.Now().UTC()
	sched, err := stores.OnCall.CreateSchedule(context.Background(), &OnCallScheduleRecord{
		Layers: []ScheduleLayerRecord{
			{Name: "hourly", RotationType: "hourly", RotationInterval: 4, StartDate: now, Timezone: "UTC", StartTime: "00:00"},
			{Name: "daily", RotationType: "daily", StartDate: now, Timezone: "UTC", StartTime: "00:00", Priority: 1},
			{Name: "weekly", RotationType: "weekly", StartDate: now, Timezone: "UTC", StartTime: "00:00", Priority: 2},
			{Name: "monthly", RotationType: "monthly", StartDate: now, Timezone: "UTC", StartTime: "00:00", Priority: 3},
		},
	})
	if err != nil {
		t.Fatalf("creating layers with hourly/daily/weekly/monthly must succeed on a migrated DB: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.OnCallSchedule)(nil)).Where("id = ?", sched.ID).Exec(context.Background())
	})

	got, err := stores.OnCall.GetSchedule(context.Background(), sched.ID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if len(got.Layers) != 4 {
		t.Fatalf("layers = %d, want 4", len(got.Layers))
	}
	byType := map[string]string{}
	for _, l := range got.Layers {
		byType[l.RotationType] = l.Name
	}
	for _, rt := range []string{"hourly", "daily", "weekly", "monthly"} {
		if _, ok := byType[rt]; !ok {
			t.Errorf("rotation_type %q missing from persisted layers: %+v", rt, got.Layers)
		}
	}

	// The legacy `custom` value is no longer accepted by the constraint.
	custom := &models.ScheduleLayer{
		ID:               models.NewUUID(),
		ScheduleID:       sched.ID,
		Name:             "legacy",
		RotationType:     "custom",
		RotationInterval: 1,
		StartDate:        now,
		Timezone:         "UTC",
		StartTime:        "00:00",
		Priority:         9,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if _, err := bunDB.NewInsert().Model(custom).Exec(context.Background()); err == nil {
		t.Error("inserting rotation_type=custom must violate the migrated CHECK constraint")
	} else {
		t.Logf("custom rejected as expected: %v", err)
	}
}
