package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entincident "alga/ent/incident"
	"alga/ent/incidenttimelineentry"
	"alga/ent/predicate"

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
	UpdateIncidentStatus(ctx context.Context, incidentNumber int64, status string) error
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

func newPGIncidentStore(client *ent.Client) IncidentStore {
	return &pgIncidentStore{pgStoreBase{client: client}}
}

func (s *pgIncidentStore) ReserveIncidentNumber(ctx context.Context) (int64, error) {
	return nextPgCounter(ctx, s.client, "incident_number")
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
		n, err := nextPgCounter(ctx, s.client, "incident_number")
		if err != nil {
			return nil, fmt.Errorf("failed to allocate incident number: %w", err)
		}
		record.IncidentNumber = n
	}

	b := s.client.Incident.Create().
		SetIncidentNumber(record.IncidentNumber).
		SetTitle(record.Title).
		SetDescription(record.Description).
		SetStatus(entincident.Status(record.Status)).
		SetSeverity(entincident.Severity(record.Severity)).
		SetImpactLevel(entincident.ImpactLevel(record.ImpactLevel)).
		SetPriority(entincident.Priority(record.Priority)).
		SetIncidentType(entincident.IncidentType(record.IncidentType)).
		SetConferenceURL(record.ConferenceURL).
		SetStatusPageIncidentID(record.StatusPageIncidentID).
		SetCreatedAt(record.CreatedAt).
		SetUpdatedAt(record.UpdatedAt)

	if record.SlackChannelID != "" {
		b.SetSlackChannelID(record.SlackChannelID)
	}
	if record.SlackChannelName != "" {
		b.SetSlackChannelName(record.SlackChannelName)
	}
	b.SetSlackChannelArchived(record.SlackChannelArchived)

	if record.CommanderID != nil {
		b.SetCommanderID(*record.CommanderID)
	}
	if record.CommunicatorID != nil {
		b.SetCommunicatorID(*record.CommunicatorID)
	}
	if record.OnCallResponderID != nil {
		b.SetOnCallResponderID(*record.OnCallResponderID)
	}
	if record.ServiceID != nil {
		b.SetServiceID(*record.ServiceID)
	}
	if record.EscalationPolicyID != nil {
		b.SetEscalationPolicyID(*record.EscalationPolicyID)
	}
	if record.SLATargetRespondAt != nil {
		b.SetSLATargetRespondAt(*record.SLATargetRespondAt)
	}
	if record.SLATargetResolveAt != nil {
		b.SetSLATargetResolveAt(*record.SLATargetResolveAt)
	}
	if record.SLAAcknowledgedAt != nil {
		b.SetSLAAcknowledgedAt(*record.SLAAcknowledgedAt)
	}
	if record.SLAResolvedAt != nil {
		b.SetSLAResolvedAt(*record.SLAResolvedAt)
	}
	if record.StartedAt != nil {
		b.SetStartedAt(*record.StartedAt)
	}
	if record.MitigatedAt != nil {
		b.SetMitigatedAt(*record.MitigatedAt)
	}
	if record.ResolvedAt != nil {
		b.SetResolvedAt(*record.ResolvedAt)
	}
	if record.ClosedAt != nil {
		b.SetClosedAt(*record.ClosedAt)
	}
	if record.TriagedAt != nil {
		b.SetTriagedAt(*record.TriagedAt)
	}
	if record.TriageReport != nil {
		b.SetTriageReport(record.TriageReport)
	}
	b.SetAutoConfirmed(record.AutoConfirmed)
	if len(record.Tags) > 0 {
		b.SetTags(record.Tags)
	}
	if record.CustomFields != nil {
		b.SetCustomFields(record.CustomFields)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident: %w", err)
	}
	record.ID = saved.ID
	return record, nil
}

func (s *pgIncidentStore) GetIncident(ctx context.Context, incidentNumber int64) (*IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber)).
		WithTimeline(func(q *ent.IncidentTimelineEntryQuery) {
			q.Order(ent.Asc(incidenttimelineentry.FieldCreatedAt))
		}).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*IncidentRecord](err, "incident")
	}
	return s.toIncidentRecord(inc), nil
}

