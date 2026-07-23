package alga

import (
	"time"
)

// Note: this SDK is stdlib-only. IDs that the backend persists as UUIDs are
// surfaced as opaque strings so callers do not need a UUID library to use the
// SDK. The wire format is unchanged.

type AlertEvent struct {
	Type             string    `json:"type"`
	Timestamp        time.Time `json:"timestamp"`
	ActorUsername    string    `json:"actor_username,omitempty"`
	ActorDisplayName string    `json:"actor_display_name,omitempty"`
	ActorUserID      string    `json:"actor_user_id,omitempty"`
	Source           string    `json:"source,omitempty"`
}

type DeliveryTarget struct {
	Provider    string `json:"provider"`
	Channel     string `json:"channel"`
	ChannelName string `json:"channel_name,omitempty"`
	PostID      string `json:"post_id"`
}

type Alert struct {
	Fingerprint     string             `json:"fingerprint"`
	Status          string             `json:"status"`
	Acknowledged    bool               `json:"acknowledged"`
	Silenced        bool               `json:"silenced"`
	Labels          map[string]string  `json:"labels"`
	Annotations     map[string]string  `json:"annotations"`
	Values          map[string]float64 `json:"values,omitempty"`
	StartsAt        *time.Time         `json:"starts_at,omitempty"`
	EndsAt          *time.Time         `json:"ends_at,omitempty"`
	GeneratorURL    string             `json:"generator_url,omitempty"`
	Events          []AlertEvent       `json:"events,omitempty"`
	DeliveryTargets []DeliveryTarget   `json:"delivery_targets,omitempty"`
	AlertNumber     int64              `json:"alert_number,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type CorrelatedAlert struct {
	Fingerprint  string             `json:"fingerprint"`
	AlertNumber  int64              `json:"alert_number,omitempty"`
	Labels       map[string]string  `json:"labels"`
	Annotations  map[string]string  `json:"annotations"`
	Status       string             `json:"status"`
	StartsAt     string             `json:"starts_at"`
	Values       map[string]float64 `json:"values,omitempty"`
	GeneratorURL string             `json:"generator_url,omitempty"`
}

type InvestigationResult struct {
	Status             string   `json:"status"`
	RootCause          string   `json:"root_cause,omitempty"`
	Resolution         string   `json:"resolution,omitempty"`
	Summary            string   `json:"summary"`
	Evidence           []string `json:"evidence,omitempty"`
	RecommendedActions []string `json:"recommended_actions,omitempty"`
	SeverityAssessment string   `json:"severity_assessment,omitempty"`
	EscalationLevel    string   `json:"escalation_level"`
	RawResponse        string   `json:"raw_response,omitempty"`
}

type InvestigationUpdate struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Message        string    `json:"message"`
	Source         string    `json:"source"`
	Internal       bool      `json:"internal,omitempty"`
	Edited         bool      `json:"edited,omitempty"`
	UserID         *string   `json:"user_id,omitempty"`
	Username       *string   `json:"username,omitempty"`
	MMPostID       string    `json:"mm_post_id,omitempty"`
	SlackMessageTS string    `json:"slack_message_ts,omitempty"`
	QuotedUpdateID *string   `json:"quoted_update_id,omitempty"`
	Mentions       []string  `json:"mentions,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Investigation struct {
	ID                      string                `json:"id"`
	InvestigationID         string                `json:"investigation_id"`
	InvestigationNumber     int64                 `json:"investigation_number,omitempty"`
	Alerts                  []CorrelatedAlert     `json:"alerts"`
	Severity                string                `json:"severity"`
	CorrelationKey          string                `json:"correlation_key"`
	Status                  string                `json:"status"`
	Result                  *InvestigationResult  `json:"result,omitempty"`
	MMPostID                string                `json:"mm_post_id,omitempty"`
	MMThreadID              string                `json:"mm_thread_id,omitempty"`
	PrimaryThreadID         string                `json:"primary_thread_id,omitempty"`
	SlackChannelID          string                `json:"slack_channel_id,omitempty"`
	SlackThreadTS           string                `json:"slack_thread_ts,omitempty"`
	TwilioCallSID           string                `json:"twilio_call_sid,omitempty"`
	AgentID                 string                `json:"agent_id,omitempty"`
	AgentName               string                `json:"agent_name,omitempty"`
	AgentType               string                `json:"agent_type,omitempty"`
	EscalationLevel         string                `json:"escalation_level"`
	Updates                 []InvestigationUpdate `json:"updates"`
	CreatedAt               time.Time             `json:"created_at"`
	UpdatedAt               time.Time             `json:"updated_at"`
	CompletedAt             *time.Time            `json:"completed_at,omitempty"`
	StartedAt               *time.Time            `json:"started_at,omitempty"`
	InvestigatingDurationMs int64                 `json:"investigating_duration_ms"`
}

