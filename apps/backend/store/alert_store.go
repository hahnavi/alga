// Package store type definitions: alert-domain records, events, and the
// AlertStore / Store interfaces backed by pgAlertStore (see alert.go).
package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AlertEvent struct {
	Type             string    `json:"type"`
	Timestamp        time.Time `json:"timestamp"`
	ActorUsername    string    `json:"actor_username,omitempty"`
	ActorDisplayName string    `json:"actor_display_name,omitempty"`
	ActorUserID      string    `json:"actor_user_id,omitempty"`
	Source           string    `json:"source,omitempty"`
}

type EventActor struct {
	UserID      string
	Username    string
	DisplayName string
	Source      string
}

type DeliveryTarget struct {
	Provider    string `json:"provider"`
	Channel     string `json:"channel"`
	ChannelName string `json:"channel_name,omitempty"`
	PostID      string `json:"post_id"`
}

type AlertEventPublisher interface {
	PublishAlertEvent(action string, record AlertRecord)
}

// AlertRef identifies an alert within a cascade result.
type AlertRef struct {
	AlertNumber int64  `json:"alert_number"`
	Fingerprint string `json:"fingerprint"`
}

// AlertCascadeResult is returned by ResolveAlertsByIncident.
type AlertCascadeResult struct {
	Resolved []AlertRecord // full records, so callers can broadcast alert_updated
	Skipped  []AlertRef    // linked but already resolved
	Failed   []AlertRef    // update errored; collected for surfacing
}

// ShiftAlertMetrics summarizes alert activity attributed to one on-call
// shift window [start, end): every alert that fired while the shift holder
// was on call counts as received by the shift. Acknowledge attribution is
// deliberately window-based (not actor-based): ack/resolve events arrive from
// surfaces whose actor ids are not always Alga user IDs (e.g. Slack).
type ShiftAlertMetrics struct {
	Received      int64   `json:"received"`
	Acknowledged  int64   `json:"acknowledged"`
	Resolved      int64   `json:"resolved"`
	Missed        int64   `json:"missed"`
	AvgAckSeconds float64 `json:"avg_ack_seconds"`
}

// AlertStore is the narrow read/link interface over alerts used by callers
// that do not need the full mutation surface of Store.
type AlertStore interface {
	GetByFingerprint(fingerprint string) (*AlertRecord, error)
	QueryAlerts(filter map[string]any) ([]AlertRecord, error)
	LinkAlertToIncident(ctx context.Context, fingerprint string, incidentNumber int64) error
	UnlinkAlertFromIncident(ctx context.Context, fingerprint string, incidentNumber int64) error
	GetAlertsByIncident(ctx context.Context, incidentNumber int64) ([]string, error)
}

type Store interface {
	Create(record AlertRecord) (int64, error)
	GetByFingerprint(fingerprint string) (*AlertRecord, error)
	GetOpenByFingerprint(fingerprint string) (*AlertRecord, error)
	UpdateStatus(fingerprint, status string, resolvedEvent *AlertEvent) error
	UpdateStatusSilenced(fingerprint string) error
	UpdateDeliveryTargets(fingerprint string, targets []DeliveryTarget) error
	AcknowledgeAlert(fingerprint string, actor *EventActor) error
	ReopenAlert(fingerprint string, ev AlertEvent) error
	ResolveAlertByUser(fingerprint string, actor *EventActor) error
	DeleteAlert(fingerprint string) error
	GetByAlertNumber(alertNumber int64) (*AlertRecord, error)
	AcknowledgeAlertByNumber(alertNumber int64, actor *EventActor) error
	ReopenAlertByNumber(alertNumber int64, ev AlertEvent) error
	ResolveAlertByNumber(alertNumber int64, actor *EventActor) error
	DeleteAlertByNumber(alertNumber int64) error
	QueryAlerts(filter map[string]any) ([]AlertRecord, error)
	ListUninvestigatedAlerts(ctx context.Context, threshold time.Duration) ([]AlertRecord, error)
	DeleteOlderThan(ctx context.Context, olderThan time.Time) (int64, error)
	CountOlderThan(ctx context.Context, olderThan time.Time) (int64, error)
	LinkAlertToIncident(ctx context.Context, fingerprint string, incidentNumber int64) error
	UnlinkAlertFromIncident(ctx context.Context, fingerprint string, incidentNumber int64) error
	GetAlertsByIncident(ctx context.Context, incidentNumber int64) ([]string, error)
	GetIncidentsByAlertNumber(ctx context.Context, alertNumber int64) ([]IncidentRecord, error)
	ShiftAlertMetrics(ctx context.Context, start, end time.Time) (ShiftAlertMetrics, error)
	ResolveAlertsByIncident(ctx context.Context, incidentNumber int64, actor *EventActor) (AlertCascadeResult, error)
	Close()
	TriageResultStore() TriageResultStore
	TriageRuleStore() TriageRuleStore
}

type AlertRecord struct {
	ID              uuid.UUID         `json:"-"`
	Fingerprint     string            `json:"fingerprint"`
	Status          string            `json:"status"`
	Acknowledged    bool              `json:"acknowledged"`
	Silenced        bool              `json:"silenced"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	Values          map[string]any    `json:"values"`
	StartsAt        time.Time         `json:"starts_at"`
	EndsAt          *time.Time        `json:"ends_at"`
	GeneratorURL    string            `json:"generator_url"`
	Events          []AlertEvent      `json:"events,omitempty"`
	DeliveryTargets []DeliveryTarget  `json:"delivery_targets,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	AlertNumber     int64             `json:"alert_number,omitempty"`

	// Investigation, if non-nil, is a slim summary of the alert's current
	// alert investigation (assigned agent + status). Populated by the
	// store when callers request it; nil when the alert has no
	// investigation or the caller opted out of the join.
	Investigation *AlertInvestigationSummary `json:"investigation,omitempty"`

	// DeletedAt is non-nil when the alert has been soft-deleted. Tombstone-only;
	// excluded from lists, dashboard counts, episodic memory, and dedup.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`

	// InitialEvent, if non-nil, replaces the default grafana "fired" event that
	// Create inserts alongside the new alert. Not persisted on the record.
	InitialEvent *AlertEvent `json:"-"`
}

func AlertEventWithActor(evType string, now time.Time, actor *EventActor) AlertEvent {
	src := "user"
	if actor != nil && actor.Source != "" {
		src = actor.Source
	}
	ev := AlertEvent{Type: evType, Timestamp: now, Source: src}
	if actor != nil {
		ev.ActorUserID = actor.UserID
		ev.ActorUsername = actor.Username
		if actor.DisplayName != "" {
			ev.ActorDisplayName = actor.DisplayName
		}
	}
	return ev
}