func (s *pgIncidentStore) GetIncidentByID(ctx context.Context, id uuid.UUID) (*IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.ID(id)).
		WithTimeline(func(q *ent.IncidentTimelineEntryQuery) {
			q.Order(ent.Asc(incidenttimelineentry.FieldCreatedAt))
		}).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*IncidentRecord](err, "incident")
	}
	return s.toIncidentRecord(inc), nil
}

func (s *pgIncidentStore) UpdateIncident(ctx context.Context, incidentNumber int64, record *IncidentRecord) (*IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to lookup incident for update: %w", err)
	}

	b := s.client.Incident.UpdateOneID(inc.ID).
		SetTitle(record.Title).
		SetDescription(record.Description).
		SetSummary(record.Summary).
		SetSeverity(entincident.Severity(record.Severity)).
		SetImpactLevel(entincident.ImpactLevel(record.ImpactLevel)).
		SetPriority(entincident.Priority(record.Priority)).
		SetIncidentType(entincident.IncidentType(record.IncidentType)).
		SetConferenceURL(record.ConferenceURL).
		SetStatusPageIncidentID(record.StatusPageIncidentID).
		SetUpdatedAt(time.Now().UTC())

	if record.SlackChannelID != "" {
		b.SetSlackChannelID(record.SlackChannelID)
	} else {
		b.ClearSlackChannelID()
	}
	if record.SlackChannelName != "" {
		b.SetSlackChannelName(record.SlackChannelName)
	}
	b.SetSlackChannelArchived(record.SlackChannelArchived)

	if record.Status != "" {
		b.SetStatus(entincident.Status(record.Status))
	}
	if record.CommanderID != nil {
		b.SetCommanderID(*record.CommanderID)
	} else {
		b.ClearCommanderID()
	}
	if record.CommunicatorID != nil {
		b.SetCommunicatorID(*record.CommunicatorID)
	} else {
		b.ClearCommunicatorID()
	}
	if record.OnCallResponderID != nil {
		b.SetOnCallResponderID(*record.OnCallResponderID)
	} else {
		b.ClearOnCallResponderID()
	}
	if record.ServiceID != nil {
		b.SetServiceID(*record.ServiceID)
	} else {
		b.ClearServiceID()
	}
	if record.EscalationPolicyID != nil {
		b.SetEscalationPolicyID(*record.EscalationPolicyID)
	} else {
		b.ClearEscalationPolicyID()
	}
	if record.SLATargetRespondAt != nil {
		b.SetSLATargetRespondAt(*record.SLATargetRespondAt)
	}
	if record.SLATargetResolveAt != nil {
		b.SetSLATargetResolveAt(*record.SLATargetResolveAt)
	}
	if record.StartedAt != nil {
		b.SetStartedAt(*record.StartedAt)
	}
	if record.MitigatedAt != nil {
		b.SetMitigatedAt(*record.MitigatedAt)
	}
	if record.ResolvedAt != nil {
		b.SetResolvedAt(*record.ResolvedAt)
	}
	if record.ClosedAt != nil {
		b.SetClosedAt(*record.ClosedAt)
	}
	if record.TriagedAt != nil {
		b.SetTriagedAt(*record.TriagedAt)
	}
	if record.TriageReport != nil {
		b.SetTriageReport(record.TriageReport)
	}
	b.SetAutoConfirmed(record.AutoConfirmed)
	if record.Tags != nil {
		b.SetTags(record.Tags)
	}
	if record.CustomFields != nil {
		b.SetCustomFields(record.CustomFields)
	}

	updated, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update incident: %w", err)
	}
	return s.toIncidentRecord(updated), nil
}

func (s *pgIncidentStore) DeleteIncident(ctx context.Context, incidentNumber int64) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to lookup incident: %w", err)
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin incident delete tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := hardDeleteIncidentCascade(ctx, tx, inc); err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := tx.Incident.UpdateOneID(inc.ID).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to soft-delete incident: %w", err)
	}

	return tx.Commit()
}

