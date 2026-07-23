package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"alga/logger"
	"alga/oncall"
	"alga/rbac"
	"alga/sse"
	"alga/store"
	"alga/valkey"

	"github.com/google/uuid"
)

func (s *Server) handleOnCallSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListOnCallSchedules(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "schedules are auto-created from teams and cannot be created directly")
	}
}

// scheduleDisplayName derives a schedule's display name dynamically from its
// team. Schedules no longer store a name field; the title always tracks the
// owning team's name.
func (s *Server) scheduleDisplayName(ctx context.Context, sched *store.OnCallScheduleRecord) string {
	if sched.TeamID != nil && s.teamStore != nil {
		if name, err := s.teamStore.GetTeamName(ctx, *sched.TeamID); err == nil && name != "" {
			return name
		}
	}
	return "On-Call"
}

// enrichSchedule populates the dynamically-derived team name on a schedule
// record before it is serialized for the API.
func (s *Server) enrichSchedule(ctx context.Context, sched *store.OnCallScheduleRecord) *store.OnCallScheduleRecord {
	sched.TeamName = s.scheduleDisplayName(ctx, sched)
	return sched
}

// loadTeamMemberSet returns the set of user IDs belonging to the schedule's
// owning team, used to validate on-call participants. It writes an error
// response and returns ok=false when the team store is unavailable or members
// cannot be loaded. It returns a nil set with ok=true when the schedule has no
// owning team, in which case membership cannot be determined and is not
// enforced.
func (s *Server) loadTeamMemberSet(w http.ResponseWriter, ctx context.Context, sched *store.OnCallScheduleRecord) (map[string]struct{}, bool) {
	if sched.TeamID == nil {
		return nil, true
	}
	if !s.requireTeamStore(w) {
		return nil, false
	}
	members, err := s.teamStore.GetMembers(ctx, *sched.TeamID)
	if err != nil {
		writeInternalError(w, err, "failed to load team members")
		return nil, false
	}
	set := make(map[string]struct{}, len(members))
	for _, m := range members {
		set[m.UserID.String()] = struct{}{}
	}
	return set, true
}

// scheduleDisplayTimezone returns a representative timezone for a schedule,
// used by the iCal export header. It prefers the first layer's timezone and
// falls back to UTC when there are no layers.
func scheduleDisplayTimezone(sched *store.OnCallScheduleRecord) string {
	for _, l := range sched.Layers {
		if l.Timezone != "" {
			return l.Timezone
		}
	}
	return "UTC"
}

func (s *Server) handleListOnCallSchedules(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}

	limit, skip := parseLimitSkip(r, 50)
	records, total, err := s.onCallStore.ListSchedules(r.Context(), int(limit), int(skip))
	if err != nil {
		writeInternalError(w, err, "failed to list on-call schedules")
		return
	}
	for i := range records {
		records[i].TeamName = s.scheduleDisplayName(r.Context(), &records[i])
	}
	writePaginatedJSON(w, ensureSlice(records), int64(total))
}

func (s *Server) handleOnCallScheduleRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/on-call/schedules/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing schedule id")
		return
	}

	if suffix == "overrides" || strings.HasSuffix(suffix, "/overrides") {
		scheduleID := strings.TrimSuffix(suffix, "/overrides")
		if scheduleID == "overrides" {
			scheduleID = ""
		}
		if scheduleID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing schedule id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleListOverrides(w, r, scheduleID)
		case http.MethodPost:
			s.handleCreateOverride(w, r, scheduleID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if suffix == "current" || strings.HasSuffix(suffix, "/current") {
		scheduleID := strings.TrimSuffix(suffix, "/current")
		if scheduleID == "current" {
			scheduleID = ""
		}
		if scheduleID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing schedule id")
			return
		}
		s.handleCurrentOnCall(w, r, scheduleID)
		return
	}

	if suffix == "timeline" || strings.HasSuffix(suffix, "/timeline") {
		scheduleID := strings.TrimSuffix(suffix, "/timeline")
		if scheduleID == "timeline" {
			scheduleID = ""
		}
		if scheduleID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing schedule id")
			return
		}
		s.handleScheduleTimeline(w, r, scheduleID)
		return
	}

	if suffix == "ical" || strings.HasSuffix(suffix, "/ical") {
		scheduleID := strings.TrimSuffix(suffix, "/ical")
		if scheduleID == "ical" {
			scheduleID = ""
		}
		if scheduleID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing schedule id")
			return
		}
		s.handleScheduleICal(w, r, scheduleID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetOnCallSchedule(w, r, suffix)
	case http.MethodPatch:
		s.handlePatchOnCallSchedule(w, r, suffix)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) getScheduleOrError(w http.ResponseWriter, r *http.Request, id string) (*store.OnCallScheduleRecord, bool) {
	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule id")
		return nil, false
	}
	record, err := s.onCallStore.GetSchedule(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get on-call schedule")
		return nil, false
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "schedule not found")
		return nil, false
	}
	return record, true
}

