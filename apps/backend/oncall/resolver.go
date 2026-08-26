package oncall

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
)

type Resolver struct {
	onCallStore store.OnCallStore
}

func NewResolver(onCallStore store.OnCallStore) *Resolver {
	return &Resolver{onCallStore: onCallStore}
}

func (r *Resolver) ResolveWhoIsOnCall(ctx context.Context, scheduleID uuid.UUID, at time.Time) (*uuid.UUID, error) {
	sched, err := r.onCallStore.GetSchedule(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to load schedule %s: %w", scheduleID, err)
	}
	if sched == nil {
		return nil, nil
	}

	overrides, err := r.onCallStore.ListOverrides(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to list overrides: %w", err)
	}

	userID, source := resolveAtTime(sched.Layers, overrides, at)
	if userID != nil {
		logger.Debug("on-call resolved", "component", "oncall", "schedule_id", scheduleID, "user_id", userID, "source", source)
	}
	return userID, nil
}

// ShiftSource constants label how a shift was derived. The timeline view uses
// them to color override blocks differently from rotation blocks.
const (
	ShiftSourceRotation = "rotation"
	ShiftSourceOverride = "override"
)

// resolveAtTime is the pure resolution core: given a schedule's layers and
// overrides (already loaded) and an instant, it returns the on-call user and
// how it was determined. Overrides take precedence over rotation layers; among
// layers, higher priority wins (then created_at order). Each layer's daily
// active window is evaluated in that layer's own timezone.
func resolveAtTime(layers []store.ScheduleLayerRecord, overrides []store.ScheduleOverrideRecord, at time.Time) (*uuid.UUID, string) {
	for _, o := range overrides {
		if (at.Equal(o.StartAt) || at.After(o.StartAt)) && (at.Equal(o.EndAt) || at.Before(o.EndAt)) {
			uid := o.UserID
			return &uid, ShiftSourceOverride
		}
	}

	sorted := sortedLayers(layers)
	for _, layer := range sorted {
		if layer.EndDate != nil && at.After(*layer.EndDate) {
			continue
		}
		if len(layer.UserIds) == 0 {
			continue
		}
		localAt := at.In(locationFor(layer.Timezone))
		if !matchesActiveWindow(layer, localAt) {
			continue
		}
		slot := calculateSlot(layer, localAt)
		if slot >= 0 && slot < len(layer.UserIds) {
			if id, err := uuid.Parse(layer.UserIds[slot]); err == nil {
				return &id, ShiftSourceRotation
			}
		}
	}

	return nil, ""
}

// sortedLayers returns a copy of layers ordered by descending priority. Equal
// priorities keep the input order (created_at ascending), preserved by the
// stable sort.
func sortedLayers(layers []store.ScheduleLayerRecord) []store.ScheduleLayerRecord {
	out := make([]store.ScheduleLayerRecord, len(layers))
	copy(out, layers)
	slices.SortStableFunc(out, func(a, b store.ScheduleLayerRecord) int {
		return cmp.Compare(b.Priority, a.Priority)
	})
	return out
}

// ValidRotationType reports whether rt is a rotation type supported by the
// resolver and accepted by the schedule_layers CHECK constraint (migration
// 00016). The legacy `custom` value was folded to weekly by that migration.
func ValidRotationType(rt string) bool {
	switch rt {
	case "hourly", "daily", "weekly", "monthly":
		return true
	}
	return false
}

func locationFor(timezone string) *time.Location {
	if timezone == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(timezone); err == nil {
		return loc
	}
	return time.UTC
}

func calculateSlot(layer store.ScheduleLayerRecord, at time.Time) int {
	n := len(layer.UserIds)
	if n == 0 {
		return -1
	}

	rt := layer.RotationType
	if rt == "" {
		rt = "weekly"
	}
	interval := layer.RotationInterval
	if interval <= 0 {
		interval = 1
	}

	if rt == "monthly" {
		monthsElapsed := calendarMonthsBetween(layer.StartDate, at)
		slot := (monthsElapsed / interval) % n
		slot = max(slot, 0)
		return slot
	}

	duration := rotationDuration(rt, interval)
	if duration == 0 {
		return 0
	}

	elapsed := at.Sub(layer.StartDate)
	if elapsed < 0 {
		return 0
	}

	return int(elapsed/duration) % n
}

