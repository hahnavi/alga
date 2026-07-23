package store

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	AlertInvestigationStatusPending       = "pending"
	AlertInvestigationStatusAssigned      = "assigned"
	AlertInvestigationStatusInvestigating = "investigating"
	AlertInvestigationStatusPromoted      = "promoted"
	AlertInvestigationStatusComplete      = "complete"
	AlertInvestigationStatusFailed        = "failed"
	AlertInvestigationStatusCancelled     = "cancelled"
	AlertInvestigationStatusTimedOut      = "timed_out"
	AlertInvestigationStatusPaused        = "paused"

	AlertInvestigationCompletedReasonAgentResolved      = "agent_resolved"
	AlertInvestigationCompletedReasonMonitoringResolved = "monitoring_resolved"
	AlertInvestigationCompletedReasonAlertsResolved     = "alerts_resolved"
	AlertInvestigationCompletedReasonCancelled          = "cancelled"
	AlertInvestigationCompletedReasonTimedOut           = "timed_out"
	AlertInvestigationCompletedReasonPromoted           = "promoted"

	InvestigationActorAgent   = "agent"
	InvestigationActorUser    = "user"
	InvestigationActorSystem  = "system"
	InvestigationActorGrafana = "grafana"

	AlertInvestigationEventAssigned                = "assigned"
	AlertInvestigationEventStarted                 = "started"
	AlertInvestigationEventRequeued                = "requeued"
	AlertInvestigationEventAgentOfflineBeforeStart = "agent_offline_before_start"
	AlertInvestigationEventAgentOfflineDuringWork  = "agent_offline_during_work"
	AlertInvestigationEventDispatchFailed          = "dispatch_failed"
	AlertInvestigationEventAutoCompleted           = "auto_completed"
	AlertInvestigationEventCompleted               = "completed"

	IncidentInvestigationStatusPending       = "pending"
	IncidentInvestigationStatusAssigned      = "assigned"
	IncidentInvestigationStatusInvestigating = "investigating"
	IncidentInvestigationStatusPaused        = "paused"
	IncidentInvestigationStatusComplete      = "complete"
	IncidentInvestigationStatusCancelled     = "cancelled"
	// IncidentInvestigationStatusCoordinating marks a commander-owned investigation
	// that coordinates child investigations rather than investigating directly. It is
	// excluded from activeIncidentInvestigationStatuses so the scheduler does not
	// dispatch it as a normal investigation work item.
	IncidentInvestigationStatusCoordinating = "coordinating"
)

// InvestigationTerminalStatuses lists alert-investigation statuses that mark
// the alert-level investigation lifecycle as finished. A promoted investigation
// is also terminal from the alert perspective: its work has been handed off to
// the incident, so the correlator and the stale-alert sweep must treat it as
// closed instead of shadowing future firing alerts that share the fingerprint.
var InvestigationTerminalStatuses = []string{"complete", "failed", "cancelled", "timed_out", "promoted"}

func IsTerminalInvestigationStatus(status string) bool {
	return slices.Contains(InvestigationTerminalStatuses, status)
}

// reopenableAlertInvestigationStatuses is the subset of terminal/paused
// statuses from which an alert-level investigation may be revived when an
// operator reopens the linked alert. Promoted is excluded on purpose: a
// promoted investigation has been handed off to an incident, and reopening the
// alert must not silently demote it back into active duty while the incident
// still owns it.
var reopenableAlertInvestigationStatuses = []string{
	"complete", "failed", "cancelled", "timed_out", "paused",
}

func IsReopenableInvestigationStatus(status string) bool {
	return slices.Contains(reopenableAlertInvestigationStatuses, status)
}

var activeIncidentInvestigationStatuses = []string{
	IncidentInvestigationStatusPending,
	IncidentInvestigationStatusAssigned,
	IncidentInvestigationStatusInvestigating,
	IncidentInvestigationStatusPaused,
}

var ErrActiveIncidentInvestigationExists = errors.New("active incident investigation exists")

type FingerprintChecker interface {
	GetByFingerprint(fingerprint string) (*AlertRecord, error)
}

type InvestigationUpdateType string

const (
	UpdateTypeProgress   InvestigationUpdateType = "progress"
	UpdateTypeFinding    InvestigationUpdateType = "finding"
	UpdateTypeAction     InvestigationUpdateType = "action"
	UpdateTypeResolution InvestigationUpdateType = "resolution"
	UpdateTypeComment    InvestigationUpdateType = "comment"
	UpdateTypeDeadLetter InvestigationUpdateType = "dead_lettered"
)

type InvestigationUpdateSource string

const (
	UpdateSourceAgent      InvestigationUpdateSource = "agent"
	UpdateSourceUser       InvestigationUpdateSource = "user"
	UpdateSourceMattermost InvestigationUpdateSource = "mattermost"
	UpdateSourceSlack      InvestigationUpdateSource = "slack"
	UpdateSourceSystem     InvestigationUpdateSource = "system"
)

type InvestigationUpdate struct {
	ID             uuid.UUID                 `json:"id"`
	Type           InvestigationUpdateType   `json:"type"`
	Message        string                    `json:"message"`
	Source         InvestigationUpdateSource `json:"source"`
	Internal       bool                      `json:"internal,omitempty"`
	Edited         bool                      `json:"edited,omitempty"`
	UserID         *string                   `json:"user_id,omitempty"`
	Username       *string                   `json:"username,omitempty"`
	MMPostID       string                    `json:"mm_post_id,omitempty"`
	SlackMessageTS string                    `json:"slack_message_ts,omitempty"`
	QuotedUpdateID *string                   `json:"quoted_update_id,omitempty"`
	Mentions       []string                  `json:"mentions,omitempty"`
	CreatedAt      time.Time                 `json:"created_at"`
}