// ExpungeSoftDeletedIncidentsChildren hard-deletes the investigation artifacts
// of every already-tombstoned incident. One-time idempotent backfill. Not part
// of the IncidentStore interface; callers reach it via type assertion.
func (s *pgIncidentStore) ExpungeSoftDeletedIncidentsChildren(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := s.client.Incident.Query().Where(entincident.DeletedAtNotNil()).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query soft-deleted incidents: %w", err)
	}
	processed := 0
	for _, inc := range rows {
		err := func() error {
			tx, err := s.client.Tx(ctx)
			if err != nil {
				return err
			}
			defer rollbackTx(tx)
			if err := hardDeleteIncidentCascade(ctx, tx, inc); err != nil {
				return err
			}
			return tx.Commit()
		}()
		if err != nil {
			return processed, fmt.Errorf("expunge incident %d: %w", inc.IncidentNumber, err)
		}
		processed++
	}
	return processed, nil
}

func (s *pgIncidentStore) buildIncidentPredicates(filter IncidentListFilter) []predicate.Incident {
	preds := []predicate.Incident{entincident.DeletedAtIsNil()}
	if filter.Status != "" {
		preds = append(preds, entincident.StatusEQ(entincident.Status(filter.Status)))
	}
	if filter.Severity != "" {
		preds = append(preds, entincident.SeverityEQ(entincident.Severity(filter.Severity)))
	}
	if filter.Priority != "" {
		preds = append(preds, entincident.PriorityEQ(entincident.Priority(filter.Priority)))
	}
	if filter.ServiceID != "" {
		if sid, err := uuid.Parse(filter.ServiceID); err == nil {
			preds = append(preds, entincident.ServiceIDEQ(sid))
		}
	}
	if filter.CommanderID != "" {
		if cid, err := uuid.Parse(filter.CommanderID); err == nil {
			preds = append(preds, entincident.CommanderIDEQ(cid))
		}
	}
	if filter.Search != "" {
		preds = append(preds, entincident.TitleContainsFold(filter.Search))
	}
	if filter.StartDate != nil {
		preds = append(preds, entincident.CreatedAtGTE(*filter.StartDate))
	}
	if filter.EndDate != nil {
		preds = append(preds, entincident.CreatedAtLTE(*filter.EndDate))
	}
	return preds
}

func (s *pgIncidentStore) ListIncidents(ctx context.Context, filter IncidentListFilter) ([]IncidentRecord, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	preds := s.buildIncidentPredicates(filter)
	query := s.client.Incident.Query().Where(preds...)

	switch filter.Sort {
	case "created_at", "":
		query = query.Order(ent.Desc(entincident.FieldCreatedAt))
	case "-created_at":
		query = query.Order(ent.Asc(entincident.FieldCreatedAt))
	case "updated_at":
		query = query.Order(ent.Desc(entincident.FieldUpdatedAt))
	case "-updated_at":
		query = query.Order(ent.Asc(entincident.FieldUpdatedAt))
	case "severity":
		query = query.Order(ent.Asc(entincident.FieldSeverity))
	case "-severity":
		query = query.Order(ent.Desc(entincident.FieldSeverity))
	case "priority", "priority_asc":
		query = query.Order(ent.Asc(entincident.FieldCreatedAt))
	case "priority_desc":
		query = query.Order(ent.Desc(entincident.FieldCreatedAt))
	default:
		query = query.Order(ent.Desc(entincident.FieldCreatedAt))
	}

	countQuery := s.client.Incident.Query().Where(preds...)
	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count incidents: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query = query.Limit(limit)
	if filter.Skip > 0 {
		query = query.Offset(filter.Skip)
	}

	incs, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list incidents: %w", err)
	}

	records := make([]IncidentRecord, 0, len(incs))
	for _, inc := range incs {
		records = append(records, *s.toIncidentRecord(inc))
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

func applyStatusTimestamps(b *ent.IncidentUpdate, toStatus string, now time.Time) {
	switch toStatus {
	case "triaging":
		b.SetTriagedAt(now)
	case "active":
		b.SetSLAAcknowledgedAt(now)
	case "mitigated":
		b.SetMitigatedAt(now)
	case "resolved":
		b.SetResolvedAt(now)
		b.SetSLAResolvedAt(now)
	case "closed":
		b.SetClosedAt(now)
	}
}

func (s *pgIncidentStore) UpdateIncidentStatus(ctx context.Context, incidentNumber int64, status string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	b := s.client.Incident.Update().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		SetStatus(entincident.Status(status)).
		SetUpdatedAt(now)

	applyStatusTimestamps(b, status, now)

	n, err := b.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update incident status: %w", err)
	}
	if n == 0 {
		return ErrIncidentNotFound
	}
	return nil
}

