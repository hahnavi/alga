package agent

import (
	"context"
	"fmt"
	"net/url"

	"alga/logger"
	"alga/mattermost"
	"alga/slack"
	"alga/store"
)

type ChatSyncService struct {
	mmClient                *mattermost.Client
	slClient                *slack.Client
	alertInvestigationStore store.AlertInvestigationStore
}

func NewChatSyncService(mm *mattermost.Client, sl *slack.Client, alertInvStore store.AlertInvestigationStore) *ChatSyncService {
	return &ChatSyncService{mmClient: mm, slClient: sl, alertInvestigationStore: alertInvStore}
}

func (c *ChatSyncService) Rebuild(mm *mattermost.Client, sl *slack.Client) {
	c.mmClient = mm
	c.slClient = sl
}

func (c *ChatSyncService) PostToMattermostThread(threadID, msg string) string {
	if c.mmClient == nil || !c.mmClient.Enabled() || threadID == "" {
		return ""
	}
	ctx := context.Background() // phase 1: ChatSyncService has no caller ctx yet
	postID, err := c.mmClient.ReplyToPost(ctx, threadID, msg, nil)
	if err != nil {
		logger.Warn("Failed to sync to Mattermost thread", "thread_id", threadID, "error", err)
		return ""
	}
	return postID
}

func (c *ChatSyncService) postToSlackThread(channelID, threadTS, msg string) string {
	if c.slClient == nil || !c.slClient.Enabled() || threadTS == "" || channelID == "" {
		return ""
	}
	ctx := context.Background() // phase 1: ChatSyncService has no caller ctx yet
	ts, err := c.slClient.PostThreadReply(ctx, channelID, threadTS, msg)
	if err != nil {
		logger.Warn("Failed to sync to Slack thread", "channel_id", channelID, "thread_ts", threadTS, "error", err)
		return ""
	}
	return ts
}

func (c *ChatSyncService) PostToSlackThreadWithCustomize(channelID, threadTS, msg string, customize *slack.PostCustomize) string {
	if c.slClient == nil || !c.slClient.Enabled() || channelID == "" {
		return ""
	}
	ctx := context.Background() // phase 1: ChatSyncService has no caller ctx yet
	var ts string
	var err error
	if customize != nil {
		if threadTS == "" {
			ts, err = c.slClient.PostMessage(ctx, channelID, msg)
		} else {
			ts, err = c.slClient.PostThreadReply(ctx, channelID, threadTS, msg, *customize)
		}
	} else {
		if threadTS == "" {
			ts, err = c.slClient.PostMessage(ctx, channelID, msg)
		} else {
			ts, err = c.slClient.PostThreadReply(ctx, channelID, threadTS, msg)
		}
	}
	if err != nil {
		logger.Warn("Failed to sync to Slack", "channel_id", channelID, "thread_ts", threadTS, "error", err)
		return ""
	}
	return ts
}

type ChatSyncOptions struct {
	slackCustomize *slack.PostCustomize
	saveMMPostID   func(postID string)
	saveSlackTS    func(ts string)
}

func (c *ChatSyncService) postToInvestigationThread(record *store.AlertInvestigationRecord, mmMsg, slMsg string, opts *ChatSyncOptions) {
	if record == nil {
		return
	}

	mmThread := record.PrimaryThreadID
	if mmThread == "" {
		mmThread = record.MMThreadID
	}

	if mmThread != "" {
		if postID := c.PostToMattermostThread(mmThread, mmMsg); postID != "" && opts != nil && opts.saveMMPostID != nil {
			opts.saveMMPostID(postID)
		}
	}

	if record.SlackChannelID != "" && record.SlackThreadTS != "" {
		ctx := context.Background() // phase 1: ChatSyncService has no caller ctx yet
		effectiveMsg := slMsg
		var customize *slack.PostCustomize
		if opts != nil {
			customize = opts.slackCustomize
		}
		var ts string
		if customize != nil {
			ts, _ = c.slClient.PostThreadReply(ctx, record.SlackChannelID, record.SlackThreadTS, effectiveMsg, *customize)
		} else {
			ts = c.postToSlackThread(record.SlackChannelID, record.SlackThreadTS, effectiveMsg)
		}
		if ts != "" && opts != nil && opts.saveSlackTS != nil {
			opts.saveSlackTS(ts)
		}
	}
}

func (c *ChatSyncService) UserSlackThreadMessage(user *store.UserRecord, text string) (string, *slack.PostCustomize) {
	if user == nil || user.SlackUserID == "" {
		displayName := "User"
		if user != nil {
			displayName = user.DisplayName()
			if displayName == "" {
				displayName = user.Email
			}
		}
		return slack.MrkdwnPrefixed(displayName, text), nil
	}

	displayName := user.SlackDisplayName
	if displayName == "" {
		displayName = user.DisplayName()
	}
	if displayName == "" {
		displayName = user.Email
	}

	iconURL := ""
	if c.slClient != nil && c.slClient.Enabled() {
		iconURL = c.slClient.GetUserAvatarURL(context.Background(), user.SlackUserID) // phase 1: no caller ctx yet
	}
	if iconURL == "" {
		iconURL = fmt.Sprintf("https://api.dicebear.com/9.x/initials/png?seed=%s&size=128", url.QueryEscape(user.SlackUserID))
	}

	return slack.Mrkdwn(text), &slack.PostCustomize{
		Username: displayName,
		IconURL:  iconURL,
	}
}

func (c *ChatSyncService) syncAgentMessage(investigationID, updateID, senderName, text string, record *store.AlertInvestigationRecord) {
	if record == nil {
		return
	}
	ctx := context.Background() // phase 1: ChatSyncService has no caller ctx yet
	canSave := investigationID != "" && updateID != ""
	mmThread := record.PrimaryThreadID
	if mmThread == "" {
		mmThread = record.MMThreadID
	}
	displayText := fmt.Sprintf("**%s**: %s", senderName, text)

	if mmThread != "" && c.mmClient != nil && c.mmClient.Enabled() {
		if postID, err := c.mmClient.ReplyToPost(ctx, mmThread, displayText, nil); err != nil {
			logger.Warn("agent Mattermost post failed for investigation", "investigation_id", investigationID, "error", err)
		} else if postID != "" && canSave {
			if err := c.alertInvestigationStore.SetAlertInvestigationUpdateMMPostID(context.Background(), investigationID, updateID, postID); err != nil {
				logger.Warn("Failed to save MM post ID for update in investigation", "update_id", updateID, "investigation_id", investigationID, "error", err)
			}
		}
	}
	if record.SlackChannelID != "" && record.SlackThreadTS != "" && c.slClient != nil && c.slClient.Enabled() {
		slackText := slack.Mrkdwn(text)
		cz := slack.PostCustomize{
			Username: senderName,
			IconURL:  fmt.Sprintf("https://api.dicebear.com/9.x/bottts-neutral/png?seed=%s&size=128", url.QueryEscape(senderName)),
		}
		if ts, err := c.slClient.PostThreadReply(ctx, record.SlackChannelID, record.SlackThreadTS, slackText, cz); err != nil {
			logger.Warn("agent Slack post failed for investigation", "investigation_id", investigationID, "error", err)
		} else if ts != "" && canSave {
			if err := c.alertInvestigationStore.SetAlertInvestigationUpdateSlackMessageTS(context.Background(), investigationID, updateID, ts); err != nil {
				logger.Warn("Failed to save Slack message TS for update in investigation", "update_id", updateID, "investigation_id", investigationID, "error", err)
			}
		}
	}
}