// calendarMonthsBetween calculates the number of full calendar months between two dates.
func calendarMonthsBetween(start, at time.Time) int {
	years := at.Year() - start.Year()
	months := int(at.Month()) - int(start.Month())
	total := years*12 + months
	startDay := start.Day()
	lastDayOfAtMonth := time.Date(at.Year(), at.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	startDay = min(startDay, lastDayOfAtMonth)
	if at.Day() < startDay {
		total--
	}
	if total < 0 {
		return 0
	}
	return total
}

func rotationDuration(rotationType string, interval int) time.Duration {
	if interval <= 0 {
		interval = 1
	}
	switch rotationType {
	case "hourly":
		return time.Duration(interval) * time.Hour
	case "daily":
		return time.Duration(interval) * 24 * time.Hour
	case "weekly":
		return time.Duration(interval) * 7 * 24 * time.Hour
	case "monthly":
		// Note: monthly rotation is handled separately in calculateSlot
		// for calendar-aware calculation
		return 0
	default:
		return time.Duration(interval) * 7 * 24 * time.Hour
	}
}

// matchesActiveWindow applies the layer's optional daily-active-window
// restriction (days_of_week + start_time/end_time). An empty days_of_week or an
// empty end_time means no restriction on that axis. The window is evaluated in
// the layer's timezone (localAt is already localized by the caller).
func matchesActiveWindow(layer store.ScheduleLayerRecord, at time.Time) bool {
	if len(layer.DaysOfWeek) > 0 {
		dayMatched := false
		weekday := at.Weekday().String()
		for _, d := range layer.DaysOfWeek {
			if d == weekday {
				dayMatched = true
				break
			}
		}
		if !dayMatched {
			return false
		}
	}

	if layer.StartTime != "" && layer.EndTime != "" {
		cur := at.Format("15:04")
		start := layer.StartTime
		end := layer.EndTime
		if start == end {
			return true
		}
		if start < end {
			return cur >= start && cur < end
		}
		// Wrap-around window (e.g. 22:00-06:00).
		return cur >= start || cur < end
	}

	return true
}

// ShiftBoundary is a contiguous period during which a single user is on call.
type ShiftBoundary struct {
	UserID uuid.UUID
	Start  time.Time
	End    time.Time
	Source string
}

// shiftResolutionStep is the finest granularity at which shifts are sampled.
// One hour matches the finest rotation type (hourly) and captures
// day-of-week/time-of-day window transitions precisely for whole-hour timezones.
const shiftResolutionStep = time.Hour

// GenerateShifts expands a schedule's rotations and overrides into a list of
// contiguous shifts over [start, end). It loads the schedule and overrides once
// and resolves every hour in-memory (no per-sample DB access), then coalesces
// adjacent hours with the same user into shift boundaries. Gaps (no coverage)
// are omitted. The returned shifts are ordered by start time.
func (r *Resolver) GenerateShifts(ctx context.Context, scheduleID uuid.UUID, start, end time.Time) []ShiftBoundary {
	sched, err := r.onCallStore.GetSchedule(ctx, scheduleID)
	if err != nil || sched == nil {
		return nil
	}
	overrides, err := r.onCallStore.ListOverrides(ctx, scheduleID)
	if err != nil {
		return nil
	}
	sorted := sortedLayers(sched.Layers)

	// Align the walk to whole hours so window transitions (start_time/end_time)
	// land on sample boundaries. time.Truncate is location-independent, which
	// keeps whole-hour-offset timezones exact even though each layer may carry
	// its own timezone.
	first := start.Truncate(shiftResolutionStep)
	if first.Before(start) {
		first = first.Add(shiftResolutionStep)
	}

	type sample struct {
		at     time.Time
		userID string
		source string
	}
	var samples []sample
	cur := first
	for cur.Before(end) {
		uid, source := resolveAtTime(sorted, overrides, cur)
		s := sample{at: cur}
		if uid != nil {
			s.userID = uid.String()
			s.source = source
		}
		samples = append(samples, s)
		cur = cur.Add(shiftResolutionStep)
	}

	if len(samples) == 0 {
		return nil
	}

	var shifts []ShiftBoundary
	curUser := samples[0].userID
	curSource := samples[0].source
	curStart := samples[0].at
	flush := func(endAt time.Time) {
		if curUser == "" {
			return
		}
		uid, err := uuid.Parse(curUser)
		if err != nil {
			return
		}
		shifts = append(shifts, ShiftBoundary{
			UserID: uid,
			Start:  curStart,
			End:    endAt,
			Source: curSource,
		})
	}

	for i := 1; i < len(samples); i++ {
		if samples[i].userID != curUser || samples[i].source != curSource {
			flush(samples[i].at)
			curUser = samples[i].userID
			curSource = samples[i].source
			curStart = samples[i].at
		}
	}
	flush(cur)

	return shifts
}