// SetIncidentWarRoomMeet persists the Google Meet war room for an incident.
// spaceName/conferenceURL are written as a pair: creating a Meet space makes it
// the incident's conference bridge (overwriting any prior conference_url), and
// unlinking (empty strings) clears both. Pass empty strings to unlink.
func (s *pgIncidentStore) SetIncidentWarRoomMeet(ctx context.Context, incidentNumber int64, spaceName, conferenceURL string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	n, err := s.client.Incident.Update().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		SetGoogleMeetSpaceName(spaceName).
		SetConferenceURL(conferenceURL).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("set incident google meet war room: %w", err)
	}
	if n == 0 {
		return ErrIncidentNotFound
	}
	return nil
}

func (s *pgIncidentStore) TransitionIncidentStatus(ctx context.Context, incidentNumber int64, fromStatuses []string, toStatus string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	fromEnt := make([]entincident.Status, len(fromStatuses))
	for i, s := range fromStatuses {
		fromEnt[i] = entincident.Status(s)
	}
	b := s.client.Incident.Update().
		Where(
			entincident.IncidentNumber(incidentNumber),
			entincident.StatusIn(fromEnt...),
			entincident.DeletedAtIsNil(),
		).
		SetStatus(entincident.Status(toStatus)).
		SetUpdatedAt(now)

	applyStatusTimestamps(b, toStatus, now)

	n, err := b.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to transition incident status: %w", err)
	}
	if n == 0 {
		return ErrIncidentStatusConflict
	}
	return nil
}

func (s *pgIncidentStore) AddTimelineEntry(ctx context.Context, record *IncidentTimelineEntryRecord) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(record.IncidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to find incident for timeline entry: %w", err)
	}

	b := s.client.IncidentTimelineEntry.Create().
		SetEventType(record.EventType).
		SetActorType(record.ActorType).
		SetMessage(record.Message).
		SetCreatedAt(time.Now().UTC()).
		SetIncidentID(inc.ID)

	if record.ActorID != nil {
		b.SetActorID(*record.ActorID)
	}
	if record.Metadata != nil {
		b.SetMetadata(record.Metadata)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to add timeline entry: %w", err)
	}
	record.ID = saved.ID
	return nil
}

