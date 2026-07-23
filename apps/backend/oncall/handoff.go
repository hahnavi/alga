package oncall

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	valkeygo "github.com/valkey-io/valkey-go"

	"alga/logger"
	"alga/store"
	"alga/valkey"
)

type HandoffDetector struct {
	onCallStore  store.OnCallStore
	handoffStore store.HandoffStore
	valkeyClient *valkey.Client
	resolver     *Resolver
}

func NewHandoffDetector(
	onCallStore store.OnCallStore,
	handoffStore store.HandoffStore,
	valkeyClient *valkey.Client,
	resolver *Resolver,
) *HandoffDetector {
	return &HandoffDetector{
		onCallStore:  onCallStore,
		handoffStore: handoffStore,
		valkeyClient: valkeyClient,
		resolver:     resolver,
	}
}

type handoffCursor struct {
	UserID     string    `json:"user_id,omitempty"`
	ResolvedAt time.Time `json:"resolved_at"`
}

func cursorKey(scheduleID uuid.UUID) string {
	return fmt.Sprintf("alga:oncall:cursor:%s", scheduleID.String())
}

func (d *HandoffDetector) Tick(ctx context.Context, now time.Time) ([]store.OnCallScheduleRecord, map[string]*store.HandoffRecordRecord, error) {
	schedules, _, err := d.onCallStore.ListSchedules(ctx, 1000, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("list schedules: %w", err)
	}

	records := make(map[string]*store.HandoffRecordRecord)
	var changed []store.OnCallScheduleRecord

	for _, sched := range schedules {
		rec, err := d.tickSchedule(ctx, now, sched)
		if err != nil {
			logger.Warn("handoff tick for schedule failed", "component", "oncall", "schedule_id", sched.ID, "error", err)
			continue
		}
		if rec != nil {
			records[sched.ID.String()] = rec
			changed = append(changed, sched)
		}
	}

	return changed, records, nil
}

func (d *HandoffDetector) tickSchedule(ctx context.Context, now time.Time, sched store.OnCallScheduleRecord) (*store.HandoffRecordRecord, error) {
	incomingUID, err := d.resolver.ResolveWhoIsOnCall(ctx, sched.ID, now)
	if err != nil {
		return nil, fmt.Errorf("resolve who is on call: %w", err)
	}

	var incomingStr string
	if incomingUID != nil {
		incomingStr = incomingUID.String()
	}

	cursor, err := d.readCursor(ctx, sched.ID)
	if err != nil {
		logger.Warn("handoff: read cursor failed", "component", "oncall", "schedule_id", sched.ID, "error", err)
		return nil, nil
	}

	if cursor == nil {
		d.seedCursor(ctx, sched.ID, incomingStr, now)
		return nil, nil
	}

	if cursor.UserID == incomingStr {
		return nil, nil
	}

	var outgoingUserID *uuid.UUID
	if cursor.UserID != "" {
		uid, err := uuid.Parse(cursor.UserID)
		if err == nil {
			outgoingUserID = &uid
		}
	}

	var incomingUserID *uuid.UUID
	if incomingUID != nil {
		incomingUserID = incomingUID
	}

	record := &store.HandoffRecordRecord{
		ScheduleID:     sched.ID,
		OutgoingUserID: outgoingUserID,
		IncomingUserID: incomingUserID,
		HandoffAt:      now,
		Status:         "pending",
	}

	created, err := d.handoffStore.Create(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("create handoff record: %w", err)
	}

	d.seedCursor(ctx, sched.ID, incomingStr, now)
	return created, nil
}

func (d *HandoffDetector) readCursor(ctx context.Context, scheduleID uuid.UUID) (*handoffCursor, error) {
	if d.valkeyClient == nil {
		return nil, nil
	}
	data, err := d.valkeyClient.Do(ctx, d.valkeyClient.Builder().Get().Key(cursorKey(scheduleID)).Build()).ToString()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	var cv handoffCursor
	if err := json.Unmarshal([]byte(data), &cv); err != nil {
		return nil, err
	}
	return &cv, nil
}

func (d *HandoffDetector) seedCursor(ctx context.Context, scheduleID uuid.UUID, userID string, now time.Time) {
	if d.valkeyClient == nil {
		return
	}
	cv := handoffCursor{UserID: userID, ResolvedAt: now}
	data, err := json.Marshal(cv)
	if err != nil {
		logger.WarnCtx(ctx, "Handoff: failed to encode cursor; skipping seed", "component", "oncall-handoff", "schedule_id", scheduleID, "error", err)
		return
	}
	if err := d.valkeyClient.Do(ctx, d.valkeyClient.Builder().Set().Key(cursorKey(scheduleID)).Value(string(data)).ExSeconds(604800).Build()).Error(); err != nil {
		logger.WarnCtx(ctx, "Handoff: failed to seed cursor in valkey", "component", "oncall-handoff", "schedule_id", scheduleID, "error", err)
	}
}
