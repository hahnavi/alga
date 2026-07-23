package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/email"
	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/slack"
	"alga/store"
	"alga/strutil"
	"alga/valkey"
)

type SlackDMSender interface {
	Enabled() bool
	OpenConversation(ctx context.Context, users []string) (string, error)
	PostMessage(ctx context.Context, channelID, text string) (string, error)
}

type VoiceProvider interface {
	Enabled() bool
	ProviderName() string
	Call(ctx context.Context, to string, incidentNumber int64, level int, opts CallOptions) (string, error)
}

// CallOptions carries per-call context to a voice provider. UserID lets the
// provider attribute downstream DTMF callbacks (the provider embeds it in the
// webhook URL so the inbound callback can resolve who was paged). Title carries
// a short incident brief that providers speak in the IVR announcement; empty
// means "no brief" and the provider falls back to the menu-only announcement.
type CallOptions struct {
	UserID *uuid.UUID
	Title  string
}

type Dispatcher struct {
	userStore     store.UserStore
	deliveryStore store.NotificationDeliveryStore
	incidentStore store.IncidentStore
	emailSender   *email.Sender
	slackClient   SlackDMSender
	voiceProvider VoiceProvider
	publisher     *rabbitmq.Publisher
	vkClient      *valkey.Client
}

func NewDispatcher(userStore store.UserStore, deliveryStore store.NotificationDeliveryStore, incidentStore store.IncidentStore, emailSender *email.Sender, slackClient *slack.Client, voiceProvider VoiceProvider, publisher *rabbitmq.Publisher, vkClient *valkey.Client) *Dispatcher {
	var dmSender SlackDMSender
	if slackClient != nil {
		dmSender = slackClient
	}
	return &Dispatcher{
		userStore:     userStore,
		deliveryStore: deliveryStore,
		incidentStore: incidentStore,
		emailSender:   emailSender,
		slackClient:   dmSender,
		voiceProvider: voiceProvider,
		publisher:     publisher,
		vkClient:      vkClient,
	}
}