func (s *Server) handleGetOnCallSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}
	record, ok := s.getScheduleOrError(w, r, id)
	if !ok {
		return
	}
	writeData(w, http.StatusOK, s.enrichSchedule(r.Context(), record))
}

func (s *Server) handlePatchOnCallSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule id")
		return
	}

	current, err := s.onCallStore.GetSchedule(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get on-call schedule")
		return
	}
	if current == nil {
		writeError(w, ErrorCodeNotFound, "schedule not found")
		return
	}

	// Only rotations (layers) are editable. A schedule's identity (team) and
	// display name are derived from its team and cannot be patched directly.
	var req struct {
		Layers []struct {
			Name             string   `json:"name"`
			RotationType     string   `json:"rotation_type"`
			RotationInterval int      `json:"rotation_interval,omitempty"`
			StartDate        string   `json:"start_date"`
			EndDate          string   `json:"end_date,omitempty"`
			Timezone         string   `json:"timezone,omitempty"`
			StartTime        string   `json:"start_time,omitempty"`
			EndTime          string   `json:"end_time,omitempty"`
			DaysOfWeek       []string `json:"days_of_week,omitempty"`
			Priority         int      `json:"priority,omitempty"`
			UserIds          []string `json:"user_ids,omitempty"`
		} `json:"layers"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	current.Layers = make([]store.ScheduleLayerRecord, 0, len(req.Layers))
	for _, l := range req.Layers {
		startDate, err := time.Parse(time.RFC3339, l.StartDate)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid start_date format, use RFC3339")
			return
		}
		layer := store.ScheduleLayerRecord{
			Name:             l.Name,
			RotationType:     l.RotationType,
			RotationInterval: l.RotationInterval,
			StartDate:        startDate,
			Timezone:         l.Timezone,
			StartTime:        l.StartTime,
			EndTime:          l.EndTime,
			DaysOfWeek:       l.DaysOfWeek,
			Priority:         l.Priority,
			UserIds:          l.UserIds,
		}
		if l.EndDate != "" {
			endDate, err := time.Parse(time.RFC3339, l.EndDate)
			if err == nil {
				layer.EndDate = &endDate
			}
		}
		if layer.RotationType == "" {
			layer.RotationType = "weekly"
		}
		if layer.RotationInterval == 0 {
			layer.RotationInterval = 1
		}
		if layer.Timezone == "" {
			layer.Timezone = "UTC"
		}
		if layer.StartTime == "" {
			layer.StartTime = "00:00"
		}
		current.Layers = append(current.Layers, layer)
	}

	memberSet, ok := s.loadTeamMemberSet(w, r.Context(), current)
	if !ok {
		return
	}
	seen := make(map[string]struct{})
	var missing []string
	for _, l := range current.Layers {
		for _, uidStr := range l.UserIds {
			if _, ok := seen[uidStr]; ok {
				continue
			}
			seen[uidStr] = struct{}{}
			layerUserUID, perr := uuid.Parse(uidStr)
			if perr != nil {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid user_id in layer: "+uidStr)
				return
			}
			target, gerr := s.userStore.GetByID(layerUserUID)
			if gerr != nil || target == nil {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "user not found: "+uidStr)
				return
			}
			if memberSet != nil {
				if _, isMember := memberSet[layerUserUID.String()]; !isMember {
					writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "user is not a member of the schedule's team: "+target.DisplayName())
					return
				}
			}
			if target.Phone == "" {
				missing = append(missing, target.DisplayName())
			}
		}
	}
	if len(missing) > 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "the following users must have a phone number to be added to an on-call schedule: "+strings.Join(missing, ", "))
		return
	}

	updated, err := s.onCallStore.UpdateSchedule(r.Context(), uid, current)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to update on-call schedule", "component", "api", "schedule_id", id, "error", err)
		writeInternalError(w, err, "failed to update on-call schedule")
		return
	}

	logger.InfoCtx(r.Context(), "on-call schedule updated", "component", "api", "schedule_id", id)
	s.audit(r, store.AuditScheduleUpdated, map[string]any{
		"schedule_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "schedule_updated", Data: s.enrichSchedule(r.Context(), updated)})
	}
	s.invalidateOnCallCache(r)
	writeData(w, http.StatusOK, s.enrichSchedule(r.Context(), updated))
}

func (s *Server) handleCurrentOnCall(w http.ResponseWriter, r *http.Request, scheduleID string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}

	s.mu.RLock()
	resolver := s.onCallResolver
	s.mu.RUnlock()
	if resolver == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "on-call resolver not configured")
		return
	}

	uid, err := uuid.Parse(scheduleID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule id")
		return
	}

	userID, err := resolver.ResolveWhoIsOnCall(r.Context(), uid, time.Now().UTC())
	if err != nil {
		writeInternalError(w, err, "failed to resolve on-call")
		return
	}

	result := map[string]any{
		"schedule_id": scheduleID,
	}
	if userID != nil {
		result["user_id"] = userID.String()
		if s.userStore != nil {
			if user, err := s.userStore.GetByID(*userID); err == nil && user != nil {
				result["user_display_name"] = user.DisplayName()
			}
		}
	}
	writeData(w, http.StatusOK, result)
}

func (s *Server) handleScheduleTimeline(w http.ResponseWriter, r *http.Request, scheduleID string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}
	s.mu.RLock()
	resolver := s.onCallResolver
	s.mu.RUnlock()
	if resolver == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "on-call resolver not configured")
		return
	}

	if _, ok := s.getScheduleOrError(w, r, scheduleID); !ok {
		return
	}

	uid, err := uuid.Parse(scheduleID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule id")
		return
	}

	from, to, ok := parseTimelineRange(w, r)
	if !ok {
		return
	}

	shifts := resolver.GenerateShifts(r.Context(), uid, from, to)
	writeData(w, http.StatusOK, s.shiftEntries(shifts))
}

// parseTimelineRange extracts the from/to query params for the timeline view.
// Both default to a forward-looking 14-day window starting now when absent, and
// the window is clamped to at most 90 days to bound the cost of shift expansion.
func parseTimelineRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	now := time.Now().UTC()
	from := now
	to := now.Add(14 * 24 * time.Hour)

	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid from, use RFC3339")
			return time.Time{}, time.Time{}, false
		}
		from = parsed.UTC()
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid to, use RFC3339")
			return time.Time{}, time.Time{}, false
		}
		to = parsed.UTC()
	}

	if !to.After(from) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "to must be after from")
		return time.Time{}, time.Time{}, false
	}
	if to.Sub(from) > 90*24*time.Hour {
		to = from.Add(90 * 24 * time.Hour)
	}
	return from, to, true
}

// handleScheduleICal exports a schedule's upcoming shifts as an iCalendar
// (.ics) document over a one-year forward window. The response is served as
// text/calendar so browsers offer it as a download and calendar apps can
// import or subscribe to it.
func (s *Server) handleScheduleICal(w http.ResponseWriter, r *http.Request, scheduleID string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}
	s.mu.RLock()
	resolver := s.onCallResolver
	s.mu.RUnlock()
	if resolver == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "on-call resolver not configured")
		return
	}

	record, ok := s.getScheduleOrError(w, r, scheduleID)
	if !ok {
		return
	}

	uid, err := uuid.Parse(scheduleID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule id")
		return
	}

	from := time.Now().UTC()
	to := from.Add(365 * 24 * time.Hour)
	shifts := resolver.GenerateShifts(r.Context(), uid, from, to)

	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Alga//On-Call//EN\r\n")
	b.WriteString("X-WR-CALNAME:" + icalEscape(s.scheduleDisplayName(r.Context(), record)) + "\r\n")
	b.WriteString("X-WR-TIMEZONE:" + icalEscape(scheduleDisplayTimezone(record)) + "\r\n")

	names := make(map[string]string)
	for _, shift := range shifts {
		uidStr := shift.UserID.String()
		name, cached := names[uidStr]
		if !cached {
			name = uidStr
			if s.userStore != nil {
				if user, err := s.userStore.GetByID(shift.UserID); err == nil && user != nil {
					name = user.DisplayName()
				}
			}
			names[uidStr] = name
		}
		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString("UID:" + scheduleID + "-" + shift.Start.Format("20060102T150405Z") + "@alga\r\n")
		b.WriteString("DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("DTSTART:" + shift.Start.UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("DTEND:" + shift.End.UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("SUMMARY:" + icalEscape("On-Call: "+name) + "\r\n")
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+scheduleID+".ics\"")
	_, _ = w.Write([]byte(b.String()))
}

// icalEscape escapes the characters with special meaning in iCalendar text
// values (comma, semicolon, newline, backslash).
func icalEscape(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\n", "\\n",
		"\r", "",
	)
	return r.Replace(s)
}

func (s *Server) shiftEntries(shifts []oncall.ShiftBoundary) []map[string]any {
	entries := make([]map[string]any, 0, len(shifts))
	names := make(map[string]string)
	for _, shift := range shifts {
		entry := map[string]any{
			"user_id": shift.UserID.String(),
			"start":   shift.Start.Format(time.RFC3339),
			"end":     shift.End.Format(time.RFC3339),
			"source":  shift.Source,
		}
		if name, ok := names[shift.UserID.String()]; ok {
			entry["user_display_name"] = name
		} else if s.userStore != nil {
			if user, err := s.userStore.GetByID(shift.UserID); err == nil && user != nil {
				names[shift.UserID.String()] = user.DisplayName()
				entry["user_display_name"] = user.DisplayName()
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func (s *Server) handleListOverrides(w http.ResponseWriter, r *http.Request, scheduleID string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}

	uid, err := uuid.Parse(scheduleID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule id")
		return
	}

	overrides, err := s.onCallStore.ListOverrides(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to list overrides")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(overrides))
}

func (s *Server) handleCreateOverride(w http.ResponseWriter, r *http.Request, scheduleID string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}

	schedUID, err := uuid.Parse(scheduleID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule id")
		return
	}

	var req struct {
		UserID  string `json:"user_id"`
		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.UserID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "user_id is required")
		return
	}
	userUID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid user_id")
		return
	}
	target, err := s.userStore.GetByID(userUID)
	if err != nil || target == nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "user not found")
		return
	}
	if target.Phone == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, target.DisplayName()+" must have a phone number to be added to an on-call schedule")
		return
	}
	sched, ok := s.getScheduleOrError(w, r, scheduleID)
	if !ok {
		return
	}
	memberSet, ok := s.loadTeamMemberSet(w, r.Context(), sched)
	if !ok {
		return
	}
	if memberSet != nil {
		if _, isMember := memberSet[userUID.String()]; !isMember {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "user is not a member of the schedule's team: "+target.DisplayName())
			return
		}
	}
	startAt, err := time.Parse(time.RFC3339, req.StartAt)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid start_at format, use RFC3339")
		return
	}
	endAt, err := time.Parse(time.RFC3339, req.EndAt)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid end_at format, use RFC3339")
		return
	}

	user := userFromContext(r.Context())
	record := &store.ScheduleOverrideRecord{
		ScheduleID: schedUID,
		UserID:     userUID,
		StartAt:    startAt,
		EndAt:      endAt,
	}
	if user != nil {
		record.CreatedBy = &user.ID
	}

	created, err := s.onCallStore.CreateOverride(r.Context(), record)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to create override", "component", "api", "schedule_id", scheduleID, "error", err)
		writeInternalError(w, err, "failed to create override")
		return
	}

	logger.InfoCtx(r.Context(), "on-call override created", "component", "api", "schedule_id", scheduleID, "user_id", req.UserID)
	s.audit(r, store.AuditScheduleOverrideCreated, map[string]any{
		"schedule_id": scheduleID,
		"user_id":     req.UserID,
	})
	s.invalidateOnCallCache(r)
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handleWhoIsOnCall(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}

	s.mu.RLock()
	resolver := s.onCallResolver
	s.mu.RUnlock()
	if resolver == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "on-call resolver not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resultsJSON, err := s.cache.GetOrSet(ctx, valkey.PrefixOnCallWho+"all", valkey.TTLOnCallWho, func(ctx context.Context) ([]byte, error) {
		schedules, _, err := s.onCallStore.ListSchedules(ctx, 100, 0)
		if err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		results := make([]map[string]any, 0, len(schedules))
		for _, sched := range schedules {
			userID, err := resolver.ResolveWhoIsOnCall(ctx, sched.ID, now)
			if err != nil || userID == nil {
				continue
			}
			entry := map[string]any{
				"schedule_id":   sched.ID.String(),
				"schedule_name": s.scheduleDisplayName(ctx, &sched),
				"user_id":       userID.String(),
			}
			if s.userStore != nil {
				if user, err := s.userStore.GetByID(*userID); err == nil && user != nil {
					entry["user_display_name"] = user.DisplayName()
				}
			}
			results = append(results, entry)
		}
		return json.Marshal(results)
	})
	if err != nil {
		writeInternalError(w, err, "failed to resolve on-call")
		return
	}

	writeRawJSON(w, http.StatusOK, resultsJSON)
}

func (s *Server) handleMyOnCall(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireOnCallStore(w) {
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}

	s.mu.RLock()
	resolver := s.onCallResolver
	s.mu.RUnlock()
	if resolver == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "on-call resolver not configured")
		return
	}

	schedules, _, err := s.onCallStore.ListSchedules(r.Context(), 100, 0)
	if err != nil {
		writeInternalError(w, err, "failed to list schedules")
		return
	}

	myUserID := user.ID.String()
	now := time.Now().UTC()
	results := make([]map[string]any, 0)
	for _, sched := range schedules {
		for _, layer := range sched.Layers {
			found := false
			for _, uid := range layer.UserIds {
				if uid == myUserID {
					found = true
					break
				}
			}
			if !found {
				continue
			}

			resolved, err := resolver.ResolveWhoIsOnCall(r.Context(), sched.ID, now)
			if err != nil || resolved == nil {
				continue
			}
			if resolved.String() != myUserID {
				continue
			}

			results = append(results, map[string]any{
				"schedule_id":   sched.ID.String(),
				"schedule_name": s.scheduleDisplayName(r.Context(), &sched),
				"layer_name":    layer.Name,
			})
		}
	}
	writeData(w, http.StatusOK, results)
}

func (s *Server) handleOnCallOverrideRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/on-call/overrides/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing override id")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.OnCallWrite) {
			return
		}
		id, err := uuid.Parse(suffix)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid override id")
			return
		}
		if err := s.onCallStore.DeleteOverride(r.Context(), id); err != nil {
			writeInternalError(w, err, "failed to delete override")
			return
		}
		s.audit(r, store.AuditScheduleOverrideRemoved, map[string]any{
			"override_id": id.String(),
		})
		s.invalidateOnCallCache(r)
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleOnCallMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	s.mu.RLock()
	resolver := s.onCallResolver
	s.mu.RUnlock()
	if resolver == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "on-call resolver not available")
		return
	}

	scheduleIDStr := r.URL.Query().Get("schedule_id")
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")
	groupBy := r.URL.Query().Get("group_by")

	if scheduleIDStr == "" || startDateStr == "" || endDateStr == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "schedule_id, start_date, end_date are required")
		return
	}
	scheduleID, err := uuid.Parse(scheduleIDStr)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid schedule_id")
		return
	}
	startDate, err := time.Parse(time.RFC3339, startDateStr)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid start_date")
		return
	}
	endDate, err := time.Parse(time.RFC3339, endDateStr)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid end_date")
		return
	}

	shifts := resolver.GenerateShifts(r.Context(), scheduleID, startDate, endDate)

	type shiftMetric struct {
		UserID             string  `json:"user_id"`
		UserDisplayName    string  `json:"user_display_name"`
		ShiftStart         string  `json:"shift_start"`
		ShiftEnd           string  `json:"shift_end"`
		AlertsReceived     int     `json:"alerts_received"`
		AlertsAcknowledged int     `json:"alerts_acknowledged"`
		AlertsResolved     int     `json:"alerts_resolved"`
		AlertsMissed       int     `json:"alerts_missed"`
		AvgAckTimeSeconds  float64 `json:"avg_ack_time_seconds"`
	}

	var result []shiftMetric
	names := make(map[string]string)
	for _, shift := range shifts {
		uid := shift.UserID.String()
		name := names[uid]
		if name == "" && s.userStore != nil {
			if user, err := s.userStore.GetByID(shift.UserID); err == nil && user != nil {
				name = user.DisplayName()
				names[uid] = name
			}
		}
		result = append(result, shiftMetric{
			UserID:          uid,
			UserDisplayName: name,
			ShiftStart:      shift.Start.Format(time.RFC3339),
			ShiftEnd:        shift.End.Format(time.RFC3339),
		})
	}

	if groupBy == "user" {
		userMap := make(map[string]shiftMetric)
		for _, m := range result {
			entry := userMap[m.UserID]
			entry.UserID = m.UserID
			entry.UserDisplayName = m.UserDisplayName
			entry.AlertsReceived += m.AlertsReceived
			entry.AlertsAcknowledged += m.AlertsAcknowledged
			entry.AlertsResolved += m.AlertsResolved
			entry.AlertsMissed += m.AlertsMissed
			entry.AvgAckTimeSeconds += m.AvgAckTimeSeconds
			userMap[m.UserID] = entry
		}
		users := make([]shiftMetric, 0, len(userMap))
		for _, v := range userMap {
			users = append(users, v)
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": users})
		return
	}

	// Aggregate per-shift metrics into a summary. Note: per-shift alert
	// counts are populated by callers above (currently the resolver does not
	// fill them, so these sums may be 0 until that wiring lands); the
	// aggregation itself is correct regardless.
	var totalAck, totalResolved, totalReceived, totalMissed int
	var ackTimeSum float64
	var ackTimeCount int
	for _, m := range result {
		totalReceived += m.AlertsReceived
		totalAck += m.AlertsAcknowledged
		totalResolved += m.AlertsResolved
		totalMissed += m.AlertsMissed
		if m.AlertsReceived > 0 {
			ackTimeSum += m.AvgAckTimeSeconds * float64(m.AlertsReceived)
			ackTimeCount += m.AlertsReceived
		}
	}
	avgAckRate := 0.0
	if totalReceived > 0 {
		avgAckRate = float64(totalAck) / float64(totalReceived)
	}
	avgAckTime := 0.0
	if ackTimeCount > 0 {
		avgAckTime = ackTimeSum / float64(ackTimeCount)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"shifts": result,
		"summary": map[string]any{
			"total_shifts":         len(result),
			"avg_ack_rate":         avgAckRate,
			"avg_ack_time_seconds": avgAckTime,
		},
	})
}

func (s *Server) invalidateOnCallCache(r *http.Request) {
	if s.cache != nil {
		_ = s.cache.InvalidatePrefix(r.Context(), valkey.PrefixOnCallWho)
		_ = s.cache.Invalidate(r.Context(), valkey.PrefixOnCallSchedules)
	}
}
