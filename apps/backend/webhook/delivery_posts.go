package webhook

import (
	"context"
	"fmt"
	"time"

	"alga/logger"
	"alga/store"
	"alga/types"
)

// AlertFromStoreRecord builds a Grafana-shaped alert for formatting chat updates from persisted state.
func AlertFromStoreRecord(rec *store.AlertRecord) types.Alert {
	if rec == nil {
		return types.Alert{}
	}
	a := types.Alert{
		Status:       rec.Status,
		Labels:       rec.Labels,
		Annotations:  rec.Annotations,
		Values:       rec.Values,
		Fingerprint:  rec.Fingerprint,
		GeneratorURL: rec.GeneratorURL,
		Acknowledged: rec.Acknowledged,
	}
	if !rec.StartsAt.IsZero() {
		a.StartsAt = rec.StartsAt.UTC().Format(time.RFC3339)
	}
	if rec.EndsAt != nil && !rec.EndsAt.IsZero() {
		a.EndsAt = rec.EndsAt.UTC().Format(time.RFC3339)
	}
	return a
}

// UpdateChatPostsForAlert updates Mattermost/Slack messages for every delivery target to match alert.
func UpdateChatPostsForAlert(ctx context.Context, router *ChatRouter, existing *store.AlertRecord, alert types.Alert) error {
	if existing == nil {
		return nil
	}
	var firstErr error
	for _, dt := range existing.DeliveryTargets {
		if err := updateDeliveryTargetPost(ctx, router, dt, alert); err != nil {
			logger.Error("Failed to update delivery target post for alert", "component", "webhook", "post_id", dt.PostID, "fingerprint", alert.Fingerprint, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func updateDeliveryTargetPost(ctx context.Context, router *ChatRouter, dt store.DeliveryTarget, alert types.Alert) error {
	provider := router.Provider(dt.Provider)
	if provider == nil || !provider.Enabled() {
		return fmt.Errorf("%s integration is not configured", dt.Provider)
	}
	return provider.UpdateAlertPost(ctx, dt.PostID, dt.Channel, alert)
}

// formatAlertAttachment builds a card-style Mattermost attachment from an alert.
// Returns a fallback text string and the rich attachment.
func formatAlertAttachment(alert types.Alert) (string, types.MattermostAttachment) {
	alertName := alert.Labels["alertname"]

	color := "#FF0000"
	statusText := "Firing"
	if alert.Status == "resolved" {
		color = "#36A64F"
		statusText = "Resolved"
	} else if alert.Acknowledged {
		color = "#FFA500"
		statusText = "Firing · Acknowledged"
	}

	fallback := fmt.Sprintf("[%s] %s", statusText, alertName)

	attach := types.MattermostAttachment{
		Fallback: fallback,
		Color:    color,
		Title:    fmt.Sprintf("[%s] %s", statusText, alertName),
		Fields:   []types.AttachmentField{},
	}

	// Description / summary
	if desc, ok := alert.Annotations["description"]; ok {
		attach.Text = desc
	} else if summary, ok := alert.Annotations["summary"]; ok {
		attach.Text = summary
	}

	// Value
	if alert.ValueString != "" {
		attach.Fields = append(attach.Fields, types.AttachmentField{
			Title: "Value",
			Value: alert.ValueString,
			Short: true,
		})
	} else if len(alert.Values) > 0 {
		for k, v := range alert.Values {
			attach.Fields = append(attach.Fields, types.AttachmentField{
				Title: k,
				Value: fmt.Sprintf("%v", v),
				Short: true,
			})
		}
	}

	// Labels (skip alertname since it's in the title)
	for k, v := range alert.Labels {
		if k != "alertname" {
			attach.Fields = append(attach.Fields, types.AttachmentField{
				Title: k,
				Value: v,
				Short: true,
			})
		}
	}

	// Annotations (skip description/summary since they're in text)
	for k, v := range alert.Annotations {
		if k != "description" && k != "summary" && k != "runbook_url" {
			attach.Fields = append(attach.Fields, types.AttachmentField{
				Title: k,
				Value: v,
				Short: false,
			})
		}
	}

	// Timestamps
	if alert.StartsAt != "" && alert.StartsAt != "0001-01-01T00:00:00Z" {
		if t, err := time.Parse(time.RFC3339, alert.StartsAt); err == nil {
			attach.Fields = append(attach.Fields, types.AttachmentField{
				Title: "Started",
				Value: t.Format("2006-01-02 15:04:05 MST"),
				Short: true,
			})
		}
	}

	if alert.Status == "resolved" && alert.EndsAt != "" && alert.EndsAt != "0001-01-01T00:00:00Z" {
		if t, err := time.Parse(time.RFC3339, alert.EndsAt); err == nil {
			attach.Fields = append(attach.Fields, types.AttachmentField{
				Title: "Resolved",
				Value: t.Format("2006-01-02 15:04:05 MST"),
				Short: true,
			})
		}
	}

	// Links in footer
	var footerParts []string
	if alert.GeneratorURL != "" {
		footerParts = append(footerParts, fmt.Sprintf("[View in Grafana](%s)", alert.GeneratorURL))
	}
	if alert.SilenceURL != "" {
		footerParts = append(footerParts, fmt.Sprintf("[Silence](%s)", alert.SilenceURL))
	}
	if runbookURL, ok := alert.Annotations["runbook_url"]; ok && runbookURL != "" {
		footerParts = append(footerParts, fmt.Sprintf("[Runbook](%s)", runbookURL))
	}
	if len(footerParts) > 0 {
		attach.Footer = fmt.Sprintf("Alga | %s", alert.Fingerprint)
		attach.Pretext = ""
		for i, link := range footerParts {
			if i > 0 {
				attach.Pretext += " | "
			}
			attach.Pretext += link
		}
	}

	return fallback, attach
}
