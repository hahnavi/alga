package incidentchannel

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/slack"
	"alga/store"
)

// OnCallResolver resolves the user currently on-call for an incident's
// escalation policy. It is an optional dependency used to auto-invite the
// on-call human to a newly created incident Slack channel.
type OnCallResolver interface {
	ResolveOnCallUser(ctx context.Context, incident *store.IncidentRecord) (*uuid.UUID, error)
}

type Manager struct {
	slackClient    *slack.Client
	incidentStore  store.IncidentStore
	userStore      store.UserStore
	onCallResolver OnCallResolver
	algaBaseURL    string
}

func NewManager(sc *slack.Client, is store.IncidentStore, us store.UserStore, baseURL string) *Manager {
	return &Manager{
		slackClient:   sc,
		incidentStore: is,
		userStore:     us,
		algaBaseURL:   baseURL,
	}
}

func (m *Manager) SetOnCallResolver(r OnCallResolver) {
	m.onCallResolver = r
}

func (m *Manager) IsSupported() bool {
	return m.slackClient != nil && m.slackClient.Enabled()
}

func (m *Manager) CreateIncidentChannel(
	ctx context.Context,
	incident *store.IncidentRecord,
	isPrivate bool,
) error {
	if incident.SlackChannelID != "" {
		return errors.New("slack channel already exists for this incident")
	}
	if !m.IsSupported() {
		return errors.New("slack is not configured")
	}

	baseName := generateChannelName(incident)

	var channelID string
	var err error
	var name string
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt == 1 {
			name = baseName
		} else {
			name = fmt.Sprintf("%s-%d", baseName, attempt)
		}
		channelID, err = m.slackClient.CreateChannel(ctx, name, isPrivate)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "name_taken") {
			continue
		}
		return fmt.Errorf("failed to create slack channel: %w", err)
	}
	if err != nil {
		return fmt.Errorf("failed to create slack channel after retries: %w", err)
	}

	topic := fmt.Sprintf("[%d] %s — %s", incident.IncidentNumber, incident.Title, incident.Severity)
	purpose := fmt.Sprintf("Alga incident %d — %s/incidents/%d", incident.IncidentNumber, m.algaBaseURL, incident.IncidentNumber)

	if err := m.slackClient.SetChannelTopic(ctx, channelID, topic); err != nil {
		logger.WarnCtx(ctx, "failed to set slack channel topic", "component", "incidentchannel", "error", err)
	}
	if err := m.slackClient.SetChannelPurpose(ctx, channelID, purpose); err != nil {
		logger.WarnCtx(ctx, "failed to set slack channel purpose", "component", "incidentchannel", "error", err)
	}

	userIDs := m.collectSlackUserIDs(ctx, incident)
	if len(userIDs) > 0 {
		if err := m.slackClient.InviteUsers(ctx, channelID, userIDs); err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "already_in_channel") ||
				strings.Contains(errStr, "no_such_user") {
				logger.WarnCtx(ctx, "slack invite skipped some users", "component", "incidentchannel", "error", err)
			} else {
				logger.WarnCtx(ctx, "failed to invite users to slack channel", "component", "incidentchannel", "error", err)
			}
		}
	}

	summary := fmt.Sprintf("Incident *%d* has been created.\n"+
		"*Severity:* %s\n"+
		"*Title:* %s",
		incident.IncidentNumber, incident.Severity, incident.Title)
	blocks := []slack.Block{
		{Type: "section", Text: &slack.TextObject{Type: "mrkdwn", Text: summary}},
		{
			Type: "actions",
			Elements: []slack.BlockElement{
				{
					Type:     "button",
					Text:     &slack.TextObject{Type: "plain_text", Text: "📋 Request Status Update", Emoji: true},
					ActionID: "request_incident_summary",
					Value:    strconv.FormatInt(incident.IncidentNumber, 10),
					Style:    "primary",
				},
			},
		},
		{
			Type: "context",
			Elements: []*slack.TextObject{
				{Type: "mrkdwn", Text: fmt.Sprintf("<%s/incidents/%d|View in Alga>", m.algaBaseURL, incident.IncidentNumber)},
			},
		},
	}
	if _, err := m.slackClient.PostMessageWithBlocks(ctx, channelID, "Incident created", blocks); err != nil {
		logger.WarnCtx(ctx, "failed to post initial summary to slack channel", "component", "incidentchannel", "error", err)
	}

	incident.SlackChannelID = channelID
	incident.SlackChannelName = name
	incident.SlackChannelArchived = false
	if _, err := m.incidentStore.UpdateIncident(ctx, incident.IncidentNumber, incident); err != nil {
		return fmt.Errorf("failed to save slack channel id on incident: %w", err)
	}

	return nil
}

func (m *Manager) ArchiveIncidentChannel(ctx context.Context, incident *store.IncidentRecord) error {
	if incident.SlackChannelID == "" {
		return nil
	}
	if !m.IsSupported() {
		return errors.New("slack is not configured")
	}

	if err := m.slackClient.ArchiveChannel(ctx, incident.SlackChannelID); err != nil {
		return fmt.Errorf("failed to archive slack channel: %w", err)
	}

	incident.SlackChannelArchived = true
	if _, err := m.incidentStore.UpdateIncident(ctx, incident.IncidentNumber, incident); err != nil {
		logger.WarnCtx(ctx, "failed to update slack_channel_archived on incident", "component", "incidentchannel", "error", err)
	}
	return nil
}

