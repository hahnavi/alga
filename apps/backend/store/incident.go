package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"

	"alga/incident"
	"alga/incmetrics"
)

type IncidentRecord struct {
	ID                       uuid.UUID      `json:"id"`
	IncidentNumber           int64          `json:"incident_number"`
	Title                    string         `json:"title"`
	Description              string         `json:"description"`
	Summary                  string         `json:"summary,omitempty"`
	Status                   string         `json:"status"`
	Severity                 string         `json:"severity"`
	ImpactLevel              string         `json:"impact_level"`
	Priority                 string         `json:"priority"`
	IncidentType             string         `json:"incident_type"`
	CommanderID              *uuid.UUID     `json:"commander_id,omitempty"`
	CommunicatorID           *uuid.UUID     `json:"communicator_id,omitempty"`
	OnCallResponderID        *uuid.UUID     `json:"on_call_responder_id,omitempty"`
	CommanderAssigneeType    string         `json:"commander_assignee_type"`
	CommunicatorAssigneeType string         `json:"communicator_assignee_type"`
	ServiceID                *uuid.UUID     `json:"service_id,omitempty"`
	EscalationPolicyID       *uuid.UUID     `json:"escalation_policy_id,omitempty"`
	ConferenceURL            string         `json:"conference_url,omitempty"`
	SlackChannelID           string         `json:"slack_channel_id,omitempty"`
	SlackChannelName         string         `json:"slack_channel_name,omitempty"`
	SlackChannelArchived     bool           `json:"slack_channel_archived"`
	WarRoomChannelID         string         `json:"war_room_channel_id,omitempty"`
	WarRoomChannelProvider   string         `json:"war_room_channel_provider,omitempty"`
	GoogleMeetSpaceName      string         `json:"google_meet_space_name,omitempty"`
	StatusPageIncidentID     string         `json:"status_page_incident_id,omitempty"`
	SLATargetRespondAt       *time.Time     `json:"sla_target_respond_at,omitempty"`
	SLATargetResolveAt       *time.Time     `json:"sla_target_resolve_at,omitempty"`
	SLAAcknowledgedAt        *time.Time     `json:"sla_acknowledged_at,omitempty"`
	SLAResolvedAt            *time.Time     `json:"sla_resolved_at,omitempty"`
	StartedAt                *time.Time     `json:"started_at,omitempty"`
	MitigatedAt              *time.Time     `json:"mitigated_at,omitempty"`
	ResolvedAt               *time.Time     `json:"resolved_at,omitempty"`
	ClosedAt                 *time.Time     `json:"closed_at,omitempty"`
	TriagedAt                *time.Time     `json:"triaged_at,omitempty"`
	TriageReport             map[string]any `json:"triage_report,omitempty"`
	AutoConfirmed            bool           `json:"auto_confirmed"`
	Tags                     []string       `json:"tags,omitempty"`
	CustomFields             map[string]any `json:"custom_fields,omitempty"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	// DeletedAt is non-nil when the incident has been soft-deleted. Tombstone-only.
	DeletedAt *time.Time                    `json:"deleted_at,omitempty"`
	Timeline  []IncidentTimelineEntryRecord `json:"timeline,omitempty"`
}

type IncidentTimelineEntryRecord struct {
	ID             uuid.UUID      `json:"id"`
	IncidentNumber int64          `json:"-"`
	EventType      string         `json:"event_type"`
	ActorID        *uuid.UUID     `json:"actor_id,omitempty"`
	ActorType      string         `json:"actor_type"`
	Message        string         `json:"message"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type IncidentListFilter struct {
	Status      string     `json:"status,omitempty"`
	Severity    string     `json:"severity,omitempty"`
	Priority    string     `json:"priority,omitempty"`
	ServiceID   string     `json:"service_id,omitempty"`
	CommanderID string     `json:"commander_id,omitempty"`
	Search      string     `json:"search,omitempty"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	Limit       int        `json:"limit,omitempty"`
	Skip        int        `json:"skip,omitempty"`
	Sort        string     `json:"sort,omitempty"`
}

var ErrIncidentStatusConflict = errors.New("incident status changed concurrently")

// IncidentTerminalStatuses are the incident statuses that no longer count as
// active. Alerts linked only to incidents in these states remain eligible for
// the stale alert sweep.
var IncidentTerminalStatuses = []string{"resolved", "closed", "cancelled"}

type IncidentStore interface {
	ReserveIncidentNumber(ctx context.Context) (int64, error)
	CreateIncident(ctx context.Context, record *IncidentRecord) (*IncidentRecord, error)
	GetIncident(ctx context.Context, incidentNumber int64) (*IncidentRecord, error)
	GetIncidentByID(ctx context.Context, id uuid.UUID) (*IncidentRecord, error)
	UpdateIncident(ctx context.Context, incidentNumber int64, record *IncidentRecord) (*IncidentRecord, error)
	DeleteIncident(ctx context.Context, incidentNumber int64) error
	ListIncidents(ctx context.Context, filter IncidentListFilter) ([]IncidentRecord, int64, error)
	ListSLAEligibleIncidents(ctx context.Context) ([]IncidentRecord, error)
	TransitionIncidentStatus(ctx context.Context, incidentNumber int64, fromStatuses []string, toStatus string) error
	AddTimelineEntry(ctx context.Context, record *IncidentTimelineEntryRecord) error
	GetTimeline(ctx context.Context, incidentNumber int64) ([]IncidentTimelineEntryRecord, error)
	GetIncidentMetrics(ctx context.Context, startDate, endDate time.Time) (*incmetrics.Metrics, error)
	CountActiveByService(ctx context.Context) (map[string]int64, error)
	CountActiveByServiceID(ctx context.Context, serviceID string) (int, error)
	CountActiveByPriority(ctx context.Context, serviceID string) (map[string]int, error)
	ListActiveSummarizableIncidents(ctx context.Context) ([]IncidentRecord, error)
	ListActiveIncidents(ctx context.Context) ([]IncidentRecord, error)
	GetIncidentBySlackChannel(ctx context.Context, channelID string) (*IncidentRecord, error)
	SetIncidentWarRoomMeet(ctx context.Context, incidentNumber int64, spaceName, conferenceURL string) error
}

type pgIncidentStore struct {
	pgStoreBase
}

func newPGIncidentStore(db *bun.DB) IncidentStore {
	return &pgIncidentStore{pgStoreBase{db: db}}
}

func (s *pgIncidentStore) ReserveIncidentNumber(ctx context.Context) (int64, error) {
	return nextSeq(ctx, s.db, "incident_number_seq")
}

func (s *pgIncidentStore) CreateIncident(ctx context.Context, record *IncidentRecord) (*IncidentRecord, error) {
	now := time.Now().UTC()
	if record.Status == "" {
		record.Status = "detected"
	}
	if record.Severity == "" {
		record.Severity = "warning"
	}
	if record.ImpactLevel == "" {
		record.ImpactLevel = "medium"
	}
	if record.Priority == "" {
		record.Priority = "P4"
	}
	if record.IncidentType == "" {
		record.IncidentType = "real"
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}

	if record.IncidentNumber == 0 {
		n, err := nextSeq(ctx, s.db, "incident_number_seq")
		if err != nil {
			return nil, fmt.Errorf("failed to allocate incident number: %w", err)
		}
		record.IncidentNumber = n
	}

	var slackChannelID *string
	if record.SlackChannelID != "" {
		slackChannelID = &record.SlackChannelID
	}

	m := &models.Incident{
		BaseModel: models.BaseModel{
			ID:        models.NewUUID(),
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		},
		IncidentNumber:       record.IncidentNumber,
		Title:                record.Title,
		Description:          record.Description,
		Status:               record.Status,
		Severity:             record.Severity,
		ImpactLevel:          record.ImpactLevel,
		Priority:             record.Priority,
		IncidentType:         record.IncidentType,
		CommanderID:          record.CommanderID,
		CommunicatorID:       record.CommunicatorID,
		OnCallResponderID:    record.OnCallResponderID,
		ServiceID:            record.ServiceID,
		EscalationPolicyID:   record.EscalationPolicyID,
		ConferenceURL:        record.ConferenceURL,
		SlackChannelID:       slackChannelID,
		SlackChannelName:     record.SlackChannelName,
		SlackChannelArchived: record.SlackChannelArchived,
		StatusPageIncidentID: record.StatusPageIncidentID,
		SLATargetRespondAt:   record.SLATargetRespondAt,
		SLATargetResolveAt:   record.SLATargetResolveAt,
		SLAAcknowledgedAt:    record.SLAAcknowledgedAt,
		SLAResolvedAt:        record.SLAResolvedAt,
		StartedAt:            record.StartedAt,
		MitigatedAt:          record.MitigatedAt,
		ResolvedAt:           record.ResolvedAt,
		ClosedAt:             record.ClosedAt,
		TriagedAt:            record.TriagedAt,
		TriageReport:         record.TriageReport,
		AutoConfirmed:        record.AutoConfirmed,
		Tags:                 record.Tags,
		CustomFields:         record.CustomFields,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident: %w", err)
	}
	record.ID = m.ID
	return record, nil
}

func (s *pgIncidentStore) GetIncident(ctx context.Context, incidentNumber int64) (*IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).Where("incident_number = ?", incidentNumber).Scan(ctx)
	if err != nil {
		return handleQueryErr[*IncidentRecord](err, "incident")
	}

	rec := s.toIncidentRecord(&inc)

	// Load timeline
	var entries []models.IncidentTimelineEntry
	err = s.db.NewSelect().Model(&entries).
		Where("incident_id = ?", inc.ID).
		Order("created_at ASC").
		Scan(ctx)
	if err == nil {
		rec.Timeline = make([]IncidentTimelineEntryRecord, 0, len(entries))
		for i := range entries {
			e := &entries[i]
			rec.Timeline = append(rec.Timeline, IncidentTimelineEntryRecord{
				ID:        e.ID,
				EventType: e.EventType,
				ActorID:   e.ActorID,
				ActorType: e.ActorType,
				Message:   e.Message,
				Metadata:  e.Metadata,
				CreatedAt: e.CreatedAt,
			})
		}
	}

	return rec, nil
}

func (s *pgIncidentStore) GetIncidentByID(ctx context.Context, id uuid.UUID) (*IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*IncidentRecord](err, "incident")
	}

	rec := s.toIncidentRecord(&inc)

	// Load timeline
	var entries []models.IncidentTimelineEntry
	err = s.db.NewSelect().Model(&entries).
		Where("incident_id = ?", inc.ID).
		Order("created_at ASC").
		Scan(ctx)
	if err == nil {
		rec.Timeline = make([]IncidentTimelineEntryRecord, 0, len(entries))
		for i := range entries {
			e := &entries[i]
			rec.Timeline = append(rec.Timeline, IncidentTimelineEntryRecord{
				ID:        e.ID,
				EventType: e.EventType,
				ActorID:   e.ActorID,
				ActorType: e.ActorType,
				Message:   e.Message,
				Metadata:  e.Metadata,
				CreatedAt: e.CreatedAt,
			})
		}
	}

	return rec, nil
}

func (s *pgIncidentStore) UpdateIncident(ctx context.Context, incidentNumber int64, record *IncidentRecord) (*IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", incidentNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to lookup incident for update: %w", err)
	}

	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.Incident)(nil)).
		Set("title = ?", record.Title).
		Set("description = ?", record.Description).
		Set("summary = ?", record.Summary).
		Set("severity = ?", record.Severity).
		Set("impact_level = ?", record.ImpactLevel).
		Set("priority = ?", record.Priority).
		Set("incident_type = ?", record.IncidentType).
		Set("conference_url = ?", record.ConferenceURL).
		Set("status_page_incident_id = ?", record.StatusPageIncidentID).
		Set("updated_at = ?", now).
		Where("id = ?", inc.ID)

	if record.SlackChannelID != "" {
		q = q.Set("slack_channel_id = ?", record.SlackChannelID)
	} else {
		q = q.Set("slack_channel_id = NULL")
	}
	if record.SlackChannelName != "" {
		q = q.Set("slack_channel_name = ?", record.SlackChannelName)
	}
	q = q.Set("slack_channel_archived = ?", record.SlackChannelArchived)

	if record.Status != "" {
		q = q.Set("status = ?", record.Status)
	}
	if record.CommanderID != nil {
		q = q.Set("commander_id = ?", *record.CommanderID)
	} else {
		q = q.Set("commander_id = NULL")
	}
	if record.CommunicatorID != nil {
		q = q.Set("communicator_id = ?", *record.CommunicatorID)
	} else {
		q = q.Set("communicator_id = NULL")
	}
	if record.OnCallResponderID != nil {
		q = q.Set("on_call_responder_id = ?", *record.OnCallResponderID)
	} else {
		q = q.Set("on_call_responder_id = NULL")
	}
	if record.ServiceID != nil {
		q = q.Set("service_id = ?", *record.ServiceID)
	} else {
		q = q.Set("service_id = NULL")
	}
	if record.EscalationPolicyID != nil {
		q = q.Set("escalation_policy_id = ?", *record.EscalationPolicyID)
	} else {
		q = q.Set("escalation_policy_id = NULL")
	}
	if record.SLATargetRespondAt != nil {
		q = q.Set("sla_target_respond_at = ?", *record.SLATargetRespondAt)
	}
	if record.SLATargetResolveAt != nil {
		q = q.Set("sla_target_resolve_at = ?", *record.SLATargetResolveAt)
	}
	if record.StartedAt != nil {
		q = q.Set("started_at = ?", *record.StartedAt)
	}
	if record.MitigatedAt != nil {
		q = q.Set("mitigated_at = ?", *record.MitigatedAt)
	}
	if record.ResolvedAt != nil {
		q = q.Set("resolved_at = ?", *record.ResolvedAt)
	}
	if record.ClosedAt != nil {
		q = q.Set("closed_at = ?", *record.ClosedAt)
	}
	if record.TriagedAt != nil {
		q = q.Set("triaged_at = ?", *record.TriagedAt)
	}
	if record.TriageReport != nil {
		q = q.Set("triage_report = ?", record.TriageReport)
	}
	q = q.Set("auto_confirmed = ?", record.AutoConfirmed)
	if record.Tags != nil {
		q = q.Set("tags = ?", record.Tags)
	}
	if record.CustomFields != nil {
		q = q.Set("custom_fields = ?", record.CustomFields)
	}

	_, err = q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update incident: %w", err)
	}

	// Re-fetch to return the updated record
	var updated models.Incident
	if err := s.db.NewSelect().Model(&updated).Where("id = ?", inc.ID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to re-fetch updated incident: %w", err)
	}
	return s.toIncidentRecord(&updated), nil
}

func (s *pgIncidentStore) DeleteIncident(ctx context.Context, incidentNumber int64) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", incidentNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to lookup incident: %w", err)
	}

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := hardDeleteIncidentCascade(ctx, tx, inc.ID, inc.IncidentNumber); err != nil {
			return err
		}

		now := time.Now().UTC()
		res, err := tx.NewUpdate().Model((*models.Incident)(nil)).
			Set("deleted_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", inc.ID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to soft-delete incident: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil
	})
}

// ExpungeSoftDeletedIncidentsChildren hard-deletes the investigation artifacts
// of every already-tombstoned incident. One-time idempotent backfill. Not part
// of the IncidentStore interface; callers reach it via type assertion.
func (s *pgIncidentStore) ExpungeSoftDeletedIncidentsChildren(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var rows []models.Incident
	err := s.db.NewSelect().Model(&rows).WhereDeleted().Scan(ctx)
	if err != nil {
		return 0, fmt.Errorf("query soft-deleted incidents: %w", err)
	}
	processed := 0
	for i := range rows {
		inc := &rows[i]
		err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			return hardDeleteIncidentCascade(ctx, tx, inc.ID, inc.IncidentNumber)
		})
		if err != nil {
			return processed, fmt.Errorf("expunge incident %d: %w", inc.IncidentNumber, err)
		}
		processed++
	}
	return processed, nil
}

func (s *pgIncidentStore) ListIncidents(ctx context.Context, filter IncidentListFilter) ([]IncidentRecord, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Build count query
	countQ := s.db.NewSelect().Model((*models.Incident)(nil)).Where("deleted_at IS NULL")
	if filter.Status != "" {
		countQ = countQ.Where("status = ?", filter.Status)
	}
	if filter.Severity != "" {
		countQ = countQ.Where("severity = ?", filter.Severity)
	}
	if filter.Priority != "" {
		countQ = countQ.Where("priority = ?", filter.Priority)
	}
	if filter.ServiceID != "" {
		if sid, err := uuid.Parse(filter.ServiceID); err == nil {
			countQ = countQ.Where("service_id = ?", sid)
		}
	}
	if filter.CommanderID != "" {
		if cid, err := uuid.Parse(filter.CommanderID); err == nil {
			countQ = countQ.Where("commander_id = ?", cid)
		}
	}
	if filter.Search != "" {
		countQ = countQ.Where("title ILIKE ?", "%"+filter.Search+"%")
	}
	if filter.StartDate != nil {
		countQ = countQ.Where("created_at >= ?", *filter.StartDate)
	}
	if filter.EndDate != nil {
		countQ = countQ.Where("created_at <= ?", *filter.EndDate)
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count incidents: %w", err)
	}

	// Build list query
	var incs []models.Incident
	listQ := s.db.NewSelect().Model(&incs).Where("deleted_at IS NULL")
	if filter.Status != "" {
		listQ = listQ.Where("status = ?", filter.Status)
	}
	if filter.Severity != "" {
		listQ = listQ.Where("severity = ?", filter.Severity)
	}
	if filter.Priority != "" {
		listQ = listQ.Where("priority = ?", filter.Priority)
	}
	if filter.ServiceID != "" {
		if sid, err := uuid.Parse(filter.ServiceID); err == nil {
			listQ = listQ.Where("service_id = ?", sid)
		}
	}
	if filter.CommanderID != "" {
		if cid, err := uuid.Parse(filter.CommanderID); err == nil {
			listQ = listQ.Where("commander_id = ?", cid)
		}
	}
	if filter.Search != "" {
		listQ = listQ.Where("title ILIKE ?", "%"+filter.Search+"%")
	}
	if filter.StartDate != nil {
		listQ = listQ.Where("created_at >= ?", *filter.StartDate)
	}
	if filter.EndDate != nil {
		listQ = listQ.Where("created_at <= ?", *filter.EndDate)
	}

	switch filter.Sort {
	case "created_at", "":
		listQ = listQ.Order("created_at DESC")
	case "-created_at":
		listQ = listQ.Order("created_at ASC")
	case "updated_at":
		listQ = listQ.Order("updated_at DESC")
	case "-updated_at":
		listQ = listQ.Order("updated_at ASC")
	case "severity":
		listQ = listQ.Order("severity ASC")
	case "-severity":
		listQ = listQ.Order("severity DESC")
	case "priority", "priority_asc":
		listQ = listQ.Order("created_at ASC")
	case "priority_desc":
		listQ = listQ.Order("created_at DESC")
	default:
		listQ = listQ.Order("created_at DESC")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	listQ = listQ.Limit(limit)
	if filter.Skip > 0 {
		listQ = listQ.Offset(filter.Skip)
	}

	err = listQ.Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list incidents: %w", err)
	}

	records := make([]IncidentRecord, 0, len(incs))
	for i := range incs {
		records = append(records, *s.toIncidentRecord(&incs[i]))
	}

	switch filter.Sort {
	case "priority", "priority_asc":
		slices.SortFunc(records, func(a, b IncidentRecord) int {
			return cmp.Compare(incident.PriorityRank(a.Priority), incident.PriorityRank(b.Priority))
		})
	case "priority_desc":
		slices.SortFunc(records, func(a, b IncidentRecord) int {
			return cmp.Compare(incident.PriorityRank(b.Priority), incident.PriorityRank(a.Priority))
		})
	}

	return records, int64(total), nil
}

func applyStatusTimestampsBun(q *bun.UpdateQuery, toStatus string, now time.Time) *bun.UpdateQuery {
	switch toStatus {
	case "triaging":
		q = q.Set("triaged_at = ?", now)
	case "active":
		q = q.Set("sla_acknowledged_at = ?", now)
	case "mitigated":
		q = q.Set("mitigated_at = ?", now)
	case "resolved":
		q = q.Set("resolved_at = ?", now).Set("sla_resolved_at = ?", now)
	case "closed":
		q = q.Set("closed_at = ?", now)
	}
	return q
}

// SetIncidentWarRoomMeet persists the Google Meet war room for an incident.
// spaceName/conferenceURL are written as a pair: creating a Meet space makes it
// the incident's conference bridge (overwriting any prior conference_url), and
// unlinking (empty strings) clears both. Pass empty strings to unlink.
func (s *pgIncidentStore) SetIncidentWarRoomMeet(ctx context.Context, incidentNumber int64, spaceName, conferenceURL string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.Incident)(nil)).
		Set("google_meet_space_name = ?", spaceName).
		Set("conference_url = ?", conferenceURL).
		Set("updated_at = ?", time.Now().UTC()).
		Where("incident_number = ?", incidentNumber).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("set incident google meet war room: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIncidentNotFound
	}
	return nil
}

func (s *pgIncidentStore) TransitionIncidentStatus(ctx context.Context, incidentNumber int64, fromStatuses []string, toStatus string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.Incident)(nil)).
		Set("status = ?", toStatus).
		Set("updated_at = ?", now).
		Where("incident_number = ?", incidentNumber).
		Where("status IN (?)", bun.List(fromStatuses)).
		Where("deleted_at IS NULL")

	q = applyStatusTimestampsBun(q, toStatus, now)

	res, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to transition incident status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIncidentStatusConflict
	}
	return nil
}

func (s *pgIncidentStore) AddTimelineEntry(ctx context.Context, record *IncidentTimelineEntryRecord) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", record.IncidentNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to find incident for timeline entry: %w", err)
	}

	m := &models.IncidentTimelineEntry{
		ID:         models.NewUUID(),
		EventType:  record.EventType,
		ActorID:    record.ActorID,
		ActorType:  record.ActorType,
		Message:    record.Message,
		Metadata:   record.Metadata,
		CreatedAt:  time.Now().UTC(),
		IncidentID: inc.ID,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to add timeline entry: %w", err)
	}
	record.ID = m.ID
	return nil
}

func (s *pgIncidentStore) GetTimeline(ctx context.Context, incidentNumber int64) ([]IncidentTimelineEntryRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).Where("incident_number = ?", incidentNumber).Scan(ctx)
	if err != nil {
		return handleQueryErr[[]IncidentTimelineEntryRecord](err, "incident timeline")
	}

	var entries []models.IncidentTimelineEntry
	err = s.db.NewSelect().Model(&entries).
		Where("incident_id = ?", inc.ID).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeline entries: %w", err)
	}

	records := make([]IncidentTimelineEntryRecord, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		records = append(records, IncidentTimelineEntryRecord{
			ID:        e.ID,
			EventType: e.EventType,
			ActorID:   e.ActorID,
			ActorType: e.ActorType,
			Message:   e.Message,
			Metadata:  e.Metadata,
			CreatedAt: e.CreatedAt,
		})
	}
	return records, nil
}

func (s *pgIncidentStore) GetIncidentMetrics(ctx context.Context, startDate, endDate time.Time) (*incmetrics.Metrics, error) {
	var incs []models.Incident
	err := s.db.NewSelect().Model(&incs).
		Where("created_at >= ?", startDate).
		Where("created_at <= ?", endDate).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query incidents for metrics: %w", err)
	}

	data := make([]incmetrics.IncidentData, 0, len(incs))
	for i := range incs {
		inc := &incs[i]
		d := incmetrics.IncidentData{
			CreatedAt:         inc.CreatedAt,
			AcknowledgedAt:    inc.SLAAcknowledgedAt,
			MitigatedAt:       inc.MitigatedAt,
			ResolvedAt:        inc.ResolvedAt,
			Severity:          inc.Severity,
			SLATargetRespond:  inc.SLATargetRespondAt,
			SLATargetResolve:  inc.SLATargetResolveAt,
			SLAAcknowledgedAt: inc.SLAAcknowledgedAt,
			SLAResolvedAt:     inc.SLAResolvedAt,
		}
		if inc.ServiceID != nil {
			d.ServiceID = inc.ServiceID.String()
		}
		data = append(data, d)
	}

	return incmetrics.ComputeMetrics(data, 30), nil
}

func (s *pgIncidentStore) ListSLAEligibleIncidents(ctx context.Context) ([]IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var incs []models.Incident
	err := s.db.NewSelect().Model(&incs).
		Where("status IN (?)", bun.List([]string{"detected", "triaging", "active", "mitigated"})).
		Where("(sla_target_respond_at IS NOT NULL OR sla_target_resolve_at IS NOT NULL)").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list SLA-eligible incidents: %w", err)
	}

	records := make([]IncidentRecord, 0, len(incs))
	for i := range incs {
		records = append(records, *s.toIncidentRecord(&incs[i]))
	}
	return records, nil
}

func (s *pgIncidentStore) CountActiveByService(ctx context.Context) (map[string]int64, error) {
	var incs []models.Incident
	err := s.db.NewSelect().Model(&incs).
		Column("service_id").
		Where("status NOT IN (?)", bun.List(IncidentTerminalStatuses)).
		Where("service_id IS NOT NULL").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active incidents by service: %w", err)
	}

	counts := make(map[string]int64)
	for i := range incs {
		if incs[i].ServiceID != nil {
			counts[incs[i].ServiceID.String()]++
		}
	}
	return counts, nil
}

func (s *pgIncidentStore) CountActiveByServiceID(ctx context.Context, serviceID string) (int, error) {
	svcUUID, err := uuid.Parse(serviceID)
	if err != nil {
		return 0, fmt.Errorf("invalid service ID: %w", err)
	}
	return s.db.NewSelect().Model((*models.Incident)(nil)).
		Where("status NOT IN (?)", bun.List(IncidentTerminalStatuses)).
		Where("service_id = ?", svcUUID).
		Where("deleted_at IS NULL").
		Count(ctx)
}

func (s *pgIncidentStore) CountActiveByPriority(ctx context.Context, serviceID string) (map[string]int, error) {
	svcUUID, err := uuid.Parse(serviceID)
	if err != nil {
		return nil, fmt.Errorf("invalid service ID: %w", err)
	}

	ctx, cancel := pgctx(ctx)
	defer cancel()

	type groupResult struct {
		Priority string `bun:"priority"`
		Count    int    `bun:"count"`
	}

	var results []groupResult
	err = s.db.NewSelect().
		TableExpr("incidents").
		ColumnExpr("priority, COUNT(*) as count").
		Where("status NOT IN (?)", bun.List([]string{"resolved", "closed", "cancelled"})).
		Where("service_id = ?", svcUUID).
		Where("deleted_at IS NULL").
		Group("priority").
		Scan(ctx, &results)
	if err != nil {
		return nil, fmt.Errorf("failed to query incidents by priority: %w", err)
	}

	result := make(map[string]int)
	for _, r := range results {
		p := r.Priority
		if p == "" {
			p = "P5"
		}
		result[p] = r.Count
	}
	return result, nil
}

func (s *pgIncidentStore) ListActiveSummarizableIncidents(ctx context.Context) ([]IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var rows []models.Incident
	err := s.db.NewSelect().Model(&rows).
		Where("status IN (?)", bun.List([]string{"detected", "triaging", "active", "mitigated"})).
		Where("slack_channel_id != ''").
		Where("deleted_at IS NULL").
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("query summarizable incidents: %w", err)
	}
	out := make([]IncidentRecord, 0, len(rows))
	for i := range rows {
		out = append(out, *s.toIncidentRecord(&rows[i]))
	}
	return out, nil
}

func (s *pgIncidentStore) ListActiveIncidents(ctx context.Context) ([]IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var rows []models.Incident
	err := s.db.NewSelect().Model(&rows).
		Where("status NOT IN (?)", bun.List([]string{"resolved", "closed", "cancelled"})).
		Where("deleted_at IS NULL").
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("query active incidents: %w", err)
	}
	out := make([]IncidentRecord, 0, len(rows))
	for i := range rows {
		out = append(out, *s.toIncidentRecord(&rows[i]))
	}
	return out, nil
}

func (s *pgIncidentStore) GetIncidentBySlackChannel(ctx context.Context, channelID string) (*IncidentRecord, error) {
	var item models.Incident
	err := s.db.NewSelect().Model(&item).Where("slack_channel_id = ?", channelID).Scan(ctx)
	if err != nil {
		return handleQueryErr[*IncidentRecord](err, "incident by slack channel")
	}
	return s.toIncidentRecord(&item), nil
}

func (s *pgIncidentStore) toIncidentRecord(inc *models.Incident) *IncidentRecord {
	rec := &IncidentRecord{
		ID:                       inc.ID,
		IncidentNumber:           inc.IncidentNumber,
		Title:                    inc.Title,
		Description:              inc.Description,
		Summary:                  inc.Summary,
		Status:                   inc.Status,
		Severity:                 inc.Severity,
		ImpactLevel:              inc.ImpactLevel,
		Priority:                 inc.Priority,
		IncidentType:             inc.IncidentType,
		CommanderID:              inc.CommanderID,
		CommunicatorID:           inc.CommunicatorID,
		OnCallResponderID:        inc.OnCallResponderID,
		CommanderAssigneeType:    inc.CommanderAssigneeType,
		CommunicatorAssigneeType: inc.CommunicatorAssigneeType,
		ServiceID:                inc.ServiceID,
		EscalationPolicyID:       inc.EscalationPolicyID,
		ConferenceURL:            inc.ConferenceURL,
		SlackChannelID: func() string {
			if inc.SlackChannelID != nil {
				return *inc.SlackChannelID
			}
			return ""
		}(),
		SlackChannelName:     inc.SlackChannelName,
		SlackChannelArchived: inc.SlackChannelArchived,
		WarRoomChannelID: func() string {
			if inc.WarRoomChannelID != nil {
				return *inc.WarRoomChannelID
			}
			return ""
		}(),
		WarRoomChannelProvider: func() string {
			if inc.WarRoomChannelProvider != nil {
				return *inc.WarRoomChannelProvider
			}
			return ""
		}(),
		GoogleMeetSpaceName: func() string {
			if inc.GoogleMeetSpaceName != nil {
				return *inc.GoogleMeetSpaceName
			}
			return ""
		}(),
		StatusPageIncidentID: inc.StatusPageIncidentID,
		SLATargetRespondAt:   inc.SLATargetRespondAt,
		SLATargetResolveAt:   inc.SLATargetResolveAt,
		SLAAcknowledgedAt:    inc.SLAAcknowledgedAt,
		SLAResolvedAt:        inc.SLAResolvedAt,
		StartedAt:            inc.StartedAt,
		MitigatedAt:          inc.MitigatedAt,
		ResolvedAt:           inc.ResolvedAt,
		ClosedAt:             inc.ClosedAt,
		TriagedAt:            inc.TriagedAt,
		TriageReport:         inc.TriageReport,
		AutoConfirmed:        inc.AutoConfirmed,
		Tags:                 inc.Tags,
		CustomFields:         inc.CustomFields,
		CreatedAt:            inc.CreatedAt,
		UpdatedAt:            inc.UpdatedAt,
		DeletedAt:            inc.DeletedAt,
	}
	return rec
}