func (s *pgIncidentStore) GetTimeline(ctx context.Context, incidentNumber int64) ([]IncidentTimelineEntryRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber)).
		WithTimeline(func(q *ent.IncidentTimelineEntryQuery) {
			q.Order(ent.Asc(incidenttimelineentry.FieldCreatedAt))
		}).
		Only(ctx)
	if err != nil {
		return handleQueryErr[[]IncidentTimelineEntryRecord](err, "incident timeline")
	}

	entries := inc.Edges.Timeline
	records := make([]IncidentTimelineEntryRecord, 0, len(entries))
	for _, e := range entries {
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
	incs, err := s.client.Incident.Query().
		Where(
			entincident.CreatedAtGTE(startDate),
			entincident.CreatedAtLTE(endDate),
			entincident.DeletedAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query incidents for metrics: %w", err)
	}

	data := make([]incmetrics.IncidentData, 0, len(incs))
	for _, inc := range incs {
		d := incmetrics.IncidentData{
			CreatedAt:         inc.CreatedAt,
			AcknowledgedAt:    inc.SLAAcknowledgedAt,
			MitigatedAt:       inc.MitigatedAt,
			ResolvedAt:        inc.ResolvedAt,
			Severity:          string(inc.Severity),
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

	incs, err := s.client.Incident.Query().
		Where(
			entincident.StatusIn(entincident.StatusDetected, entincident.StatusTriaging, entincident.StatusActive, entincident.StatusMitigated),
			entincident.Or(
				entincident.SLATargetRespondAtNotNil(),
				entincident.SLATargetResolveAtNotNil(),
			),
			entincident.DeletedAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list SLA-eligible incidents: %w", err)
	}

	records := make([]IncidentRecord, 0, len(incs))
	for _, inc := range incs {
		records = append(records, *s.toIncidentRecord(inc))
	}
	return records, nil
}

func (s *pgIncidentStore) CountActiveByService(ctx context.Context) (map[string]int64, error) {
	terminalStatuses := make([]entincident.Status, len(IncidentTerminalStatuses))
	for i, st := range IncidentTerminalStatuses {
		terminalStatuses[i] = entincident.Status(st)
	}
	incs, err := s.client.Incident.Query().
		Where(
			entincident.StatusNotIn(terminalStatuses...),
			entincident.ServiceIDNotNil(),
			entincident.DeletedAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active incidents by service: %w", err)
	}

	counts := make(map[string]int64)
	for _, inc := range incs {
		if inc.ServiceID != nil {
			counts[inc.ServiceID.String()]++
		}
	}
	return counts, nil
}

func (s *pgIncidentStore) CountActiveByServiceID(ctx context.Context, serviceID string) (int, error) {
	svcUUID, err := uuid.Parse(serviceID)
	if err != nil {
		return 0, fmt.Errorf("invalid service ID: %w", err)
	}
	terminalStatuses := make([]entincident.Status, len(IncidentTerminalStatuses))
	for i, st := range IncidentTerminalStatuses {
		terminalStatuses[i] = entincident.Status(st)
	}
	return s.client.Incident.Query().
		Where(
			entincident.StatusNotIn(terminalStatuses...),
			entincident.ServiceID(svcUUID),
			entincident.DeletedAtIsNil(),
		).
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
		Priority string
		Count    int
	}

	var results []groupResult
	err = s.client.Incident.Query().
		Where(
			entincident.StatusNotIn(entincident.StatusResolved, entincident.StatusClosed, entincident.StatusCancelled),
			entincident.ServiceID(svcUUID),
			entincident.DeletedAtIsNil(),
		).
		GroupBy(entincident.FieldPriority).
		Aggregate(ent.Count()).
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

	rows, err := s.client.Incident.Query().
		Where(
			entincident.StatusIn(entincident.StatusDetected, entincident.StatusTriaging, entincident.StatusActive, entincident.StatusMitigated),
			entincident.SlackChannelIDNEQ(""),
			entincident.DeletedAtIsNil(),
		).
		Order(ent.Asc(entincident.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query summarizable incidents: %w", err)
	}
	out := make([]IncidentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, *s.toIncidentRecord(r))
	}
	return out, nil
}

func (s *pgIncidentStore) ListActiveIncidents(ctx context.Context) ([]IncidentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	rows, err := s.client.Incident.Query().
		Where(
			entincident.StatusNotIn(entincident.StatusResolved, entincident.StatusClosed, entincident.StatusCancelled),
			entincident.DeletedAtIsNil(),
		).
		Order(ent.Asc(entincident.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query active incidents: %w", err)
	}
	out := make([]IncidentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, *s.toIncidentRecord(r))
	}
	return out, nil
}

func (s *pgIncidentStore) GetIncidentBySlackChannel(ctx context.Context, channelID string) (*IncidentRecord, error) {
	item, err := s.client.Incident.Query().
		Where(entincident.SlackChannelIDEQ(channelID)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*IncidentRecord](err, "incident by slack channel")
	}
	return s.toIncidentRecord(item), nil
}

func (s *pgIncidentStore) toIncidentRecord(inc *ent.Incident) *IncidentRecord {
	rec := &IncidentRecord{
		ID:                       inc.ID,
		IncidentNumber:           inc.IncidentNumber,
		Title:                    inc.Title,
		Description:              inc.Description,
		Summary:                  inc.Summary,
		Status:                   string(inc.Status),
		Severity:                 string(inc.Severity),
		ImpactLevel:              string(inc.ImpactLevel),
		Priority:                 string(inc.Priority),
		IncidentType:             string(inc.IncidentType),
		CommanderID:              inc.CommanderID,
		CommunicatorID:           inc.CommunicatorID,
		OnCallResponderID:        inc.OnCallResponderID,
		CommanderAssigneeType:    string(inc.CommanderAssigneeType),
		CommunicatorAssigneeType: string(inc.CommunicatorAssigneeType),
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

	if timeline := inc.Edges.Timeline; timeline != nil {
		rec.Timeline = make([]IncidentTimelineEntryRecord, 0, len(timeline))
		for _, e := range timeline {
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

	return rec
}