func (d *Dispatcher) ResolveChannels(ctx context.Context, userID, notificationType string) []string {
	prefs, err := d.userStore.GetNotificationPreferences(ctx, userID)
	if err != nil {
		logger.WarnCtx(ctx, "Failed to load notification preferences for user", "component", "notification", "user_id", userID, "error", err)
		return []string{"in_app"}
	}

	if prefs == nil {
		return []string{"in_app"}
	}

	rules, ok := prefs["rules"].([]any)
	if !ok || len(rules) == 0 {
		defaultCh, _ := prefs["default_channel"].(string)
		if defaultCh != "" {
			return []string{defaultCh}
		}
		return []string{"in_app"}
	}

	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok {
			continue
		}
		ruleType, _ := rule["notification_type"].(string)
		if ruleType != notificationType && ruleType != "*" {
			continue
		}
		channels, _ := rule["channels"].([]any)
		if len(channels) == 0 {
			continue
		}
		var result []string
		for _, ch := range channels {
			if s, ok := ch.(string); ok {
				result = append(result, s)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	defaultCh, _ := prefs["default_channel"].(string)
	if defaultCh != "" {
		return []string{defaultCh}
	}

	return []string{"in_app"}
}

func (d *Dispatcher) Dispatch(ctx context.Context, userID, notificationType, title, message, resourceType, resourceID string, incidentID *uuid.UUID) error {
	return d.DispatchChannels(ctx, userID, notificationType, title, message, resourceType, resourceID, nil, incidentID, 0, 0)
}

func (d *Dispatcher) DispatchChannels(ctx context.Context, userID, notificationType, title, message, resourceType, resourceID string, channels []string, incidentID *uuid.UUID, incidentNumber int64, level int) error {
	if len(channels) == 0 {
		channels = d.ResolveChannels(ctx, userID, notificationType)
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID %s: %w", userID, err)
	}

	var firstErr error
	for _, channel := range channels {
		switch channel {
		case "in_app":
			logger.DebugCtx(ctx, "notification dispatch: in_app channel handled by worker", "component", "notification", "user_id", userID)

		case "email":
			if d.emailSender != nil && d.emailSender.Enabled() {
				user, err := d.userStore.GetByID(uid)
				if err != nil {
					logger.ErrorCtx(ctx, "Failed to get user for email dispatch", "component", "notification", "user_id", userID, "error", err)
					if firstErr == nil {
						firstErr = fmt.Errorf("failed to get user for email dispatch: %w", err)
					}
				} else if user != nil {
					emailMsg := rabbitmq.EmailMessage{
						To:       user.Email,
						Subject:  title,
						TextBody: message,
					}
					if err := d.publisher.PublishEmail(ctx, emailMsg); err != nil {
						logger.ErrorCtx(ctx, "Failed to publish email for user", "component", "notification", "user_id", userID, "error", err)
						if firstErr == nil {
							firstErr = fmt.Errorf("failed to publish email: %w", err)
						}
					} else {
						d.logDelivery(ctx, uid, incidentID, notificationType, "email", "queued")
					}
				}
			} else {
				d.logDelivery(ctx, uid, incidentID, notificationType, "email", "skipped")
			}

		case "mattermost":
			logger.DebugCtx(ctx, "notification dispatch: mattermost delivery", "component", "notification", "user_id", userID)
			d.logDelivery(ctx, uid, incidentID, notificationType, "mattermost", "skipped")

		case "slack":
			if d.slackClient != nil && d.slackClient.Enabled() {
				d.dispatchSlackDM(ctx, uid, incidentID, notificationType, title, message)
			} else {
				d.logDelivery(ctx, uid, incidentID, notificationType, "slack", "skipped")
			}

		case "voice":
			if d.voiceProvider != nil && d.voiceProvider.Enabled() {
				d.dispatchVoiceCall(ctx, uid, incidentID, notificationType, title, message, incidentNumber, level)
			} else {
				d.logDelivery(ctx, uid, incidentID, notificationType, "voice", "skipped")
			}

		default:
			logger.WarnCtx(ctx, "notification dispatch: unknown channel for user", "component", "notification", "channel", channel, "user_id", userID)
		}
	}

	return firstErr
}

func (d *Dispatcher) dispatchSlackDM(ctx context.Context, uid uuid.UUID, incidentID *uuid.UUID, notificationType, title, message string) {
	user, err := d.userStore.GetByID(uid)
	if err != nil {
		logger.ErrorCtx(ctx, "Failed to get user for Slack DM dispatch", "component", "notification", "user_id", uid, "error", err)
		d.logDelivery(ctx, uid, incidentID, notificationType, "slack", "failed")
		return
	}
	if user == nil || user.SlackUserID == "" {
		d.logDelivery(ctx, uid, incidentID, notificationType, "slack", "skipped_no_slack_id")
		return
	}

	dmChannel, err := d.slackClient.OpenConversation(ctx, []string{user.SlackUserID})
	if err != nil {
		logger.ErrorCtx(ctx, "Failed to open Slack DM channel", "component", "notification", "user_id", uid, "slack_user_id", user.SlackUserID, "error", err)
		d.logDelivery(ctx, uid, incidentID, notificationType, "slack", "failed")
		return
	}

	text := fmt.Sprintf("*%s*\n%s", title, message)
	if _, err := d.slackClient.PostMessage(ctx, dmChannel, text); err != nil {
		logger.ErrorCtx(ctx, "Failed to send Slack DM", "component", "notification", "user_id", uid, "channel", dmChannel, "error", err)
		d.logDelivery(ctx, uid, incidentID, notificationType, "slack", "failed")
		return
	}

	d.logDelivery(ctx, uid, incidentID, notificationType, "slack", "delivered")
}

func (d *Dispatcher) dispatchVoiceCall(ctx context.Context, uid uuid.UUID, incidentID *uuid.UUID, notificationType, title, message string, incidentNumber int64, level int) {
	user, err := d.userStore.GetByID(uid)
	if err != nil {
		logger.ErrorCtx(ctx, "Failed to get user for voice dispatch", "component", "notification", "user_id", uid, "error", err)
		d.logDelivery(ctx, uid, incidentID, notificationType, "voice", "failed")
		return
	}
	if user == nil || user.Phone == "" {
		d.logDelivery(ctx, uid, incidentID, notificationType, "voice", "skipped_no_phone")
		return
	}
	if user.VoiceOptOut {
		metrics.VoiceCallsSuppressed.Add(1)
		d.logDelivery(ctx, uid, incidentID, notificationType, "voice", "skipped_opt_out")
		logger.InfoCtx(ctx, "voice dispatch suppressed: user opted out", "component", "notification", "user_id", uid, "incident_number", incidentNumber, "level", level)
		return
	}

	// Per-(incident,user,level) dedup. Prevents retry tiers, parallel publishers,
	// and the manual /escalate click-spam from placing the same call twice.
	// TTL is the level's window: long enough to absorb retries, short enough to
	// permit re-paging once the next escalation level fires.
	if !claimVoiceCallSlot(ctx, d.vkClient, incidentNumber, uid, level) {
		metrics.VoiceCallsSuppressed.Add(1)
		d.logDelivery(ctx, uid, incidentID, notificationType, "voice", "skipped_dedup")
		logger.InfoCtx(ctx, "voice dispatch suppressed: dedup hit", "component", "notification", "user_id", uid, "incident_number", incidentNumber, "level", level)
		return
	}

	userID := uid
	brief := d.incidentBrief(ctx, incidentNumber)
	if _, err := d.voiceProvider.Call(ctx, user.Phone, incidentNumber, level, CallOptions{UserID: &userID, Title: brief}); err != nil {
		logger.ErrorCtx(ctx, "Failed to place voice call", "component", "notification", "user_id", uid, "phone", user.Phone, "provider", d.voiceProvider.ProviderName(), "error", err)
		d.logDelivery(ctx, uid, incidentID, notificationType, "voice", "failed")
		return
	}

	metrics.VoiceCallsPlaced.Add(1)
	d.logDelivery(ctx, uid, incidentID, notificationType, "voice", "delivered")
}

// incidentBriefMaxLen caps the spoken incident title so the announcement stays
// near a few seconds of speech (~3 words/sec → ~120 chars ≈ 6s).
const incidentBriefMaxLen = 120

// incidentBrief returns a short, single-line truncation of the incident's Title
// for inclusion in the spoken IVR announcement. Returns "" when the incident
// cannot be resolved, so callers fall back to the menu-only announcement.
func (d *Dispatcher) incidentBrief(ctx context.Context, incidentNumber int64) string {
	if d.incidentStore == nil || incidentNumber <= 0 {
		return ""
	}
	inc, err := d.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		return ""
	}
	return strutil.TruncateOneLine(inc.Title, incidentBriefMaxLen)
}

// claimVoiceCallSlot returns true when this caller is the first to claim the
// (incident, user, level) slot, false when a previous attempt has already
// attempted the call within the dedup window. Default TTL is 5 minutes; that
// covers the notification-dispatch retry tail while still allowing prompt
// legitimate re-paging once the window expires.
func claimVoiceCallSlot(ctx context.Context, vk *valkey.Client, incidentNumber int64, uid uuid.UUID, level int) bool {
	if vk == nil || incidentNumber == 0 {
		return true
	}
	key := fmt.Sprintf("alga:voice:call:%d:%s:%d", incidentNumber, uid.String(), level)
	ok, err := vk.SetNX(ctx, key, "1", 5*time.Minute)
	if err != nil {
		// Fail open: if Valkey is down we'd rather place a call than drop
		// an escalation. Cost > missed paging for this specific guard.
		logger.WarnCtx(ctx, "voice dedup setnx failed, allowing call", "component", "notification", "key", key, "error", err)
		return true
	}
	return ok
}

func (d *Dispatcher) logDelivery(ctx context.Context, userID uuid.UUID, incidentID *uuid.UUID, notificationType, channel, status string) {
	_, err := d.deliveryStore.Create(ctx, &store.NotificationDeliveryRecord{
		UserID:           userID,
		IncidentID:       incidentID,
		NotificationType: notificationType,
		Channel:          channel,
		Status:           status,
	})
	if err != nil {
		logger.ErrorCtx(ctx, "Failed to log notification delivery", "component", "notification", "user_id", userID, "channel", channel, "error", err)
	}
}