func (m *Manager) UnarchiveIncidentChannel(ctx context.Context, incident *store.IncidentRecord) error {
	if incident.SlackChannelID == "" || !incident.SlackChannelArchived {
		return nil
	}
	if !m.IsSupported() {
		return errors.New("slack is not configured")
	}

	if err := m.slackClient.UnarchiveChannel(ctx, incident.SlackChannelID); err != nil {
		return fmt.Errorf("failed to unarchive slack channel: %w", err)
	}

	incident.SlackChannelArchived = false
	if _, err := m.incidentStore.UpdateIncident(ctx, incident.IncidentNumber, incident); err != nil {
		logger.WarnCtx(ctx, "failed to update slack_channel_archived on incident", "component", "incidentchannel", "error", err)
	}

	msg := fmt.Sprintf(":arrows_counterclockwise: Incident *%d* has been reopened.", incident.IncidentNumber)
	if _, err := m.slackClient.PostMessage(ctx, incident.SlackChannelID, msg); err != nil {
		logger.WarnCtx(ctx, "failed to post reopen message to slack channel", "component", "incidentchannel", "error", err)
	}
	return nil
}

func (m *Manager) PostStatusChange(ctx context.Context, incident *store.IncidentRecord, newStatus string) {
	if incident.SlackChannelID == "" || !m.IsSupported() {
		return
	}

	msg := fmt.Sprintf(":arrow_right: Incident *%d* status changed to *%s* — <%s/incidents/%d|View in Alga>",
		incident.IncidentNumber, newStatus, m.algaBaseURL, incident.IncidentNumber)
	if _, err := m.slackClient.PostMessage(ctx, incident.SlackChannelID, msg); err != nil {
		logger.WarnCtx(ctx, "failed to post status change to slack channel", "component", "incidentchannel", "error", err)
	}
}

func (m *Manager) PostResolutionSummary(ctx context.Context, incident *store.IncidentRecord) {
	if incident.SlackChannelID == "" || !m.IsSupported() {
		return
	}

	durationStr := "N/A"
	if !incident.CreatedAt.IsZero() {
		d := time.Since(incident.CreatedAt)
		if incident.ResolvedAt != nil {
			d = incident.ResolvedAt.Sub(incident.CreatedAt)
		}
		durationStr = d.Truncate(time.Second).String()
	}

	msg := fmt.Sprintf(":white_check_mark: Incident *%d* has been resolved.\n"+
		"*Title:* %s\n"+
		"*Severity:* %s\n"+
		"*Duration:* %s\n"+
		"<%s/incidents/%d|View incident details>",
		incident.IncidentNumber, incident.Title, incident.Severity, durationStr,
		m.algaBaseURL, incident.IncidentNumber)
	if _, err := m.slackClient.PostMessage(ctx, incident.SlackChannelID, msg); err != nil {
		logger.WarnCtx(ctx, "failed to post resolution summary to slack channel", "component", "incidentchannel", "error", err)
	}
}

func (m *Manager) PostAgentSummary(ctx context.Context, incident *store.IncidentRecord, agentName string, text string) error {
	if incident.SlackChannelID == "" || !m.IsSupported() {
		return fmt.Errorf("slack channel not available for incident %d", incident.IncidentNumber)
	}

	now := time.Now().Format("3:04 PM")
	header := fmt.Sprintf(":clipboard: *Status Update from %s* (%s)", agentName, now)
	divider := "───────────────────"
	footer := fmt.Sprintf("<%s/incidents/%d|View in Alga>", m.algaBaseURL, incident.IncidentNumber)

	msg := header + "\n" + divider + "\n" + slack.MrkdwnPlain(text) + "\n" + footer
	if _, err := m.slackClient.PostMessage(ctx, incident.SlackChannelID, msg); err != nil {
		return fmt.Errorf("failed to post agent summary to slack: %w", err)
	}
	return nil
}

func (m *Manager) collectSlackUserIDs(ctx context.Context, incident *store.IncidentRecord) []string {
	var ids []string
	seen := make(map[string]bool)

	addUser := func(uid uuid.UUID) {
		key := uid.String()
		if seen[key] {
			return
		}
		seen[key] = true
		record, err := m.userStore.GetByID(uid)
		if err != nil || record == nil || record.SlackUserID == "" {
			return
		}
		ids = append(ids, record.SlackUserID)
	}

	if incident.CommanderID != nil {
		addUser(*incident.CommanderID)
	}
	if incident.CommunicatorID != nil {
		addUser(*incident.CommunicatorID)
	}
	if incident.OnCallResponderID != nil {
		addUser(*incident.OnCallResponderID)
	}
	// Auto-invite the current on-call human from the incident's escalation
	// policy even when no responder role has been assigned yet.
	if m.onCallResolver != nil {
		if uid, err := m.onCallResolver.ResolveOnCallUser(ctx, incident); err != nil {
			logger.WarnCtx(ctx, "failed to resolve on-call user for slack invite", "component", "incidentchannel", "error", err)
		} else if uid != nil {
			addUser(*uid)
		}
	}
	return ids
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)
var multiHyphen = regexp.MustCompile(`-+`)

func generateChannelName(incident *store.IncidentRecord) string {
	number := strconv.FormatInt(incident.IncidentNumber, 10)

	var slug string
	if incident.Title != "" {
		slug = strings.ToLower(incident.Title)
		if len(slug) > 30 {
			slug = slug[:30]
		}
		slug = nonAlphaNum.ReplaceAllString(slug, "-")
		slug = multiHyphen.ReplaceAllString(slug, "-")
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = number
	}

	base := fmt.Sprintf("inc-%s-%s", number, slug)
	if len(base) > 80 {
		base = base[:80]
	}
	return base
}