// CoordinationTask describes a unit of work dispatched by an incident
// commander to a role (responder, communicator, verifier). Tasks are the
// modern coordination primitive; the older post_handoff flow is deprecated.
type CoordinationTask struct {
	TaskID          string         `json:"task_id,omitempty"`
	IncidentNumber  int64          `json:"incident_number,omitempty"`
	Kind            string         `json:"kind,omitempty"`
	Goal            string         `json:"goal,omitempty"`
	AssigneeRole    string         `json:"assignee_role,omitempty"`
	AssigneeAgentID string         `json:"assignee_agent_id,omitempty"`
	Status          string         `json:"status,omitempty"`
	Result          map[string]any `json:"result,omitempty"`
	InputContext    map[string]any `json:"input_context,omitempty"`
	ParentTaskID    string         `json:"parent_task_id,omitempty"`
	CreatedAt       *time.Time     `json:"created_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
}

type KnowledgeNote struct {
	ID                    string            `json:"id"`
	Kind                  string            `json:"kind"`
	Title                 string            `json:"title"`
	BodyMarkdown          string            `json:"body_markdown"`
	Tags                  []string          `json:"tags,omitempty"`
	Selectors             map[string]string `json:"selectors,omitempty"`
	SourceInvestigationID string            `json:"source_investigation_id,omitempty"`
	Confidence            *float64          `json:"confidence,omitempty"`
	ExpiresAt             *time.Time        `json:"expires_at,omitempty"`
	AuthorType            string            `json:"author_type"`
	AuthorName            string            `json:"author_name,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

type Memory struct {
	ID              string            `json:"id"`
	Content         string            `json:"content"`
	MemoryType      string            `json:"memory_type"`
	Hash            string            `json:"hash"`
	AgentID         string            `json:"agent_id,omitempty"`
	AgentName       string            `json:"agent_name,omitempty"`
	AgentType       string            `json:"agent_type,omitempty"`
	InvestigationID string            `json:"investigation_id,omitempty"`
	CorrelationKey  string            `json:"correlation_key,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Entities        []string          `json:"entities,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	Confidence      *float64          `json:"confidence,omitempty"`
	AccessCount     int               `json:"access_count"`
	Score           float64           `json:"score,omitempty"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type PeerAsk struct {
	ID                 string     `json:"id"`
	FromAgentID        string     `json:"from_agent_id"`
	FromAgentName      string     `json:"from_agent_name"`
	FromAgentType      string     `json:"from_agent_type"`
	InvestigationID    string     `json:"investigation_id,omitempty"`
	ToAgentID          string     `json:"to_agent_id,omitempty"`
	ToAgentType        string     `json:"to_agent_type,omitempty"`
	Question           string     `json:"question"`
	Reply              string     `json:"reply,omitempty"`
	RepliedByAgentID   string     `json:"replied_by_agent_id,omitempty"`
	RepliedByAgentName string     `json:"replied_by_agent_name,omitempty"`
	Status             string     `json:"status"`
	ExpiresAt          time.Time  `json:"expires_at"`
	CreatedAt          time.Time  `json:"created_at"`
	AnsweredAt         *time.Time `json:"answered_at,omitempty"`
}

type Service struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels,omitempty"`
	TeamID      string            `json:"team_id,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Incident struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Status             string     `json:"status"`
	Severity           string     `json:"severity"`
	Priority           string     `json:"priority"`
	CommanderID        string     `json:"commander_id,omitempty"`
	ServiceID          string     `json:"service_id,omitempty"`
	TeamID             string     `json:"team_id,omitempty"`
	SLATargetRespondAt *time.Time `json:"sla_target_respond_at,omitempty"`
	SLATargetResolveAt *time.Time `json:"sla_target_resolve_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
}

type PlaybookStep struct {
	ID               string `json:"id"`
	StepNumber       int    `json:"step_number"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	ExpectedDuration string `json:"expected_duration,omitempty"`
	Command          string `json:"command,omitempty"`
}

type Playbook struct {
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	Kind           string              `json:"kind"`
	Summary        string              `json:"summary"`
	ServiceID      string              `json:"service_id,omitempty"`
	LabelSelectors []map[string]string `json:"label_selectors,omitempty"`
	Tags           []string            `json:"tags,omitempty"`
	Steps          []PlaybookStep      `json:"steps,omitempty"`
	CreatedBy      string              `json:"created_by,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type Capability struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// --- Server-Sent Events ---

type ConnectedEvent struct {
	ClientID string `json:"client_id"`
	AgentID  string `json:"agent_id"`
}

type MessageEvent struct {
	Type       string `json:"type"`
	ChatID     string `json:"chat_id"`
	Text       string `json:"text"`
	SenderID   string `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	MessageID  string `json:"message_id,omitempty"`
}

type TypingEvent struct {
	Type   string `json:"type"`
	ChatID string `json:"chat_id"`
	Active bool   `json:"active"`
}

type InvestigationSignalEvent struct {
	InvestigationID string `json:"investigation_id"`
	Reason          string `json:"reason,omitempty"`
	Actor           string `json:"actor,omitempty"`
}

type PeerFindingEvent struct {
	Type            string            `json:"type"`
	InvestigationID string            `json:"investigation_id"`
	PeerAgentID     string            `json:"peer_agent_id,omitempty"`
	PeerAgentType   string            `json:"peer_agent_type,omitempty"`
	Text            string            `json:"text"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type PeerAskEvent struct {
	Type            string    `json:"type"`
	AskID           string    `json:"ask_id"`
	FromAgentID     string    `json:"from_agent_id"`
	FromAgentName   string    `json:"from_agent_name"`
	FromAgentType   string    `json:"from_agent_type"`
	InvestigationID string    `json:"investigation_id,omitempty"`
	Question        string    `json:"question"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type PeerReplyEvent struct {
	Type               string    `json:"type"`
	AskID              string    `json:"ask_id"`
	InvestigationID    string    `json:"investigation_id,omitempty"`
	Reply              string    `json:"reply"`
	RepliedByAgentID   string    `json:"replied_by_agent_id,omitempty"`
	RepliedByAgentName string    `json:"replied_by_agent_name,omitempty"`
	AnsweredAt         time.Time `json:"answered_at"`
}

type AgentPresenceEvent struct {
	AgentID string `json:"agent_id"`
	Online  bool   `json:"online"`
}

// CoordinationTaskEvent is published by the backend when an incident commander
// dispatches a task to this agent's role. Agents respond by claiming the task,
// doing the work, then completing it with a typed result.
type CoordinationTaskEvent struct {
	Type            string         `json:"type"`
	TaskID          string         `json:"task_id"`
	IncidentNumber  int64          `json:"incident_number"`
	Kind            string         `json:"kind"`
	Goal            string         `json:"goal"`
	AssigneeRole    string         `json:"assignee_role,omitempty"`
	AssigneeAgentID string         `json:"assignee_agent_id,omitempty"`
	InputContext    map[string]any `json:"input_context,omitempty"`
	ParentTaskID    string         `json:"parent_task_id,omitempty"`
}

// --- List responses ---
//
// The backend historically returns resources under either of two JSON keys
// (e.g. "alerts" or "items"). The SDK accepts both and exposes a single
// normalized accessor (e.g. AlertListResponse.All()) so callers do not have
// to repeat the dual-key workaround at every call site.

type AlertListResponse struct {
	Alerts []Alert `json:"alerts,omitempty"`
	Items  []Alert `json:"items,omitempty"`
	Total  int     `json:"total,omitempty"`
}

// All returns every alert in the response, regardless of which JSON key
// the backend populated.
func (r *AlertListResponse) All() []Alert {
	if len(r.Alerts) > 0 {
		return r.Alerts
	}
	if r.Items != nil {
		return r.Items
	}
	return nil
}

type InvestigationListResponse struct {
	Investigations []Investigation `json:"investigations,omitempty"`
	Items          []Investigation `json:"items,omitempty"`
	Total          int             `json:"total,omitempty"`
	Limit          int             `json:"limit,omitempty"`
	Skip           int             `json:"skip,omitempty"`
}

// All returns every investigation in the response.
func (r *InvestigationListResponse) All() []Investigation {
	if len(r.Investigations) > 0 {
		return r.Investigations
	}
	if r.Items != nil {
		return r.Items
	}
	return nil
}

type KnowledgeListResponse struct {
	Notes []KnowledgeNote `json:"notes,omitempty"`
	Items []KnowledgeNote `json:"items,omitempty"`
	Total int             `json:"total,omitempty"`
	Limit int             `json:"limit,omitempty"`
	Skip  int             `json:"skip,omitempty"`
}

// All returns every knowledge note in the response.
func (r *KnowledgeListResponse) All() []KnowledgeNote {
	if len(r.Notes) > 0 {
		return r.Notes
	}
	if r.Items != nil {
		return r.Items
	}
	return nil
}

type MemoryListResponse struct {
	Memories []Memory `json:"memories"`
	Total    int      `json:"total"`
}

type PeerAskListResponse struct {
	Asks  []PeerAsk `json:"asks"`
	Total int       `json:"total"`
}

type SendMessageResponse struct {
	Status    string `json:"status"`
	MessageID string `json:"message_id,omitempty"`
}

// CommandResponse mirrors the backend InvToolOutcome for inv_tool messages.
// IncidentNumber and IncidentInvestigationID are populated when a command
// promotes an investigation or mutates an incident.
type CommandResponse struct {
	Ok                      bool   `json:"ok"`
	Op                      string `json:"op"`
	InvestigationID         string `json:"investigation_id,omitempty"`
	IncidentNumber          int64  `json:"incident_number,omitempty"`
	IncidentInvestigationID string `json:"incident_investigation_id,omitempty"`
	Error                   string `json:"error,omitempty"`
}
