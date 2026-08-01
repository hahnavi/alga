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
	IncidentNumber     int64      `json:"incident_number,omitempty"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Summary            string     `json:"summary,omitempty"`
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

// IncidentRole is a role assignment (commander, communications lead,
// responder, ...) attached to an incident.
type IncidentRole struct {
	RoleType     string `json:"role_type"`
	AssigneeType string `json:"assignee_type"`
	AgentTokenID string `json:"agent_token_id,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	UserName     string `json:"user_name,omitempty"`
	Status       string `json:"status"`
}

// IncidentContext is the agent-facing incident read model: the incident
// record plus its active role assignments.
type IncidentContext struct {
	Incident Incident       `json:"incident"`
	Roles    []IncidentRole `json:"roles"`
}

// OnCallEntry describes who is currently on call for one schedule.
type OnCallEntry struct {
	ScheduleID   string `json:"schedule_id"`
	ScheduleName string `json:"schedule_name"`
	UserID       string `json:"user_id,omitempty"`
	UserName     string `json:"user_name,omitempty"`
}

// SecretValue is the payload of GET /api/v1/agent/secrets/{secret_id}. The
// value is plaintext for immediate use; never persist or log it.
type SecretValue struct {
	SecretID  string    `json:"secret_id"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	FetchedAt time.Time `json:"fetched_at"`
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
	// Trigger distinguishes actionable deliveries from passive ones:
	// "dispatch"/"mention" mean the agent should act, "observe" means the
	// message is context only (append to transcript, do not wake the agent).
	Trigger          string   `json:"trigger,omitempty"`
	ReplyToMessageID string   `json:"reply_to_message_id,omitempty"`
	ReplyToText      string   `json:"reply_to_text,omitempty"`
	Mentions         []string `json:"mentions,omitempty"`
	// SystemContext carries behavioral rules (investigation instructions,
	// tool allowlists, role constraints) that agents supporting system-prompt
	// injection should place in the LLM system message. Agents that do not
	// support this can ignore it; the full rules are also present in Text.
	SystemContext string `json:"system_context,omitempty"`
}

type TypingEvent struct {
	Type   string `json:"type"`
	ChatID string `json:"chat_id"`
	Active bool   `json:"active"`
}

// InvestigationSignalEvent is the payload of investigation_resume. The
// backend populates either investigation_id or alert_investigation_id
// depending on the emitting path.
type InvestigationSignalEvent struct {
	InvestigationID      string `json:"investigation_id,omitempty"`
	AlertInvestigationID string `json:"alert_investigation_id,omitempty"`
	Reason               string `json:"reason,omitempty"`
	Actor                string `json:"actor,omitempty"`
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

// CoordinationTaskEvent is published by the backend when an incident commander
// dispatches a task to this agent's role. Agents respond by claiming the task,
// doing the work, then completing it with a typed result.
type CoordinationTaskEvent struct {
	Type            string         `json:"type"`
	ChatID          string         `json:"chat_id,omitempty"`
	TaskID          string         `json:"task_id"`
	IncidentNumber  int64          `json:"incident_number"`
	Kind            string         `json:"kind"`
	Goal            string         `json:"goal"`
	Text            string         `json:"text,omitempty"`
	AssigneeRole    string         `json:"assignee_role,omitempty"`
	AssigneeAgentID string         `json:"assignee_agent_id,omitempty"`
	InputContext    map[string]any `json:"input_context,omitempty"`
	ParentTaskID    string         `json:"parent_task_id,omitempty"`
}

// GoalText returns the task objective, preferring Goal and falling back to
// Text (the backend historically sent the goal under the "text" key).
func (e CoordinationTaskEvent) GoalText() string {
	if e.Goal != "" {
		return e.Goal
	}
	return e.Text
}

// SummarizeIncidentEvent asks a communicate-capable agent to produce an
// incident summary (reply with SendIncidentSummary).
type SummarizeIncidentEvent struct {
	IncidentNumber int64          `json:"incident_number"`
	ChatID         string         `json:"chat_id"`
	Incident       map[string]any `json:"incident,omitempty"`
}

// AlertAutoResolvedEvent notifies the agent that an alert it was investigating
// auto-resolved (e.g. the alert cleared at the source).
type AlertAutoResolvedEvent struct {
	InvestigationID string `json:"investigation_id"`
	Fingerprint     string `json:"fingerprint,omitempty"`
	AlertName       string `json:"alert_name,omitempty"`
}

// IncidentCommsStaleEvent nudges the incident-commander agent when incident
// communications have gone quiet past the SLA threshold.
type IncidentCommsStaleEvent struct {
	IncidentNumber int64  `json:"incident_number"`
	Trigger        string `json:"trigger,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// --- List responses ---
//
// List endpoints use the backend's paginated envelope; doJSON unwraps the
// outer {"data": ...} layer, leaving {"items": [...], "total": N}.

type KnowledgeListResponse struct {
	Items []KnowledgeNote `json:"items"`
	Total int64           `json:"total"`
}

type MemoryListResponse struct {
	Items []Memory `json:"items"`
	Total int64    `json:"total"`
}

type PeerAskListResponse struct {
	Items []PeerAsk `json:"items"`
	Total int64     `json:"total"`
}

type ServiceListResponse struct {
	Items []Service `json:"items"`
	Total int64     `json:"total"`
}

type SendMessageResponse struct {
	Status    string `json:"status"`
	MessageID string `json:"message_id,omitempty"`
}

// CommandResponse mirrors the backend inv_tool outcome object.
type CommandResponse struct {
	Ok     bool   `json:"ok"`
	Op     string `json:"op"`
	ChatID string `json:"chat_id,omitempty"`
	// InvestigationID is a deprecated backend alias of ChatID.
	InvestigationID string `json:"investigation_id,omitempty"`
	IncidentNumber  int64  `json:"incident_number,omitempty"`
	Error           string `json:"error,omitempty"`
}
